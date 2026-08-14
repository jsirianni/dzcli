package shellvalidate

import (
	"reflect"
	"testing"
)

type snapshotFile struct {
	Dialect     Dialect
	Interpreter string
	Nodes       []snapshotNode
	Comments    []snapshotText
}

type snapshotNode struct {
	Kind         NodeKind
	Role         NodeRole
	Operator     string
	Span         [2]int
	Incomplete   bool
	Words        []snapshotWord
	Assignments  []snapshotWord
	Redirections []snapshotRedirection
	Children     []snapshotNode
	Expressions  []snapshotExpression
}

type snapshotWord struct {
	Span  [2]int
	Parts []snapshotPart
}

type snapshotPart struct {
	Kind  WordPartKind
	Quote QuoteKind
	Text  string
	Span  [2]int
}

type snapshotRedirection struct {
	IONumber    int
	HasIONumber bool
	Operator    string
	Target      snapshotWord
	Span        [2]int
	Here        *snapshotHereDocument
}

type snapshotHereDocument struct {
	Delimiter      snapshotWord
	Body           string
	BodySpan       [2]int
	TerminatorSpan [2]int
	Quoted         bool
	StripTabs      bool
}

type snapshotExpression struct {
	Kind     ExpressionKind
	Operator string
	Value    string
	Span     [2]int
	Children []snapshotExpression
}

type snapshotText struct {
	Text string
	Span [2]int
}

func snapshotAST(file *File) snapshotFile {
	result := snapshotFile{Dialect: file.Dialect(), Interpreter: file.Interpreter()}
	for _, node := range file.Nodes() {
		result.Nodes = append(result.Nodes, snapshotNodeValue(node))
	}
	for _, comment := range file.Comments() {
		result.Comments = append(result.Comments, snapshotText{Text: string(comment.Text()), Span: offsets(comment.Span())})
	}
	return result
}

func snapshotNodeValue(node Node) snapshotNode {
	result := snapshotNode{
		Kind: node.Kind(), Role: node.Role(), Operator: node.Operator(),
		Span: offsets(node.Span()), Incomplete: node.Incomplete(),
	}
	for _, word := range node.Words() {
		result.Words = append(result.Words, snapshotWordValue(word))
	}
	for _, assignment := range node.Assignments() {
		result.Assignments = append(result.Assignments, snapshotWordValue(assignment))
	}
	for _, redirection := range node.Redirections() {
		item := snapshotRedirection{Operator: redirection.Operator(), Target: snapshotWordValue(redirection.Target()), Span: offsets(redirection.Span())}
		item.IONumber, item.HasIONumber = redirection.IONumber()
		if document := redirection.HereDocument(); document != nil {
			item.Here = &snapshotHereDocument{
				Delimiter: snapshotWordValue(document.Delimiter()), Body: string(document.Body()),
				BodySpan: offsets(document.BodySpan()), TerminatorSpan: offsets(document.TerminatorSpan()),
				Quoted: document.Quoted(), StripTabs: document.StripTabs(),
			}
		}
		result.Redirections = append(result.Redirections, item)
	}
	for _, child := range node.Children() {
		result.Children = append(result.Children, snapshotNodeValue(child))
	}
	for _, expression := range node.Expressions() {
		result.Expressions = append(result.Expressions, snapshotExpressionValue(expression))
	}
	return result
}

func snapshotWordValue(word Word) snapshotWord {
	result := snapshotWord{Span: offsets(word.Span())}
	for _, part := range word.Parts() {
		result.Parts = append(result.Parts, snapshotPart{Kind: part.Kind(), Quote: part.Quote(), Text: string(part.Text()), Span: offsets(part.Span())})
	}
	return result
}

func snapshotExpressionValue(expression Expression) snapshotExpression {
	result := snapshotExpression{Kind: expression.Kind(), Operator: expression.Operator(), Value: expression.Value(), Span: offsets(expression.Span())}
	for _, child := range expression.Children() {
		result.Children = append(result.Children, snapshotExpressionValue(child))
	}
	return result
}

func offsets(span Span) [2]int { return [2]int{span.Start.Offset, span.End.Offset} }

func TestASTSnapshotContract(t *testing.T) {
	source := []byte("# heading\nname='value'; printf \"%s\\n\" \"$name\" 2>err | cat <<-'EOF'\n\tbody $name\n\tEOF\n")
	file, diagnostics, err := Parse("snapshot.sh", source, DialectPOSIX)
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("parse: %v %#v", err, diagnostics)
	}

	got := snapshotAST(file)
	literal := func(text string, quote QuoteKind, start, end int) snapshotPart {
		return snapshotPart{Kind: WordLiteral, Quote: quote, Text: text, Span: [2]int{start, end}}
	}
	word := func(start, end int, parts ...snapshotPart) snapshotWord {
		return snapshotWord{Span: [2]int{start, end}, Parts: parts}
	}
	assignment := word(10, 22,
		literal("name=", QuoteUnquoted, 10, 15),
		literal("value", QuoteSingle, 16, 21),
	)
	want := snapshotFile{
		Dialect: DialectPOSIX,
		Nodes: []snapshotNode{
			{
				Kind: NodeAssignment, Span: [2]int{10, 22},
				Words: []snapshotWord{assignment}, Assignments: []snapshotWord{assignment},
			},
			{
				Kind: NodePipeline, Operator: "|", Span: [2]int{24, 66},
				Children: []snapshotNode{
					{
						Kind: NodeCommand, Role: RolePipelineCommand, Span: [2]int{24, 51},
						Words: []snapshotWord{
							word(24, 30, literal("printf", QuoteUnquoted, 24, 30)),
							word(31, 37, literal("%s\\n", QuoteDouble, 32, 36)),
							word(38, 45, snapshotPart{Kind: WordParameterExpansion, Quote: QuoteDouble, Text: "$name", Span: [2]int{39, 44}}),
						},
						Redirections: []snapshotRedirection{{
							IONumber: 2, HasIONumber: true, Operator: ">", Span: [2]int{46, 51},
							Target: word(48, 51, literal("err", QuoteUnquoted, 48, 51)),
						}},
					},
					{
						Kind: NodeCommand, Role: RolePipelineCommand, Span: [2]int{54, 66},
						Words: []snapshotWord{word(54, 57, literal("cat", QuoteUnquoted, 54, 57))},
						Redirections: []snapshotRedirection{{
							Operator: "<<-", Span: [2]int{58, 66},
							Target: word(61, 66, literal("EOF", QuoteSingle, 62, 65)),
							Here: &snapshotHereDocument{
								Delimiter: word(61, 66, literal("EOF", QuoteSingle, 62, 65)),
								Body:      "\tbody $name\n", BodySpan: [2]int{67, 79}, TerminatorSpan: [2]int{80, 83},
								Quoted: true, StripTabs: true,
							},
						}},
					},
				},
			},
		},
		Comments: []snapshotText{{Text: "# heading", Span: [2]int{0, 9}}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("AST snapshot changed:\n%#v", got)
	}

	// Mutating every returned aggregate must not alter the immutable file.
	nodes := file.Nodes()
	nodes[0] = Node{}
	children := file.Nodes()[1].Children()
	children[0] = Node{}
	assignments := file.Nodes()[0].Assignments()
	assignments[0] = Word{}
	redirections := file.Nodes()[1].Children()[0].Redirections()
	redirections[0] = Redirection{}
	document := file.Nodes()[1].Children()[1].Redirections()[0].HereDocument()
	body := document.Body()
	body[0] = 'X'
	parts := file.Nodes()[1].Children()[0].Words()[2].Parts()
	parts[0] = WordPart{}
	if after := snapshotAST(file); !reflect.DeepEqual(after, want) {
		t.Fatalf("AST accessors exposed mutable storage:\n%#v", after)
	}
}
