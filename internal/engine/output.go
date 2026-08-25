package engine

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/TacoContent/ironstate/internal/secrets"
	"github.com/TacoContent/ironstate/internal/ui"
)

// statusCell returns a Result's plain-text status label and the ui style
// function it should be rendered with - a failed leaf always reads as
// "danger" red regardless of action, a real applied change reads bright
// green, a dry-run preview of a change reads cyan, and an already-
// satisfied (skipped) leaf reads dim/gray - the "changed reads brighter
// than unchanged, failed reads as danger" scheme requested for the CLI's
// output.
func statusCell(r Result) (string, func(string) string) {
	verb := "install"
	if r.Action == ActionUninstall {
		verb = "remove"
	}
	switch {
	case r.Failed:
		return "✖ failed", ui.BoldRed
	case r.Action == ActionSkip:
		return "⏭️ skip", ui.Dim
	case r.Apply:
		return "✔ " + verb + "ed", ui.BoldGreen
	default:
		return "› would " + verb, ui.BrightCyan
	}
}

// PrintTable renders results as an aligned, emoji/colored table on w -
// ports ironstate.ps1's final 'Format-Table -Property Module, Package,
// State, Action, Failed -AutoSize', restyled as a modern CLI summary
// table. Column widths are computed from the plain (uncolored) text so
// ANSI escape codes never throw off alignment.
func PrintTable(w io.Writer, results []Result) error {
	widths := [3]int{len("MODULE"), len("PACKAGE"), len("STATE")}
	statusWidth := len("STATUS")
	type row struct {
		emoji, module, pkg, state, status string
		colorFn                           func(string) string
	}
	rows := make([]row, len(results))
	for i, r := range results {
		status, colorFn := statusCell(r)
		rows[i] = row{ui.ModuleEmoji(r.Module), secrets.Redact(r.Module), secrets.Redact(r.Package), secrets.Redact(r.State), status, colorFn}
		widths[0] = max(widths[0], len(rows[i].module))
		widths[1] = max(widths[1], len(rows[i].pkg))
		widths[2] = max(widths[2], len(rows[i].state))
		statusWidth = max(statusWidth, len(status))
	}

	if _, err := fmt.Fprintf(w, "   %-*s  %-*s  %-*s  %-*s\n", widths[0], "MODULE", widths[1], "PACKAGE", widths[2], "STATE", statusWidth, "STATUS"); err != nil {
		return err
	}
	for _, rr := range rows {
		paddedStatus := fmt.Sprintf("%-*s", statusWidth, rr.status)
		if _, err := fmt.Fprintf(w, "%s  %-*s  %-*s  %-*s  %s\n", rr.emoji, widths[0], rr.module, widths[1], rr.pkg, widths[2], rr.state, rr.colorFn(paddedStatus)); err != nil {
			return err
		}
	}
	return nil
}

// Stats summarizes a run's results - counts consumed by PrintSummary.
type Stats struct {
	Total       int
	Installed   int
	Uninstalled int
	Skipped     int
	Failed      int
}

// ComputeStats tallies results into a Stats summary.
func ComputeStats(results []Result) Stats {
	var s Stats
	s.Total = len(results)
	for _, r := range results {
		switch {
		case r.Failed:
			s.Failed++
		case r.Action == ActionInstall:
			s.Installed++
		case r.Action == ActionUninstall:
			s.Uninstalled++
		default:
			s.Skipped++
		}
	}
	return s
}

// PrintSummary renders a final "modern CLI" stats block on w - the
// requested "final stats at the end" - with elapsed wall-clock time and
// color-coded counts (green for changes, dim for skips, red/bold for any
// failures).
func PrintSummary(w io.Writer, stats Stats, elapsed time.Duration) error {
	rule := ui.Dim("──────────────────────────────")
	failedColor := ui.Dim
	if stats.Failed > 0 {
		failedColor = ui.BoldRed
	}
	// Pad the plain label first, then color the whole padded string -
	// coloring before padding would count the ANSI escape bytes towards
	// the width and misalign the counts column (same pitfall PrintTable
	// avoids).
	statLine := func(colorFn func(string) string, symbol, label string, count int) string {
		return fmt.Sprintf("  %s  %s %d", colorFn(symbol), colorFn(fmt.Sprintf("%-11s", label)), count)
	}
	lines := []string{
		rule,
		ui.Bold("✨ Summary"),
		rule,
		statLine(ui.BoldGreen, "✔", "Installed", stats.Installed),
		statLine(ui.BoldYellow, "✔", "Uninstalled", stats.Uninstalled),
		statLine(ui.Dim, "⏭️", "Skipped", stats.Skipped),
		statLine(failedColor, "✖", "Failed", stats.Failed),
		rule,
		fmt.Sprintf("  Total: %d task(s) in %s", stats.Total, elapsed.Round(time.Millisecond)),
		rule,
	}
	for _, line := range lines {
		if _, err := fmt.Fprintln(w, line); err != nil {
			return err
		}
	}
	return nil
}

// jsonResult is Result's '--output json' shape: exported field names,
// snake_case exec sub-fields, matching internal/expr's/YAML's own
// convention elsewhere in this codebase.
type jsonResult struct {
	Module  string         `json:"module"`
	Package string         `json:"package"`
	State   string         `json:"state"`
	Action  Action         `json:"action"`
	Apply   bool           `json:"apply"`
	Failed  bool           `json:"failed"`
	Exec    jsonExecResult `json:"exec"`
}

type jsonExecResult struct {
	RC          int      `json:"rc"`
	Stdout      string   `json:"stdout"`
	StdoutLines []string `json:"stdout_lines"`
	Stderr      string   `json:"stderr"`
	StderrLines []string `json:"stderr_lines"`
}

// PrintJSON renders results as a JSON array on w - the '--output json'
// format (additive, not a compatibility requirement; see
// docs/plans/go-rewrite.md §1).
func PrintJSON(w io.Writer, results []Result) error {
	out := make([]jsonResult, len(results))
	for i, r := range results {
		out[i] = jsonResult{
			Module:  secrets.Redact(r.Module),
			Package: secrets.Redact(r.Package),
			State:   secrets.Redact(r.State),
			Action:  r.Action,
			Apply:   r.Apply,
			Failed:  r.Failed,
			Exec: jsonExecResult{
				RC:          r.Exec.RC,
				Stdout:      secrets.Redact(r.Exec.Stdout),
				StdoutLines: redactStrings(r.Exec.StdoutLines),
				Stderr:      secrets.Redact(r.Exec.Stderr),
				StderrLines: redactStrings(r.Exec.StderrLines),
			},
		}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func redactStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, len(values))
	for i, v := range values {
		out[i] = secrets.Redact(v)
	}
	return out
}
