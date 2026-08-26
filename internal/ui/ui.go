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

	"golang.org/x/term"
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
	"registry":       "🗃️",
	"scheduled_task": "⏰",
	"assert":         "✅",
	"async":          "⚡",
	"wait_for":       "⏳",
}

// ModuleEmoji returns module's representative glyph, or a generic bullet
// for an unrecognized module name.
func ModuleEmoji(module string) string {
	if e, ok := moduleEmoji[module]; ok {
		return e
	}
	return "🏷️"
}

// PrintFacts renders a small "modern CLI" panel of gathered host facts on
// w, sorted by key for stable/diffable output — the "display facts info
// after gathered" request. Values are formatted with fmt's default verb
// since facts.Gather() only ever produces scalars (string/bool/float64).
func PrintFacts(w io.Writer, hostFacts map[string]any) error {
	keys := make([]string, 0, len(hostFacts))
	keyWidth := 0
	for k := range hostFacts {
		keys = append(keys, k)
		if len(k) > keyWidth {
			keyWidth = len(k)
		}
	}
	sort.Strings(keys)

	rule := Dim("──────────────────────────────")
	if _, err := fmt.Fprintln(w, rule); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, Bold("🧠 Host facts")); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, rule); err != nil {
		return err
	}
	for _, k := range keys {
		line := fmt.Sprintf("  %s  %v", Cyan(fmt.Sprintf("%-*s", keyWidth, k)), hostFacts[k])
		if _, err := fmt.Fprintln(w, line); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(w, rule); err != nil {
		return err
	}
	return nil
}
