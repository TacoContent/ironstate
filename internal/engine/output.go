package engine

import (
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"
)

// PrintTable renders results as an aligned table on w - ports
// ironstate.ps1's final 'Format-Table -Property Module, Package, State,
// Action, Failed -AutoSize'.
func PrintTable(w io.Writer, results []Result) error {
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "MODULE\tPACKAGE\tSTATE\tACTION\tFAILED"); err != nil {
		return err
	}
	for _, r := range results {
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%v\n", r.Module, r.Package, r.State, r.Action, r.Failed); err != nil {
			return err
		}
	}
	return tw.Flush()
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
			Module:  r.Module,
			Package: r.Package,
			State:   r.State,
			Action:  r.Action,
			Apply:   r.Apply,
			Failed:  r.Failed,
			Exec: jsonExecResult{
				RC:          r.Exec.RC,
				Stdout:      r.Exec.Stdout,
				StdoutLines: r.Exec.StdoutLines,
				Stderr:      r.Exec.Stderr,
				StderrLines: r.Exec.StderrLines,
			},
		}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
