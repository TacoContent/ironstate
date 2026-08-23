package templateengines

// Package templateengines implements the render engines the 'template'
// module (and blockinfile's 'template' field) can select via 'engine':
// 'jinja' (native port of modules/TemplateEngines/Jinja.psm1, reusing
// internal/expr's tokenizer/parser/evaluator) and 'gotemplate' (Go stdlib
// text/template, additive - see jinja.go/gotemplate.go and
// docs/plans/go-rewrite.md §4.7 for the build-vs-buy rationale). 'eps'/
// 'herestring' are dropped per §1/§11 (audited as unused in this repo).

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/TacoContent/ironstate/internal/expr"
)

type jinjaTokenKind int

const (
	jinjaTokText jinjaTokenKind = iota
	jinjaTokExpr
	jinjaTokTag
	jinjaTokEOF
)

type jinjaTag struct {
	Keyword string
	Text    string
	VarName string
}

type jinjaToken struct {
	Kind jinjaTokenKind
	Text string
	Tag  jinjaTag
}

var (
	jinjaIfPattern     = regexp.MustCompile(`(?s)^if\s+(.+)$`)
	jinjaElifPattern   = regexp.MustCompile(`(?s)^elif\s+(.+)$`)
	jinjaElsePattern   = regexp.MustCompile(`^else\s*$`)
	jinjaEndifPattern  = regexp.MustCompile(`^endif\s*$`)
	jinjaForPattern    = regexp.MustCompile(`(?s)^for\s+([A-Za-z_]\w*)\s+in\s+(.+)$`)
	jinjaEndforPattern = regexp.MustCompile(`^endfor\s*$`)
	jinjaSetPattern    = regexp.MustCompile(`(?s)^set\s+([A-Za-z_]\w*)\s*=\s*(.+)$`)
)

func parseJinjaTag(inner string) (jinjaTag, error) {
	if m := jinjaIfPattern.FindStringSubmatch(inner); m != nil {
		return jinjaTag{Keyword: "if", Text: m[1]}, nil
	}
	if m := jinjaElifPattern.FindStringSubmatch(inner); m != nil {
		return jinjaTag{Keyword: "elif", Text: m[1]}, nil
	}
	if jinjaElsePattern.MatchString(inner) {
		return jinjaTag{Keyword: "else"}, nil
	}
	if jinjaEndifPattern.MatchString(inner) {
		return jinjaTag{Keyword: "endif"}, nil
	}
	if m := jinjaForPattern.FindStringSubmatch(inner); m != nil {
		return jinjaTag{Keyword: "for", VarName: m[1], Text: m[2]}, nil
	}
	if jinjaEndforPattern.MatchString(inner) {
		return jinjaTag{Keyword: "endfor"}, nil
	}
	if m := jinjaSetPattern.FindStringSubmatch(inner); m != nil {
		return jinjaTag{Keyword: "set", VarName: m[1], Text: m[2]}, nil
	}
	return jinjaTag{}, fmt.Errorf("unknown or malformed '{%% %%}' tag: '%s'", inner)
}

// tokenizeJinja scans content for '{{ ... }}'/'{% ... %}' spans,
// quote-aware (a filter argument's string literal can safely contain
// '}}'/'%}') - ports Get-JinjaTemplateTokens. An unterminated tag leaves
// the remainder as plain text.
func tokenizeJinja(content string) ([]jinjaToken, error) {
	runes := []rune(content)
	n := len(runes)
	var tokens []jinjaToken
	i := 0
	textStart := 0

	for i < n {
		openExpr := indexOfRunes(runes, i, "{{")
		openTag := indexOfRunes(runes, i, "{%")
		if openExpr < 0 && openTag < 0 {
			break
		}

		var open int
		var closer string
		isTag := false
		if openTag < 0 || (openExpr >= 0 && openExpr < openTag) {
			open, closer = openExpr, "}}"
		} else {
			open, closer, isTag = openTag, "%}", true
		}

		j := open + 2
		var quote rune
		end := -1
		for j < n {
			c := runes[j]
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
			if c == rune(closer[0]) && j+1 < n && runes[j+1] == rune(closer[1]) {
				end = j
				break
			}
			j++
		}

		if end < 0 {
			break // unterminated tag - leave the rest of the content untouched
		}

		if open > textStart {
			tokens = append(tokens, jinjaToken{Kind: jinjaTokText, Text: string(runes[textStart:open])})
		}

		inner := strings.TrimSpace(string(runes[open+2 : end]))
		if isTag {
			tag, err := parseJinjaTag(inner)
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, jinjaToken{Kind: jinjaTokTag, Tag: tag})
		} else {
			tokens = append(tokens, jinjaToken{Kind: jinjaTokExpr, Text: inner})
		}

		i = end + 2
		textStart = i
	}

	if textStart < n {
		tokens = append(tokens, jinjaToken{Kind: jinjaTokText, Text: string(runes[textStart:])})
	}
	tokens = append(tokens, jinjaToken{Kind: jinjaTokEOF})
	return tokens, nil
}

func indexOfRunes(runes []rune, from int, needle string) int {
	nr := []rune(needle)
	for i := from; i+len(nr) <= len(runes); i++ {
		match := true
		for k, r := range nr {
			if runes[i+k] != r {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

// --- node tree -----------------------------------------------------------

type jinjaNode interface{ isJinjaNode() }

type jinjaTextNode struct{ Value string }
type jinjaExprNode struct{ Ast expr.Node }
type jinjaSetNode struct {
	VarName  string
	ValueAst expr.Node
}
type jinjaIfBranch struct {
	ConditionAst expr.Node // nil for the trailing 'else'
	Body         []jinjaNode
}
type jinjaIfNode struct{ Branches []jinjaIfBranch }
type jinjaForNode struct {
	VarName     string
	IterableAst expr.Node
	Body        []jinjaNode
}

func (*jinjaTextNode) isJinjaNode() {}
func (*jinjaExprNode) isJinjaNode() {}
func (*jinjaSetNode) isJinjaNode()  {}
func (*jinjaIfNode) isJinjaNode()   {}
func (*jinjaForNode) isJinjaNode()  {}

// --- parser (recursive descent over the token list) -----------------------

type jinjaParser struct {
	tokens []jinjaToken
	pos    int
}

func (p *jinjaParser) current() jinjaToken { return p.tokens[p.pos] }
func (p *jinjaParser) advance()            { p.pos++ }

func (p *jinjaParser) isTagKeyword(keyword string) bool {
	t := p.current()
	return t.Kind == jinjaTokTag && t.Tag.Keyword == keyword
}

func parseJinjaNodeList(p *jinjaParser, stopKeywords map[string]bool) ([]jinjaNode, error) {
	var nodes []jinjaNode
	for {
		tok := p.current()
		switch tok.Kind {
		case jinjaTokEOF:
			return nodes, nil
		case jinjaTokText:
			nodes = append(nodes, &jinjaTextNode{Value: tok.Text})
			p.advance()
		case jinjaTokExpr:
			ast, err := expr.Parse(tok.Text)
			if err != nil {
				return nil, err
			}
			nodes = append(nodes, &jinjaExprNode{Ast: ast})
			p.advance()
		case jinjaTokTag:
			if stopKeywords[tok.Tag.Keyword] {
				return nodes, nil
			}
			switch tok.Tag.Keyword {
			case "if":
				n, err := parseJinjaIf(p)
				if err != nil {
					return nil, err
				}
				nodes = append(nodes, n)
			case "for":
				n, err := parseJinjaFor(p)
				if err != nil {
					return nil, err
				}
				nodes = append(nodes, n)
			case "set":
				ast, err := expr.Parse(tok.Tag.Text)
				if err != nil {
					return nil, err
				}
				nodes = append(nodes, &jinjaSetNode{VarName: tok.Tag.VarName, ValueAst: ast})
				p.advance()
			default:
				return nil, fmt.Errorf("unexpected '{%% %s %%}' tag with no matching opening tag", tok.Tag.Keyword)
			}
		default:
			return nil, fmt.Errorf("unexpected token in template")
		}
	}
}

func parseJinjaIf(p *jinjaParser) (*jinjaIfNode, error) {
	ifTok := p.current()
	p.advance()

	condAst, err := expr.Parse(ifTok.Tag.Text)
	if err != nil {
		return nil, err
	}
	body, err := parseJinjaNodeList(p, map[string]bool{"elif": true, "else": true, "endif": true})
	if err != nil {
		return nil, err
	}
	node := &jinjaIfNode{Branches: []jinjaIfBranch{{ConditionAst: condAst, Body: body}}}

	for p.isTagKeyword("elif") {
		elifTok := p.current()
		p.advance()
		elifAst, err := expr.Parse(elifTok.Tag.Text)
		if err != nil {
			return nil, err
		}
		elifBody, err := parseJinjaNodeList(p, map[string]bool{"elif": true, "else": true, "endif": true})
		if err != nil {
			return nil, err
		}
		node.Branches = append(node.Branches, jinjaIfBranch{ConditionAst: elifAst, Body: elifBody})
	}

	if p.isTagKeyword("else") {
		p.advance()
		elseBody, err := parseJinjaNodeList(p, map[string]bool{"endif": true})
		if err != nil {
			return nil, err
		}
		node.Branches = append(node.Branches, jinjaIfBranch{ConditionAst: nil, Body: elseBody})
	}

	if !p.isTagKeyword("endif") {
		return nil, fmt.Errorf("missing '{%% endif %%}' for '{%% if %s %%}'", ifTok.Tag.Text)
	}
	p.advance()
	return node, nil
}

func parseJinjaFor(p *jinjaParser) (*jinjaForNode, error) {
	forTok := p.current()
	p.advance()
	body, err := parseJinjaNodeList(p, map[string]bool{"endfor": true})
	if err != nil {
		return nil, err
	}
	if !p.isTagKeyword("endfor") {
		return nil, fmt.Errorf("missing '{%% endfor %%}' for '{%% for %s in %s %%}'", forTok.Tag.VarName, forTok.Tag.Text)
	}
	p.advance()

	iterAst, err := expr.Parse(forTok.Tag.Text)
	if err != nil {
		return nil, err
	}
	return &jinjaForNode{VarName: forTok.Tag.VarName, IterableAst: iterAst, Body: body}, nil
}

func parseJinjaTokens(tokens []jinjaToken) ([]jinjaNode, error) {
	p := &jinjaParser{tokens: tokens}
	nodes, err := parseJinjaNodeList(p, nil)
	if err != nil {
		return nil, err
	}
	if tok := p.current(); tok.Kind != jinjaTokEOF {
		return nil, fmt.Errorf("unexpected '{%% %s %%}' tag with no matching opening tag", tok.Tag.Keyword)
	}
	return nodes, nil
}

// --- executor ---------------------------------------------------------------

// execJinjaNodeList clones ctx once into a local scope - 'set' mutates
// only that clone, and a 'for' iteration clones the scope again per
// value - so nothing here is ever visible to the caller's own context,
// matching Jinja's per-block scoping (ports Invoke-JinjaNodeList).
func execJinjaNodeList(nodes []jinjaNode, ctx map[string]any, filters expr.Filters) (string, error) {
	scope := make(map[string]any, len(ctx))
	for k, v := range ctx {
		scope[k] = v
	}

	var sb strings.Builder
	for _, node := range nodes {
		switch n := node.(type) {
		case *jinjaTextNode:
			sb.WriteString(n.Value)
		case *jinjaExprNode:
			v, err := expr.Eval(n.Ast, scope, filters)
			if err != nil {
				return "", err
			}
			sb.WriteString(expr.DisplayString(v))
		case *jinjaSetNode:
			v, err := expr.Eval(n.ValueAst, scope, filters)
			if err != nil {
				return "", err
			}
			scope[n.VarName] = v
		case *jinjaIfNode:
			for _, branch := range n.Branches {
				take := branch.ConditionAst == nil
				if !take {
					v, err := expr.Eval(branch.ConditionAst, scope, filters)
					if err != nil {
						return "", err
					}
					take = expr.Truthy(v)
				}
				if take {
					rendered, err := execJinjaNodeList(branch.Body, scope, filters)
					if err != nil {
						return "", err
					}
					sb.WriteString(rendered)
					break
				}
			}
		case *jinjaForNode:
			iterable, err := expr.Eval(n.IterableAst, scope, filters)
			if err != nil {
				return "", err
			}
			for _, value := range asAnyList(iterable) {
				child := make(map[string]any, len(scope)+1)
				for k, v := range scope {
					child[k] = v
				}
				child[n.VarName] = value
				rendered, err := execJinjaNodeList(n.Body, child, filters)
				if err != nil {
					return "", err
				}
				sb.WriteString(rendered)
			}
		default:
			return "", fmt.Errorf("cannot render node of unknown type")
		}
	}
	return sb.String(), nil
}

func asAnyList(v any) []any {
	switch val := v.(type) {
	case nil:
		return nil
	case []any:
		return val
	default:
		return []any{val}
	}
}

// RenderJinja ports Render-JinjaTemplate: tokenizes, parses, and executes
// content against ctx (facts/vars/id-registry, plus this task's own
// 'vars', already resolved by the caller - see the 'template' handler).
func RenderJinja(content string, ctx map[string]any, filters expr.Filters) (string, error) {
	tokens, err := tokenizeJinja(content)
	if err != nil {
		return "", err
	}
	nodes, err := parseJinjaTokens(tokens)
	if err != nil {
		return "", err
	}
	return execJinjaNodeList(nodes, ctx, filters)
}
