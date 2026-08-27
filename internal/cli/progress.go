package cli

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/briandowns/spinner"

	"github.com/TacoContent/ironstate/internal/ui"
)

type progressReporter struct {
	spin *spinner.Spinner
	// mu serializes Start/Stop/Pause against Message/Step - needed once
	// Pause is used to bracket engine.Info/Warn/Danger, which can fire
	// from a background goroutine (an 'async' task) concurrently with the
	// main goroutine's own progress.Step calls.
	mu sync.Mutex
}

func newProgressReporter() *progressReporter {
	spin := spinner.New([]string{"◐", "◓", "◑", "◒"}, 120*time.Millisecond,
		spinner.WithWriterFile(os.Stderr),
		spinner.WithHiddenCursor(true),
	)
	if !ui.Enabled {
		spin.Disable()
	}
	return &progressReporter{spin: spin}
}

func (p *progressReporter) Start() {
	p.startSpin()
}

func (p *progressReporter) Stop() {
	p.spin.Stop()
}

// startSpin calls spinner.Start() and then waits for its background redraw
// goroutine to actually confirm it's running before returning. That
// goroutine is launched asynchronously (Start() returns immediately after
// setting an 'active' flag and firing 'go func(){...}'), so a caller that
// immediately turns around and calls Stop() again - exactly what write()
// and Pause() do for every interleaved print - can race it: if the
// goroutine hasn't reached its first tick by the time Stop() clears
// 'active', it sees itself already stopped and exits having drawn nothing.
// Reproduced live: a run with six back-to-back Message() calls at startup
// left the spinner's last resume silently losing that race, so 'active'
// stayed true internally but no goroutine was left to act on it - the
// spinner then stayed invisible for the rest of the run (a 10+ minute
// package-processing phase) even though Step() kept updating Suffix the
// whole time, because nothing ever called Start() again to relaunch it.
// Confirming the first tick here closes that window.
func (p *progressReporter) startSpin() {
	tick := make(chan struct{}, 1)
	p.spin.Lock()
	p.spin.PostUpdate = func(*spinner.Spinner) {
		select {
		case tick <- struct{}{}:
		default:
		}
	}
	p.spin.Unlock()

	p.spin.Start()

	select {
	case <-tick:
	case <-time.After(100 * time.Millisecond):
		// Spinner disabled/not a terminal, or some other reason it'll
		// never tick - don't block the run waiting for it.
	}

	p.spin.Lock()
	p.spin.PostUpdate = nil
	p.spin.Unlock()
}

// Message updates the spinner's suffix to reflect the current phase (e.g.
// "gathering host facts"), the same way Step does for per-task progress -
// no persistent line is printed. This has no plain-text fallback for
// terminals where the spinner doesn't render (see Pause's doc comment for
// how actual output - engine.Info/Warn/Danger, the facts table - still
// gets a real printed line): if the spinner is disabled or not a
// terminal, phase progress goes unseen. The final Message before the run
// ends is also lost, since Stop() erases the spinner's line on exit with
// nothing to replace it - the run's actual results (installed/would-install
// output, warnings, errors) are what's expected to remain in scrollback,
// not the phase titles.
func (p *progressReporter) Message(message string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	trimmed := strings.TrimSpace(message)
	if trimmed == "" {
		return
	}
	p.spin.Suffix = " " + trimmed
}

// Step updates the spinner's suffix to reflect current progress in place -
// see Message's doc comment for the same no-persistent-line tradeoff.
func (p *progressReporter) Step(stage string, index, total int, detail string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	message := strings.TrimSpace(progressMessage(stage, index, total, detail))
	if message == "" {
		return
	}
	p.spin.Suffix = " " + message
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
		p.startSpin()
	}
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
