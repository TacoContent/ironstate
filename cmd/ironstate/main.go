// Command ironstate is the single-binary replacement for ironstate.ps1.
package main

import (
	"fmt"
	"os"

	"github.com/TacoContent/ironstate/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(cli.ExitCodeFor(err))
	}
}
