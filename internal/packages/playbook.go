package packages

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/TacoContent/ironstate/internal/pathutil"
)

// defaultPlaybookNames is the set of default document names searched
// for, in order, when a '--playbook' value resolves to a directory (or
// is empty/omitted) rather than a specific file — docs/plans/notes.md.
var defaultPlaybookNames = []string{"site.yml", "site.yaml", "main.yml", "main.yaml"}

// ResolvePlaybookPath resolves the '--playbook' flag's value — a
// specific file, a directory, a bare name with no extension, or "" for
// the current directory — to an actual YAML document to load. It does
// not require the exact file path, matching docs/plans/notes.md:
//
//  1. input itself, if it already names an existing file.
//  2. "<input>.yml" / "<input>.yaml" — input is a bare name with a
//     sibling YAML file (e.g. "playbooks/camalot" -> "playbooks/camalot.yml"),
//     only tried when input doesn't already exist as a directory.
//  3. "<input>/site.yml", "<input>/site.yaml", "<input>/main.yml",
//     "<input>/main.yaml".
//
// Returns an error naming every path tried when none exist.
func ResolvePlaybookPath(input string) (string, error) {
	if input == "" {
		input = "."
	}
	input = pathutil.ResolveUserPath(input)

	var tried []string
	if fi, err := os.Stat(input); err == nil { //nolint:gosec // configured playbook location, same trust boundary as the rest of this tool
		if !fi.IsDir() {
			return input, nil
		}
	} else {
		for _, ext := range []string{".yml", ".yaml"} {
			candidate := input + ext
			tried = append(tried, candidate)
			if fi, err := os.Stat(candidate); err == nil && !fi.IsDir() { //nolint:gosec // same as above
				return candidate, nil
			}
		}
	}

	for _, name := range defaultPlaybookNames {
		candidate := filepath.Join(input, name)
		tried = append(tried, candidate)
		if fi, err := os.Stat(candidate); err == nil && !fi.IsDir() { //nolint:gosec // same as above
			return candidate, nil
		}
	}

	return "", fmt.Errorf("no playbook could be located for %q (tried: %s)", input, strings.Join(tried, ", "))
}
