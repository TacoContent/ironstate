// Package becomeexec runs plugin commands using ironstate's become behavior.
package becomeexec

import (
	"github.com/TacoContent/ironstate/internal/exec"
	"github.com/TacoContent/ironstate/sdk/handler"
)

// WrapForBecome prefixes a command with the platform's supported elevation
// command when the handler context requested become.
func WrapForBecome(become handler.Become, executable string, args []string) (string, []string, error) {
	return exec.WrapForBecome(exec.Become{
		Enabled: become.Enabled,
		User:    become.User,
	}, executable, args)
}
