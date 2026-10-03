package bundle

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTestFile(t *testing.T, p, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestBuildExtractRoundTripHonorsExclusions(t *testing.T) {
	src := t.TempDir()
	writeTestFile(t, filepath.Join(src, "site.yml"), "tasks: []")
	writeTestFile(t, filepath.Join(src, "roles", "a", "main.yml"), "role")
	writeTestFile(t, filepath.Join(src, ".env"), "SECRET=1")
	writeTestFile(t, filepath.Join(src, "roles", ".secrets"), "SECRET=1")
	writeTestFile(t, filepath.Join(src, ".git", "HEAD"), "ref")
	writeTestFile(t, filepath.Join(src, "big.iso"), "iso")
	writeTestFile(t, filepath.Join(src, "cache", "x"), "x")
	writeTestFile(t, filepath.Join(src, IgnoreFileName), "# comment\n*.iso\ncache/\n")
	extra := filepath.Join(t.TempDir(), "vars.yml")
	writeTestFile(t, extra, "vars: {}")

	var buf bytes.Buffer
	info, err := Build(&buf, Spec{
		Dirs:      []Dir{{Source: src, Dest: "playbook"}},
		Files:     []File{{Source: extra, Dest: "vars-files/00-vars.yml"}},
		Generated: map[string][]byte{"ironstate.yaml": []byte("filters: {}")},
	})
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(buf.Bytes())
	if info.Size != int64(buf.Len()) || info.SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("info = %+v, want size %d sha %x", info, buf.Len(), sum)
	}

	dest := t.TempDir()
	if err := Extract(&buf, dest); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"playbook/site.yml", "playbook/roles/a/main.yml", "vars-files/00-vars.yml", "ironstate.yaml"} {
		if _, err := os.Stat(filepath.Join(dest, filepath.FromSlash(want))); err != nil {
			t.Errorf("missing %s: %v", want, err)
		}
	}
	for _, unwanted := range []string{"playbook/.env", "playbook/roles/.secrets", "playbook/.git", "playbook/big.iso", "playbook/cache"} {
		if _, err := os.Stat(filepath.Join(dest, filepath.FromSlash(unwanted))); err == nil {
			t.Errorf("%s should not be bundled", unwanted)
		}
	}
}

func tarGz(t *testing.T, entries ...*tar.Header) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, hdr := range entries {
		if hdr.Typeflag == tar.TypeReg {
			hdr.Size = 1
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if hdr.Typeflag == tar.TypeReg {
			_, _ = tw.Write([]byte("x"))
		}
	}
	_ = tw.Close()
	_ = gz.Close()
	return buf.Bytes()
}

func TestExtractRejectsUnsafeEntries(t *testing.T) {
	cases := map[string]*tar.Header{
		"traversal":        {Name: "../evil", Typeflag: tar.TypeReg, Mode: 0o600},
		"nested":           {Name: "a/../../evil", Typeflag: tar.TypeReg, Mode: 0o600},
		"absolute":         {Name: "/etc/evil", Typeflag: tar.TypeReg, Mode: 0o600},
		"backslash":        {Name: `..\evil`, Typeflag: tar.TypeReg, Mode: 0o600},
		"drive":            {Name: "C:/evil", Typeflag: tar.TypeReg, Mode: 0o600},
		"symlink escape":   {Name: "link", Typeflag: tar.TypeSymlink, Linkname: "../outside"},
		"symlink abs":      {Name: "link", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd"},
		"device":           {Name: "dev", Typeflag: tar.TypeChar},
		"hardlink":         {Name: "hard", Typeflag: tar.TypeLink, Linkname: "x"},
		"deep link escape": {Name: "a/b/link", Typeflag: tar.TypeSymlink, Linkname: "../../../x"},
	}
	for name, hdr := range cases {
		t.Run(name, func(t *testing.T) {
			parent := t.TempDir()
			dest := filepath.Join(parent, "out")
			if err := os.Mkdir(dest, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := Extract(bytes.NewReader(tarGz(t, hdr)), dest); err == nil {
				t.Fatal("Extract accepted an unsafe entry")
			}
			assertNothingOutside(t, parent)
		})
	}
}

func TestBuildRejectsEscapingSymlink(t *testing.T) {
	src := t.TempDir()
	if err := os.Symlink(filepath.Join(t.TempDir(), "elsewhere"), filepath.Join(src, "link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := Build(&bytes.Buffer{}, Spec{Dirs: []Dir{{Source: src, Dest: "playbook"}}}); err == nil || !strings.Contains(err.Error(), "outside the bundle") {
		t.Fatalf("err = %v, want escaping-symlink error", err)
	}
}

func TestIsSafeEntryName(t *testing.T) {
	for name, want := range map[string]bool{
		"playbook/site.yml": true, "a/b/": true, "video..final.mp4": true,
		"": false, "..": false, "../x": false, "a/../../x": false, "/x": false, `a\b`: false, "C:x": false, "a/..": false,
	} {
		if got := IsSafeEntryName(name); got != want {
			t.Errorf("IsSafeEntryName(%q) = %v, want %v", name, got, want)
		}
	}
}

func assertNothingOutside(t *testing.T, parent string) {
	t.Helper()
	entries, err := os.ReadDir(parent)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != "out" {
			t.Fatalf("Extract wrote %q outside dest", e.Name())
		}
	}
}

func FuzzExtract(f *testing.F) {
	f.Add([]byte{})
	var buf bytes.Buffer
	_, _ = Build(&buf, Spec{Generated: map[string][]byte{"a/b.txt": []byte("hi")}})
	f.Add(buf.Bytes())
	f.Fuzz(func(t *testing.T, data []byte) {
		parent := t.TempDir()
		dest := filepath.Join(parent, "out")
		if err := os.Mkdir(dest, 0o700); err != nil {
			t.Fatal(err)
		}
		_ = Extract(bytes.NewReader(data), dest)
		assertNothingOutside(t, parent)
	})
}
