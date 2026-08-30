package model

import "testing"

func TestParseVarOverride(t *testing.T) {
	key, value, err := ParseVarOverride("ssh.port=22")
	if err != nil {
		t.Fatal(err)
	}
	if key != "ssh.port" || value != "22" {
		t.Fatalf("got key=%q value=%q", key, value)
	}
}

func TestParseVarOverrideKeepsEqualsInValue(t *testing.T) {
	key, value, err := ParseVarOverride("filter=a=b")
	if err != nil {
		t.Fatal(err)
	}
	if key != "filter" || value != "a=b" {
		t.Fatalf("got key=%q value=%q", key, value)
	}
}

func TestParseVarOverrideRejectsMissingEquals(t *testing.T) {
	if _, _, err := ParseVarOverride("noequals"); err == nil {
		t.Fatal("expected an error for a --var argument with no '='")
	}
}

func TestParseVarOverrideRejectsEmptyKey(t *testing.T) {
	if _, _, err := ParseVarOverride("=value"); err == nil {
		t.Fatal("expected an error for a --var argument with an empty key")
	}
}

func TestCoerceVarValueBooleans(t *testing.T) {
	if got := CoerceVarValue("true"); got != true {
		t.Fatalf("CoerceVarValue(true) = %#v, want bool true", got)
	}
	if got := CoerceVarValue("false"); got != false {
		t.Fatalf("CoerceVarValue(false) = %#v, want bool false", got)
	}
}

func TestCoerceVarValueLeavesOtherStringsAlone(t *testing.T) {
	cases := []string{"22", "nvim", "Eclipse.Temurin.21", "True", "FALSE", ""}
	for _, c := range cases {
		if got := CoerceVarValue(c); got != c {
			t.Fatalf("CoerceVarValue(%q) = %#v, want unchanged string", c, got)
		}
	}
}

func TestSetVarPathTopLevel(t *testing.T) {
	vars := map[string]any{}
	SetVarPath(vars, "editor", "nvim")
	if vars["editor"] != "nvim" {
		t.Fatalf("vars = %#v", vars)
	}
}

func TestSetVarPathCreatesNestedMaps(t *testing.T) {
	vars := map[string]any{}
	SetVarPath(vars, "ssh.port", "22")
	ssh, ok := vars["ssh"].(map[string]any)
	if !ok {
		t.Fatalf("vars[ssh] = %#v, want a map", vars["ssh"])
	}
	if ssh["port"] != "22" {
		t.Fatalf("ssh.port = %#v", ssh["port"])
	}
}

func TestSetVarPathOverwritesNonMapSegment(t *testing.T) {
	vars := map[string]any{"shell": "bash"}
	SetVarPath(vars, "shell.pwsh.profile", "~/profile.ps1")
	shell, ok := vars["shell"].(map[string]any)
	if !ok {
		t.Fatalf("vars[shell] = %#v, want a map after override", vars["shell"])
	}
	pwsh, ok := shell["pwsh"].(map[string]any)
	if !ok {
		t.Fatalf("shell[pwsh] = %#v, want a map", shell["pwsh"])
	}
	if pwsh["profile"] != "~/profile.ps1" {
		t.Fatalf("pwsh.profile = %#v", pwsh["profile"])
	}
}

func TestSetVarPathPreservesExistingSiblings(t *testing.T) {
	vars := map[string]any{"ssh": map[string]any{"host": "example.com"}}
	SetVarPath(vars, "ssh.port", "22")
	ssh := vars["ssh"].(map[string]any)
	if ssh["host"] != "example.com" || ssh["port"] != "22" {
		t.Fatalf("ssh = %#v, want both host and port set", ssh)
	}
}
