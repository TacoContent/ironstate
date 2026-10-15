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
	"strings"

	"github.com/spf13/cobra"

	ironexec "github.com/TacoContent/ironstate/internal/exec"
	"github.com/TacoContent/ironstate/internal/remote"
	"github.com/TacoContent/ironstate/internal/remoteexec"
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
	cmd.Flags().Bool("detached", false, "run without a live controller control channel (internal)")
	cmd.Flags().String("detached-job-file", "", "staged detached job file to remove after loading (internal)")
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
	detached, _ := cmd.Flags().GetBool("detached")
	protoOut := cmd.OutOrStdout()
	// Nothing but protocol events may reach the protocol stream.
	if detached {
		protoOut = io.Discard
	} else if protoOut == os.Stdout {
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
	detached, _ := cmd.Flags().GetBool("detached")
	if v, _ := cmd.Flags().GetInt("protocol"); v != protocol.Version {
		return 2, phaseJob, fmt.Errorf("controller requested protocol %d, agent speaks %d", v, protocol.Version)
	}
	in := bufio.NewReader(cmd.InOrStdin())
	job, err := protocol.ReadJob(in)
	if err != nil {
		return 2, phaseJob, err
	}
	if job.BecomePassword != "" {
		secrets.Register(job.BecomePassword)
		ironexec.SetBecomePassword(job.BecomePassword)
		defer ironexec.SetBecomePassword("")
	}
	ironexec.SetRemoteWindowsMode(runtime.GOOS == "windows", ironexec.WindowsAdmin())
	defer ironexec.SetRemoteWindowsMode(false, false)

	stateDir, _ := cmd.Flags().GetString("state-dir")
	if stateDir == "" {
		if stateDir, err = runstate.DefaultDir(); err != nil {
			return 2, phaseLock, err
		}
	}
	detachedJobFile, _ := cmd.Flags().GetString("detached-job-file")
	if detached != (detachedJobFile != "") {
		return 2, phaseJob, errors.New("detached mode requires exactly one staged job file")
	}
	if detached {
		want := filepath.Join(stateDir, "detached", job.RunID+".job")
		matches := filepath.Clean(detachedJobFile) == filepath.Clean(want)
		if runtime.GOOS == "windows" {
			matches = strings.EqualFold(filepath.Clean(detachedJobFile), filepath.Clean(want))
		}
		if !matches {
			return 2, phaseJob, errors.New("detached job file is outside the expected state directory")
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
	if detached {
		_ = os.Stdin.Close()
		if err := os.Remove(detachedJobFile); err != nil {
			return 2, phaseBundle, fmt.Errorf("remove staged detached job: %w", err)
		}
	} else {
		go watchControl(in, w)
	}

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

	// No TTY on the agent: trust was decided on the controller, which also
	// pre-fetched every approved git source into the bundle.
	approved := map[string]bool{}
	for _, source := range job.ApprovedUses {
		approved[source] = true
	}
	origConfirm, origCacheRoot, origOffline := remote.Confirm, remote.CacheRoot, remote.Offline
	remote.Confirm = func(_ remote.Kind, source string) (bool, error) { return approved[source], nil }
	usesRoot := filepath.Join(workDir, remoteexec.UsesBundleDir)
	remote.CacheRoot = func() (string, error) { return usesRoot, nil }
	remote.Offline = true
	defer func() { remote.Confirm, remote.CacheRoot, remote.Offline = origConfirm, origCacheRoot, origOffline }()

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
	if opts.DisableBecome {
		args = append(args, "--disable-become")
	}
	if opts.AllowPluginInstall {
		args = append(args, "--allow-plugin-install")
	}
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
