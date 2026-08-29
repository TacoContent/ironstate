package ui

// Package ui provides small terminal styling helpers (ANSI colors, module
// emoji) shared by internal/engine's per-leaf output and internal/cli's
// facts/summary panels — additive CLI polish, not a compatibility
// requirement (docs/plans/go-rewrite.md §1). Honors NO_COLOR
// (https://no-color.org) and IRONSTATE_NO_COLOR, and auto-disables when
// stdout isn't a real terminal (e.g. piped/redirected output, matching
// how `--output json` should never carry escape codes).

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"golang.org/x/term"
	"gopkg.in/yaml.v3"

	"github.com/TacoContent/ironstate/internal/secrets"
)

// Enabled controls whether style functions actually emit ANSI escape
// codes. Auto-detected at startup; callers (e.g. a future '--no-color'
// flag) may override it directly.
var Enabled = detectEnabled()

func detectEnabled() bool {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("IRONSTATE_NO_COLOR") != "" {
		return false
	}
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	return term.IsTerminal(int(os.Stdout.Fd()))
}

const (
	codeReset  = "\x1b[0m"
	codeBold   = "\x1b[1m"
	codeDim    = "\x1b[2m"
	codeItalic = "\x1b[3m"

	codeRed     = "\x1b[31m"
	codeGreen   = "\x1b[32m"
	codeYellow  = "\x1b[33m"
	codeBlue    = "\x1b[34m"
	codeMagenta = "\x1b[35m"
	codeCyan    = "\x1b[36m"
	codeGray    = "\x1b[90m"

	codeBrightRed   = "\x1b[91m"
	codeBrightGreen = "\x1b[92m"
	codeBrightCyan  = "\x1b[96m"
)

func style(code, s string) string {
	if !Enabled || s == "" {
		return s
	}
	return code + s + codeReset
}

func Bold(s string) string   { return style(codeBold, s) }
func Dim(s string) string    { return style(codeDim, s) }
func Italic(s string) string { return style(codeItalic, s) }

func Red(s string) string     { return style(codeRed, s) }
func Green(s string) string   { return style(codeGreen, s) }
func Yellow(s string) string  { return style(codeYellow, s) }
func Blue(s string) string    { return style(codeBlue, s) }
func Magenta(s string) string { return style(codeMagenta, s) }
func Cyan(s string) string    { return style(codeCyan, s) }
func Gray(s string) string    { return style(codeGray, s) }

// BoldRed/BoldGreen/BoldCyan are the "danger"/"changed"/"preview" tones
// used for a failed task, an applied change, and a dry-run preview
// respectively - deliberately brighter/bolder than Gray's muted
// already-satisfied look (docs/plans/go-rewrite.md-adjacent CLI polish
// request: "changed" state should read brighter than "unchanged").
func BoldRed(s string) string    { return style(codeBold+codeBrightRed, s) }
func BoldGreen(s string) string  { return style(codeBold+codeBrightGreen, s) }
func BoldYellow(s string) string { return style(codeBold+codeYellow, s) }
func BrightCyan(s string) string { return style(codeBrightCyan, s) }

// moduleEmoji maps a leaf's module name to a single representative glyph
// for quick visual scanning - purely decorative, never load-bearing.
var moduleEmoji = map[string]string{
	"winget":         "📦",
	"chocolatey":     "🍫",
	"pipx":           "🐍",
	"npm":            "📦",
	"cargo":          "🦀",
	"go":             "🐹",
	"gem":            "💎",
	"eget":           "📦",
	"brew":           "🍺",
	"homebrew":       "🍺",
	"apt":            "📦",
	"pacman":         "📦",
	"dnf":            "📦",
	"yum":            "📦",
	"apk":            "📦",
	"git":            "🌿",
	"cron":           "⏰",
	"cron_file":      "⏰",
	"iptables":       "🧱",
	"ufw":            "🧱",
	"advfirewall":    "🧱",
	"firewall":       "🧱",
	"zip":            "🗜️",
	"symlinks":       "🔗",
	"file":           "📄",
	"copy":           "📋",
	"template":       "📝",
	"shell":          "💻",
	"blockinfile":    "🧩",
	"lineinfile":     "📏",
	"ssh_host_block": "🔐",
	"log":            "📢",
	"fail":           "❌",
	"path":           "📁",
	"fact":           "🔎",
	"mount_facts":    "🔎",
	"registry":       "🗃️",
	"scheduled_task": "⏰",
	"assert":         "✅",
	"async":          "⚡",
	"wait_for":       "⏳",
	"user":           "👤",
	"group":          "👥",
}

// ModuleEmoji returns module's representative glyph, or a generic bullet
// for an unrecognized module name.
func ModuleEmoji(module string) string {
	if e, ok := moduleEmoji[module]; ok {
		return e
	}
	return "🏷️"
}

// Newline is the line terminator every human-facing multi-line panel
// (PrintFacts here, engine.PrintTable/PrintSummary) should end each
// printed line with, instead of plain "\n": explicit "\r\n" whenever
// attached to a real terminal (Enabled). Observed live: on at least one
// Windows terminal, a bare '\n' only moves the cursor down a row without
// returning it to column 0 (real VT100 "index" semantics - a full
// "newline" is CR+LF together) - a single engine.Info/Warn/Danger line
// never shows this because the progress spinner's own '\r'-prefixed
// erase/redraw happens to run immediately before and after it, but a
// block of several Fprintln calls with no spinner activity in between
// (the facts panel, the final results table + summary) drifted one
// line's width further right with every line, producing a garbled
// "staircase". Piped/redirected output (not Enabled) stays plain "\n" -
// a stray \r there would just be noise for a script/file consumer, not a
// rendering fix.
func Newline() string {
	if Enabled {
		return "\r\n"
	}
	return "\n"
}

// WriteLine writes s terminated by Newline() - see Newline's doc comment
// for why this isn't plain fmt.Fprintln.
func WriteLine(w io.Writer, s string) error {
	_, err := io.WriteString(w, s+Newline())
	return err
}

// PrintFacts renders a small "modern CLI" panel of every gathered fact on
// w, sorted by key for stable/diffable output — the "display facts info
// after gathered" request. Callers should pass the complete, final set of
// facts (gathered host facts plus every user-registered 'fact'/
// 'mount_facts' value - see engine.Options.OnFactsGathered), not a
// snapshot taken before those had run, since this only ever renders once.
// facts.Gather()'s own values are scalars (string/bool/float64), printed
// as one "key  value" line; a user-defined 'fact' can set 'value' to any
// YAML shape, and 'mount_facts' produces a list of objects - either
// renders as an indented YAML block under "key:" instead of Go's default
// '%v' map/slice syntax. Every line is passed through secrets.Redact
// before printing - the same mechanism a '$name'-prefixed 'fact' value is
// already registered under (see engine.go's applyFactResult) - so a
// secret fact never appears here unredacted, scalar or not.
func PrintFacts(w io.Writer, allFacts map[string]any) error {
	keys := make([]string, 0, len(allFacts))
	keyWidth := 0
	for k := range allFacts {
		keys = append(keys, k)
		if len(k) > keyWidth {
			keyWidth = len(k)
		}
	}
	sort.Strings(keys)

	rule := Dim("──────────────────────────────")
	if err := WriteLine(w, rule); err != nil {
		return err
	}
	if err := WriteLine(w, Bold("🧠 Facts")); err != nil {
		return err
	}
	if err := WriteLine(w, rule); err != nil {
		return err
	}
	for _, k := range keys {
		if err := printFactLine(w, k, allFacts[k], keyWidth); err != nil {
			return err
		}
	}
	if err := WriteLine(w, rule); err != nil {
		return err
	}
	return nil
}

// printFactLine renders one fact: a scalar as a single "key  value" line
// aligned to keyWidth, a map/slice as "key:" followed by its value
// YAML-block-indented underneath - the shape a 'fact'/'mount_facts' value
// can take that a bare '%v' would otherwise dump as Go's map/slice syntax
// (e.g. "[map[device:... fstype:NTFS] map[device:...]]").
func printFactLine(w io.Writer, key string, value any, keyWidth int) error {
	if isScalarFactValue(value) {
		line := fmt.Sprintf("  %s  %s", Cyan(fmt.Sprintf("%-*s", keyWidth, key)), secrets.Redact(fmt.Sprintf("%v", value)))
		return WriteLine(w, line)
	}

	if err := WriteLine(w, fmt.Sprintf("  %s:", Cyan(key))); err != nil {
		return err
	}
	rendered, err := yaml.Marshal(value)
	if err != nil {
		// Falls back to the plain scalar path rather than failing the
		// whole panel over one bad value - '%v' always succeeds.
		return WriteLine(w, fmt.Sprintf("    %s", secrets.Redact(fmt.Sprintf("%v", value))))
	}
	for _, line := range strings.Split(strings.TrimRight(string(rendered), "\n"), "\n") {
		if err := WriteLine(w, fmt.Sprintf("    %s", secrets.Redact(line))); err != nil {
			return err
		}
	}
	return nil
}

// isScalarFactValue reports whether v should render as a single "key
// value" line rather than an indented YAML block - true for everything
// except the two shapes a YAML-decoded map/list value takes
// (map[string]any, []any), which is every non-scalar shape a 'fact'/
// 'mount_facts' value can actually be.
func isScalarFactValue(v any) bool {
	switch v.(type) {
	case map[string]any, []any:
		return false
	default:
		return true
	}
}
