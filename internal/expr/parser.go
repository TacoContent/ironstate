package expr

// Parse tokenizes and parses expression into an AST, mirroring
// Expressions.psm1's Read-Expression (ConvertTo-ExpressionTokens +
// recursive-descent parse over the grammar documented in the package
// doc comment).
func Parse(expression string) (Node, error) {
	tokens, err := tokenize(expression)
	if err != nil {
		return nil, err
	}
	p := &parser{tokens: tokens, expression: expression}
	node, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if p.current().typ != tokEOF {
		return nil, &SyntaxError{Expression: expression, Message: "unexpected trailing tokens"}
	}
	return node, nil
}

type parser struct {
	tokens     []token
	pos        int
	expression string
}

func (p *parser) current() token { return p.tokens[p.pos] }
func (p *parser) advance()       { p.pos++ }

func (p *parser) errf(msg string) error {
	return &SyntaxError{Expression: p.expression, Message: msg}
}

func (p *parser) parseExpr() (Node, error) { return p.parseOr() }

func (p *parser) parseOr() (Node, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.current().typ == tokOr {
		p.advance()
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = &OrNode{Left: left, Right: right}
	}
	return left, nil
}

func (p *parser) parseAnd() (Node, error) {
	left, err := p.parseNot()
	if err != nil {
		return nil, err
	}
	for p.current().typ == tokAnd {
		p.advance()
		right, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		left = &AndNode{Left: left, Right: right}
	}
	return left, nil
}

func (p *parser) parseNot() (Node, error) {
	if p.current().typ == tokNot {
		p.advance()
		operand, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		return &NotNode{Operand: operand}, nil
	}
	return p.parseComparison()
}

var compareOps = map[tokenType]string{
	tokEq: "==", tokNe: "!=", tokLt: "<", tokLe: "<=", tokGt: ">", tokGe: ">=",
}

func (p *parser) parseComparison() (Node, error) {
	left, err := p.parseMembership()
	if err != nil {
		return nil, err
	}
	if op, ok := compareOps[p.current().typ]; ok {
		p.advance()
		right, err := p.parseMembership()
		if err != nil {
			return nil, err
		}
		return &CompareNode{Op: op, Left: left, Right: right}, nil
	}
	return left, nil
}

func (p *parser) parseMembership() (Node, error) {
	left, err := p.parsePipeline()
	if err != nil {
		return nil, err
	}

	switch p.current().typ {
	case tokIn:
		p.advance()
		right, err := p.parsePrimary()
		if err != nil {
			return nil, err
		}
		return &InNode{Negate: false, Left: left, Right: right}, nil
	case tokNot:
		saved := p.pos
		p.advance()
		if p.current().typ == tokIn {
			p.advance()
			right, err := p.parsePrimary()
			if err != nil {
				return nil, err
			}
			return &InNode{Negate: true, Left: left, Right: right}, nil
		}
		p.pos = saved
	case tokIs:
		p.advance()
		negate := false
		if p.current().typ == tokNot {
			p.advance()
			negate = true
		}
		testTok := p.current()
		if testTok.typ != tokIdent {
			return nil, p.errf("expected a test name after 'is'")
		}
		p.advance()
		return &IsNode{Negate: negate, Left: left, Test: testTok.str}, nil
	}

	return left, nil
}

func (p *parser) parseCallArguments() ([]Node, error) {
	p.advance() // consume '('
	var args []Node
	if p.current().typ != tokRParen {
		arg, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		args = append(args, arg)
		for p.current().typ == tokComma {
			p.advance()
			arg, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			args = append(args, arg)
		}
	}
	if p.current().typ != tokRParen {
		return nil, p.errf("expected ')' after call arguments")
	}
	p.advance()
	return args, nil
}

func (p *parser) parsePipeline() (Node, error) {
	left, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}
	for p.current().typ == tokPipe {
		p.advance()
		nameTok := p.current()
		if nameTok.typ != tokIdent {
			return nil, p.errf("expected filter name after '|'")
		}
		p.advance()
		var args []Node
		if p.current().typ == tokLParen {
			args, err = p.parseCallArguments()
			if err != nil {
				return nil, err
			}
		}
		left = &FilterNode{Target: left, Name: nameTok.str, Args: args}
	}
	return left, nil
}

func (p *parser) parsePrimary() (Node, error) {
	tok := p.current()

	switch tok.typ {
	case tokString:
		p.advance()
		return &LiteralNode{Value: tok.str}, nil
	case tokNumber:
		p.advance()
		return &LiteralNode{Value: tok.num}, nil
	case tokTrue, tokFalse:
		p.advance()
		return &LiteralNode{Value: tok.boolV}, nil
	case tokNull:
		p.advance()
		return &LiteralNode{Value: nil}, nil
	case tokLBracket:
		p.advance()
		var items []Node
		if p.current().typ != tokRBracket {
			item, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			items = append(items, item)
			for p.current().typ == tokComma {
				p.advance()
				item, err := p.parseExpr()
				if err != nil {
					return nil, err
				}
				items = append(items, item)
			}
		}
		if p.current().typ != tokRBracket {
			return nil, p.errf("expected ']'")
		}
		p.advance()
		return &ListNode{Items: items}, nil
	case tokLParen:
		p.advance()
		inner, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		if p.current().typ != tokRParen {
			return nil, p.errf("expected ')'")
		}
		p.advance()
		return inner, nil
	case tokIdent:
		name := tok.str
		p.advance()
		if p.current().typ == tokLParen {
			args, err := p.parseCallArguments()
			if err != nil {
				return nil, err
			}
			return &CallNode{Name: name, Args: args}, nil
		}
		segments := []PathSegment{{Key: name}}
		for {
			switch p.current().typ {
			case tokDot:
				p.advance()
				idTok := p.current()
				if idTok.typ != tokIdent {
					return nil, p.errf("expected identifier after '.'")
				}
				p.advance()
				segments = append(segments, PathSegment{Key: idTok.str})
				continue
			case tokLBracket:
				p.advance()
				idxTok := p.current()
				if idxTok.typ != tokNumber {
					return nil, p.errf("expected number inside '[' ']'")
				}
				p.advance()
				if p.current().typ != tokRBracket {
					return nil, p.errf("expected ']'")
				}
				p.advance()
				segments = append(segments, PathSegment{Index: int(idxTok.num), IsIndex: true})
				continue
			}
			break
		}
		return &PathNode{Segments: segments}, nil
	}

	return nil, p.errf("unexpected token in expression")
}
