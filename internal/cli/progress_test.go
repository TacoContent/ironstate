package cli

import (
	"strings"
	"sync"
	"testing"
)

// Note: briandowns/spinner only ever actually animates against a real
// terminal file descriptor (term.IsTerminal) - under `go test` (no tty)
// the spinner's own Start() is always a no-op and Active() always false.
// Message/Step only ever touch the spinner's Suffix (no persistent line is
// printed for either - see their doc comments), so these tests check
// Suffix directly rather than any captured output, and otherwise exercise
// Pause's locking/call-through behavior, not the spinner's visual erase/
// redraw (which briandowns/spinner itself owns and which isn't
// meaningfully testable without a pty).

func TestProgressReporterMessageUpdatesSuffix(t *testing.T) {
	p := newProgressReporter()
	p.Start()
	defer p.Stop()

	p.Message("loading playbook hierarchy")

	if got := p.spin.Suffix; got != " loading playbook hierarchy" {
		t.Fatalf("Message() left Suffix = %q", got)
	}
}

func TestProgressReporterMessageIgnoresBlank(t *testing.T) {
	p := newProgressReporter()
	p.Start()
	defer p.Stop()

	p.Message("   ")

	if got := p.spin.Suffix; got != "" {
		t.Fatalf("Message(blank) left Suffix = %q, want unchanged/empty", got)
	}
}

func TestProgressReporterPauseRunsFn(t *testing.T) {
	var buf strings.Builder
	p := newProgressReporter()
	p.Start()
	defer p.Stop()

	ran := false
	p.Pause(func() {
		ran = true
		buf.WriteString("direct write\n")
	})

	if !ran {
		t.Fatal("Pause did not run fn")
	}
	if !strings.Contains(buf.String(), "direct write\n") {
		t.Fatalf("Pause's fn output missing: %q", buf.String())
	}

	// Message must still work normally afterward - Pause shouldn't leave
	// the spinner (or the mutex) in a stuck state.
	p.Message("still going")
	if got := p.spin.Suffix; got != " still going" {
		t.Fatalf("Message after Pause left Suffix = %q", got)
	}
}

// TestProgressReporterConcurrentStepAndPause exercises Step (as the main
// dispatch goroutine calls per leaf) and Pause (as a wrapped engine.Info/
// Warn/Danger call from an 'async' task's background goroutine would) at
// the same time - run with -race, this catches any missing lock around
// the shared spinner state that a single-goroutine test wouldn't.
func TestProgressReporterConcurrentStepAndPause(t *testing.T) {
	var buf strings.Builder
	p := newProgressReporter()
	p.Start()
	defer p.Stop()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			p.Step("running tasks", i+1, 100, "leaf")
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			p.Pause(func() { buf.WriteString("x") })
		}
	}()
	wg.Wait()
}
