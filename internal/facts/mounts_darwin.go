//go:build darwin

package facts

import (
	"strings"

	"golang.org/x/sys/unix"
)

// darwinMountSource labels every entry the same way, since macOS has no
// single fstab/mtab-equivalent file — unix.Getfsstat(MNT_NOWAIT) asks the
// kernel directly for the live mount table, without a shell-out (mirrors
// mount(8)'s own default, cached view — no network stalls waiting on an
// unresponsive NFS/SMB server, unlike MNT_WAIT).
const darwinMountSource = "getfsstat"

func init() {
	platformMounts = darwinMounts
}

func darwinMounts() ([]MountFact, error) {
	n, err := unix.Getfsstat(nil, unix.MNT_NOWAIT)
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, nil
	}
	buf := make([]unix.Statfs_t, n)
	n, err = unix.Getfsstat(buf, unix.MNT_NOWAIT)
	if err != nil {
		return nil, err
	}
	buf = buf[:n]

	mounts := make([]MountFact, 0, len(buf))
	for _, st := range buf {
		mounts = append(mounts, MountFact{
			Source:  darwinMountSource,
			Device:  cString(st.Mntfromname[:]),
			Path:    cString(st.Mntonname[:]),
			FSType:  cString(st.Fstypename[:]),
			Options: darwinMountOptions(st.Flags),
		})
	}
	return mounts, nil
}

// darwinMountOptions decodes Statfs_t.Flags' MNT_* bits into the same
// comma-joined "rw,nosuid,..." shape Linux's /proc/mounts already reports,
// so 'options' reads consistently across both platforms.
func darwinMountOptions(flags uint32) string {
	opts := []string{"rw"}
	if flags&unix.MNT_RDONLY != 0 {
		opts[0] = "ro"
	}
	type flagName struct {
		bit  uint32
		name string
	}
	for _, fn := range []flagName{
		{unix.MNT_SYNCHRONOUS, "sync"},
		{unix.MNT_NOEXEC, "noexec"},
		{unix.MNT_NOSUID, "nosuid"},
		{unix.MNT_NODEV, "nodev"},
		{unix.MNT_LOCAL, "local"},
		{unix.MNT_JOURNALED, "journaled"},
		{unix.MNT_NOATIME, "noatime"},
	} {
		if flags&fn.bit != 0 {
			opts = append(opts, fn.name)
		}
	}
	return strings.Join(opts, ",")
}

// cString trims a fixed-size NUL-padded byte array (Statfs_t's C-string
// fields) down to its Go string content.
func cString(b []byte) string {
	if i := strings.IndexByte(string(b), 0); i >= 0 {
		return string(b[:i])
	}
	return string(b)
}
