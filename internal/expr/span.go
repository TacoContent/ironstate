package expr

import "strings"

// Span is one '${{ ... }}' occurrence found by ScanSpans.
type Span struct {
	Start      int // byte offset of '$'
	End        int // byte offset just past the closing '}}' (exclusive)
	Expression string
}

// positionedRune pairs a decoded rune with its byte offset in the
// original string, from ranging over the string directly.
type positionedRune struct {
	r      rune
	offset int
}

// ScanSpans finds every '${{ ... }}' occurrence in text, hand-rolled
// rather than regex so a filter argument's string literal can safely
// contain '}}' without falsely terminating the span — ports
// Get-TemplateExpressionSpans, tracking quote state with the same
// backslash-escaping rule as the tokenizer. Shared by this package's own
// nested-string-literal expansion and internal/template's field-level
// substitution.
func ScanSpans(text string) []Span {
	var spans []Span

	// Built via range over the string directly (not []rune(text) plus a
	// derived byte-length table) so byte offsets stay correct even for
	// invalid UTF-8 input: range's own decoding already accounts for how
	// many bytes each rune (or invalid-byte replacement) actually consumed,
	// which utf8.RuneLen(r) cannot reconstruct after the fact for a
	// replacement rune.
	var chars []positionedRune
	for i, r := range text {
		chars = append(chars, positionedRune{r: r, offset: i})
	}
	n := len(chars)
	end := len(text)

	offsetAt := func(idx int) int {
		if idx >= n {
			return end
		}
		return chars[idx].offset
	}
	runeAt := func(idx int) rune { return chars[idx].r }

	i := 0
	for i < n {
		start := indexOfSpanStart(chars, i)
		if start < 0 {
			break
		}

		j := start + 3
		var quote rune
		closeIdx := -1
		for j < n {
			c := runeAt(j)
			if quote != 0 {
				if c == '\\' && j+1 < n {
					j += 2
					continue
				}
				if c == quote {
					quote = 0
				}
				j++
				continue
			}
			if c == '\'' || c == '"' {
				quote = c
				j++
				continue
			}
			if c == '}' && j+1 < n && runeAt(j+1) == '}' {
				closeIdx = j
				break
			}
			j++
		}

		if closeIdx < 0 {
			break // unterminated '${{' - leave the rest of the string untouched
		}

		innerStart := offsetAt(start + 3)
		innerEnd := offsetAt(closeIdx)
		spans = append(spans, Span{
			Start:      offsetAt(start),
			End:        offsetAt(closeIdx + 2),
			Expression: strings.TrimSpace(text[innerStart:innerEnd]),
		})
		i = closeIdx + 2
	}

	return spans
}

func indexOfSpanStart(chars []positionedRune, from int) int {
	for i := from; i+2 < len(chars); i++ {
		if chars[i].r == '$' && chars[i+1].r == '{' && chars[i+2].r == '{' {
			return i
		}
	}
	return -1
}
