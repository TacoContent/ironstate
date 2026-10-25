package packages

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/TacoContent/ironstate/internal/facts"
)

// LoadHierarchy loads baseFile, then layers (in order): hosts/'s default
// (main.yml) plus chained hostname/os_family/distribution/platform/architecture
// overlays, variables/'s default (main.yml) plus the same chained
// overlays, then the username-keyed variables/<user>.yml overlay (the
// user_name fact and the USERNAME environment variable are both tried; see
// facts.UserNameCandidates) — ports Packages.psm1's Import-PackagesHierarchy, extended per
// docs/_plans/notes.md so a host/variant-specific overlay isn't limited to
// a single hosts/<COMPUTERNAME>.yml file. Every overlay file resolves its
// own 'copy.src'/'shell.script' paths relative to the same root
// (baseFile's directory), regardless of which subdirectory the overlay
// file itself lives in.
func LoadHierarchy(baseFile string, hostFacts map[string]any) (any, error) {
	dir := filepath.Dir(baseFile)
	data, err := LoadFile(baseFile, dir)
	if err != nil {
		return nil, err
	}

	hostsDir := filepath.Join(dir, "hosts")
	if data, err = MergeDefaultOverlay(data, hostsDir, dir); err != nil {
		return nil, err
	}
	if data, err = MergeChainOverlays(data, hostsDir, dir, hostFacts); err != nil {
		return nil, err
	}

	varsDir := filepath.Join(dir, "variables")
	if data, err = MergeDefaultOverlay(data, varsDir, dir); err != nil {
		return nil, err
	}
	if data, err = MergeChainOverlays(data, varsDir, dir, hostFacts); err != nil {
		return nil, err
	}

	var merged []os.FileInfo
	var userNames []string
	for _, userName := range facts.UserNameCandidates(hostFacts) {
		// 'ironstate init' writes the lowercased name.
		userNames = append(userNames, userName, strings.ToLower(userName))
	}
	for _, userName := range userNames {
		userFile := findChainFile(varsDir, userName)
		if userFile == "" {
			continue
		}
		info, statErr := os.Stat(userFile)
		if statErr != nil || sameFileAsAny(info, merged) {
			continue
		}
		merged = append(merged, info)
		if data, err = mergeOverlayFile(data, userFile, dir); err != nil {
			return nil, err
		}
	}
	return data, nil
}

// sameFileAsAny guards against merging one file twice when two user name
// candidates differ only by case on a case-insensitive filesystem.
func sameFileAsAny(info os.FileInfo, others []os.FileInfo) bool {
	for _, other := range others {
		if os.SameFile(info, other) {
			return true
		}
	}
	return false
}
