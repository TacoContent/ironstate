package handlers

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/TacoContent/ironstate/internal/engine"
)

// blockInFileHandler ports Handlers/BlockInFile.psm1: inserts/updates/
// removes a marker-delimited block of text in a file. Only the text
// between an exact pair of marker lines is ever touched. A 'template'
// field (instead of a literal 'block' string) renders through the same
// jinja/gotemplate engines the 'template' module uses (see template.go).
type blockInFileHandler struct{}

const defaultBlockMarker = "# {mark} IRONSTATE MANAGED - {name}"

type blockMarkers struct {
	Begin string
	End   string
}

func getBlockMarkers(marker, markerBegin, markerEnd, name string) blockMarkers {
	begin := strings.ReplaceAll(strings.ReplaceAll(marker, "{mark}", markerBegin), "{name}", name)
	end := strings.ReplaceAll(strings.ReplaceAll(marker, "{mark}", markerEnd), "{name}", name)
	return blockMarkers{Begin: begin, End: end}
}

// resolveBlockIdentifier ports Resolve-BlockIdentifier: 'marker_name' wins
// if set, else the task's own display name, else dest's file name.
func resolveBlockIdentifier(item map[string]any, name string) string {
	if override := getString(item, "marker_name"); override != "" {
		return override
	}
	if name != "" {
		return name
	}
	if dest := getString(item, "dest"); dest != "" {
		return filepath.Base(resolvePath(dest))
	}
	return ""
}

func getBlockInFileContent(item map[string]any, ctx engine.Context) (string, error) {
	if tpl := getMap(item, "template"); tpl != nil {
		rendered, err := getTemplateRenderedContent(tpl, ctx)
		if err != nil {
			if errors.Is(err, errTemplateSourceMissing) {
				return "", nil
			}
			return "", err
		}
		return rendered, nil
	}
	return getString(item, "block"), nil
}

// getFileLines ports Get-FileLines: splits a file's content into lines,
// normalizing CRLF -> LF first; a missing/empty file yields no lines.
func getFileLines(path string) []string {
	raw, err := os.ReadFile(path) //nolint:gosec // path is authored YAML content, same trust boundary as the rest of this tool
	if err != nil || len(raw) == 0 {
		return nil
	}
	normalized := strings.ReplaceAll(string(raw), "\r\n", "\n")
	return strings.Split(normalized, "\n")
}

func getDesiredBlockLines(block string) []string {
	if block == "" {
		return nil
	}
	normalized := strings.TrimRight(strings.ReplaceAll(block, "\r\n", "\n"), "\n")
	return strings.Split(normalized, "\n")
}

type blockRange struct {
	BeginIndex int
	EndIndex   int
}

func findBlockRange(lines []string, beginMarker, endMarker string) *blockRange {
	beginIndex := -1
	for i, l := range lines {
		if strings.TrimRight(l, " \t") == beginMarker {
			beginIndex = i
			break
		}
	}
	if beginIndex < 0 {
		return nil
	}
	endIndex := -1
	for i := beginIndex + 1; i < len(lines); i++ {
		if strings.TrimRight(lines[i], " \t") == endMarker {
			endIndex = i
			break
		}
	}
	if endIndex < 0 {
		return nil
	}
	return &blockRange{BeginIndex: beginIndex, EndIndex: endIndex}
}

// getBlockInsertIndex ports Get-BlockInsertIndex: 'insertbefore' wins if
// both are given; 'BOF'/'EOF' are literal positions, anything else is a
// regex matched against existing lines (insertbefore -> first match,
// insertafter -> last match); no match falls back to EOF.
func getBlockInsertIndex(lines []string, insertAfter, insertBefore string) int {
	if insertBefore != "" {
		if insertBefore == "BOF" {
			return 0
		}
		if re, err := regexp.Compile(insertBefore); err == nil {
			for i, l := range lines {
				if re.MatchString(l) {
					return i
				}
			}
		}
		return len(lines)
	}
	if insertAfter != "" && insertAfter != "EOF" {
		if insertAfter == "BOF" {
			return 0
		}
		if re, err := regexp.Compile(insertAfter); err == nil {
			matchIndex := -1
			for i, l := range lines {
				if re.MatchString(l) {
					matchIndex = i
				}
			}
			if matchIndex >= 0 {
				return matchIndex + 1
			}
		}
		return len(lines)
	}
	return len(lines)
}

func writeBlockInFileLines(path string, lines []string) error {
	content := strings.Join(lines, "\n")
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return os.WriteFile(path, []byte(content), 0o644) //nolint:gosec // matches ironstate.ps1's own file permissions, no tighter mode intended
}

func backupBlockInFileDest(path string) error {
	backupPath := fmt.Sprintf("%s.%s.bak", path, time.Now().Format("20060102150405"))
	data, err := os.ReadFile(path) //nolint:gosec // path is authored YAML content, same trust boundary as the rest of this tool
	if err != nil {
		return err
	}
	return os.WriteFile(backupPath, data, 0o644) //nolint:gosec // matches the source file's own permissions
}

func blockInFileMarkers(item map[string]any, name string) blockMarkers {
	return getBlockMarkers(
		getStringOr(item, "marker", defaultBlockMarker),
		getStringOr(item, "marker_begin", "BEGIN"),
		getStringOr(item, "marker_end", "END"),
		resolveBlockIdentifier(item, name),
	)
}

func testBlockInFilePresent(item map[string]any, name string, ctx engine.Context) (bool, error) {
	if itemState(item) != "absent" && hasPathMetadataDirective(item) {
		return false, nil
	}
	dest := resolvePath(getString(item, "dest"))
	if !fileExists(dest) {
		return false, nil
	}
	markers := blockInFileMarkers(item, name)
	lines := getFileLines(dest)
	r := findBlockRange(lines, markers.Begin, markers.End)
	if r == nil {
		return false, nil
	}
	var existing []string
	if r.EndIndex > r.BeginIndex+1 {
		existing = lines[r.BeginIndex+1 : r.EndIndex]
	}
	content, err := getBlockInFileContent(item, ctx)
	if err != nil {
		return false, err
	}
	desired := getDesiredBlockLines(content)
	return strings.Join(existing, "\n") == strings.Join(desired, "\n"), nil
}

func setBlockInFile(item map[string]any, name string, ctx engine.Context) error {
	dest := resolvePath(getString(item, "dest"))
	create := getBool(item, "create", false)
	exists := fileExists(dest)
	if !exists && !create {
		engine.Warn("blockinfile dest does not exist and 'create' is false, skipping: %s", dest)
		return nil
	}

	markers := blockInFileMarkers(item, name)
	var lines []string
	if exists {
		lines = getFileLines(dest)
	}

	content, err := getBlockInFileContent(item, ctx)
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
	if err := writeBlockInFileLines(dest, lines); err != nil {
		return err
	}
	return applyPathMetadata(dest, item)
}

func removeBlockInFile(item map[string]any, name string) error {
	dest := resolvePath(getString(item, "dest"))
	if !fileExists(dest) {
		return nil
	}
	markers := blockInFileMarkers(item, name)
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

func (blockInFileHandler) Test(item map[string]any, name string, ctx engine.Context) (bool, error) {
	return testBlockInFilePresent(item, name, ctx)
}

func (blockInFileHandler) Describe(item map[string]any, action engine.Action, ctx engine.Context) (string, error) {
	dest := resolvePath(getString(item, "dest"))
	if action == engine.ActionUninstall {
		return "remove ironstate managed block from " + dest, nil
	}
	return "manage block in " + dest, nil
}

func (blockInFileHandler) Install(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	return engine.ExecResult{}, setBlockInFile(item, name, ctx)
}

func (blockInFileHandler) Uninstall(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	return engine.ExecResult{}, removeBlockInFile(item, name)
}
