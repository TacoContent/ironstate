package remoteexec

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
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
	line, err := PosixCommandLine(cmd)
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
