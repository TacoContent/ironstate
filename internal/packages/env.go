package packages

import (
	"bufio"
	"os"
	"strings"

	"github.com/TacoContent/ironstate/internal/secrets"
)

// ImportEnvFile loads KEY=VALUE lines from a dotenv-style file into the
// current process's environment — ports Packages.psm1's Import-EnvFile
// (itself mirroring apply.sh's `source .env`/`.secrets`). A missing file
// is not an error (matches "if (-not (Test-Path $Path)) { return }").
func ImportEnvFile(path string) (map[string]string, error) {
	f, err := os.Open(path) //nolint:gosec // fixed, caller-configured dotenv path, same trust boundary as the rest of this tool
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, err
	}
	defer func() { _ = f.Close() }()

	loaded := map[string]string{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.Index(line, "=")
		if idx < 1 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		value := strings.TrimSpace(line[idx+1:])
		if len(value) >= 2 {
			if (strings.HasPrefix(value, `"`) && strings.HasSuffix(value, `"`)) ||
				(strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'")) {
				value = value[1 : len(value)-1]
			}
		}
		loaded[key] = value
		if err := os.Setenv(key, value); err != nil {
			return nil, err
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return loaded, nil
}

func RegisterEnvFileSecrets(path string) error {
	loaded, err := ImportEnvFile(path)
	if err != nil {
		return err
	}
	for _, value := range loaded {
		secrets.Register(value)
	}
	return nil
}
