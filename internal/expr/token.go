// Package expr implements the small Ansible/Jinja-flavored expression
// language shared by 'when' conditions and '${{ ... }}' templates — a
// direct port of modules/Expressions.psm1. One grammar/AST backs both
// consumers (internal/template wraps this package) so they can't drift
// apart, exactly as in the PowerShell implementation.
//
// Grammar (lowest to highest precedence):
//
//	expr        := or_expr
//	or_expr     := and_expr ("or" and_expr)*
//	and_expr    := not_expr ("and" not_expr)*
//	not_expr    := "not" not_expr | comparison
//	comparison  := membership (("==" | "!=" | "<" | "<=" | ">" | ">=") membership)?
//	membership  := pipeline (("in" | "not" "in") primary | ("is" | "is not") IDENT)?
//	pipeline    := primary ("|" IDENT ("(" (expr ("," expr)*)? ")")?)*
//	primary     := STRING | NUMBER | "true" | "false" | "null"
//	             | "[" (expr ("," expr)*)? "]"
//	             | IDENT "(" (expr ("," expr)*)? ")"          (bare filter call)
//	             | IDENT (("." IDENT) | ("[" NUMBER "]"))*    (dotted/indexed path)
//	             | "(" expr ")"
package expr

import "fmt"

type tokenType int

const (
	tokEOF tokenType = iota
	tokLParen
	tokRParen
	tokLBracket
	tokRBracket
	tokComma
	tokDot
	tokPipe
	tokEq
	tokNe
	tokLt
	tokLe
	tokGt
	tokGe
	tokAnd
	tokOr
	tokNot
	tokIn
	tokIs
	tokTrue
	tokFalse
	tokNull
	tokString
	tokNumber
	tokIdent
)

type token struct {
	typ   tokenType
	str   string
	num   float64
	boolV bool
}

// SyntaxError reports a lexer/parser failure against the original
// expression text, mirroring the PowerShell implementation's plain
// `throw "..."` messages closely enough to be recognizable while adding
// the position.
type SyntaxError struct {
	Expression string
	Message    string
}

func (e *SyntaxError) Error() string {
	return fmt.Sprintf("expression %q: %s", e.Expression, e.Message)
}
