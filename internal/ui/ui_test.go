package ui

import "testing"

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

func TestModuleEmoji(t *testing.T) {
	if e := ModuleEmoji("winget"); e == "" {
		t.Fatal("ModuleEmoji(\"winget\") returned empty")
	}
	if e := ModuleEmoji("totally-unknown-module"); e != "🏷️" {
		t.Fatalf("ModuleEmoji(unknown) = %q, want bullet", e)
	}
}
