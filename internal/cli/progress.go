package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/briandowns/spinner"

	"github.com/TacoContent/ironstate/internal/ui"
)

type progressReporter struct {
	prefix string
	spin   *spinner.Spinner
	out    io.Writer
	// mu serializes Start/Stop/Pause against write() (Message/Step) -
	// needed once Pause is used to bracket engine.Info/Warn/Danger, which
	// can fire from a background goroutine (an 'async' task) concurrently
	// with the main goroutine's own progress.Step calls.
	mu sync.Mutex
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
	p.mu.Lock()
	defer p.mu.Unlock()
	p.write(message)
}

func (p *progressReporter) Step(stage string, index, total int, detail string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.write(progressMessage(stage, index, total, detail))
}

// Pause stops the spinner (erasing its current line - see Spinner.Stop),
// runs fn, then restarts it if it was actually active beforehand -
// keeping any direct stdout/stderr print (engine.Info/Warn/Danger, the
// gathered-facts table, ...) that happens while the spinner is running
// from interleaving with the spinner's own repeatedly-rewritten line.
// Without this, the spinner's background goroutine can redraw its line
// (or leave a frame's worth of un-terminated text sitting there) at the
// same moment fn writes its own line, producing garbled output like
// "◒ loading playbook hierarchy──────────────────────────────". Mirrors
// wait_for's own spinner pause (internal/handlers/util.go's
// pauseSpinnerForPrint), scoped to the whole run's spinner instead of one
// task's.
func (p *progressReporter) Pause(fn func()) {
	p.mu.Lock()
	defer p.mu.Unlock()
	wasActive := p.spin.Active()
	if wasActive {
		p.spin.Stop()
	}
	fn()
	if wasActive {
		p.spin.Start()
	}
}

// write must be called with p.mu held.
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
