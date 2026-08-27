//go:build linux

package facts

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseMountsFileParsesFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mounts")
	content := "# comment, ignored\n" +
		"/dev/sda1 / ext4 rw,relatime 0 0\n" +
		"\n" +
		"tmpfs /run tmpfs rw,nosuid,size=1633736k 0 0\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	mounts, err := parseMountsFile(f, path)
	if err != nil {
		t.Fatal(err)
	}
	if len(mounts) != 2 {
		t.Fatalf("len(mounts) = %d, want 2: %#v", len(mounts), mounts)
	}
	want := MountFact{Source: path, Device: "/dev/sda1", Path: "/", FSType: "ext4", Options: "rw,relatime"}
	if mounts[0] != want {
		t.Fatalf("mounts[0] = %#v, want %#v", mounts[0], want)
	}
	if mounts[1].Device != "tmpfs" || mounts[1].Path != "/run" || mounts[1].FSType != "tmpfs" {
		t.Fatalf("mounts[1] = %#v", mounts[1])
	}
}

func TestLinuxMountsFallsBackToNextSource(t *testing.T) {
	dir := t.TempDir()
	fstab := filepath.Join(dir, "does-not-exist")
	mtab := filepath.Join(dir, "mtab")
	if err := os.WriteFile(mtab, []byte("/dev/sda1 / ext4 rw 0 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	orig := linuxMountSources
	linuxMountSources = []string{fstab, mtab}
	defer func() { linuxMountSources = orig }()

	mounts, err := linuxMounts()
	if err != nil {
		t.Fatal(err)
	}
	if len(mounts) != 1 || mounts[0].Source != mtab {
		t.Fatalf("mounts = %#v, want one entry sourced from %s", mounts, mtab)
	}
}

func TestLinuxMountsErrorsWhenNoSourceReadable(t *testing.T) {
	dir := t.TempDir()
	orig := linuxMountSources
	linuxMountSources = []string{filepath.Join(dir, "missing-a"), filepath.Join(dir, "missing-b")}
	defer func() { linuxMountSources = orig }()

	if _, err := linuxMounts(); err == nil {
		t.Fatal("expected an error when no mount source is readable")
	}
}
