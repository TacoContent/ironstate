package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunApplyLoadsDotEnvAndDotSecretsFromCWD(t *testing.T) {
	dir := t.TempDir()
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(origWD) }()

	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("TEST_ENV_CLI_VAR=from-dotenv\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".secrets"), []byte("TEST_SECRETS_CLI_VAR=from-dotsecrets\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.Unsetenv("TEST_ENV_CLI_VAR")
		_ = os.Unsetenv("TEST_SECRETS_CLI_VAR")
	}()

	sitePath := filepath.Join(dir, "main.yml")
	if err := os.WriteFile(sitePath, []byte("---\nvars: {}\ntasks: []\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd, err := newRootCommand()
	if err != nil {
		t.Fatal(err)
	}
	cmd.SetArgs([]string{"--playbook", sitePath, "--output", "json"})
	cmd.SetOut(new(bytes.Buffer))
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	if got := os.Getenv("TEST_ENV_CLI_VAR"); got != "from-dotenv" {
		t.Errorf("TEST_ENV_CLI_VAR = %q, want %q (.env not loaded)", got, "from-dotenv")
	}
	if got := os.Getenv("TEST_SECRETS_CLI_VAR"); got != "from-dotsecrets" {
		t.Errorf("TEST_SECRETS_CLI_VAR = %q, want %q (.secrets not loaded)", got, "from-dotsecrets")
	}
}

func TestRunApplyMergesVarsFileAndVarOverrides(t *testing.T) {
	dir := t.TempDir()

	sitePath := filepath.Join(dir, "main.yml")
	if err := os.WriteFile(sitePath, []byte(`
vars:
  editor: code
  ssh:
    port: 21
tasks:
  - name: verify merged vars
    assert:
      that:
        - "editor == 'code'"
        - "theme == 'dark'"
        - "font == 'mono'"
        - "ssh.port == '22'"
`), 0o600); err != nil {
		t.Fatal(err)
	}

	varsFilePath := filepath.Join(dir, "extra-vars.yml")
	if err := os.WriteFile(varsFilePath, []byte("---\nvars:\n  theme: light\n  font: mono\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// A second --vars-file, applied after the first, so it wins on the
	// overlapping 'theme' key while 'font' (only set by the first file)
	// survives untouched.
	secondVarsFilePath := filepath.Join(dir, "override-vars.yml")
	if err := os.WriteFile(secondVarsFilePath, []byte("---\nvars:\n  theme: dark\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd, err := newRootCommand()
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	cmd.SetArgs([]string{
		"--playbook", sitePath,
		"--vars-file", varsFilePath,
		"--vars-file", secondVarsFilePath,
		"--var", "ssh.port=22",
		"--output", "json",
	})
	cmd.SetOut(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute error (assertion of merged vars failed): %v\noutput: %s", err, out.String())
	}
}

// TestRunApplyProgressHasNoPersistentPhaseLines guards against a past
// regression where progress.Message printed a persistent "[playbook] ..."
// line per phase instead of only updating the spinner's suffix in place -
// see progress.go's Message doc comment. It can't assert the spinner
// itself is used (bytes.Buffer isn't a real tty, so ui.Enabled is false
// and the spinner never animates under test - see progress_test.go's top
// comment), only that the old plain-line behavior doesn't come back.
func TestRunApplyProgressHasNoPersistentPhaseLines(t *testing.T) {
	dir := t.TempDir()
	sitePath := filepath.Join(dir, "main.yml")
	if err := os.WriteFile(sitePath, []byte("---\nvars: {}\ntasks: []\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd, err := newRootCommand()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	cmd.SetArgs([]string{"--playbook", sitePath, "--output", "json"})
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(&stderr)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	output := stderr.String()
	for _, unwanted := range []string{
		"[playbook] loading playbook inputs",
		"[playbook] expanding playbook tasks",
		"[playbook] running playbook tasks",
	} {
		if strings.Contains(output, unwanted) {
			t.Fatalf("stderr = %q, want no persistent phase line %q", output, unwanted)
		}
	}
}
