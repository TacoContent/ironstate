package facts

import (
	"fmt"
	"time"
)

// MountFact is one gathered mount entry — shape requested to mirror
// Ansible's mount_facts module (source/device/fstype/options/mount), kept
// to today's "report what's currently mounted" scope rather than that
// module's full multi-source aggregation (no fstab-vs-mtab merge; per-mount
// filtering is a caller concern handled by internal/handlers/mountfacts.go's
// 'filter' field, not this package).
type MountFact struct {
	// Source is where this entry was read from — a file path
	// (e.g. "/proc/mounts") on Linux/macOS, or a fixed label describing
	// the Win32 API used on Windows, since Windows has no fstab/mtab
	// equivalent.
	Source string
	// Device identifies the underlying volume: a "/dev/..." node on
	// Linux/macOS, a "\\?\Volume{guid}\" volume path for a local Windows
	// volume, or a "\\server\share" UNC path for a mapped network drive.
	Device string
	// FSType is the filesystem type (e.g. "ext4", "apfs", "ntfs").
	FSType string
	// Options is a comma-joined mount-options string, platform-native
	// (Linux/macOS report real mount options; Windows synthesizes
	// "rw"/"ro" plus any detected volume flags, since it has no options
	// string of its own).
	Options string
	// Path is where the filesystem is mounted — a POSIX path on
	// Linux/macOS, or a drive letter root ("C:\") or NTFS folder mount
	// point on Windows.
	Path string
}

// AsMap converts m to the map[string]any shape facts/templating expects.
func (m MountFact) AsMap() map[string]any {
	return map[string]any{
		"source":  m.Source,
		"device":  m.Device,
		"fstype":  m.FSType,
		"options": m.Options,
		"path":    m.Path,
	}
}

// platformMounts gathers the current platform's mounts — implemented per
// build tag in mounts_linux.go/mounts_darwin.go/mounts_windows.go, and
// stubbed in mounts_other.go for unsupported platforms. Overridable for
// tests.
var platformMounts = func() ([]MountFact, error) { return nil, fmt.Errorf("mount facts are not supported on this platform") }

// GatherMounts runs platformMounts bounded by timeout, matching
// runVersionProbe's "never hang the whole run" contract (shells.go) —
// timeout <= 0 means "no bound". Windows' mapped-network-drive lookups are
// the main reason this matters: an unreachable share can otherwise hang
// indefinitely, since Win32's WNetGetConnection has no cancellable
// context, unlike exec.CommandContext's subprocess-kill.
//
// On timeout, the still-running platformMounts goroutine is abandoned
// (its eventual result is discarded) rather than force-killed — there is
// no safe way to cancel an in-flight Win32 syscall from Go, so this is the
// same tradeoff exec.CommandContext's process-kill exists to avoid, just
// unavailable here.
func GatherMounts(timeout time.Duration) ([]MountFact, error) {
	if timeout <= 0 {
		return platformMounts()
	}

	type gatherResult struct {
		mounts []MountFact
		err    error
	}
	done := make(chan gatherResult, 1)
	go func() {
		mounts, err := platformMounts()
		done <- gatherResult{mounts, err}
	}()

	select {
	case r := <-done:
		return r.mounts, r.err
	case <-time.After(timeout):
		return nil, fmt.Errorf("timed out after %s gathering mount facts", timeout)
	}
}
