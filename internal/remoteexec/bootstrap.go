package remoteexec

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
)

const (
	probeBegin = "__IRONSTATE_BEGIN__"
	probeEnd   = "__IRONSTATE_END__"
)

// Remote scripts run under 'sh -c' and are kept to one line with no '!'
// so any login shell (bash, zsh, fish, tcsh) passes them through intact.
const (
	probeScript = `: ironstate-probe; ` +
		`echo ` + probeBegin + `; echo "os=$(uname -s)"; echo "arch=$(uname -m)"; echo "uid=$(id -u)"; ` +
		`base="${1:-${XDG_CACHE_HOME:-$HOME/.cache}/ironstate/agent}"; echo "agent_dir=$base"; ` +
		`if command -v sha256sum >/dev/null 2>&1; then echo sha=sha256sum; elif command -v shasum >/dev/null 2>&1; then echo sha=shasum; fi; ` +
		`if command -v sudo >/dev/null 2>&1; then if sudo -n true >/dev/null 2>&1; then echo sudo=nopasswd; else echo sudo=password; fi; else echo sudo=missing; fi; ` +
		`t="$base/.probe.$$"; if mkdir -p "$base" && chmod 700 "$base" && echo "exit 0" > "$t" && chmod 700 "$t" && "$t"; then echo exec=ok; fi; rm -f "$t"; ` +
		`echo ` + probeEnd

	hashSnippet = `case "$SHA" in sha256sum) h=$(sha256sum "$F") ;; *) h=$(shasum -a 256 "$F") ;; esac; h=${h%% *}`

	checkScript = `: ironstate-check; ` +
		`SHA="$2"; F="$1"; if [ -f "$F" ]; then ` + hashSnippet + `; echo "sha256=$h"; fi`

	uploadScript = `: ironstate-upload; ` +
		`set -e; umask 077; mkdir -p "$1"; F="$1/.upload.$$"; SHA="$3"; trap 'rm -f "$F"' EXIT; cat > "$F"; chmod 700 "$F"; ` +
		hashSnippet + `; if [ "$h" = "$4" ]; then mv -f "$F" "$2"; echo "sha256=$h"; else echo "uploaded agent sha256 $h does not match $4" >&2; exit 3; fi`
)

// ProbeInfo is what the probe learned about a POSIX target.
type ProbeInfo struct {
	OS       string // linux | darwin
	Arch     string // amd64 | arm64
	UID      string
	AgentDir string
	SHATool  string // sha256sum | shasum
	Sudo     string // nopasswd | password | missing
	ExecOK   bool
}

// Platform returns "os/arch".
func (p ProbeInfo) Platform() string { return p.OS + "/" + p.Arch }

// StageError marks which bootstrap stage failed and whether the target was
// unreachable (ssh itself failed) rather than unsuitable.
type StageError struct {
	Stage       string
	Unreachable bool
	Err         error
}

func (e *StageError) Error() string { return e.Stage + ": " + e.Err.Error() }
func (e *StageError) Unwrap() error { return e.Err }

func shCommand(script string, args ...string) RemoteCommand {
	return RemoteCommand{Program: "sh", Args: append([]string{"-c", script, "sh"}, args...)}
}

// execCapture runs cmd and maps an ssh failure (255) to an unreachable
// StageError.
func execCapture(ctx context.Context, t Transport, stage string, cmd RemoteCommand, stdin io.Reader) (string, error) {
	var stdout, stderr bytes.Buffer
	if stdin == nil {
		stdin = bytes.NewReader(nil)
	}
	code, err := t.Exec(ctx, cmd, stdin, &stdout, &stderr)
	if err != nil {
		return "", &StageError{Stage: stage, Err: err}
	}
	if code == SSHExitUnreachable {
		return "", &StageError{Stage: stage, Unreachable: true, Err: errors.New(strings.TrimSpace(stderr.String()))}
	}
	if code != 0 {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = fmt.Sprintf("exit %d", code)
		}
		return stdout.String(), &StageError{Stage: stage, Err: errors.New(msg)}
	}
	return stdout.String(), nil
}

// Probe detects platform, cache dir, hashing tool, sudo and exec-ability.
// agentDir overrides the default remote agent dir when non-empty.
func Probe(ctx context.Context, t Transport, agentDir string) (ProbeInfo, error) {
	out, err := execCapture(ctx, t, "probe", shCommand(probeScript, agentDir), nil)
	if err != nil {
		return ProbeInfo{}, err
	}
	info, err := parseProbe(out)
	if err != nil {
		return info, &StageError{Stage: "probe", Err: err}
	}
	return info, nil
}

func parseProbe(out string) (ProbeInfo, error) {
	var info ProbeInfo
	values := map[string]string{}
	in, complete := false, false
	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() && !complete {
		line := strings.TrimSpace(scanner.Text())
		switch {
		case line == probeBegin:
			in = true
		case line == probeEnd:
			complete = in
		case in:
			if k, v, ok := strings.Cut(line, "="); ok {
				values[k] = v
			}
		}
	}
	if !complete {
		return info, errors.New("no probe output; the target must be a POSIX system with 'sh' (Windows targets are not supported yet)")
	}
	switch strings.ToLower(values["os"]) {
	case "linux":
		info.OS = "linux"
	case "darwin":
		info.OS = "darwin"
	default:
		return info, fmt.Errorf("unsupported target OS %q (supported: Linux, macOS)", values["os"])
	}
	switch strings.ToLower(values["arch"]) {
	case "x86_64", "amd64":
		info.Arch = "amd64"
	case "aarch64", "arm64":
		info.Arch = "arm64"
	default:
		return info, fmt.Errorf("unsupported target architecture %q (supported: x86_64, arm64)", values["arch"])
	}
	info.UID = values["uid"]
	info.AgentDir = values["agent_dir"]
	info.SHATool = values["sha"]
	info.Sudo = values["sudo"]
	info.ExecOK = values["exec"] == "ok"
	if info.AgentDir == "" || !strings.HasPrefix(info.AgentDir, "/") {
		return info, fmt.Errorf("could not determine an absolute agent dir (got %q)", info.AgentDir)
	}
	if info.SHATool == "" {
		return info, errors.New("target has neither sha256sum nor shasum; one is needed to verify the agent")
	}
	if !info.ExecOK {
		return info, fmt.Errorf("cannot execute files in %s (noexec mount?); pass --remote-agent-dir to use another directory", info.AgentDir)
	}
	return info, nil
}

// AgentSource resolves the local ironstate binary to ship for a platform.
type AgentSource struct {
	// Binaries maps "os/arch" to an explicit binary (--agent-binary).
	Binaries map[string]string
	// Version is the controller's version; the agent must match it.
	Version string
	// NoDownload disables fetching release agents (--no-agent-download).
	NoDownload bool

	mu         sync.Mutex
	hashes     map[string]string
	downloadMu sync.Mutex
}

// LocalAgent is a resolved agent binary and its sha256.
type LocalAgent struct {
	Path   string
	SHA256 string
}

// Resolve picks the binary for platform: explicit flag, then this
// executable when platforms match, then the local agent cache, then a
// verified download of the matching release.
func (s *AgentSource) Resolve(ctx context.Context, platform string) (LocalAgent, error) {
	candidate := s.Binaries[platform]
	if candidate == "" && platform == runtime.GOOS+"/"+runtime.GOARCH {
		exe, err := os.Executable()
		if err != nil {
			return LocalAgent{}, err
		}
		candidate = exe
	}
	if candidate == "" {
		if cached, err := cachedAgentPath(s.Version, platform); err == nil {
			if _, statErr := os.Stat(cached); statErr == nil {
				candidate = cached
			}
		}
	}
	if candidate == "" && !s.NoDownload && IsReleaseVersion(s.Version) {
		downloaded, err := s.download(ctx, platform)
		if err != nil {
			return LocalAgent{}, fmt.Errorf("download ironstate %s for %s: %w (or pass --agent-binary %s=<path>)", s.Version, platform, err, platform)
		}
		candidate = downloaded
	}
	if candidate == "" {
		return LocalAgent{}, fmt.Errorf("no ironstate %s binary for %s; pass --agent-binary %s=<path> (e.g. built with GOOS=%s GOARCH=%s go build ./cmd/ironstate)",
			s.Version, platform, platform, path.Dir(platform), path.Base(platform))
	}
	sum, err := s.hash(candidate)
	if err != nil {
		return LocalAgent{}, err
	}
	return LocalAgent{Path: candidate, SHA256: sum}, nil
}

func (s *AgentSource) hash(p string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sum, ok := s.hashes[p]; ok {
		return sum, nil
	}
	f, err := os.Open(p) //nolint:gosec // operator-chosen agent binary
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	sum := hex.EncodeToString(h.Sum(nil))
	if s.hashes == nil {
		s.hashes = map[string]string{}
	}
	s.hashes[p] = sum
	return sum, nil
}

func cachedAgentPath(version, platform string) (string, error) {
	base, err := UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "ironstate", "agents", safeVersion(version), strings.ReplaceAll(platform, "/", "_"), "ironstate"), nil
}

// UserCacheDir is the controller's cache root; overridable for tests.
var UserCacheDir = os.UserCacheDir

var unsafeVersionChars = regexp.MustCompile(`[^A-Za-z0-9._+-]`)

func safeVersion(v string) string {
	v = unsafeVersionChars.ReplaceAllString(v, "_")
	if v == "" {
		return "unknown"
	}
	return v
}

// RemoteAgentPath is where agent lives on a target with the given probe.
func RemoteAgentPath(info ProbeInfo, version string, agent LocalAgent) string {
	return path.Join(info.AgentDir, safeVersion(version)+"-"+agent.SHA256[:12], "ironstate")
}

// EnsureAgent makes sure the exact agent binary is at its remote path,
// uploading it only when missing or different. Returns whether it uploaded.
func EnsureAgent(ctx context.Context, t Transport, info ProbeInfo, version string, agent LocalAgent) (string, bool, error) {
	remotePath := RemoteAgentPath(info, version, agent)
	out, err := execCapture(ctx, t, "check agent", shCommand(checkScript, remotePath, info.SHATool), nil)
	if err != nil {
		return remotePath, false, err
	}
	if remoteSHA(out) == agent.SHA256 {
		return remotePath, false, nil
	}
	f, err := os.Open(agent.Path)
	if err != nil {
		return remotePath, false, err
	}
	defer func() { _ = f.Close() }()
	out, err = execCapture(ctx, t, "upload agent", shCommand(uploadScript, path.Dir(remotePath), remotePath, info.SHATool, agent.SHA256), f)
	if err != nil {
		return remotePath, false, err
	}
	if got := remoteSHA(out); got != agent.SHA256 {
		return remotePath, false, &StageError{Stage: "upload agent", Err: fmt.Errorf("remote sha256 %q does not match %s", got, agent.SHA256)}
	}
	return remotePath, true, nil
}

func remoteSHA(out string) string {
	for _, line := range strings.Split(out, "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), "sha256="); ok {
			return v
		}
	}
	return ""
}
