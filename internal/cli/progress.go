package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/briandowns/spinner"

	"github.com/TacoContent/ironstate/internal/ui"
)

type progressReporter struct {
	prefix string
	spin   *spinner.Spinner
	out    io.Writer
}

func newProgressReporter(prefix string, out io.Writer) *progressReporter {
	spin := spinner.New([]string{"◐", "◓", "◑", "◒"}, 120*time.Millisecond,
		spinner.WithWriterFile(os.Stderr),
		spinner.WithHiddenCursor(true),
	)
	if !ui.Enabled {
		spin.Disable()
	}
	return &progressReporter{prefix: prefix, spin: spin, out: out}
}

func (p *progressReporter) Start() {
	p.spin.Start()
}

func (p *progressReporter) Stop() {
	p.spin.Stop()
}

func (p *progressReporter) Message(message string) {
	p.write(message)
}

func (p *progressReporter) Step(stage string, index, total int, detail string) {
	p.write(progressMessage(stage, index, total, detail))
}

func (p *progressReporter) write(message string) {
	trimmed := strings.TrimSpace(message)
	if trimmed == "" {
		return
	}
	if p.spin.Active() {
		p.spin.Suffix = " " + trimmed
		return
	}
	if p.out == nil {
		return
	}
	_, _ = fmt.Fprintf(p.out, "[%s] %s\n", p.prefix, trimmed)
}

func progressMessage(stage string, index, total int, detail string) string {
	message := strings.TrimSpace(stage)
	if index > 0 && total > 0 {
		message = fmt.Sprintf("%s (%d/%d)", message, index, total)
	}
	if detail != "" {
		message = fmt.Sprintf("%s: %s", message, strings.TrimSpace(detail))
	}
	return message
}
