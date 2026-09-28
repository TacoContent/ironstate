package ui

import (
	"strings"
	"unicode/utf8"

	"github.com/briandowns/spinner"
)

// PadSpinnerSuffix installs a PreUpdate hook on s that pads each frame's
// Suffix with trailing spaces out to the width of the last frame actually
// drawn. briandowns/spinner redraws under Windows Terminal with a bare '\r'
// (no end-of-line erase), so a shorter Suffix would otherwise leave the tail
// of a longer one visible. Padding against the last *drawn* frame (not the
// last one set) matters: several Suffix updates can land within one frame.
// Callers set s.Suffix (under s.Lock) as usual; trailing spaces are reserved
// for this padding. Replaces any existing PreUpdate.
func PadSpinnerSuffix(s *spinner.Spinner) {
	drawnLen := 0
	s.PreUpdate = func(s *spinner.Spinner) {
		suffix := strings.TrimRight(s.Suffix, " ")
		length := utf8.RuneCountInString(suffix)
		if pad := drawnLen - length; pad > 0 {
			suffix += strings.Repeat(" ", pad)
		}
		s.Suffix = suffix
		drawnLen = length
	}
}
