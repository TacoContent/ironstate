//go:build linux

package facts

import (
	"bufio"
	"os"
	"strings"
)

// osReleasePaths are checked in order, matching os-release(5)'s documented
// lookup order (/etc/os-release takes precedence over the vendor default).
var osReleasePaths = []string{"/etc/os-release", "/usr/lib/os-release"}

// distroAliases maps a handful of os-release(5) IDs to the names more
// commonly used elsewhere (e.g. Ansible's ansible_distribution), since
// 'facts.os_family == "archlinux"' reads more naturally in a main.yml
// than the raw freedesktop ID "arch".
var distroAliases = map[string]string{
	"arch": "archlinux",
	"rhel": "redhat",
}

// distro identifies the Linux distribution from /etc/os-release's ID
// field (falling back to ID_LIKE's first token, e.g. Linux Mint's
// ID_LIKE=ubuntu), or "" if neither file is readable/set.
func distro() string {
	for _, path := range osReleasePaths {
		id, idLike, _, ok := readOSRelease(path)
		if !ok {
			continue
		}
		return normalizeDistro(id, idLike)
	}
	return ""
}

// distroVersionID returns /etc/os-release's VERSION_ID (e.g. "22.04",
// "12", "3.19.1") for os_version - or "" if neither file is
// readable/set, or the distro doesn't publish one at all (e.g. Arch
// Linux's rolling release has no VERSION_ID), in which case osVersion
// falls back to the kernel release instead.
func distroVersionID() string {
	for _, path := range osReleasePaths {
		_, _, versionID, ok := readOSRelease(path)
		if !ok {
			continue
		}
		if versionID != "" {
			return versionID
		}
	}
	return ""
}

func readOSRelease(path string) (id, idLike, versionID string, ok bool) {
	f, err := os.Open(path) //nolint:gosec // fixed set of well-known paths, not user input
	if err != nil {
		return "", "", "", false
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		key, val, found := strings.Cut(strings.TrimSpace(scanner.Text()), "=")
		if !found {
			continue
		}
		val = strings.Trim(val, `"'`)
		switch key {
		case "ID":
			id = val
		case "ID_LIKE":
			idLike = val
		case "VERSION_ID":
			versionID = val
		}
	}
	return id, idLike, versionID, true
}

func normalizeDistro(id, idLike string) string {
	if v := aliasOrLower(id); v != "" {
		return v
	}
	for _, like := range strings.Fields(idLike) {
		if v := aliasOrLower(like); v != "" {
			return v
		}
	}
	return ""
}

func aliasOrLower(id string) string {
	id = strings.ToLower(strings.TrimSpace(id))
	if alias, ok := distroAliases[id]; ok {
		return alias
	}
	return id
}
