package remoteexec

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
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
		`case "$(uname -s)" in Darwin) state="$HOME/Library/Caches/ironstate" ;; *) state="${XDG_CACHE_HOME:-$HOME/.cache}/ironstate" ;; esac; echo "state_dir=$state"; ` +
		`if command -v sha256sum >/dev/null 2>&1; then echo sha=sha256sum; elif command -v shasum >/dev/null 2>&1; then echo sha=shasum; elif command -v openssl >/dev/null 2>&1; then echo sha=openssl; elif command -v busybox >/dev/null 2>&1 && busybox sha256sum /dev/null >/dev/null 2>&1; then echo sha=busybox; fi; ` +
		`if command -v sudo >/dev/null 2>&1; then if sudo -n true >/dev/null 2>&1; then echo sudo=nopasswd; else echo sudo=password; fi; else echo sudo=missing; fi; ` +
		`t="$base/.probe.$$"; if mkdir -p "$base" && chmod 700 "$base" && echo "exit 0" > "$t" && chmod 700 "$t" && "$t"; then echo exec=ok; fi; rm -f "$t"; ` +
		`echo ` + probeEnd

	hashSnippet = `case "$SHA" in sha256sum) h=$(sha256sum "$F"); h=${h%% *} ;; shasum) h=$(shasum -a 256 "$F"); h=${h%% *} ;; openssl) h=$(openssl dgst -sha256 "$F"); h=${h##* } ;; busybox) h=$(busybox sha256sum "$F"); h=${h%% *} ;; esac`

	checkScript = `: ironstate-check; ` +
		`SHA="$2"; F="$1"; if [ -f "$F" ]; then ` + hashSnippet + `; echo "sha256=$h"; fi`

	uploadScript = `: ironstate-upload; ` +
		`set -e; umask 077; mkdir -p "$1"; F="$1/.upload.$$"; SHA="$3"; trap 'rm -f "$F"' EXIT; cat > "$F"; chmod 700 "$F"; ` +
		hashSnippet + `; if [ "$h" = "$4" ]; then mv -f "$F" "$2"; echo "sha256=$h"; else echo "uploaded agent sha256 $h does not match $4" >&2; exit 3; fi`

	uploadUnverifiedScript = `: ironstate-upload-unverified; ` +
		`set -e; umask 077; mkdir -p "$1"; F="$1/.upload.$$"; trap 'rm -f "$F"' EXIT; cat > "$F"; chmod 700 "$F"; mv -f "$F" "$2"; echo uploaded=1`
)

func powerShellScript(script string) RemoteCommand {
	return RemoteCommand{Windows: true, Script: script}
}

func powerShellProbeScript(agentDir string) string {
	override := base64.StdEncoding.EncodeToString([]byte(agentDir))
	return `$ErrorActionPreference='Stop'; ` +
		`Write-Output '` + probeBegin + `'; ` +
		`$a=$env:PROCESSOR_ARCHITEW6432; if (!$a) {$a=$env:PROCESSOR_ARCHITECTURE}; ` +
		`Write-Output 'os=windows'; Write-Output ("arch="+$a); ` +
		`Write-Output ("uid="+$env:USERNAME); Write-Output 'sha=powershell'; Write-Output 'sudo=na'; ` +
		`$o=[Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('` + override + `')); ` +
		`if ($o) {$b=$o} else {$b=Join-Path $env:LOCALAPPDATA 'ironstate\agent'}; ` +
		`[IO.Directory]::CreateDirectory($b)|Out-Null; Write-Output ("agent_dir="+$b); Write-Output ("state_dir="+(Join-Path $env:LOCALAPPDATA 'ironstate')); ` +
		`$t=Join-Path $b ('.probe.'+[guid]::NewGuid().ToString('N')); ` +
		`[IO.File]::WriteAllText($t,''); Remove-Item -LiteralPath $t -Force; ` +
		`Write-Output 'exec=ok'; ` +
		`$id=[Security.Principal.WindowsIdentity]::GetCurrent(); ` +
		`$p=[Security.Principal.WindowsPrincipal]::new($id); ` +
		`if ($p.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {Write-Output 'admin=yes'} else {Write-Output 'admin=no'}; ` +
		`Write-Output '` + probeEnd + `'`
}

func powerShellCheckScript(file string) string {
	encoded := base64.StdEncoding.EncodeToString([]byte(file))
	return `$f=[Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('` + encoded + `')); ` +
		`if (Test-Path -LiteralPath $f -PathType Leaf) {Write-Output ('sha256='+(Get-FileHash -LiteralPath $f -Algorithm SHA256).Hash.ToLowerInvariant())}`
}

func powerShellPrepareUploadScript(dir string) string {
	encoded := base64.StdEncoding.EncodeToString([]byte(dir))
	return `$ErrorActionPreference='Stop'; $d=[Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('` + encoded + `')); ` +
		`[IO.Directory]::CreateDirectory($d)|Out-Null; Write-Output 'ready'`
}

func powerShellFinalizeUploadScript(tempFile, file, expected string) string {
	encode := func(value string) string { return base64.StdEncoding.EncodeToString([]byte(value)) }
	return `$ErrorActionPreference='Stop'; $tmp=[Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('` + encode(tempFile) + `')); ` +
		`$f=[Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('` + encode(file) + `')); ` +
		`$e='` + expected + `'; try {$h=(Get-FileHash -LiteralPath $tmp -Algorithm SHA256).Hash.ToLowerInvariant(); ` +
		`if ($h -ne $e) {throw 'uploaded agent sha256 mismatch'}; Move-Item -LiteralPath $tmp -Destination $f -Force; ` +
		`Write-Output ('sha256='+$h)} catch {Remove-Item -LiteralPath $tmp -Force -ErrorAction SilentlyContinue; throw}`
}

// ProbeInfo is what bootstrap learned about a target.
type ProbeInfo struct {
	OS                    string // linux | darwin | windows
	Arch                  string // amd64 | arm64
	UID                   string
	AgentDir              string
	StateDir              string
	SHATool               string // sha256sum | shasum | openssl | busybox | powershell
	Sudo                  string // nopasswd | password | missing | na
	Admin                 bool
	ExecOK                bool
	SkipAgentVerification bool
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

// Probe detects platform, cache dir, hashing tool, sudo/elevation and
// whether the agent can execute from the selected cache directory.
func Probe(ctx context.Context, t Transport, agentDir string) (ProbeInfo, error) {
	return ProbeWithHint(ctx, t, agentDir, "")
}

// ProbeWithHint avoids a failing POSIX-shell attempt for a known Windows host.
func ProbeWithHint(ctx context.Context, t Transport, agentDir, platformHint string) (ProbeInfo, error) {
	if strings.EqualFold(platformHint, "windows") {
		return probeWindows(ctx, t, agentDir)
	}
	out, posixErr := execCapture(ctx, t, "probe", shCommand(probeScript, agentDir), nil)
	if posixErr == nil {
		info, parseErr := parseProbe(out)
		if parseErr == nil || strings.Contains(out, probeBegin) {
			if parseErr != nil {
				return info, &StageError{Stage: "probe", Err: parseErr}
			}
			return info, nil
		}
	} else {
		var stage *StageError
		if errors.As(posixErr, &stage) && stage.Unreachable {
			return ProbeInfo{}, posixErr
		}
	}
	info, windowsErr := probeWindows(ctx, t, agentDir)
	if windowsErr == nil {
		return info, nil
	}
	return info, windowsErr
}

func probeWindows(ctx context.Context, t Transport, agentDir string) (ProbeInfo, error) {
	out, err := execCapture(ctx, t, "probe", powerShellScript(powerShellProbeScript(agentDir)), nil)
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
		return info, errors.New("no probe output; the target must provide POSIX sh or a Windows OpenSSH PowerShell shell")
	}
	switch strings.ToLower(values["os"]) {
	case "linux":
		info.OS = "linux"
	case "darwin":
		info.OS = "darwin"
	case "windows":
		info.OS = "windows"
	default:
		return info, fmt.Errorf("unsupported target OS %q (supported: Linux, macOS, Windows)", values["os"])
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
	info.StateDir = values["state_dir"]
	info.SHATool = values["sha"]
	info.Sudo = values["sudo"]
	info.Admin = values["admin"] == "yes"
	info.ExecOK = values["exec"] == "ok"
	if info.AgentDir == "" || (info.OS == "windows" && !isWindowsAbs(info.AgentDir)) || (info.OS != "windows" && !strings.HasPrefix(info.AgentDir, "/")) {
		return info, fmt.Errorf("could not determine an absolute agent dir (got %q)", info.AgentDir)
	}
	if info.StateDir == "" {
		if info.OS == "windows" {
			if separator := strings.LastIndex(info.AgentDir, `\`); separator > 0 {
				info.StateDir = info.AgentDir[:separator]
			}
		} else {
			info.StateDir = path.Dir(info.AgentDir)
		}
	}
	if info.StateDir == "" || (info.OS == "windows" && !isWindowsAbs(info.StateDir)) || (info.OS != "windows" && !strings.HasPrefix(info.StateDir, "/")) {
		return info, fmt.Errorf("could not determine an absolute state dir (got %q)", info.StateDir)
	}
	if info.OS != "windows" && !info.ExecOK {
		return info, fmt.Errorf("cannot execute files in %s (noexec mount?); pass --remote-agent-dir to use another directory", info.AgentDir)
	}
	return info, nil
}

func isWindowsAbs(p string) bool {
	return (len(p) >= 3 && ((p[0] >= 'A' && p[0] <= 'Z') || (p[0] >= 'a' && p[0] <= 'z')) && p[1] == ':' && (p[2] == '\\' || p[2] == '/')) || strings.HasPrefix(p, `\\`)
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
	name := "ironstate"
	if info.OS == "windows" {
		name += ".exe"
		base := strings.ReplaceAll(strings.TrimRight(info.AgentDir, `\/`), "/", `\`)
		return base + `\` + safeVersion(version) + "-" + agent.SHA256[:12] + `\` + name
	}
	return path.Join(info.AgentDir, safeVersion(version)+"-"+agent.SHA256[:12], name)
}

// EnsureAgent makes sure the exact agent binary is at its remote path,
// uploading it only when missing or different. Returns whether it uploaded.
func EnsureAgent(ctx context.Context, t Transport, info ProbeInfo, version string, agent LocalAgent) (string, bool, error) {
	remotePath := RemoteAgentPath(info, version, agent)
	uploaded, err := ensureRemoteFile(ctx, t, info, agent, remotePath, "agent")
	return remotePath, uploaded, err
}

// ensureRemoteFile makes remotePath byte-identical to local, uploading only
// when the target's SHA-256 differs. what names the file in errors.
func ensureRemoteFile(ctx context.Context, t Transport, info ProbeInfo, local LocalAgent, remotePath, what string) (bool, error) {
	if info.SkipAgentVerification {
		_, uploaded, err := uploadAgentUnverified(ctx, t, info, remotePath, local)
		return uploaded, err
	}
	if info.SHATool == "" {
		return false, &StageError{Stage: "probe", Err: errors.New("target has no supported SHA-256 tool (need sha256sum, shasum, openssl, or BusyBox sha256sum); use --skip-agent-verification only for a trusted target")}
	}
	var check RemoteCommand
	if info.OS == "windows" {
		check = powerShellScript(powerShellCheckScript(remotePath))
	} else {
		check = shCommand(checkScript, remotePath, info.SHATool)
	}
	out, err := execCapture(ctx, t, "check "+what, check, nil)
	if err != nil {
		return false, err
	}
	if remoteSHA(out) == local.SHA256 {
		return false, nil
	}
	if info.OS == "windows" {
		dir := remotePath[:strings.LastIndex(remotePath, `\`)]
		if _, err := execCapture(ctx, t, "prepare "+what+" upload", powerShellScript(powerShellPrepareUploadScript(dir)), nil); err != nil {
			return false, err
		}
		random := make([]byte, 12)
		if _, err := rand.Read(random); err != nil {
			return false, fmt.Errorf("create upload staging name: %w", err)
		}
		name := ".upload-" + hex.EncodeToString(random)
		tempPath := dir + `\` + name
		uploader, ok := t.(FileUploader)
		if !ok {
			return false, &StageError{Stage: "upload " + what, Err: errors.New("SSH transport does not support SFTP file upload")}
		}
		if err := uploader.UploadFile(ctx, local.Path, tempPath); err != nil {
			return false, &StageError{Stage: "upload " + what, Err: err}
		}
		out, err = execCapture(ctx, t, "verify uploaded "+what, powerShellScript(powerShellFinalizeUploadScript(tempPath, remotePath, local.SHA256)), nil)
	} else {
		f, openErr := os.Open(local.Path)
		if openErr != nil {
			return false, openErr
		}
		defer func() { _ = f.Close() }()
		upload := shCommand(uploadScript, path.Dir(remotePath), remotePath, info.SHATool, local.SHA256)
		out, err = execCapture(ctx, t, "upload "+what, upload, f)
	}
	if err != nil {
		return false, err
	}
	if got := remoteSHA(out); got != local.SHA256 {
		return false, &StageError{Stage: "upload " + what, Err: fmt.Errorf("remote sha256 %q does not match %s", got, local.SHA256)}
	}
	return true, nil
}

func uploadAgentUnverified(ctx context.Context, t Transport, info ProbeInfo, remotePath string, agent LocalAgent) (string, bool, error) {
	if info.OS == "windows" {
		dir := remotePath[:strings.LastIndex(remotePath, `\`)]
		if _, err := execCapture(ctx, t, "prepare agent upload", powerShellScript(powerShellPrepareUploadScript(dir)), nil); err != nil {
			return remotePath, false, err
		}
		random := make([]byte, 12)
		if _, err := rand.Read(random); err != nil {
			return remotePath, false, fmt.Errorf("create upload staging name: %w", err)
		}
		tempPath := dir + `\.upload-` + hex.EncodeToString(random)
		uploader, ok := t.(FileUploader)
		if !ok {
			return remotePath, false, &StageError{Stage: "upload agent", Err: errors.New("SSH transport does not support SFTP file upload")}
		}
		if err := uploader.UploadFile(ctx, agent.Path, tempPath); err != nil {
			_ = removeRemoteFile(ctx, t, info.OS, tempPath)
			return remotePath, false, &StageError{Stage: "upload agent", Err: err}
		}
		script := `$ErrorActionPreference='Stop'; $t=[Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('` + base64.StdEncoding.EncodeToString([]byte(tempPath)) + `')); ` +
			`$f=[Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('` + base64.StdEncoding.EncodeToString([]byte(remotePath)) + `')); Move-Item -LiteralPath $t -Destination $f -Force; Write-Output 'uploaded=1'`
		if _, err := execCapture(ctx, t, "install unverified agent", powerShellScript(script), nil); err != nil {
			_ = removeRemoteFile(ctx, t, info.OS, tempPath)
			return remotePath, false, err
		}
		return remotePath, true, nil
	}
	f, err := os.Open(agent.Path)
	if err != nil {
		return remotePath, false, err
	}
	defer func() { _ = f.Close() }()
	upload := shCommand(uploadUnverifiedScript, path.Dir(remotePath), remotePath)
	if _, err := execCapture(ctx, t, "upload unverified agent", upload, f); err != nil {
		return remotePath, false, err
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
