package engine

import "github.com/TacoContent/ironstate/internal/tasks"

// TagsMatch ports Common.psm1's Test-TagsMatch: no filter means "include
// everything"; a leaf with no effective tags at all (neither its own nor
// any ancestor's) can never be deliberately targeted OR excluded by any
// tag filter, so it's always included rather than always excluded - 'when'
// remains the only gate such a leaf needs (e.g. a tagless prerequisite
// 'fact' another task's 'when' depends on).
func TagsMatch(tags []string, filter []string) bool {
	if len(filter) == 0 {
		return true
	}
	if len(tags) == 0 {
		return true
	}
	for _, want := range filter {
		for _, have := range tags {
			if have == want {
				return true
			}
		}
	}
	return false
}

// FilterByTags returns only the leaves whose Tags match filter, preserving
// order - ports ironstate.ps1's '$filteredLeaves' pipeline.
func FilterByTags(leaves []tasks.Leaf, filter []string) []tasks.Leaf {
	if len(filter) == 0 {
		return leaves
	}
	out := make([]tasks.Leaf, 0, len(leaves))
	for _, l := range leaves {
		if TagsMatch(l.Tags, filter) {
			out = append(out, l)
		}
	}
	return out
}
