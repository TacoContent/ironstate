package cli

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/TacoContent/ironstate/internal/remote"
	"github.com/TacoContent/ironstate/internal/remoteexec/bundle"
	"github.com/TacoContent/ironstate/internal/remoteexec/protocol"
	"github.com/TacoContent/ironstate/internal/remoteexec/runstate"
	"github.com/TacoContent/ironstate/internal/secrets"
)

// newAgentCommand is the target-side half of remote apply: it reads a job
// header + bundle on stdin and streams protocol events on stdout. Not meant
// to be run by hand.
func newAgentCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "agent",
		Short:  "Run a remote-apply job read from stdin (internal)",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE:   runAgent,
	}
	cmd.Flags().Int("protocol", protocol.Version, "protocol version the controller speaks")
	cmd.Flags().Bool("keep-work-dir", false, "keep the unpacked bundle after the run (debugging)")
	cmd.Flags().String("state-dir", "", "directory for the apply lock and run logs (default: user cache dir/ironstate)")
	return cmd
}

// Agent failure phases reported in 'error' events. Bootstrap phases mean
// nothing was applied, so the controller may treat them as retryable.
const (
	phaseJob    = "job"
	phaseLock   = "lock"
	phaseBundle = "bundle"
	phaseApply  = "apply"
)

func runAgent(cmd *cobra.Command, _ []string) error {
	protoOut := cmd.OutOrStdout()
	// Nothing but protocol events may reach the protocol stream.
	if protoOut == os.Stdout {
		protoFile, err := protectStdout()
		if err != nil {
			return &ExitCodeError{Code: 2, Err: fmt.Errorf("protect stdout: %w", err)}
		}
		defer func() { _ = protoFile.Close() }()
		protoOut = protoFile
		origStdout := os.Stdout
		os.Stdout = os.Stderr
		defer func() { os.Stdout = origStdout }()
	}
	defer agentSignals()()
	w := protocol.NewWriter(protoOut)

	code, phase, err := agentRun(cmd, w)
	if !w.IsDone() {
		_ = w.Fail(phase, err, code)
	}
	if code != 0 {
		if err == nil {
			err = fmt.Errorf("agent run exited %d", code)
		}
		return &ExitCodeError{Code: code, Err: err}
	}
	return nil
}

// agentRun returns the exit code to report, the failing phase, and the
// error behind it.
func agentRun(cmd *cobra.Command, w *protocol.Writer) (int, string, error) {
	if v, _ := cmd.Flags().GetInt("protocol"); v != protocol.Version {
		return 2, phaseJob, fmt.Errorf("controller requested protocol %d, agent speaks %d", v, protocol.Version)
	}
	in := bufio.NewReader(cmd.InOrStdin())
	job, err := protocol.ReadJob(in)
	if err != nil {
		return 2, phaseJob, err
	}

	stateDir, _ := cmd.Flags().GetString("state-dir")
	if stateDir == "" {
		if stateDir, err = runstate.DefaultDir(); err != nil {
			return 2, phaseLock, err
		}
	}
	lock, err := runstate.Acquire(stateDir, job.RunID)
	if err != nil {
		return 2, phaseLock, err
	}
	defer func() { _ = lock.Release() }()
	runLog, err := runstate.OpenRunLog(stateDir, job.RunID)
	if err != nil {
		return 2, phaseLock, err
	}
	defer func() { _ = runLog.Close() }()
	w.SetLog(runLog)

	if err := w.Hello(job.RunID, version, runtime.GOOS, runtime.GOARCH, os.Getpid(), runLog.Name()); err != nil {
		return 1, phaseJob, err
	}

	tempDir, err := os.MkdirTemp("", "ironstate-agent-")
	if err != nil {
		return 2, phaseBundle, err
	}
	if keep, _ := cmd.Flags().GetBool("keep-work-dir"); !keep {
		defer func() { _ = os.RemoveAll(tempDir) }()
	}
	workDir := filepath.Join(tempDir, "work")
	if err := os.Mkdir(workDir, 0o700); err != nil {
		return 2, phaseBundle, err
	}
	if err := receiveBundle(in, job.Bundle, tempDir, workDir); err != nil {
		return 2, phaseBundle, err
	}
	go watchControl(in, w)

	for key, value := range job.SecretEnv {
		secrets.Register(value)
		if err := os.Setenv(key, value); err != nil {
			return 2, phaseApply, err
		}
	}
	for key, value := range job.Env {
		if err := os.Setenv(key, value); err != nil {
			return 2, phaseApply, err
		}
	}

	// No TTY on the agent: remote 'uses:' must be approved on the controller.
	origConfirm := remote.Confirm
	remote.Confirm = func(remote.Kind, string) (bool, error) { return false, nil }
	defer func() { remote.Confirm = origConfirm }()

	origWD, err := os.Getwd()
	if err != nil {
		return 2, phaseApply, err
	}
	if err := os.Chdir(workDir); err != nil {
		return 2, phaseApply, err
	}
	defer func() { _ = os.Chdir(origWD) }()

	root, err := newRootCommand()
	if err != nil {
		return 2, phaseApply, err
	}
	root.SetArgs(agentApplyArgs(job.Options))
	root.SetIn(eofReader{})
	root.SetOut(os.Stderr)
	root.SetErr(os.Stderr)
	runErr := root.ExecuteContext(withProtocolWriter(cmd.Context(), w))
	return ExitCodeFor(runErr), phaseApply, runErr
}

// watchControl turns controller control lines into cancellation: an
// explicit cancel, or the channel closing (controller gone).
func watchControl(in *bufio.Reader, w *protocol.Writer) {
	for {
		typ, err := protocol.ReadControl(in)
		if err != nil {
			w.Cancel(errors.New("controller disconnected"))
			return
		}
		if typ == protocol.TypeCancel {
			w.Cancel(errors.New("cancelled by controller"))
		}
	}
}

// receiveBundle copies exactly info.Size bytes from in to a temp file,
// verifies the sha256, and only then extracts into workDir.
func receiveBundle(in io.Reader, info protocol.BundleInfo, tempDir, workDir string) error {
	archive := filepath.Join(tempDir, "bundle.tar.gz")
	f, err := os.OpenFile(archive, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600) //nolint:gosec // path inside our own 0700 temp dir
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	hash := sha256.New()
	if _, err := io.CopyN(io.MultiWriter(f, hash), in, info.Size); err != nil {
		return fmt.Errorf("receive bundle: %w", err)
	}
	if got := hex.EncodeToString(hash.Sum(nil)); got != info.SHA256 {
		return fmt.Errorf("bundle sha256 mismatch: got %s, want %s", got, info.SHA256)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	if err := bundle.Extract(f, workDir); err != nil {
		return fmt.Errorf("extract bundle: %w", err)
	}
	return nil
}

func agentApplyArgs(opts protocol.JobOptions) []string {
	args := []string{"--playbook", opts.Playbook, "--output", "ndjson", "--no-color"}
	if opts.Apply {
		args = append(args, "--apply")
	}
	if opts.Verbose {
		args = append(args, "--verbose")
	}
	for _, tag := range opts.Tags {
		args = append(args, "--tags", tag)
	}
	for _, file := range opts.VarsFiles {
		args = append(args, "--vars-file", file)
	}
	for _, override := range opts.VarOverrides {
		args = append(args, "--var", override)
	}
	return args
}

type eofReader struct{}

func (eofReader) Read([]byte) (int, error) { return 0, io.EOF }
