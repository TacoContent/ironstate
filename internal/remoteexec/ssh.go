package remoteexec

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
)

// LocalTarget is the reserved target name for running the agent as a
// local child process instead of over SSH.
const LocalTarget = "local"

// SSHExitUnreachable is the exit code OpenSSH uses for its own failures
// (connection, auth, host key), as opposed to the remote command's.
const SSHExitUnreachable = 255

var (
	sshUserPattern = regexp.MustCompile(`^[A-Za-z0-9._][A-Za-z0-9._-]*$`)
	sshHostPattern = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._-]*$`)
	sshIPv6Pattern = regexp.MustCompile(`^\[[0-9A-Fa-f:.]+\]$`)
)

// SSHTarget is a parsed [user@]host[:port] destination.
type SSHTarget struct {
	User string
	Host string
	Port int
}

// String renders the target as given.
func (t SSHTarget) String() string {
	s := t.Host
	if strings.Contains(s, ":") {
		s = "[" + s + "]"
	}
	if t.User != "" {
		s = t.User + "@" + s
	}
	if t.Port != 0 {
		s += ":" + strconv.Itoa(t.Port)
	}
	return s
}

// ParseSSHTarget validates [user@]host[:port]. The host may be an
// ~/.ssh/config alias or a bracketed IPv6 literal. Anything that could be
// read as an ssh option is rejected.
func ParseSSHTarget(s string) (SSHTarget, error) {
	var t SSHTarget
	rest := s
	if at := strings.LastIndex(rest, "@"); at >= 0 {
		t.User, rest = rest[:at], rest[at+1:]
		if !sshUserPattern.MatchString(t.User) {
			return t, fmt.Errorf("invalid ssh user in target %q", s)
		}
	}
	host := rest
	if strings.HasPrefix(rest, "[") {
		end := strings.Index(rest, "]")
		if end < 0 {
			return t, fmt.Errorf("invalid IPv6 target %q", s)
		}
		host = rest[:end+1]
		rest = rest[end+1:]
		if rest != "" {
			if !strings.HasPrefix(rest, ":") {
				return t, fmt.Errorf("invalid target %q", s)
			}
			if err := t.setPort(rest[1:], s); err != nil {
				return t, err
			}
		}
		if !sshIPv6Pattern.MatchString(host) {
			return t, fmt.Errorf("invalid IPv6 target %q", s)
		}
		t.Host = strings.Trim(host, "[]")
		return t, nil
	}
	if colon := strings.LastIndex(rest, ":"); colon >= 0 {
		host = rest[:colon]
		if err := t.setPort(rest[colon+1:], s); err != nil {
			return t, err
		}
	}
	if !sshHostPattern.MatchString(host) {
		return t, fmt.Errorf("invalid ssh host in target %q", s)
	}
	t.Host = host
	return t, nil
}

func (t *SSHTarget) setPort(p, original string) error {
	port, err := strconv.Atoi(p)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("invalid port in target %q", original)
	}
	t.Port = port
	return nil
}

// SSHOptions configures the OpenSSH client invocation.
type SSHOptions struct {
	// SSHPath defaults to "ssh" on PATH.
	SSHPath string
	// SFTPPath defaults to "sftp" on PATH.
	SFTPPath string
	// ConfigFile, if set, is passed as 'ssh -F'.
	ConfigFile        string
	ConnectTimeout    time.Duration
	AcceptNewHostKeys bool
}

// SSHTransport runs commands through the system OpenSSH client, so the
// user's ssh config, agent and known_hosts all apply.
type SSHTransport struct {
	Target      SSHTarget
	Options     SSHOptions
	controlPath string
}

// NewSSHTransport returns a transport for target. On POSIX controllers it
// enables connection multiplexing when a private socket dir is available.
func NewSSHTransport(target SSHTarget, opts SSHOptions) *SSHTransport {
	return &SSHTransport{Target: target, Options: opts, controlPath: controlPath()}
}

func (t *SSHTransport) sshPath() string {
	if t.Options.SSHPath != "" {
		return t.Options.SSHPath
	}
	return "ssh"
}

func (t *SSHTransport) sftpPath() string {
	if t.Options.SFTPPath != "" {
		return t.Options.SFTPPath
	}
	return "sftp"
}

// baseArgs are the ssh options shared by every invocation, ending with
// '--' and the host so nothing after can be read as an option.
func (t *SSHTransport) baseArgs() []string {
	timeout := t.Options.ConnectTimeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	args := []string{"-T",
		"-o", "BatchMode=yes",
		"-o", fmt.Sprintf("ConnectTimeout=%d", int(timeout.Seconds())),
		"-o", "ServerAliveInterval=15",
		"-o", "ServerAliveCountMax=4",
	}
	if t.Options.ConfigFile != "" {
		args = append([]string{"-F", t.Options.ConfigFile}, args...)
	}
	if t.Options.AcceptNewHostKeys {
		args = append(args, "-o", "StrictHostKeyChecking=accept-new")
	}
	if t.controlPath != "" {
		args = append(args, "-o", "ControlMaster=auto", "-o", "ControlPath="+t.controlPath, "-o", "ControlPersist=60")
	}
	if t.Target.Port != 0 {
		args = append(args, "-p", strconv.Itoa(t.Target.Port))
	}
	if t.Target.User != "" {
		args = append(args, "-l", t.Target.User)
	}
	return append(args, "--", t.Target.Host)
}

// Exec implements Transport. Exit code 255 is OpenSSH's own failure.
func (t *SSHTransport) Exec(ctx context.Context, cmd RemoteCommand, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	var line string
	var err error
	if cmd.Windows {
		if cmd.Script != "" {
			line, err = PowerShellCommandLine(cmd.Script)
		} else {
			line, err = PowerShellProgramCommandLine(cmd.Program, cmd.Args)
		}
	} else {
		line, err = PosixCommandLine(cmd)
	}
	if err != nil {
		return -1, err
	}
	args := append(t.baseArgs(), line)
	c := exec.CommandContext(ctx, t.sshPath(), args...) //nolint:gosec // argv only; host validated, remote command quoted
	c.Stdin = stdin
	c.Stdout = stdout
	c.Stderr = stderr
	c.WaitDelay = waitDelay
	isolateSignals(c)
	code, err := runCommand(c)
	if errors.Is(err, exec.ErrNotFound) {
		return -1, fmt.Errorf("ssh client not found on PATH: %w", err)
	}
	return code, err
}

// UploadFile sends a file using OpenSSH's SFTP subsystem. The batch command
// contains only quoted paths; file contents never pass through a shell.
func (t *SSHTransport) UploadFile(ctx context.Context, localPath, remotePath string) error {
	args := []string{"-b", "-", "-o", "BatchMode=yes"}
	timeout := t.Options.ConnectTimeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	args = append(args, "-o", fmt.Sprintf("ConnectTimeout=%d", int(timeout.Seconds())), "-o", "ServerAliveInterval=15", "-o", "ServerAliveCountMax=4")
	if t.Options.ConfigFile != "" {
		args = append(args, "-F", t.Options.ConfigFile)
	}
	if t.Options.AcceptNewHostKeys {
		args = append(args, "-o", "StrictHostKeyChecking=accept-new")
	}
	if t.controlPath != "" {
		args = append(args, "-o", "ControlMaster=auto", "-o", "ControlPath="+t.controlPath, "-o", "ControlPersist=60")
	}
	if t.Target.Port != 0 {
		args = append(args, "-P", strconv.Itoa(t.Target.Port))
	}
	if t.Target.User != "" {
		args = append(args, "-o", "User="+t.Target.User)
	}
	args = append(args, t.Target.Host)

	c := exec.CommandContext(ctx, t.sftpPath(), args...) //nolint:gosec // destination and local path are passed as argv/batch data
	c.Stdin = strings.NewReader(sftpBatchPut(localPath, remotePath))
	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr
	c.WaitDelay = waitDelay
	isolateSignals(c)
	code, err := runCommand(c)
	if err != nil {
		return fmt.Errorf("run sftp: %w", err)
	}
	if code != 0 {
		detail := strings.TrimSpace(strings.Join([]string{stdout.String(), stderr.String()}, "\n"))
		if hint := sftpStartupNoiseHint(detail); hint != "" {
			return fmt.Errorf("sftp upload failed: %s", hint)
		}
		return fmt.Errorf("sftp exited %d: %s", code, detail)
	}
	return nil
}

func sftpStartupNoiseHint(message string) string {
	const marker = "Received message too long "
	_, suffix, ok := strings.Cut(message, marker)
	if !ok {
		return ""
	}
	digits := strings.TrimLeft(suffix, " \t")
	end := strings.IndexFunc(digits, func(r rune) bool { return r < '0' || r > '9' })
	if end >= 0 {
		digits = digits[:end]
	}
	value, err := strconv.ParseUint(digits, 10, 32)
	if err != nil {
		return ""
	}
	var prefix [4]byte
	binary.BigEndian.PutUint32(prefix[:], uint32(value))
	for _, b := range prefix {
		if b < 0x20 || b > 0x7e {
			return ""
		}
	}
	return fmt.Sprintf("remote SFTP startup emitted %q before its protocol handshake; silence stdout from non-interactive shell/PowerShell startup scripts", string(prefix[:]))
}

func sftpBatchPut(localPath, remotePath string) string {
	localPath = filepath.ToSlash(localPath)
	remotePath = strings.ReplaceAll(remotePath, `\`, "/")
	return "put " + sftpQuote(localPath) + " " + sftpQuote(remotePath) + "\n"
}

func sftpQuote(value string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(value) + `"`
}

// PowerShellCommandLine encodes a script as UTF-16LE, as required by
// powershell.exe -EncodedCommand. The encoded text contains no shell syntax.
func PowerShellCommandLine(script string) (string, error) {
	if strings.ContainsRune(script, '\x00') {
		return "", errors.New("PowerShell script contains a NUL")
	}
	encoded := base64.StdEncoding.EncodeToString(utf16LE(script))
	return "powershell.exe -NoLogo -NoProfile -NonInteractive -EncodedCommand " + encoded, nil
}

func utf16LE(script string) []byte {
	units := utf16.Encode([]rune(script))
	encoded := make([]byte, len(units)*2)
	for i, unit := range units {
		binary.LittleEndian.PutUint16(encoded[i*2:], unit)
	}
	return encoded
}

// PowerShellProgramCommandLine invokes a structured program/argv pair without
// interpolating any of its values into PowerShell source.
func PowerShellProgramCommandLine(program string, args []string) (string, error) {
	payload, err := json.Marshal(struct {
		Program string   `json:"program"`
		Args    []string `json:"args"`
	}{program, args})
	if err != nil {
		return "", err
	}
	data := base64.StdEncoding.EncodeToString(payload)
	script := "$d=[Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('" + data + "'))|ConvertFrom-Json; $a=@($d.args); & $d.program @a; if ($null -ne $LASTEXITCODE) { exit $LASTEXITCODE }; exit 0"
	return PowerShellCommandLine(script)
}

// Close tears down the multiplexed master connection, if any.
func (t *SSHTransport) Close() error {
	if t.controlPath == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	args := []string{"-O", "exit", "-o", "ControlPath=" + t.controlPath}
	if t.Options.ConfigFile != "" {
		args = append([]string{"-F", t.Options.ConfigFile}, args...)
	}
	args = append(args, "--", t.Target.Host)
	c := exec.CommandContext(ctx, t.sshPath(), args...) //nolint:gosec // argv only
	_ = c.Run()
	return nil
}

// PosixCommandLine renders cmd as one POSIX-shell command line with every
// word single-quoted, for sshd to hand to the remote login shell.
func PosixCommandLine(cmd RemoteCommand) (string, error) {
	words := append([]string{cmd.Program}, cmd.Args...)
	quoted := make([]string, len(words))
	for i, w := range words {
		if strings.ContainsAny(w, "\x00\n\r") {
			return "", fmt.Errorf("remote command word %q contains a NUL or newline", w)
		}
		quoted[i] = "'" + strings.ReplaceAll(w, "'", `'\''`) + "'"
	}
	return strings.Join(quoted, " "), nil
}
