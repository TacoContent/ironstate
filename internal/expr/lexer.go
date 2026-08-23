package expr

import (
	"strconv"
	"strings"
)

// tokenize ports Expressions.psm1's ConvertTo-ExpressionTokens: same token
// set, same string-escape rules (\n \r \t, else literal next char), same
// number handling (digits + '.', optional leading '-', float64-backed).
func tokenize(expression string) ([]token, error) {
	var tokens []token
	runes := []rune(expression)
	n := len(runes)
	i := 0

	for i < n {
		c := runes[i]

		if isSpace(c) {
			i++
			continue
		}

		switch c {
		case '(':
			tokens = append(tokens, token{typ: tokLParen})
			i++
			continue
		case ')':
			tokens = append(tokens, token{typ: tokRParen})
			i++
			continue
		case '[':
			tokens = append(tokens, token{typ: tokLBracket})
			i++
			continue
		case ']':
			tokens = append(tokens, token{typ: tokRBracket})
			i++
			continue
		case ',':
			tokens = append(tokens, token{typ: tokComma})
			i++
			continue
		case '.':
			tokens = append(tokens, token{typ: tokDot})
			i++
			continue
		case '|':
			tokens = append(tokens, token{typ: tokPipe})
			i++
			continue
		}

		if c == '=' && i+1 < n && runes[i+1] == '=' {
			tokens = append(tokens, token{typ: tokEq})
			i += 2
			continue
		}
		if c == '!' && i+1 < n && runes[i+1] == '=' {
			tokens = append(tokens, token{typ: tokNe})
			i += 2
			continue
		}
		if c == '<' && i+1 < n && runes[i+1] == '=' {
			tokens = append(tokens, token{typ: tokLe})
			i += 2
			continue
		}
		if c == '>' && i+1 < n && runes[i+1] == '=' {
			tokens = append(tokens, token{typ: tokGe})
			i += 2
			continue
		}
		if c == '<' {
			tokens = append(tokens, token{typ: tokLt})
			i++
			continue
		}
		if c == '>' {
			tokens = append(tokens, token{typ: tokGt})
			i++
			continue
		}

		if c == '\'' || c == '"' {
			quote := c
			j := i + 1
			var sb strings.Builder
			for j < n && runes[j] != quote {
				if runes[j] == '\\' && j+1 < n {
					switch runes[j+1] {
					case 'n':
						sb.WriteRune('\n')
					case 'r':
						sb.WriteRune('\r')
					case 't':
						sb.WriteRune('\t')
					default:
						sb.WriteRune(runes[j+1])
					}
					j += 2
					continue
				}
				sb.WriteRune(runes[j])
				j++
			}
			if j >= n {
				return nil, &SyntaxError{Expression: expression, Message: "unterminated string literal"}
			}
			tokens = append(tokens, token{typ: tokString, str: sb.String()})
			i = j + 1
			continue
		}

		if isDigit(c) || (c == '-' && i+1 < n && isDigit(runes[i+1])) {
			j := i + 1
			for j < n && (isDigit(runes[j]) || runes[j] == '.') {
				j++
			}
			numText := string(runes[i:j])
			val, err := parseFloat(numText)
			if err != nil {
				return nil, &SyntaxError{Expression: expression, Message: "invalid number literal " + numText}
			}
			tokens = append(tokens, token{typ: tokNumber, num: val})
			i = j
			continue
		}

		if isIdentStart(c) {
			j := i
			for j < n && isIdentPart(runes[j]) {
				j++
			}
			word := string(runes[i:j])
			switch word {
			case "and":
				tokens = append(tokens, token{typ: tokAnd})
			case "or":
				tokens = append(tokens, token{typ: tokOr})
			case "not":
				tokens = append(tokens, token{typ: tokNot})
			case "in":
				tokens = append(tokens, token{typ: tokIn})
			case "is":
				tokens = append(tokens, token{typ: tokIs})
			case "true":
				tokens = append(tokens, token{typ: tokTrue, boolV: true})
			case "false":
				tokens = append(tokens, token{typ: tokFalse, boolV: false})
			case "null":
				tokens = append(tokens, token{typ: tokNull})
			default:
				tokens = append(tokens, token{typ: tokIdent, str: word})
			}
			i = j
			continue
		}

		return nil, &SyntaxError{Expression: expression, Message: "unexpected character '" + string(c) + "'"}
	}

	tokens = append(tokens, token{typ: tokEOF})
	return tokens, nil
}

func isSpace(c rune) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' }
func isDigit(c rune) bool { return c >= '0' && c <= '9' }
func isIdentStart(c rune) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || c == '_'
}
func isIdentPart(c rune) bool { return isIdentStart(c) || isDigit(c) }

func parseFloat(s string) (float64, error) {
	return strconv.ParseFloat(s, 64)
}
