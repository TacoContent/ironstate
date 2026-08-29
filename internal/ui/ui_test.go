package ui

import (
	"bytes"
	"strings"
	"testing"

	"github.com/TacoContent/ironstate/internal/secrets"
)

func TestStyleDisabled(t *testing.T) {
	prev := Enabled
	Enabled = false
	defer func() { Enabled = prev }()

	if got := Red("x"); got != "x" {
		t.Fatalf("Red with Enabled=false = %q, want plain %q", got, "x")
	}
	if got := BoldGreen("y"); got != "y" {
		t.Fatalf("BoldGreen with Enabled=false = %q, want plain %q", got, "y")
	}
}

func TestStyleEnabled(t *testing.T) {
	prev := Enabled
	Enabled = true
	defer func() { Enabled = prev }()

	got := Green("ok")
	if got == "ok" {
		t.Fatalf("Green with Enabled=true should wrap in escape codes, got plain %q", got)
	}
}

func TestStyleEmptyString(t *testing.T) {
	prev := Enabled
	Enabled = true
	defer func() { Enabled = prev }()

	if got := Red(""); got != "" {
		t.Fatalf("Red(\"\") = %q, want empty", got)
	}
}

func TestNewlineIsCRLFWhenEnabled(t *testing.T) {
	prev := Enabled
	Enabled = true
	defer func() { Enabled = prev }()

	if got := Newline(); got != "\r\n" {
		t.Fatalf("Newline() with Enabled=true = %q, want \\r\\n", got)
	}
}

func TestNewlineIsLFWhenDisabled(t *testing.T) {
	prev := Enabled
	Enabled = false
	defer func() { Enabled = prev }()

	if got := Newline(); got != "\n" {
		t.Fatalf("Newline() with Enabled=false = %q, want \\n", got)
	}
}

func TestPrintFactsUsesCRLFWhenEnabled(t *testing.T) {
	prev := Enabled
	Enabled = true
	defer func() { Enabled = prev }()

	var buf bytes.Buffer
	if err := PrintFacts(&buf, map[string]any{"a": "1", "b": "2"}); err != nil {
		t.Fatalf("PrintFacts error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "\r\n") {
		t.Fatalf("PrintFacts with Enabled=true should use \\r\\n line endings, got %q", out)
	}
	if strings.Contains(strings.ReplaceAll(out, "\r\n", ""), "\n") {
		t.Fatalf("PrintFacts with Enabled=true should never emit a bare \\n, got %q", out)
	}
}

func TestPrintFactsSortsKeysAndIncludesEverything(t *testing.T) {
	prev := Enabled
	Enabled = false
	defer func() { Enabled = prev }()

	var buf bytes.Buffer
	facts := map[string]any{
		"zebra":   "z",
		"apple":   "a",
		"count":   float64(3),
		"enabled": true,
	}
	if err := PrintFacts(&buf, facts); err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	appleIdx := strings.Index(out, "apple")
	countIdx := strings.Index(out, "count")
	enabledIdx := strings.Index(out, "enabled")
	zebraIdx := strings.Index(out, "zebra")
	if appleIdx >= countIdx || countIdx >= enabledIdx || enabledIdx >= zebraIdx {
		t.Fatalf("keys not sorted: %q", out)
	}
	for _, want := range []string{"apple", "a", "count", "3", "enabled", "true", "zebra", "z"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q: %q", want, out)
		}
	}
}

func TestPrintFactsRedactsSecretValues(t *testing.T) {
	secrets.Reset()
	defer secrets.Reset()

	prev := Enabled
	Enabled = false
	defer func() { Enabled = prev }()

	secrets.Register("super-secret-value")

	var buf bytes.Buffer
	facts := map[string]any{"github_token": "super-secret-value", "plain": "visible"}
	if err := PrintFacts(&buf, facts); err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	if strings.Contains(out, "super-secret-value") {
		t.Fatalf("PrintFacts leaked a registered secret value: %q", out)
	}
	if !strings.Contains(out, "***") {
		t.Fatalf("PrintFacts should mask the secret fact, got %q", out)
	}
	if !strings.Contains(out, "visible") {
		t.Fatalf("PrintFacts should leave a non-secret fact alone, got %q", out)
	}
}

func TestPrintFactsRendersListOfMapsAsIndentedYAML(t *testing.T) {
	prev := Enabled
	Enabled = false
	defer func() { Enabled = prev }()

	var buf bytes.Buffer
	facts := map[string]any{
		"mounts": []any{
			map[string]any{"path": "/", "fstype": "ext4"},
			map[string]any{"path": "/boot", "fstype": "vfat"},
		},
	}
	if err := PrintFacts(&buf, facts); err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	// The key itself gets its own line ("key:"), not "key  value" like a
	// scalar fact - no Go map/slice syntax ("[map[...")  anywhere.
	if !strings.Contains(out, "mounts:\n") {
		t.Fatalf("expected a bare 'mounts:' line, got %q", out)
	}
	if strings.Contains(out, "[map[") {
		t.Fatalf("PrintFacts leaked Go's default map/slice syntax instead of YAML, got %q", out)
	}
	for _, want := range []string{"- fstype: ext4", "path: /\n", "- fstype: vfat", "path: /boot\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q: %q", want, out)
		}
	}
}

func TestPrintFactsRendersMapAsIndentedYAML(t *testing.T) {
	prev := Enabled
	Enabled = false
	defer func() { Enabled = prev }()

	var buf bytes.Buffer
	facts := map[string]any{
		"nested": map[string]any{"items": []any{"a", "b"}, "another-property": map[string]any{"sub-item": "foo"}},
	}
	if err := PrintFacts(&buf, facts); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "nested:\n") {
		t.Fatalf("expected a bare 'nested:' line, got %q", out)
	}
	for _, want := range []string{"another-property:", "sub-item: foo", "items:", "- a", "- b"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q: %q", want, out)
		}
	}
}

func TestPrintFactsRedactsSecretsInsideNestedValues(t *testing.T) {
	secrets.Reset()
	defer secrets.Reset()

	prev := Enabled
	Enabled = false
	defer func() { Enabled = prev }()

	secrets.Register("super-secret-value")

	var buf bytes.Buffer
	facts := map[string]any{
		"config": map[string]any{"token": "super-secret-value", "name": "visible"},
	}
	if err := PrintFacts(&buf, facts); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if strings.Contains(out, "super-secret-value") {
		t.Fatalf("PrintFacts leaked a registered secret value inside a nested fact: %q", out)
	}
	if !strings.Contains(out, "***") || !strings.Contains(out, "visible") {
		t.Fatalf("PrintFacts should mask the nested secret while leaving the rest alone, got %q", out)
	}
}

func TestModuleEmoji(t *testing.T) {
	if e := ModuleEmoji("winget"); e == "" {
		t.Fatal("ModuleEmoji(\"winget\") returned empty")
	}
	if e := ModuleEmoji("totally-unknown-module"); e != "🏷️" {
		t.Fatalf("ModuleEmoji(unknown) = %q, want bullet", e)
	}
}
