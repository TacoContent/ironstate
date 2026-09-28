package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/briandowns/spinner"
)

func frame(s *spinner.Spinner, suffix string) {
	if suffix != "" {
		s.Suffix = suffix
	}
	s.PreUpdate(s)
}

func TestPadSpinnerSuffixPadsShorterFollowUp(t *testing.T) {
	s := spinner.New([]string{"-"}, time.Millisecond)
	PadSpinnerSuffix(s)

	frame(s, " a much longer status message")
	frame(s, " short")

	want := " short" + strings.Repeat(" ", len(" a much longer status message")-len(" short"))
	if s.Suffix != want {
		t.Fatalf("Suffix = %q, want %q", s.Suffix, want)
	}
}

// Several Suffix updates between two frames must still pad against the
// last frame actually drawn, not the intermediate (never drawn) one.
func TestPadSpinnerSuffixPadsAgainstLastDrawnFrame(t *testing.T) {
	s := spinner.New([]string{"-"}, time.Millisecond)
	PadSpinnerSuffix(s)

	frame(s, " a much longer status message")
	s.Suffix = " medium message"
	frame(s, " short")

	want := " short" + strings.Repeat(" ", len(" a much longer status message")-len(" short"))
	if s.Suffix != want {
		t.Fatalf("Suffix = %q, want %q", s.Suffix, want)
	}
}

// An unchanged Suffix (still carrying last frame's padding) must shrink back
// to its real width once the longer frame has been overwritten.
func TestPadSpinnerSuffixDropsPaddingOnceCleared(t *testing.T) {
	s := spinner.New([]string{"-"}, time.Millisecond)
	PadSpinnerSuffix(s)

	frame(s, " a much longer status message")
	frame(s, " short")
	frame(s, "")

	if s.Suffix != " short" {
		t.Fatalf("Suffix = %q, want %q", s.Suffix, " short")
	}
}
