package cli

import (
	"bytes"
	"os"
	"path/filepath"
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

	sitePath := filepath.Join(dir, "site.yml")
	if err := os.WriteFile(sitePath, []byte("---\nvars: {}\ntasks: []\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd, err := newRootCommand()
	if err != nil {
		t.Fatal(err)
	}
	cmd.SetArgs([]string{"--file", sitePath, "--output", "json"})
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
