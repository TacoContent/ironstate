//go:build linux

package facts

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// linuxMountSources is tried in order — /proc/mounts is the kernel's own
// live view (always present on a real Linux kernel); /etc/mtab is the
// older userspace-maintained fallback for the rare environment where
// /proc isn't mounted (e.g. a minimal chroot/container).
var linuxMountSources = []string{"/proc/mounts", "/etc/mtab"}

func init() {
	platformMounts = linuxMounts
}

// linuxMounts parses the first readable source in linuxMountSources —
// each line is "device path fstype options dump fsck-order", the same
// fstab-derived format /proc/mounts and /etc/mtab both use (fields'
// octal-escaped spaces, e.g. "\040", are left as-is: no mount path in
// practice needs unescaping for facts/templating use).
func linuxMounts() ([]MountFact, error) {
	var lastErr error
	for _, source := range linuxMountSources {
		f, err := os.Open(source)
		if err != nil {
			lastErr = err
			continue
		}
		mounts, err := parseMountsFile(f, source)
		closeErr := f.Close()
		if err != nil {
			lastErr = err
			continue
		}
		if closeErr != nil {
			return mounts, closeErr
		}
		return mounts, nil
	}
	return nil, fmt.Errorf("no readable mount source found (tried %s): %w", strings.Join(linuxMountSources, ", "), lastErr)
}

func parseMountsFile(f *os.File, source string) ([]MountFact, error) {
	var mounts []MountFact
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		mounts = append(mounts, MountFact{
			Source:  source,
			Device:  fields[0],
			Path:    fields[1],
			FSType:  fields[2],
			Options: fields[3],
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return mounts, nil
}
