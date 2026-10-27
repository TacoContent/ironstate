package handlers

import (
	"slices"
	"strings"
	"testing"

	"github.com/TacoContent/ironstate/internal/engine"
	ironexec "github.com/TacoContent/ironstate/internal/exec"
)

func TestWingetHandlerInstallerArguments(t *testing.T) {
	tests := []struct {
		name       string
		options    map[string]any
		wantArgs   []string
		wantSuffix string
	}{
		{name: "omitted"},
		{name: "empty override", options: map[string]any{"override": ""}},
		{
			name:       "override",
			options:    map[string]any{"override": "/S /norestart"},
			wantArgs:   []string{"--override", "/S /norestart"},
			wantSuffix: " --override /S /norestart",
		},
		{
			name:       "override preserves quotes and spaces",
			options:    map[string]any{"override": `/S /D="C:\Program Files\Example"`},
			wantArgs:   []string{"--override", `/S /D="C:\Program Files\Example"`},
			wantSuffix: ` --override /S /D="C:\Program Files\Example"`,
		},
		{
			name:       "custom",
			options:    map[string]any{"custom": "/norestart"},
			wantArgs:   []string{"--custom", "/norestart"},
			wantSuffix: " --custom /norestart",
		},
		{
			name:       "override and custom",
			options:    map[string]any{"override": "/S", "custom": "/norestart"},
			wantArgs:   []string{"--override", "/S", "--custom", "/norestart"},
			wantSuffix: " --override /S --custom /norestart",
		},
		{
			name:       "empty override with custom",
			options:    map[string]any{"override": "", "custom": "/norestart"},
			wantArgs:   []string{"--custom", "/norestart"},
			wantSuffix: " --custom /norestart",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &recordingRunner{responses: []ironexec.Result{{RC: 0}}}
			withRunner(t, rec)

			h := wingetHandler{}
			item := map[string]any{"package": "FiloSottile.age", "source": "winget"}
			for key, value := range tt.options {
				item[key] = value
			}

			if _, err := h.Install(item, "", testCtx()); err != nil {
				t.Fatal(err)
			}
			want := append([]string{"install", "--id", "FiloSottile.age", "--exact", "--accept-source-agreements", "--accept-package-agreements", "--source", "winget"}, tt.wantArgs...)
			if len(rec.calls) != 1 || rec.exes[0] != "winget" || !slices.Equal(rec.calls[0], want) {
				t.Fatalf("exes=%v calls=%v, want winget %q", rec.exes, rec.calls, want)
			}
			desc, err := h.Describe(item, engine.ActionInstall, testCtx())
			if err != nil {
				t.Fatal(err)
			}
			wantDesc := "winget install --id FiloSottile.age --exact --source winget" + tt.wantSuffix
			if desc != wantDesc {
				t.Fatalf("description = %q, want %q", desc, wantDesc)
			}

			if _, err := h.Uninstall(item, "", testCtx()); err != nil {
				t.Fatal(err)
			}
			want = []string{"uninstall", "--id", "FiloSottile.age", "--exact"}
			if len(rec.calls) != 2 || rec.exes[1] != "winget" || !slices.Equal(rec.calls[1], want) {
				t.Fatalf("exes=%v calls=%v, want winget %q for uninstall", rec.exes, rec.calls, want)
			}
			desc, err = h.Describe(item, engine.ActionUninstall, testCtx())
			if err != nil {
				t.Fatal(err)
			}
			if wantDesc := "winget uninstall --id FiloSottile.age --exact"; desc != wantDesc {
				t.Fatalf("uninstall description = %q, want %q", desc, wantDesc)
			}
		})
	}
}

func TestWingetHandlerInstallArgv(t *testing.T) {
	rec := &recordingRunner{responses: []ironexec.Result{{RC: 0}}}
	withRunner(t, rec)

	h := wingetHandler{}
	item := map[string]any{"package": "FiloSottile.age", "source": "winget"}
	if _, err := h.Install(item, "", testCtx()); err != nil {
		t.Fatal(err)
	}
	if rec.exes[0] != "winget" {
		t.Fatalf("exe = %q", rec.exes[0])
	}
	want := []string{"install", "--id", "FiloSottile.age", "--exact", "--accept-source-agreements", "--accept-package-agreements", "--source", "winget"}
	if strings.Join(rec.calls[0], " ") != strings.Join(want, " ") {
		t.Fatalf("args = %v, want %v", rec.calls[0], want)
	}
}

func TestWingetHandlerTestChecksOutputText(t *testing.T) {
	rec := &recordingRunner{responses: []ironexec.Result{{RC: 0, Stdout: "No installed package found matching input criteria."}}}
	withRunner(t, rec)

	h := wingetHandler{}
	installed, err := h.Test(map[string]any{"package": "x"}, "", testCtx())
	if err != nil || installed {
		t.Fatalf("installed=%v err=%v, want false", installed, err)
	}
}

func TestWingetPackagesToEntriesKeepsIdsAndSources(t *testing.T) {
	previousLookup := wingetDisplayNameLookup
	wingetDisplayNameLookup = func(identifier string) string {
		if identifier == "9P8LTPGCBZXD" {
			return "Wintoys"
		}
		return ""
	}
	defer func() {
		wingetDisplayNameLookup = previousLookup
	}()

	items := wingetPackagesToEntries([]wingetExportPackage{
		{Identifier: "Microsoft.VCRedist.2010.x64", Source: "winget"},
		{Identifier: "Microsoft.VCRedist.2010.x86", Source: "winget"},
		{Identifier: "9P8LTPGCBZXD", Source: "msstore"},
		{Identifier: "7zip.7zip", Source: "winget"},
	})
	if len(items) != 4 {
		t.Fatalf("items = %d, want 4", len(items))
	}
	for _, want := range []wingetPackage{
		{Name: "Microsoft.VCRedist.2010.x64", Identifier: "Microsoft.VCRedist.2010.x64", Source: "winget"},
		{Name: "Microsoft.VCRedist.2010.x86", Identifier: "Microsoft.VCRedist.2010.x86", Source: "winget"},
		{Name: "Wintoys", Identifier: "9P8LTPGCBZXD", Source: "msstore"},
		{Name: "7zip.7zip", Identifier: "7zip.7zip", Source: "winget"},
	} {
		matched := false
		for _, got := range items {
			if got.Identifier == want.Identifier {
				matched = true
				if got != want {
					t.Fatalf("package %+v, want %+v", got, want)
				}
			}
		}
		if !matched {
			t.Fatalf("missing package identifier %q", want.Identifier)
		}
	}
}

func TestWingetHandlerScanRolePath(t *testing.T) {
	winget := wingetHandler{}
	if got := winget.ScanRole(); got != "roles/packages/winget" {
		t.Fatalf("ScanRole() = %q, want roles/packages/winget", got)
	}
}
