package shellvalidate

// NodeKind identifies a shell syntax node.
type NodeKind string

const (
	NodeFile        NodeKind = "file"
	NodeList        NodeKind = "list"
	NodePipeline    NodeKind = "pipeline"
	NodeCommand     NodeKind = "command"
	NodeAssignment  NodeKind = "assignment"
	NodeRedirection NodeKind = "redirection"
	NodeFunction    NodeKind = "function"
	NodeBraceGroup  NodeKind = "brace-group"
	NodeSubshell    NodeKind = "subshell"
	NodeIf          NodeKind = "if"
	NodeFor         NodeKind = "for"
	NodeWhile       NodeKind = "while"
	NodeUntil       NodeKind = "until"
	NodeCase        NodeKind = "case"
	NodeArithmetic  NodeKind = "arithmetic"
	NodeConditional NodeKind = "conditional"
	NodeCoprocess   NodeKind = "coprocess"
)

// QuoteKind identifies the quoting applied to a word part.
type QuoteKind uint8

const (
	QuoteUnquoted QuoteKind = iota
	QuoteSingle
	QuoteDouble
	QuoteANSIC
	QuoteLocale
)

// WordPartKind identifies the semantic form of a word fragment.
type WordPartKind uint8

const (
	WordLiteral WordPartKind = iota
	WordParameterExpansion
	WordCommandSubstitution
	WordArithmeticExpansion
	WordProcessSubstitution
)

// WordPart is an immutable snapshot of one word fragment.
type WordPart struct {
	kind  WordPartKind
	quote QuoteKind
	text  []byte
	span  Span
}

// Kind returns the fragment kind.
func (part WordPart) Kind() WordPartKind { return part.kind }

// Quote returns the fragment quote context.
func (part WordPart) Quote() QuoteKind { return part.quote }

// Text returns a copy of the fragment's original bytes.
func (part WordPart) Text() []byte { return append([]byte(nil), part.text...) }

// Span returns the fragment source range.
func (part WordPart) Span() Span { return part.span }

// Word is an immutable shell word.
type Word struct {
	parts []WordPart
	span  Span
}

// Parts returns independent word-part snapshots.
func (word Word) Parts() []WordPart { return append([]WordPart(nil), word.parts...) }

// Span returns the word source range.
func (word Word) Span() Span { return word.span }

// Node is an immutable shell syntax node.
type Node struct {
	kind        NodeKind
	span        Span
	words       []Word
	children    []Node
	expressions []Expression
	incomplete  bool
}

// Kind returns the node kind.
func (node Node) Kind() NodeKind { return node.kind }

// Span returns the node source range.
func (node Node) Span() Span { return node.span }

// Words returns independent word snapshots.
func (node Node) Words() []Word { return append([]Word(nil), node.words...) }

// Children returns independent child-node snapshots.
func (node Node) Children() []Node { return append([]Node(nil), node.children...) }

// Expressions returns arithmetic or conditional expression snapshots.
func (node Node) Expressions() []Expression {
	return append([]Expression(nil), node.expressions...)
}

// Incomplete reports whether recovery produced this partial node.
func (node Node) Incomplete() bool { return node.incomplete }

// ExpressionKind identifies an arithmetic or conditional expression form.
type ExpressionKind uint8

const (
	ExpressionLiteral ExpressionKind = iota
	ExpressionName
	ExpressionUnary
	ExpressionBinary
	ExpressionAssignment
	ExpressionConditional
)

// Expression is an immutable arithmetic or conditional expression.
type Expression struct {
	kind     ExpressionKind
	operator string
	value    string
	span     Span
	children []Expression
}

// Kind returns the expression form.
func (expression Expression) Kind() ExpressionKind { return expression.kind }

// Operator returns the expression operator, if any.
func (expression Expression) Operator() string { return expression.operator }

// Value returns a literal or variable spelling, if any.
func (expression Expression) Value() string { return expression.value }

// Span returns the expression source range.
func (expression Expression) Span() Span { return expression.span }

// Children returns independent operand snapshots.
func (expression Expression) Children() []Expression {
	return append([]Expression(nil), expression.children...)
}

// Comment describes a preserved source comment.
type Comment struct {
	text []byte
	span Span
}

// Text returns a copy of the comment bytes.
func (comment Comment) Text() []byte { return append([]byte(nil), comment.text...) }

// Span returns the comment source range.
func (comment Comment) Span() Span { return comment.span }

// File is an immutable parsed shell file.
type File struct {
	filename    string
	source      []byte
	dialect     Dialect
	interpreter string
	nodes       []Node
	comments    []Comment
	tokens      []token
	syntaxValid bool
}

// Filename returns the diagnostic filename.
func (file *File) Filename() string {
	if file == nil {
		return ""
	}
	return file.filename
}

// Source returns a copy of the parsed source.
func (file *File) Source() []byte {
	if file == nil {
		return nil
	}
	return append([]byte(nil), file.source...)
}

// Dialect returns the effective parse dialect.
func (file *File) Dialect() Dialect {
	if file == nil {
		return DialectAuto
	}
	return file.dialect
}

// Interpreter returns the first-line interpreter directive, if present.
func (file *File) Interpreter() string {
	if file == nil {
		return ""
	}
	return file.interpreter
}

// Nodes returns independent top-level syntax-node snapshots.
func (file *File) Nodes() []Node {
	if file == nil {
		return nil
	}
	return append([]Node(nil), file.nodes...)
}

// Comments returns independent comment snapshots.
func (file *File) Comments() []Comment {
	if file == nil {
		return nil
	}
	return append([]Comment(nil), file.comments...)
}
