package remoteexec_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TacoContent/ironstate/internal/remote"
	"github.com/TacoContent/ironstate/internal/remoteexec"
)

// fakeGit makes remote clones write a role whose task logs message.
func fakeGit(t *testing.T, message string) *int {
	t.Helper()
	calls := 0
	origGit, origRoot, origConfirm := remote.RunGit, remote.CacheRoot, remote.Confirm
	cache := t.TempDir()
	remote.CacheRoot = func() (string, error) { return cache, nil }
	remote.RunGit = func(_ string, args ...string) error {
		calls++
		dest := args[len(args)-1]
		if err := os.MkdirAll(dest, 0o750); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dest, "main.yml"), []byte("tasks:\n  - name: from remote\n    log:\n      message: "+message+"\n"), 0o600)
	}
	t.Cleanup(func() { remote.RunGit, remote.CacheRoot, remote.Confirm = origGit, origRoot, origConfirm })
	return &calls
}

func logMessages(result *remoteexec.HostResult) string {
	var logs []string
	for _, ev := range result.Logs {
		logs = append(logs, ev.Message)
	}
	return strings.Join(logs, "\n")
}

func TestRemoteUsesArePrefetchedAndRunOfflineOnTarget(t *testing.T) {
	calls := fakeGit(t, "remote role ran")
	job := prepareSimple(t, "tasks:\n  - name: r\n    uses:\n      remote: https://github.com/acme/roles.git\n      ref: v1\n      trusted: true\n")
	if *calls == 0 || len(job.PrefetchedUses) != 1 || len(job.Job.ApprovedUses) != 1 {
		t.Fatalf("git calls=%d prefetched=%v approved=%v", *calls, job.PrefetchedUses, job.Job.ApprovedUses)
	}
	result, err := runPrepared(t, job, remoteexec.RunOptions{})
	if err != nil {
		t.Fatalf("RunHost: %v (stderr %s)", err, result.Stderr)
	}
	if result.ExitCode != 0 || !strings.Contains(logMessages(result), "remote role ran") {
		t.Fatalf("exit=%d errors=%v logs:\n%s", result.ExitCode, result.Errors, logMessages(result))
	}
}

func TestRemoteUsesNeedingTrustAreApprovedOnController(t *testing.T) {
	calls := fakeGit(t, "approved role ran")
	var asked []string
	remote.Confirm = func(_ remote.Kind, source string) (bool, error) {
		asked = append(asked, source)
		return true, nil
	}
	job := prepareSimple(t, "tasks:\n  - name: r\n    uses:\n      remote: https://github.com/acme/roles.git\n")
	if len(asked) != 1 || *calls == 0 {
		t.Fatalf("asked=%v git calls=%d, want one prompt then a fetch", asked, *calls)
	}
	result, err := runPrepared(t, job, remoteexec.RunOptions{})
	if err != nil || result.ExitCode != 0 || !strings.Contains(logMessages(result), "approved role ran") {
		t.Fatalf("err=%v exit=%d errors=%v logs:\n%s", err, result.ExitCode, result.Errors, logMessages(result))
	}
}

func TestDeclinedRemoteUsesIsSkippedOnTarget(t *testing.T) {
	calls := fakeGit(t, "should not run")
	remote.Confirm = func(remote.Kind, string) (bool, error) { return false, nil }
	job := prepareSimple(t, "tasks:\n  - name: r\n    uses:\n      remote: https://github.com/acme/roles.git\n")
	if *calls != 0 || len(job.Job.ApprovedUses) != 0 {
		t.Fatalf("declined source was fetched (calls=%d) or approved (%v)", *calls, job.Job.ApprovedUses)
	}
	result, err := runPrepared(t, job, remoteexec.RunOptions{})
	if err != nil || result.ExitCode != 0 {
		t.Fatalf("err=%v exit=%d errors=%v", err, result.ExitCode, result.Errors)
	}
	logs := logMessages(result)
	if strings.Contains(logs, "should not run") || !strings.Contains(logs, "declined") {
		t.Fatalf("declined source not skipped with a warning:\n%s", logs)
	}
}

func TestTemplatedGitUsesFailsOfflineOnTarget(t *testing.T) {
	calls := fakeGit(t, "unused")
	job := prepareSimple(t, "vars:\n  repo: https://github.com/acme/roles.git\ntasks:\n  - name: r\n    uses:\n      remote: \"${{ vars.repo }}\"\n      trusted: true\n")
	if *calls != 0 {
		t.Fatalf("templated source was fetched on the controller")
	}
	result, err := runPrepared(t, job, remoteexec.RunOptions{})
	if err != nil {
		t.Fatalf("RunHost: %v", err)
	}
	if result.ExitCode == 0 || !strings.Contains(strings.Join(result.Errors, " "), "not pre-fetched") {
		t.Fatalf("exit=%d errors=%v, want an offline pre-fetch error", result.ExitCode, result.Errors)
	}
}
