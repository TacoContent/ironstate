package packages

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// chainCharacteristic is one host characteristic usable in a "chained"
// override filename, with its fixed priority weight (docs/plans/notes.md:
// priority order is hostname, then os_family, then platform, then arch).
// Weights are powers of two specifically so that any subset containing a
// higher-priority characteristic always outranks every subset that
// doesn't - e.g. hostname (8) alone already outranks os_family+platform+
// arch combined (4+2+1=7) - matching the requested "hostname always
// wins" priority tree.
type chainCharacteristic struct {
	value  string
	weight int
}

// chainFacts is the fixed set of host-characteristic values a "chained"
// override filename can be built from, in priority order (highest weight
// first): hostname, os_family, platform, arch. A characteristic with no
// value (empty string) is omitted entirely - it can't take part in any
// candidate name.
func chainFacts(facts map[string]any) []chainCharacteristic {
	all := []chainCharacteristic{
		{stringFact(facts, "computer_name"), 8},
		{stringFact(facts, "os_family"), 4},
		{stringFact(facts, "platform"), 2},
		{stringFact(facts, "arch"), 1},
	}
	present := make([]chainCharacteristic, 0, len(all))
	for _, c := range all {
		if c.value != "" {
			present = append(present, c)
		}
	}
	return present
}

func stringFact(facts map[string]any, key string) string {
	if facts == nil {
		return ""
	}
	v, _ := facts[key].(string)
	return v
}

// ChainCandidates returns override base names (no extension): every
// permutation of every non-empty subset of the available host
// characteristics (hostname, os_family, platform, arch), joined by '.' -
// e.g. "krayt" (hostname alone), "windows" (os_family or platform alone -
// handy for a host-agnostic "any Windows machine" overlay like
// hosts/windows.yml), "amd64.krayt" or "krayt.amd64" (hostname+arch, in
// either order - callers can write the characteristics in whatever order
// they want), all the way up to a full four-characteristic name. Any N of
// the characteristics may be used, in any order.
//
// Ordered least-specific to most-specific for merging, by priority
// weight ascending (a subset's weight is the sum of its characteristics'
// fixed weights - see chainFacts - so e.g. any hostname-containing name
// always outranks any name without one, and adding more characteristics
// to a name always increases its weight further). Two different
// permutations of the same subset share a weight and tie-break
// alphabetically for a deterministic order. When two different subsets
// happen to render the identical string (e.g. os_family and platform are
// both "windows" on a Windows host, so a bare "windows" could mean
// either), the higher of their weights is used.
func ChainCandidates(facts map[string]any) []string {
	present := chainFacts(facts)
	n := len(present)

	best := map[string]int{}
	for mask := 1; mask < (1 << n); mask++ {
		var idx []int
		weight := 0
		for i := 0; i < n; i++ {
			if mask&(1<<i) != 0 {
				idx = append(idx, i)
				weight += present[i].weight
			}
		}
		permuteIndices(idx, func(order []int) {
			parts := make([]string, len(order))
			for j, k := range order {
				parts[j] = present[k].value
			}
			name := strings.Join(parts, ".")
			if w, ok := best[name]; !ok || weight > w {
				best[name] = weight
			}
		})
	}

	candidates := make([]string, 0, len(best))
	for name := range best {
		candidates = append(candidates, name)
	}
	sort.Slice(candidates, func(i, j int) bool {
		wi, wj := best[candidates[i]], best[candidates[j]]
		if wi != wj {
			return wi < wj
		}
		return candidates[i] < candidates[j]
	})
	return candidates
}

// permuteIndices calls cb once per ordering of idx's elements (in place,
// via Heap's-algorithm-style backtracking) - up to 4! = 24 calls for this
// package's maximum 4 characteristics, negligible cost.
func permuteIndices(idx []int, cb func([]int)) {
	n := len(idx)
	used := make([]bool, n)
	order := make([]int, 0, n)
	var rec func()
	rec = func() {
		if len(order) == n {
			cb(order)
			return
		}
		for i := 0; i < n; i++ {
			if used[i] {
				continue
			}
			used[i] = true
			order = append(order, idx[i])
			rec()
			order = order[:len(order)-1]
			used[i] = false
		}
	}
	rec()
}

// findChainFile returns the first of "<dir>/<name>.yml" / "<dir>/<name>.yaml"
// that exists, or "" if neither does.
func findChainFile(dir, name string) string {
	for _, ext := range []string{".yml", ".yaml"} {
		candidate := filepath.Join(dir, name+ext)
		if fileExists(candidate) {
			return candidate
		}
	}
	return ""
}

func fileExists(path string) bool {
	fi, err := os.Stat(path) //nolint:gosec // configured overlay locations, same trust boundary as the rest of this tool
	return err == nil && !fi.IsDir()
}

// MergeChainOverlays merges each of ChainCandidates(facts) that exists as
// "<scanDir>/<candidate>.yml"/".yaml" onto data, least-specific first (so
// the most specific one wins on conflicting keys) — ports
// docs/plans/notes.md's "chained" hostname/os_family/platform/arch
// override filenames. Relative paths inside each overlay resolve against
// baseDir, the same root every sibling overlay in a given hierarchy uses,
// regardless of which subdirectory the overlay file itself lives in.
// Used both for the top-level hosts:/variables: overlay directories
// (internal/packages.LoadHierarchy) and, via LoadIncludedPackage, for
// every include:'s own package directory.
func MergeChainOverlays(data any, scanDir, baseDir string, facts map[string]any) (any, error) {
	seen := map[string]bool{}
	for _, name := range ChainCandidates(facts) {
		file := findChainFile(scanDir, name)
		// Two different candidate names (e.g. os_family and platform,
		// both "windows" on a Windows host) can resolve to the same
		// physical file - merge each real file at most once, or its
		// tasks would be appended twice.
		if file == "" || seen[file] {
			continue
		}
		seen[file] = true
		merged, err := mergeOverlayFile(data, file, baseDir)
		if err != nil {
			return nil, err
		}
		data = merged
	}
	return data, nil
}

// MergeDefaultOverlay merges "<scanDir>/main.yml" (or ".yaml") onto data
// if present — the "default" fallback used when no more specific chained
// file exists for a given hosts:/variables: directory or included
// package.
func MergeDefaultOverlay(data any, scanDir, baseDir string) (any, error) {
	file := findChainFile(scanDir, "main")
	if file == "" {
		return data, nil
	}
	return mergeOverlayFile(data, file, baseDir)
}

func mergeOverlayFile(data any, file, baseDir string) (any, error) {
	baseMap, ok := data.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("document must use the explicit 'tasks:'/'vars:' mapping form to merge overlay %s", file)
	}
	overlay, err := LoadFile(file, baseDir)
	if err != nil {
		return nil, err
	}
	overlayMap, ok := overlay.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("overlay file %s must use the explicit 'tasks:'/'vars:' mapping form", file)
	}
	return MergeDocuments(baseMap, overlayMap), nil
}
