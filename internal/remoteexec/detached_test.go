package remoteexec

import (
	"bufio"
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/TacoContent/ironstate/internal/remoteexec/protocol"
)

func TestCreateDetachedSpoolKeepsPrivateJobAndBundle(t *testing.T) {
	bundlePath := filepath.Join(t.TempDir(), "bundle.tar.gz")
	if err := os.WriteFile(bundlePath, []byte("bundle"), 0o600); err != nil {
		t.Fatal(err)
	}
	job := &PreparedJob{
		Job:        protocol.Job{RunID: "run-123", Options: protocol.JobOptions{Apply: true}},
		BundlePath: bundlePath,
	}
	spoolPath, err := createDetachedSpool(job)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(spoolPath) }()
	info, err := os.Stat(spoolPath)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("spool permissions = %o, want 600", info.Mode().Perm())
	}
	spool, err := os.Open(spoolPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = spool.Close() }()
	reader := bufio.NewReader(spool)
	readJob, err := protocol.ReadJob(reader)
	if err != nil {
		t.Fatal(err)
	}
	if readJob.RunID != job.Job.RunID {
		t.Fatalf("run id = %q", readJob.RunID)
	}
	if got, err := io.ReadAll(reader); err != nil || string(got) != "bundle" {
		t.Fatalf("bundle = %q, err=%v", got, err)
	}
}

func TestDetachedWindowsLaunchUsesIndependentProcessAndStagedInput(t *testing.T) {
	cmd := detachedPowerShellCommand(`C:\Program Files\ironstate.exe`, `C:\Data\run.job`)
	if !cmd.Windows || !strings.Contains(cmd.Script, "Start-Process") || !strings.Contains(cmd.Script, "-RedirectStandardInput $job") || !strings.Contains(cmd.Script, "--detached-job-file") {
		t.Fatalf("unexpected detached launch command: %+v", cmd)
	}
}

type detachedTestTransport struct{}

func (detachedTestTransport) Exec(_ context.Context, _ RemoteCommand, _ io.Reader, _, _ io.Writer) (int, error) {
	return 0, nil
}

func (detachedTestTransport) UploadFile(context.Context, string, string) error { return nil }
func (detachedTestTransport) Close() error                                     { return nil }

func TestStartDetachedRejectsEnvironmentSecrets(t *testing.T) {
	job := &PreparedJob{Job: protocol.Job{Env: map[string]string{"TOKEN": "value"}}}
	err := StartDetached(context.Background(), detachedTestTransport{}, "agent", ProbeInfo{OS: "linux", StateDir: "/cache/ironstate"}, job)
	if err == nil || !strings.Contains(err.Error(), "cannot stage") {
		t.Fatalf("error = %v", err)
	}
}
