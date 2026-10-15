package cli

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/TacoContent/ironstate/internal/engine"
	"github.com/TacoContent/ironstate/internal/remoteexec/protocol"
	"github.com/TacoContent/ironstate/internal/secrets"
	"github.com/TacoContent/ironstate/internal/ui"
)

// runOutput is where one apply run reports progress, logs, facts and
// results: the console (table/json, with a spinner) or an NDJSON event
// stream (ndjson, also what 'ironstate agent' speaks).
type runOutput interface {
	start()
	stop()
	message(msg string)
	step(stage, detail string, index, total int)
	// wrapLog returns the replacement for an engine.Info/Warn/Danger-style
	// logger; orig is the logger being replaced.
	wrapLog(level string, orig func(format string, args ...any)) func(format string, args ...any)
	// pause runs fn with any live progress display suspended.
	pause(fn func())
	facts(all map[string]any) error
	result(r engine.Result)
	finish(results []engine.Result, stopped bool, elapsed time.Duration) error
	// done reports the run's final outcome; err is runApply's return value.
	done(err error)
	// cancelled reports why the run should stop before its next leaf.
	cancelled() error
}

type protocolWriterKey struct{}

// withProtocolWriter makes an '--output ndjson' run under ctx emit through
// w instead of creating its own writer on the command's stdout.
func withProtocolWriter(ctx context.Context, w *protocol.Writer) context.Context {
	return context.WithValue(ctx, protocolWriterKey{}, w)
}

func newRunOutput(cmd *cobra.Command, format string) (runOutput, error) {
	switch format {
	case "", "table", "json":
		return &consoleOutput{progress: newProgressReporter(), w: cmd.OutOrStdout(), json: format == "json"}, nil
	case "ndjson":
		var w *protocol.Writer
		if ctx := cmd.Context(); ctx != nil {
			w, _ = ctx.Value(protocolWriterKey{}).(*protocol.Writer)
		}
		if w == nil {
			w = protocol.NewWriter(cmd.OutOrStdout())
		}
		return &ndjsonOutput{w: w}, nil
	default:
		return nil, fmt.Errorf("unknown --output %q (want table, json or ndjson)", format)
	}
}

type consoleOutput struct {
	progress *progressReporter
	w        io.Writer
	json     bool
}

func (c *consoleOutput) start()               { c.progress.Start() }
func (c *consoleOutput) stop()                { c.progress.Stop() }
func (c *consoleOutput) message(msg string)   { c.progress.Message(msg) }
func (c *consoleOutput) pause(fn func())      { c.progress.Pause(fn) }
func (c *consoleOutput) result(engine.Result) {}
func (c *consoleOutput) done(error)           {}
func (c *consoleOutput) cancelled() error     { return nil }

func (c *consoleOutput) step(stage, detail string, index, total int) {
	c.progress.Step(stage, index, total, detail)
}

// wrapLog keeps the spinner from interleaving with log lines (see
// progressReporter.Pause).
func (c *consoleOutput) wrapLog(_ string, orig func(string, ...any)) func(string, ...any) {
	return func(format string, args ...any) { c.progress.Pause(func() { orig(format, args...) }) }
}

func (c *consoleOutput) facts(all map[string]any) error {
	if c.json {
		return nil
	}
	var err error
	c.progress.Pause(func() { err = ui.PrintFacts(c.w, all) })
	return err
}

func (c *consoleOutput) finish(results []engine.Result, _ bool, elapsed time.Duration) error {
	if c.json {
		return engine.PrintJSON(c.w, results)
	}
	if err := engine.PrintTable(c.w, results); err != nil {
		return err
	}
	return engine.PrintSummary(c.w, engine.ComputeStats(results), elapsed)
}

type ndjsonOutput struct {
	w *protocol.Writer
}

func (n *ndjsonOutput) start()          {}
func (n *ndjsonOutput) stop()           {}
func (n *ndjsonOutput) message(string)  {}
func (n *ndjsonOutput) pause(fn func()) { fn() }

func (n *ndjsonOutput) step(stage, detail string, index, total int) {
	_ = n.w.LeafStart(stage, detail, index, total)
}

func (n *ndjsonOutput) wrapLog(level string, _ func(string, ...any)) func(string, ...any) {
	return func(format string, args ...any) {
		_ = n.w.Log(level, secrets.Redact(fmt.Sprintf(format, args...)))
	}
}

func (n *ndjsonOutput) facts(all map[string]any) error { return n.w.Facts(all) }
func (n *ndjsonOutput) result(r engine.Result)         { _ = n.w.LeafResult(r) }

func (n *ndjsonOutput) finish(results []engine.Result, stopped bool, elapsed time.Duration) error {
	return n.w.Summary(engine.ComputeStats(results), stopped, elapsed)
}

func (n *ndjsonOutput) done(err error) {
	_ = n.w.Fail("apply", err, ExitCodeFor(err))
}

func (n *ndjsonOutput) cancelled() error { return n.w.Cancelled() }
