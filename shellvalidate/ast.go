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
	NodeTime        NodeKind = "time"
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

// NodeRole identifies a node's structural purpose within its parent.
type NodeRole string

const (
	RoleNone            NodeRole = ""
	RoleListElement     NodeRole = "list-element"
	RolePipelineCommand NodeRole = "pipeline-command"
	RoleCondition       NodeRole = "condition"
	RoleBody            NodeRole = "body"
	RoleAlternate       NodeRole = "alternate"
	RoleCaseArm         NodeRole = "case-arm"
	RoleFunctionBody    NodeRole = "function-body"
)

// HereDocument is an immutable here-document snapshot.
type HereDocument struct {
	delimiter      Word
	body           []byte
	bodySpan       Span
	terminatorSpan Span
	quoted         bool
	stripTabs      bool
}

// Delimiter returns the word whose quote-removed value terminates the document.
func (document HereDocument) Delimiter() Word { return cloneWord(document.delimiter) }

// Body returns a copy of the original here-document body bytes.
func (document HereDocument) Body() []byte { return append([]byte(nil), document.body...) }

// BodySpan returns the original body range, excluding the terminator line.
func (document HereDocument) BodySpan() Span { return document.bodySpan }

// TerminatorSpan returns the range of the terminating delimiter spelling.
func (document HereDocument) TerminatorSpan() Span { return document.terminatorSpan }

// Quoted reports whether any part of the delimiter word was quoted.
func (document HereDocument) Quoted() bool { return document.quoted }

// StripTabs reports whether the redirection uses the <<- form.
func (document HereDocument) StripTabs() bool { return document.stripTabs }

// Redirection is an immutable redirection attached to one command.
type Redirection struct {
	ioNumber     int
	hasIONumber  bool
	operator     string
	target       Word
	span         Span
	hereDocument *HereDocument
}

// IONumber returns the explicit I/O number, when present.
func (redirection Redirection) IONumber() (int, bool) {
	return redirection.ioNumber, redirection.hasIONumber
}

// Operator returns the redirection operator spelling.
func (redirection Redirection) Operator() string { return redirection.operator }

// Target returns the redirection operand.
func (redirection Redirection) Target() Word { return cloneWord(redirection.target) }

// Span returns the redirection source range through its operand.
func (redirection Redirection) Span() Span { return redirection.span }

// HereDocument returns an independent snapshot for << and <<-, or nil.
func (redirection Redirection) HereDocument() *HereDocument {
	if redirection.hereDocument == nil {
		return nil
	}
	result := cloneHereDocument(*redirection.hereDocument)
	return &result
}

// Node is an immutable shell syntax node.
type Node struct {
	kind         NodeKind
	role         NodeRole
	operator     string
	span         Span
	words        []Word
	assignments  []Word
	redirections []Redirection
	children     []Node
	expressions  []Expression
	incomplete   bool
}

// Kind returns the node kind.
func (node Node) Kind() NodeKind { return node.kind }

// Role returns the node's structural purpose within its parent.
func (node Node) Role() NodeRole { return node.role }

// Operator returns the list or pipeline operator represented by the node.
func (node Node) Operator() string { return node.operator }

// Span returns the node source range.
func (node Node) Span() Span { return node.span }

// Words returns independent word snapshots.
func (node Node) Words() []Word { return cloneWords(node.words) }

// Assignments returns assignment words attached to the command.
func (node Node) Assignments() []Word { return cloneWords(node.assignments) }

// Redirections returns independent redirection snapshots in source order.
func (node Node) Redirections() []Redirection {
	result := make([]Redirection, len(node.redirections))
	for index, redirection := range node.redirections {
		result[index] = cloneRedirection(redirection)
	}
	return result
}

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

func cloneWord(word Word) Word {
	word.parts = append([]WordPart(nil), word.parts...)
	for index := range word.parts {
		word.parts[index].text = append([]byte(nil), word.parts[index].text...)
	}
	return word
}

func cloneWords(words []Word) []Word {
	result := make([]Word, len(words))
	for index, word := range words {
		result[index] = cloneWord(word)
	}
	return result
}

func cloneHereDocument(document HereDocument) HereDocument {
	document.delimiter = cloneWord(document.delimiter)
	document.body = append([]byte(nil), document.body...)
	return document
}

func cloneRedirection(redirection Redirection) Redirection {
	redirection.target = cloneWord(redirection.target)
	if redirection.hereDocument != nil {
		document := cloneHereDocument(*redirection.hereDocument)
		redirection.hereDocument = &document
	}
	return redirection
}
