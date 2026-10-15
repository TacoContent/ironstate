package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/TacoContent/ironstate/internal/engine"
	ironexec "github.com/TacoContent/ironstate/internal/exec"
)

// statHandler reports a path's metadata (Ansible's 'stat' module) and
// exposes it to an 'id'-registered result as '<id>.stat'. It shells out to
// a coreutils 'stat' (GNU/uutils, also 'gstat' on macOS) when one is on
// PATH, BSD 'stat -f' on other Unix hosts, and pwsh's Get-Item otherwise.
type statHandler struct{}

func (statHandler) Emoji() string { return "📊" }

func (statHandler) RequiredTools() []string { return []string{} }

// ReadOnly implements engine.ReadOnlyHandler: stat has no side effect, so
// it runs in dry runs too and a later 'when' sees a real value.
func (statHandler) ReadOnly() bool { return true }

func (statHandler) Test(item map[string]any, name string, ctx engine.Context) (bool, error) {
	return false, nil
}

func (statHandler) Describe(item map[string]any, action engine.Action, ctx engine.Context) (string, error) {
	return fmt.Sprintf("stat %s", getString(item, "path")), nil
}

func (statHandler) Install(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	rawPath := getString(item, "path")
	if rawPath == "" {
		return statFailure("'path' is required"), nil
	}
	path := resolvePath(rawPath)
	follow := getBool(item, "follow", false)

	data, err := statPath(path, follow)
	if err != nil {
		return statFailure(err.Error()), nil
	}
	message := fmt.Sprintf("%s: exists=%v", path, data["exists"])
	return engine.ExecResult{
		RC:          0,
		Stdout:      message,
		StdoutLines: []string{message},
		Extra:       map[string]any{"stat": data},
	}, nil
}

func (statHandler) Uninstall(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	return engine.ExecResult{}, nil
}

func statFailure(msg string) engine.ExecResult {
	msg = "stat: " + msg
	return engine.ExecResult{RC: 1, Stderr: msg, StderrLines: []string{msg}}
}

type statBackendKind int

const (
	statBackendCoreutils statBackendKind = iota
	statBackendBSD
	statBackendPwsh
)

func (k statBackendKind) String() string {
	switch k {
	case statBackendCoreutils:
		return "coreutils"
	case statBackendBSD:
		return "bsd"
	default:
		return "pwsh"
	}
}

type statBackend struct {
	exe  string
	kind statBackendKind
}

// resolveStatBackend probes once per process; overridable in tests.
var resolveStatBackend = sync.OnceValues(probeStatBackend)

var statLookPath = exec.LookPath

// statVersionOutput is bounded + given empty stdin: a PATH entry named
// 'stat' isn't guaranteed to honor '--version' and exit.
var statVersionOutput = func(exe string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, "--version") //nolint:gosec // exe is a fixed tool name resolved via PATH
	cmd.Stdin = strings.NewReader("")
	out, _ := cmd.Output()
	return string(out)
}

func probeStatBackend() (statBackend, error) {
	for _, name := range []string{"stat", "gstat"} {
		exe, err := statLookPath(name)
		if err != nil {
			continue
		}
		if strings.Contains(strings.ToLower(statVersionOutput(exe)), "coreutils") {
			return statBackend{exe: exe, kind: statBackendCoreutils}, nil
		}
	}
	if runtime.GOOS != "windows" {
		if exe, err := statLookPath("stat"); err == nil {
			return statBackend{exe: exe, kind: statBackendBSD}, nil
		}
	}
	if exe, err := statLookPath("pwsh"); err == nil {
		return statBackend{exe: exe, kind: statBackendPwsh}, nil
	}
	return statBackend{}, errors.New("no coreutils 'stat', BSD 'stat', or 'pwsh' found on PATH")
}

// statPath returns the registered 'stat' map for path. A missing path is
// not an error - it reports exists=false, like Ansible's stat.
func statPath(path string, follow bool) (map[string]any, error) {
	lstat := os.Lstat
	if follow {
		lstat = os.Stat
	}
	if _, err := lstat(path); errors.Is(err, fs.ErrNotExist) {
		return map[string]any{"exists": false, "path": path}, nil
	}

	backend, err := resolveStatBackend()
	if err != nil {
		return nil, err
	}

	var data map[string]any
	switch backend.kind {
	case statBackendPwsh:
		data, err = statViaPwsh(backend.exe, path, follow)
	default:
		data, err = statViaCLI(backend, path, follow)
	}
	if err != nil {
		return nil, err
	}
	data["exists"] = true
	data["path"] = path
	data["backend"] = backend.kind.String()
	return data, nil
}

// Field order: raw mode, symbolic perms, uid, owner, gid, group, inode,
// device, link count, size, atime, mtime, ctime. Coreutils gets '\n'
// escapes via --printf: MSYS2's stat.exe (Git for Windows) splits argv on
// literal newlines. BSD stat (Unix only) takes literal newlines.
const (
	statCoreutilsFormat = `%f\n%A\n%u\n%U\n%g\n%G\n%i\n%d\n%h\n%s\n%X\n%Y\n%Z\n`
	statBSDFormat       = "%p\n%Sp\n%u\n%Su\n%g\n%Sg\n%i\n%d\n%l\n%z\n%a\n%m\n%c"
)

func statViaCLI(backend statBackend, path string, follow bool) (map[string]any, error) {
	args := []string{}
	if follow {
		args = append(args, "-L")
	}
	modeBase := 16
	if backend.kind == statBackendBSD {
		args = append(args, "-f", statBSDFormat)
		modeBase = 8
	} else {
		args = append(args, "--printf="+statCoreutilsFormat)
	}
	args = append(args, "--", path)

	out, err := runStatCommand(backend.exe, args)
	if err != nil {
		return nil, err
	}
	return parseStatCLIOutput(out, modeBase, path, follow)
}

func runStatCommand(exe string, args []string) (string, error) {
	exe, args, err := ironexec.WrapForBecome(ironexec.CurrentBecome(), exe, args)
	if err != nil {
		return "", err
	}
	result, err := runner.Run(exe, args)
	if err != nil {
		return "", err
	}
	if result.RC != 0 {
		msg := strings.TrimSpace(result.Stderr)
		if msg == "" {
			msg = fmt.Sprintf("%s exited %d", exe, result.RC)
		}
		return "", errors.New(msg)
	}
	return result.Stdout, nil
}

func parseStatCLIOutput(out string, modeBase int, path string, follow bool) (map[string]any, error) {
	lines := strings.Split(strings.TrimRight(strings.ReplaceAll(out, "\r\n", "\n"), "\n"), "\n")
	if len(lines) < 13 {
		return nil, fmt.Errorf("unexpected stat output: %q", out)
	}
	rawMode, err := strconv.ParseUint(strings.TrimSpace(lines[0]), modeBase, 32)
	if err != nil {
		return nil, fmt.Errorf("parse mode %q: %w", lines[0], err)
	}

	const (
		sIFMT   = 0o170000
		sIFSOCK = 0o140000
		sIFLNK  = 0o120000
		sIFREG  = 0o100000
		sIFBLK  = 0o060000
		sIFDIR  = 0o040000
		sIFCHR  = 0o020000
		sIFIFO  = 0o010000
	)
	fileType := rawMode & sIFMT
	data := map[string]any{
		"mode":        fmt.Sprintf("%04o", rawMode&0o7777),
		"permissions": strings.TrimSpace(lines[1]),
		"isdir":       fileType == sIFDIR,
		"isreg":       fileType == sIFREG,
		"islnk":       fileType == sIFLNK,
		"isblk":       fileType == sIFBLK,
		"ischr":       fileType == sIFCHR,
		"isfifo":      fileType == sIFIFO,
		"issock":      fileType == sIFSOCK,
		"uid":         statNumber(lines[2]),
		"pw_name":     strings.TrimSpace(lines[3]),
		"gid":         statNumber(lines[4]),
		"gr_name":     strings.TrimSpace(lines[5]),
		// Kept as strings: inode/device numbers can exceed float64's exact range.
		"inode": strings.TrimSpace(lines[6]),
		"dev":   strings.TrimSpace(lines[7]),
		"nlink": statNumber(lines[8]),
		"size":  statNumber(lines[9]),
		"atime": statNumber(lines[10]),
		"mtime": statNumber(lines[11]),
		"ctime": statNumber(lines[12]),
	}
	if !follow && fileType == sIFLNK {
		if target, err := os.Readlink(path); err == nil {
			data["lnk_target"] = target
		}
	}
	return data, nil
}

func statNumber(s string) any {
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return nil
	}
	return f
}

const statPwshScript = `$ErrorActionPreference = 'Stop'
$i = Get-Item -LiteralPath %s -Force
if (%s -and $i.LinkTarget) {
  $t = $i.ResolveLinkTarget($true)
  if ($t) { $i = Get-Item -LiteralPath $t.FullName -Force }
}
$owner = $null; $group = $null
try { $acl = Get-Acl -LiteralPath $i.FullName; $owner = $acl.Owner; $group = $acl.Group } catch {}
$epoch = [DateTime]::new(1970, 1, 1, 0, 0, 0, [DateTimeKind]::Utc)
$isLink = [bool]$i.LinkTarget
$mode = $null
if (-not $IsWindows) { $mode = '{0:D4}' -f [Convert]::ToInt32([Convert]::ToString([int]$i.UnixFileMode, 8)) }
[ordered]@{
  isdir      = [bool]$i.PSIsContainer -and -not $isLink
  isreg      = -not $i.PSIsContainer -and -not $isLink
  islnk      = $isLink
  lnk_target = $i.LinkTarget
  size       = if ($i.PSIsContainer) { 0 } else { [double]$i.Length }
  mode       = $mode
  attributes = $i.Attributes.ToString()
  pw_name    = $owner
  gr_name    = $group
  atime      = ($i.LastAccessTimeUtc - $epoch).TotalSeconds
  mtime      = ($i.LastWriteTimeUtc - $epoch).TotalSeconds
  ctime      = ($i.CreationTimeUtc - $epoch).TotalSeconds
} | ConvertTo-Json -Compress`

func statViaPwsh(exe, path string, follow bool) (map[string]any, error) {
	followLiteral := "$false"
	if follow {
		followLiteral = "$true"
	}
	script := fmt.Sprintf(statPwshScript, pwshSingleQuote(path), followLiteral)
	out, err := runStatCommand(exe, []string{"-NoProfile", "-NonInteractive", "-Command", script})
	if err != nil {
		return nil, err
	}
	data := map[string]any{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &data); err != nil {
		return nil, fmt.Errorf("parse pwsh output %q: %w", truncateStatOutput(out), err)
	}
	for k, v := range data {
		if v == nil {
			delete(data, k)
		}
	}
	return data, nil
}

// pwshSingleQuote doubles every quote character PowerShell treats as a
// single-quote delimiter (including the typographic ones) so path can't
// break out of the literal.
func pwshSingleQuote(s string) string {
	var b strings.Builder
	b.WriteByte('\'')
	for _, r := range s {
		switch r {
		case '\'', '\u2018', '\u2019', '\u201A', '\u201B':
			b.WriteRune(r)
		}
		b.WriteRune(r)
	}
	b.WriteByte('\'')
	return b.String()
}

func truncateStatOutput(s string) string {
	if len(s) > 200 {
		return s[:200] + "...(truncated)"
	}
	return s
}
