package facts

import (
	"fmt"
	"testing"
	"time"
)

func TestGatherMountsRespectsTimeout(t *testing.T) {
	orig := platformMounts
	defer func() { platformMounts = orig }()

	block := make(chan struct{})
	defer close(block) // let the abandoned goroutine finish so it doesn't leak past the test
	platformMounts = func() ([]MountFact, error) {
		<-block
		return nil, nil
	}

	if _, err := GatherMounts(10 * time.Millisecond); err == nil {
		t.Fatal("expected a timeout error")
	}
}

func TestGatherMountsReturnsResultBeforeTimeout(t *testing.T) {
	orig := platformMounts
	defer func() { platformMounts = orig }()

	want := []MountFact{{Source: "test", Device: "/dev/x", Path: "/mnt", FSType: "ext4", Options: "rw"}}
	platformMounts = func() ([]MountFact, error) { return want, nil }

	got, err := GatherMounts(time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != want[0] {
		t.Fatalf("GatherMounts = %#v, want %#v", got, want)
	}
}

func TestGatherMountsZeroTimeoutMeansNoBound(t *testing.T) {
	orig := platformMounts
	defer func() { platformMounts = orig }()

	called := false
	platformMounts = func() ([]MountFact, error) {
		called = true
		return nil, nil
	}

	if _, err := GatherMounts(0); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("expected platformMounts to be called directly when timeout <= 0")
	}
}

func TestMountFactAsMap(t *testing.T) {
	m := MountFact{Source: "/proc/mounts", Device: "/dev/sda1", FSType: "ext4", Options: "rw,relatime", Path: "/"}
	got := m.AsMap()
	want := map[string]any{"source": "/proc/mounts", "device": "/dev/sda1", "fstype": "ext4", "options": "rw,relatime", "path": "/"}
	if fmt.Sprintf("%v", got) != fmt.Sprintf("%v", want) {
		t.Fatalf("AsMap() = %#v, want %#v", got, want)
	}
}
