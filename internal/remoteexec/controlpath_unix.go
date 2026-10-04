//go:build unix

package remoteexec

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// controlPath returns an ssh ControlPath in a private /tmp dir (short,
// since Unix socket paths max out around 104 bytes), or "" to disable
// multiplexing when the dir isn't safely ours.
func controlPath() string {
	uid := os.Getuid()
	dir := filepath.Join("/tmp", fmt.Sprintf("ironstate-%d", uid))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return ""
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 {
		return ""
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(st.Uid) != uid {
		return ""
	}
	return filepath.Join(dir, "%C")
}
