package remote

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/TacoContent/ironstate/internal/ui"
	"golang.org/x/term"
)

// Confirm asks the operator whether to trust and run a non-isolated
// remote source. It is called only for a genuinely remote source (see
// Classify) that has NOT opted into isolation, i.e. one that will run
// with the full playbook context and may request elevation - and it is
// called BEFORE the source is fetched, so declining never downloads
// anything.
//
// Overridable for tests and for a CLI that pre-approves remote sources.
var Confirm = func(kind Kind, source string) (bool, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		// Nothing can answer the prompt, and silently trusting remote
		// code in a non-interactive run (CI, a scheduled task) is the
		// exact case this guard exists for.
		return false, nil
	}
	fmt.Fprintln(os.Stderr, ui.Yellow("⚠ This playbook uses a remote, non-isolated source:"))
	fmt.Fprintf(os.Stderr, "    %s [%s]\n", source, kind)
	fmt.Fprintln(os.Stderr, ui.Yellow("  It will run with full access to your facts/vars and may request elevation (become)."))
	fmt.Fprint(os.Stderr, "  Fetch and run it? [y/N]: ")

	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return false, nil
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes", nil
}

// NeedsConfirmation reports whether a source must be approved by the
// operator before it is fetched and its tasks are loaded: a remote
// (non-local) source that is not isolated.
func NeedsConfirmation(kind Kind, isolated bool) bool {
	return kind != KindLocal && !isolated
}
