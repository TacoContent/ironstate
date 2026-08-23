package expr

// Node is a parsed expression AST node. The set is closed (sealed via the
// unexported marker method) — a stronger, compiler-checked representation
// than the PowerShell implementation's hashtable AST, per
// docs/plans/go-rewrite.md §4.3.
type Node interface{ isNode() }

type OrNode struct{ Left, Right Node }
type AndNode struct{ Left, Right Node }
type NotNode struct{ Operand Node }

type CompareNode struct {
	Op          string // == != < <= > >=
	Left, Right Node
}

type InNode struct {
	Negate      bool
	Left, Right Node
}

type IsNode struct {
	Negate bool
	Left   Node
	Test   string // mapping/map, boolean/bool, string, number, list, defined, none/null
}

// FilterNode represents one '| name(args)' pipeline step applied to Target.
type FilterNode struct {
	Target Node
	Name   string
	Args   []Node
}

// CallNode is a bare 'name(args)' filter call with no piped value.
type CallNode struct {
	Name string
	Args []Node
}

type ListNode struct{ Items []Node }

// LiteralNode holds a string/number/bool/nil literal. A string value may
// itself contain '${{ ... }}' spans, expanded against the same context at
// eval time (see evalStringLiteral).
type LiteralNode struct{ Value any }

// PathSegment is one dotted ('.name') or indexed ('[N]') step of a
// PathNode; exactly one of Key/Index applies, per IsIndex.
type PathSegment struct {
	Key     string
	Index   int
	IsIndex bool
}

// PathNode is a dotted/indexed variable path, e.g. 'a.b[0].c'.
type PathNode struct{ Segments []PathSegment }

func (*OrNode) isNode()      {}
func (*AndNode) isNode()     {}
func (*NotNode) isNode()     {}
func (*CompareNode) isNode() {}
func (*InNode) isNode()      {}
func (*IsNode) isNode()      {}
func (*FilterNode) isNode()  {}
func (*CallNode) isNode()    {}
func (*ListNode) isNode()    {}
func (*LiteralNode) isNode() {}
func (*PathNode) isNode()    {}
