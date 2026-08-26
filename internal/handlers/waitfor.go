package handlers

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/briandowns/spinner"

	"github.com/TacoContent/ironstate/internal/conditions"
	"github.com/TacoContent/ironstate/internal/engine"
	"github.com/TacoContent/ironstate/internal/ui"
)

// waitForSpinnerFrames is the ◐◓◑◒ rotation requested for 'wait_for' -
// distinct from any other spinner style this codebase might add later.
var waitForSpinnerFrames = []string{"◐", "◓", "◑", "◒"}

// newWaitForSpinner builds a stderr spinner describing what this
// 'wait_for' is blocked on. Writes to stderr (matching engine.Info/Warn/
// Danger's own stream) via WithWriterFile, which also makes the library
// self-disable when stderr isn't a real terminal (piped/redirected
// output) - no separate check needed here beyond 'ui.Enabled' (NO_COLOR/
// dumb-terminal/non-tty-stdout), so a run with color disabled never
// animates either.
func newWaitForSpinner(label string) *spinner.Spinner {
	s := spinner.New(waitForSpinnerFrames, 120*time.Millisecond,
		spinner.WithWriterFile(os.Stderr),
		spinner.WithSuffix(" "+label),
		spinner.WithHiddenCursor(true),
	)
	if !ui.Enabled {
		s.Disable()
	}
	return s
}

// pauseSpinnerForPrint wraps an engine.Info/Warn/Danger-shaped function so
// any call to it (including from an 'async' job's own background
// goroutine, still running while this 'wait_for' spins) stops the
// spinner, erasing its line, before printing - then restarts it
// afterward, so the two never interleave on the same terminal line.
func pauseSpinnerForPrint(spin *spinner.Spinner, mu *sync.Mutex, fn func(string, ...any)) func(string, ...any) {
	return func(format string, args ...any) {
		mu.Lock()
		defer mu.Unlock()
		wasActive := spin.Active()
		if wasActive {
			spin.Stop()
		}
		fn(format, args...)
		if wasActive {
			spin.Start()
		}
	}
}

// waitForHandler blocks the run until either every async job named in
// 'for' has completed, or 'condition' (same bare-expression grammar as
// 'when'/'that') becomes true, whichever combination is configured (both,
// if both are given - implicit AND) - up to 'timeout' seconds, polling
// every 'interval' seconds. Fails (rc=1) if the timeout elapses first, or
// if any awaited async job itself failed.
type waitForHandler struct{}

func waitForIDs(item map[string]any) []string {
	return stringSlice(item["for"])
}

func waitForConditions(item map[string]any) []any {
	return asList(item["condition"])
}

func (waitForHandler) Test(item map[string]any, name string, ctx engine.Context) (bool, error) {
	return false, nil // always "not installed" -> Install always dispatches
}

func (waitForHandler) Describe(item map[string]any, action engine.Action, ctx engine.Context) (string, error) {
	var parts []string
	if ids := waitForIDs(item); len(ids) > 0 {
		parts = append(parts, "async job(s) "+strings.Join(ids, ", "))
	}
	if conds := waitForConditions(item); len(conds) > 0 {
		parts = append(parts, "condition")
	}
	timeout := getFloat(item, "timeout", 30)
	return fmt.Sprintf("wait_for: %s (timeout %gs)", strings.Join(parts, " and "), timeout), nil
}

func (waitForHandler) Install(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	ids := waitForIDs(item)
	conds := waitForConditions(item)
	if len(ids) == 0 && len(conds) == 0 {
		return engine.ExecResult{}, fmt.Errorf("wait_for: at least one of 'for' or 'condition' is required")
	}

	timeoutSeconds := getFloat(item, "timeout", 30)
	intervalSeconds := getFloat(item, "interval", 0.5)
	if intervalSeconds <= 0 {
		intervalSeconds = 0.5
	}
	deadline := time.Now().Add(time.Duration(timeoutSeconds * float64(time.Second)))
	interval := time.Duration(intervalSeconds * float64(time.Second))

	waitLabel := "wait_for"
	if len(ids) > 0 {
		waitLabel = "waiting on async job(s) " + strings.Join(ids, ", ")
	}
	if len(conds) > 0 {
		if len(ids) > 0 {
			waitLabel += " and condition"
		} else {
			waitLabel = "waiting on condition"
		}
	}
	spin := newWaitForSpinner(waitLabel)
	spin.Start()
	defer spin.Stop() // safety net: no-op if already stopped below

	// engine.Info/Warn/Danger are process-global - an 'async' job's own
	// background goroutine (still running while this loop spins) prints
	// through them too, on the same stderr stream as the spinner. Swap in
	// wrappers that pause/resume the spinner around every such call for
	// as long as this 'wait_for' is active, so the two never interleave
	// mid-line; restored on return.
	var spinMu sync.Mutex
	origInfo, origWarn, origDanger := engine.Info, engine.Warn, engine.Danger
	engine.Info = pauseSpinnerForPrint(spin, &spinMu, origInfo)
	engine.Warn = pauseSpinnerForPrint(spin, &spinMu, origWarn)
	engine.Danger = pauseSpinnerForPrint(spin, &spinMu, origDanger)
	defer func() {
		engine.Info = origInfo
		engine.Warn = origWarn
		engine.Danger = origDanger
	}()

	for {
		jobResults := map[string]any{}
		allDone := true
		anyJobFailed := false
		for _, id := range ids {
			job, ok := engine.LookupAsyncJob(id)
			if !ok {
				allDone = false
				continue
			}
			done, failed, results, jerr := job.Snapshot()
			if !done {
				allDone = false
				continue
			}
			jobResults[id] = summarizeAsyncResults(results)
			if failed || jerr != nil {
				anyJobFailed = true
			}
		}

		conditionOK := true
		if len(conds) > 0 {
			var err error
			conditionOK, err = conditions.TestWhen(conds, ctx.Flat, ctx.Filters)
			if err != nil {
				spin.Stop() // clear the spinner line before the error message is printed by the caller
				return engine.ExecResult{}, err
			}
		}

		if allDone && conditionOK {
			spin.Stop() // clear the spinner line before printing below - Stop() must happen before Danger/Info, not after (defer runs too late)
			if anyJobFailed {
				message := fmt.Sprintf("wait_for: one or more awaited async job(s) failed: %s", strings.Join(ids, ", "))
				engine.Danger("%s", message)
				return engine.ExecResult{RC: 1, Stderr: message, StderrLines: []string{message}, Extra: map[string]any{"results": jobResults}}, nil
			}
			message := "wait_for: condition satisfied"
			return engine.ExecResult{RC: 0, Stdout: message, StdoutLines: []string{message}, Extra: map[string]any{"results": jobResults}}, nil
		}

		if time.Now().After(deadline) {
			spin.Stop() // clear the spinner line before the timeout message below
			message := fmt.Sprintf("wait_for: timed out after %gs waiting for %s", timeoutSeconds, strings.Join(append(append([]string{}, ids...), condLabel(conds)...), ", "))
			engine.Danger("%s", message)
			return engine.ExecResult{RC: 1, Stderr: message, StderrLines: []string{message}}, nil
		}

		remaining := time.Until(deadline).Round(time.Second)
		spin.Lock()
		spin.Suffix = fmt.Sprintf(" %s (%s left)", waitLabel, remaining)
		spin.Unlock()

		time.Sleep(interval)
	}
}

func condLabel(conds []any) []string {
	if len(conds) == 0 {
		return nil
	}
	return []string{"condition"}
}

func summarizeAsyncResults(results []engine.Result) []any {
	out := make([]any, 0, len(results))
	for _, r := range results {
		out = append(out, map[string]any{
			"module":       r.Module,
			"package":      r.Package,
			"changed":      r.Action != engine.ActionSkip,
			"failed":       r.Failed,
			"rc":           float64(r.Exec.RC),
			"stdout":       r.Exec.Stdout,
			"stdout_lines": r.Exec.StdoutLines,
			"stderr":       r.Exec.Stderr,
			"stderr_lines": r.Exec.StderrLines,
		})
	}
	return out
}

func (h waitForHandler) Uninstall(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	return h.Install(item, name, ctx) // never reached: Test always reports "not installed"
}
