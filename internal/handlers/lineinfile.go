package handlers

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/TacoContent/ironstate/internal/engine"
	"github.com/TacoContent/ironstate/internal/template"
	"github.com/TacoContent/ironstate/internal/templateengines"
)

// lineInFileHandler manages a single line in a text file, modeled on
// Ansible's lineinfile module.
type lineInFileHandler struct{}

func resolveLineInFilePath(item map[string]any) string {
	for _, key := range []string{"path", "dest", "destfile", "name"} {
		if raw := getString(item, key); raw != "" {
			return resolvePath(raw)
		}
	}
	return ""
}

func normalizeAnsibleBackrefs(repl string) string {
	if repl == "" {
		return repl
	}

	var out strings.Builder
	for i := 0; i < len(repl); {
		ch := repl[i]
		if ch != '\\' {
			if ch == '$' {
				j := i + 1
				for j < len(repl) && repl[j] >= '0' && repl[j] <= '9' {
					j++
				}
				if j > i+1 {
					out.WriteString("${")
					out.WriteString(repl[i+1 : j])
					out.WriteByte('}')
					i = j
					continue
				}
				out.WriteString("$$")
				i++
				continue
			}
			out.WriteByte(ch)
			i++
			continue
		}

		if i+2 < len(repl) && repl[i+1] == 'g' && repl[i+2] == '<' {
			end := strings.IndexByte(repl[i+3:], '>')
			if end >= 0 {
				name := repl[i+3 : i+3+end]
				if name != "" {
					out.WriteString("${")
					out.WriteString(name)
					out.WriteByte('}')
					i += 4 + end
					continue
				}
			}
		}

		j := i + 1
		for j < len(repl) && repl[j] >= '0' && repl[j] <= '9' {
			j++
		}
		if j > i+1 {
			out.WriteString("${")
			out.WriteString(repl[i+1 : j])
			out.WriteByte('}')
			i = j
			continue
		}

		if i+1 < len(repl) {
			next := repl[i+1]
			if next == '$' {
				out.WriteString("$$")
			} else {
				out.WriteByte(next)
			}
			i += 2
			continue
		}

		out.WriteByte('\\')
		i++
	}

	return out.String()
}

func lineInFileRenderContext(item map[string]any, ctx engine.Context) map[string]any {
	out := map[string]any{}
	for k, v := range ctx.Flat {
		out[k] = v
	}

	withMap := getMap(item, "with")
	if withMap == nil {
		return out
	}

	out["with"] = withMap
	out["input"] = withMap
	if _, hasInputs := out["inputs"]; !hasInputs {
		out["inputs"] = withMap
	}
	for k, v := range withMap {
		out[k] = v
	}
	return out
}

func renderLineInFileLine(item map[string]any, ctx engine.Context) (string, error) {
	raw := getString(item, "line")
	if raw == "" {
		return "", nil
	}

	renderCtx := lineInFileRenderContext(item, ctx)

	if strings.Contains(raw, "${{") {
		wrapper := map[string]any{"line": raw}
		if err := template.ResolveInPlace(wrapper, renderCtx, ctx.Filters, "lineinfile", false); err != nil {
			return "", err
		}
		resolved, ok := wrapper["line"].(string)
		if !ok {
			return "", fmt.Errorf("lineinfile rendered 'line' must be a string")
		}
		raw = resolved
	}

	if strings.Contains(raw, "{{") && strings.Contains(raw, "}}") {
		preferGoTemplate := strings.Contains(raw, "{{ .") || strings.Contains(raw, "{{- .")
		type engineAttempt struct {
			name string
			run  func() (string, error)
		}
		goAttempt := engineAttempt{name: "gotemplate", run: func() (string, error) {
			return templateengines.RenderGoTemplate(raw, renderCtx)
		}}
		jinjaAttempt := engineAttempt{name: "jinja", run: func() (string, error) {
			return templateengines.RenderJinja(raw, renderCtx, ctx.Filters)
		}}

		attempts := []engineAttempt{jinjaAttempt, goAttempt}
		if preferGoTemplate {
			attempts = []engineAttempt{goAttempt, jinjaAttempt}
		}

		firstErr := error(nil)
		for _, attempt := range attempts {
			rendered, err := attempt.run()
			if err == nil {
				return rendered, nil
			}
			if firstErr == nil {
				firstErr = fmt.Errorf("%s: %w", attempt.name, err)
			}
		}
		return "", firstErr
	}

	return raw, nil
}

func matchingLineIndexes(lines []string, expr *regexp.Regexp, search, exact string) []int {
	matches := make([]int, 0)
	for i, line := range lines {
		switch {
		case expr != nil && expr.MatchString(line):
			matches = append(matches, i)
		case search != "" && strings.Contains(line, search):
			matches = append(matches, i)
		case expr == nil && search == "" && line == exact:
			matches = append(matches, i)
		}
	}
	return matches
}

func lineInsertIndex(lines []string, insertAfter, insertBefore string, firstMatch bool) (int, error) {
	if insertBefore != "" {
		if insertBefore == "BOF" {
			return 0, nil
		}
		re, err := regexp.Compile(insertBefore)
		if err != nil {
			return 0, err
		}
		matches := matchingLineIndexes(lines, re, "", "")
		if len(matches) == 0 {
			return len(lines), nil
		}
		if firstMatch {
			return matches[0], nil
		}
		return matches[len(matches)-1], nil
	}

	if insertAfter == "" || insertAfter == "EOF" {
		return len(lines), nil
	}
	if insertAfter == "BOF" {
		return 0, nil
	}

	re, err := regexp.Compile(insertAfter)
	if err != nil {
		return 0, err
	}
	matches := matchingLineIndexes(lines, re, "", "")
	if len(matches) == 0 {
		return len(lines), nil
	}
	if firstMatch {
		return matches[0] + 1, nil
	}
	return matches[len(matches)-1] + 1, nil
}

func presentLineInFile(lines []string, item map[string]any, ctx engine.Context) ([]string, bool, error) {
	line, err := renderLineInFileLine(item, ctx)
	if err != nil {
		return nil, false, err
	}
	if line == "" {
		return nil, false, fmt.Errorf("lineinfile requires 'line' for state present/latest")
	}

	insertAfter := getStringOr(item, "insertafter", "EOF")
	insertBefore := getString(item, "insertbefore")
	if insertAfter != "EOF" && insertBefore != "" {
		return nil, false, fmt.Errorf("lineinfile does not allow both 'insertafter' and 'insertbefore'")
	}

	search := getString(item, "search_string")
	regexpPattern := getString(item, "regexp")
	if search != "" && regexpPattern != "" {
		return nil, false, fmt.Errorf("lineinfile does not allow both 'search_string' and 'regexp'")
	}

	backrefs := getBool(item, "backrefs", false)
	if backrefs && regexpPattern == "" {
		return nil, false, fmt.Errorf("lineinfile with backrefs requires 'regexp'")
	}

	firstMatch := getBool(item, "firstmatch", false)

	var expr *regexp.Regexp
	if regexpPattern != "" {
		re, err := regexp.Compile(regexpPattern)
		if err != nil {
			return nil, false, err
		}
		expr = re
	}

	matches := matchingLineIndexes(lines, expr, search, line)
	if len(matches) > 0 {
		index := matches[len(matches)-1]
		replacement := line
		if backrefs {
			replacement = expr.ReplaceAllString(lines[index], normalizeAnsibleBackrefs(line))
		}
		if lines[index] == replacement {
			return lines, false, nil
		}
		out := append([]string{}, lines...)
		out[index] = replacement
		return out, true, nil
	}

	if backrefs {
		return lines, false, nil
	}

	insertIndex, err := lineInsertIndex(lines, insertAfter, insertBefore, firstMatch)
	if err != nil {
		return nil, false, err
	}
	out := make([]string, 0, len(lines)+1)
	out = append(out, lines[:insertIndex]...)
	out = append(out, line)
	out = append(out, lines[insertIndex:]...)
	return out, true, nil
}

func absentLineInFile(lines []string, item map[string]any, ctx engine.Context) ([]string, bool, error) {
	search := getString(item, "search_string")
	regexpPattern := getString(item, "regexp")
	line, err := renderLineInFileLine(item, ctx)
	if err != nil {
		return nil, false, err
	}

	if search != "" && regexpPattern != "" {
		return nil, false, fmt.Errorf("lineinfile does not allow both 'search_string' and 'regexp'")
	}
	if search == "" && regexpPattern == "" && line == "" {
		return nil, false, fmt.Errorf("lineinfile state absent requires one of 'regexp', 'search_string', or 'line'")
	}

	var expr *regexp.Regexp
	if regexpPattern != "" {
		re, err := regexp.Compile(regexpPattern)
		if err != nil {
			return nil, false, err
		}
		expr = re
	}

	out := make([]string, 0, len(lines))
	changed := false
	for _, cur := range lines {
		matched := false
		switch {
		case expr != nil:
			matched = expr.MatchString(cur)
		case search != "":
			matched = strings.Contains(cur, search)
		default:
			matched = cur == line
		}
		if matched {
			changed = true
			continue
		}
		out = append(out, cur)
	}
	return out, changed, nil
}

func testLineInFilePresent(item map[string]any, ctx engine.Context) (bool, error) {
	path := resolveLineInFilePath(item)
	if path == "" {
		return false, fmt.Errorf("lineinfile requires 'path' (or alias 'dest'/'destfile'/'name')")
	}
	if !fileExists(path) {
		return false, nil
	}

	lines := getFileLines(path)
	if itemState(item) == "absent" {
		newLines, changed, err := absentLineInFile(lines, item, ctx)
		if err != nil {
			return false, err
		}
		_ = newLines
		return changed, nil
	}

	newLines, changed, err := presentLineInFile(lines, item, ctx)
	if err != nil {
		return false, err
	}
	_ = newLines
	return !changed, nil
}

func setLineInFile(item map[string]any, ctx engine.Context) error {
	path := resolveLineInFilePath(item)
	if path == "" {
		return fmt.Errorf("lineinfile requires 'path' (or alias 'dest'/'destfile'/'name')")
	}

	state := itemState(item)
	exists := fileExists(path)
	if !exists {
		if state == "absent" {
			return nil
		}
		if !getBool(item, "create", false) {
			return fmt.Errorf("lineinfile target does not exist and 'create' is false: %s", path)
		}
	}

	var lines []string
	if exists {
		lines = getFileLines(path)
	}

	var (
		newLines []string
		changed  bool
		err      error
	)
	if state == "absent" {
		newLines, changed, err = absentLineInFile(lines, item, ctx)
	} else {
		newLines, changed, err = presentLineInFile(lines, item, ctx)
	}
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}

	if exists && getBool(item, "backup", false) {
		if err := backupBlockInFileDest(path); err != nil {
			return err
		}
	}
	if err := ensureParentDir(path); err != nil {
		return err
	}
	return writeBlockInFileLines(path, newLines)
}

func (lineInFileHandler) Test(item map[string]any, name string, ctx engine.Context) (bool, error) {
	return testLineInFilePresent(item, ctx)
}

func (lineInFileHandler) Describe(item map[string]any, action engine.Action, ctx engine.Context) (string, error) {
	path := resolveLineInFilePath(item)
	if action == engine.ActionUninstall {
		return "remove line(s) from " + path, nil
	}
	return "manage line in " + path, nil
}

func (lineInFileHandler) Install(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	return engine.ExecResult{}, setLineInFile(item, ctx)
}

func (lineInFileHandler) Uninstall(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	itemCopy := map[string]any{}
	for k, v := range item {
		itemCopy[k] = v
	}
	itemCopy["state"] = "absent"
	return engine.ExecResult{}, setLineInFile(itemCopy, ctx)
}
