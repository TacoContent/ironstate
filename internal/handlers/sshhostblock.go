package handlers

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/TacoContent/ironstate/internal/engine"
)

// sshHostBlockHandler ports Handlers/SshHostBlock.psm1: renders one or
// more ssh_config "Host" blocks from structured data and writes them into
// a marker-delimited block, reusing blockinfile's marker/insert/backup
// machinery.
type sshHostBlockHandler struct{}

var sshHostNameKeys = map[string]bool{"host_name": true, "hostname": true}

// convertSshDirectiveKeyToPascalCase ports
// Convert-SshDirectiveKeyToPascalCase: 'host_name'/'hostName'/'HostName'
// all become 'HostName'.
func convertSshDirectiveKeyToPascalCase(name string) string {
	var withUnderscores strings.Builder
	runes := []rune(name)
	for i, r := range runes {
		withUnderscores.WriteRune(r)
		if i+1 < len(runes) {
			cur := runes[i]
			next := runes[i+1]
			curIsLowerOrDigit := (cur >= 'a' && cur <= 'z') || (cur >= '0' && cur <= '9')
			nextIsUpper := next >= 'A' && next <= 'Z'
			if curIsLowerOrDigit && nextIsUpper {
				withUnderscores.WriteRune('_')
			}
		}
	}
	segments := regexp.MustCompile(`[_\s-]+`).Split(withUnderscores.String(), -1)
	var sb strings.Builder
	for _, seg := range segments {
		if seg == "" {
			continue
		}
		sb.WriteString(strings.ToUpper(seg[:1]) + strings.ToLower(seg[1:]))
	}
	return sb.String()
}

func sshDirectiveValueString(v any) string {
	if b, ok := v.(bool); ok {
		if b {
			return "yes"
		}
		return "no"
	}
	return fmt.Sprintf("%v", v)
}

func sshCommentFieldString(v any) string {
	if v == nil {
		return ""
	}
	if list, ok := v.([]any); ok {
		parts := make([]string, len(list))
		for i, item := range list {
			parts[i] = fmt.Sprintf("%v", item)
		}
		return strings.Join(parts, ", ")
	}
	return sshDirectiveValueString(v)
}

var sshCommentTemplateKeyPattern = regexp.MustCompile(`\{(\w+)\}`)

func sshCommentTemplateKeys(tmpl string) []string {
	if tmpl == "" {
		return nil
	}
	matches := sshCommentTemplateKeyPattern.FindAllStringSubmatch(tmpl, -1)
	keys := make([]string, len(matches))
	for i, m := range matches {
		keys[i] = m[1]
	}
	return keys
}

func expandSshCommentTemplate(tmpl string, entry map[string]any) string {
	result := tmpl
	for _, key := range sshCommentTemplateKeys(tmpl) {
		value := ""
		if v, ok := entry[key]; ok {
			value = sshCommentFieldString(v)
		}
		result = strings.ReplaceAll(result, "{"+key+"}", value)
	}
	return result
}

func mergeSshHostEntry(entry, defaults map[string]any) map[string]any {
	merged := map[string]any{}
	for k, v := range defaults {
		merged[k] = v
	}
	for k, v := range entry {
		merged[k] = v
	}
	return merged
}

func getSshHostBlockEntryLines(entry map[string]any, commentTemplate string) ([]string, error) {
	host, _ := entry["host"].(string)
	if host == "" {
		return nil, fmt.Errorf("'ssh_host_block' requires a 'host' key on every host entry")
	}

	reserved := map[string]bool{"host": true, "comment": true}
	for _, k := range sshCommentTemplateKeys(commentTemplate) {
		reserved[k] = true
	}
	hasOwnHostName := false
	for k := range entry {
		if sshHostNameKeys[strings.ToLower(k)] {
			hasOwnHostName = true
			break
		}
	}

	var lines []string
	var comment string
	if c, ok := entry["comment"].(string); ok && c != "" {
		comment = c
	} else if commentTemplate != "" {
		comment = expandSshCommentTemplate(commentTemplate, entry)
	}
	if comment != "" {
		lines = append(lines, "# "+comment)
	}

	lines = append(lines, "Host "+host)
	if !hasOwnHostName {
		lines = append(lines, "  HostName "+host)
	}

	keys := make([]string, 0, len(entry))
	for k := range entry {
		keys = append(keys, k)
	}
	// Sorted for deterministic output - Go's map has no key order to
	// preserve, unlike the original's [ordered] hashtable; only cosmetic
	// line order changes, not ssh_config semantics.
	sort.Strings(keys)
	for _, key := range keys {
		if reserved[key] {
			continue
		}
		val := entry[key]
		if val == nil {
			continue
		}
		if list, ok := val.([]any); ok {
			directive := convertSshDirectiveKeyToPascalCase(key)
			directive = strings.TrimSuffix(directive, "s")
			for _, item := range list {
				lines = append(lines, "  "+directive+" "+sshDirectiveValueString(item))
			}
			continue
		}
		lines = append(lines, "  "+convertSshDirectiveKeyToPascalCase(key)+" "+sshDirectiveValueString(val))
	}
	return lines, nil
}

func getSshHostBlockContent(item map[string]any) (string, error) {
	defaults := getMap(item, "defaults")
	commentTemplate := getString(item, "comment_template")
	hosts := asList(item["hosts"])

	var blocks []string
	for _, raw := range hosts {
		hostEntry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		entry := mergeSshHostEntry(hostEntry, defaults)
		lines, err := getSshHostBlockEntryLines(entry, commentTemplate)
		if err != nil {
			return "", err
		}
		blocks = append(blocks, strings.Join(lines, "\n"))
	}
	return strings.Join(blocks, "\n\n"), nil
}

func testSshHostBlockPresent(item map[string]any, name string) (bool, error) {
	dest := resolvePath(getString(item, "dest"))
	if !fileExists(dest) {
		return false, nil
	}
	markers := blockInFileMarkers(item, resolveBlockIdentifier(item, name))
	lines := getFileLines(dest)
	r := findBlockRange(lines, markers.Begin, markers.End)
	if r == nil {
		return false, nil
	}
	var existing []string
	if r.EndIndex > r.BeginIndex+1 {
		existing = lines[r.BeginIndex+1 : r.EndIndex]
	}
	content, err := getSshHostBlockContent(item)
	if err != nil {
		return false, err
	}
	desired := getDesiredBlockLines(content)
	return strings.Join(existing, "\n") == strings.Join(desired, "\n"), nil
}

func setSshHostBlock(item map[string]any, name string) error {
	dest := resolvePath(getString(item, "dest"))
	create := getBool(item, "create", false)
	exists := fileExists(dest)
	if !exists && !create {
		engine.Warn("ssh_host_block dest does not exist and 'create' is false, skipping: %s", dest)
		return nil
	}

	markers := blockInFileMarkers(item, resolveBlockIdentifier(item, name))
	var lines []string
	if exists {
		lines = getFileLines(dest)
	}

	content, err := getSshHostBlockContent(item)
	if err != nil {
		return err
	}
	newBlockLines := append([]string{markers.Begin}, append(getDesiredBlockLines(content), markers.End)...)

	if r := findBlockRange(lines, markers.Begin, markers.End); r != nil {
		lines = append(lines[:r.BeginIndex], append(newBlockLines, lines[r.EndIndex+1:]...)...)
	} else {
		insertIndex := getBlockInsertIndex(lines, getStringOr(item, "insertafter", "EOF"), getString(item, "insertbefore"))
		out := make([]string, 0, len(lines)+len(newBlockLines))
		out = append(out, lines[:insertIndex]...)
		out = append(out, newBlockLines...)
		out = append(out, lines[insertIndex:]...)
		lines = out
	}

	if exists && getBool(item, "backup", false) {
		if err := backupBlockInFileDest(dest); err != nil {
			return err
		}
	}
	if err := ensureParentDir(dest); err != nil {
		return err
	}
	return writeBlockInFileLines(dest, lines)
}

func removeSshHostBlock(item map[string]any, name string) error {
	dest := resolvePath(getString(item, "dest"))
	if !fileExists(dest) {
		return nil
	}
	markers := blockInFileMarkers(item, resolveBlockIdentifier(item, name))
	lines := getFileLines(dest)
	r := findBlockRange(lines, markers.Begin, markers.End)
	if r == nil {
		return nil
	}
	if getBool(item, "backup", false) {
		if err := backupBlockInFileDest(dest); err != nil {
			return err
		}
	}
	lines = append(lines[:r.BeginIndex], lines[r.EndIndex+1:]...)
	return writeBlockInFileLines(dest, lines)
}

func (sshHostBlockHandler) Test(item map[string]any, name string, ctx engine.Context) (bool, error) {
	return testSshHostBlockPresent(item, name)
}

func (sshHostBlockHandler) Describe(item map[string]any, action engine.Action, ctx engine.Context) (string, error) {
	dest := resolvePath(getString(item, "dest"))
	if action == engine.ActionUninstall {
		return "remove ironstate managed ssh host block from " + dest, nil
	}
	return "manage ssh host block in " + dest, nil
}

func (sshHostBlockHandler) Install(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	return engine.ExecResult{}, setSshHostBlock(item, name)
}

func (sshHostBlockHandler) Uninstall(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	return engine.ExecResult{}, removeSshHostBlock(item, name)
}
