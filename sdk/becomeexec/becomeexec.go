// Package becomeexec runs plugin commands using ironstate's become behavior.
package becomeexec

import (
	"errors"
	"os/exec"
	"strings"

	"github.com/TacoContent/ironstate/sdk/handler"
)

// WrapForBecome prefixes a command with the platform's supported elevation
// command when the handler context requested become.
func WrapForBecome(become handler.Become, executable string, args []string) (string, []string, error) {
	if !become.Enabled {
		return executable, args, nil
	}
	sudoPath, err := exec.LookPath("sudo")
	if err != nil {
		return "", nil, errors.New("become requested but 'sudo' was not found on PATH")
	}
	wrapped := make([]string, 0, len(args)+3)
	if become.User != "" && !strings.EqualFold(become.User, "root") {
		wrapped = append(wrapped, "-u", become.User)
	}
	wrapped = append(wrapped, executable)
	wrapped = append(wrapped, args...)
	return sudoPath, wrapped, nil
}
