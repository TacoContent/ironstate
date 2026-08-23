package expr

import "testing"

// FuzzParse feeds arbitrary strings through Parse — cheap given the
// grammar's small size (docs/plans/go-rewrite.md §4.3/§6). Parse must
// never panic; a syntax error is an acceptable, expected outcome.
func FuzzParse(f *testing.F) {
	seeds := []string{
		`a == "b"`,
		`a.b[0].c`,
		`"hello ${{ name }}"`,
		`value | default("x") | upper`,
		`lookup("url", "${{ x }}")`,
		`not (a or b) and c is defined`,
		`[1, "two", true, null]`,
		`"unterminated`,
		`${{`,
		`a is not mapping`,
		`"esc\\aped\"string"`,
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("Parse(%q) panicked: %v", s, r)
			}
		}()
		_, _ = Parse(s)
	})
}

// FuzzScanSpans feeds arbitrary strings through ScanSpans — must never
// panic, and every returned span's Start/End must be valid slice bounds
// into the original string.
func FuzzScanSpans(f *testing.F) {
	seeds := []string{
		`${{ a }}`,
		`${{ lookup("x", "}}") }}`,
		`${{ unterminated`,
		`no spans here`,
		`${{}}${{}}`,
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("ScanSpans(%q) panicked: %v", s, r)
			}
		}()
		for _, span := range ScanSpans(s) {
			if span.Start < 0 || span.End > len(s) || span.Start > span.End {
				t.Fatalf("ScanSpans(%q) produced out-of-bounds span %#v", s, span)
			}
		}
	})
}
