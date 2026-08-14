package shellvalidate

import (
	"bytes"
	"testing"
)

func TestParserOwnsHereDocumentRecognition(t *testing.T) {
	t.Run("non-redirection shifts", func(t *testing.T) {
		cases := []struct {
			id      string
			source  string
			dialect Dialect
		}{
			{id: "arithmetic-command", source: "((value << 2))\n", dialect: DialectBash},
			{id: "arithmetic-expansion", source: "printf '%s' $((value << 2))\n", dialect: DialectPOSIX},
			{id: "comment", source: "printf x # <<NOT_A_DOCUMENT\nprintf y\n", dialect: DialectPOSIX},
		}
		for _, test := range cases {
			t.Run(test.id, func(t *testing.T) {
				file, diagnostics, err := Parse(test.id+".sh", []byte(test.source), test.dialect)
				if err != nil || file == nil || hasCode(diagnostics, "SHS1006") {
					t.Fatalf("parse: %v diagnostics=%#v", err, diagnostics)
				}
				for _, node := range file.Nodes() {
					for _, redirection := range node.Redirections() {
						if redirection.HereDocument() != nil {
							t.Fatalf("non-redirection enqueued here-document: %#v", redirection)
						}
					}
				}
			})
		}
	})

	t.Run("fifo metadata and lexical isolation", func(t *testing.T) {
		source := []byte("cat 3<<'ONE' 4<<-TWO\n'unclosed $literal\nONE\n\tsecond\n\tTWO\nprintf after\n")
		file, diagnostics, err := Parse("fifo.sh", source, DialectPOSIX)
		if err != nil || len(diagnostics) != 0 {
			t.Fatalf("parse: %v diagnostics=%#v", err, diagnostics)
		}
		nodes := file.Nodes()
		if len(nodes) != 2 || nodes[0].Kind() != NodeCommand || nodes[1].Kind() != NodeCommand {
			t.Fatalf("nodes=%#v", nodes)
		}
		redirections := nodes[0].Redirections()
		if len(redirections) != 2 {
			t.Fatalf("redirections=%#v", redirections)
		}
		first, second := redirections[0].HereDocument(), redirections[1].HereDocument()
		if first == nil || !first.Quoted() || first.StripTabs() || !bytes.Equal(first.Body(), []byte("'unclosed $literal\n")) {
			t.Fatalf("first=%#v", first)
		}
		if second == nil || second.Quoted() || !second.StripTabs() || !bytes.Equal(second.Body(), []byte("\tsecond\n")) {
			t.Fatalf("second=%#v", second)
		}
		if first.BodySpan().Start.Offset != 21 || first.TerminatorSpan().Start.Offset != 40 ||
			second.BodySpan().Start.Offset != 44 || second.TerminatorSpan().Start.Offset != 53 {
			t.Fatalf("spans: first body=%#v terminator=%#v second body=%#v terminator=%#v",
				first.BodySpan(), first.TerminatorSpan(), second.BodySpan(), second.TerminatorSpan())
		}
		if len(file.Comments()) != 0 {
			t.Fatalf("here-document body was tokenized as comments: %#v", file.Comments())
		}
	})

	t.Run("missing terminators remain fifo", func(t *testing.T) {
		source := []byte("cat <<ONE <<TWO\nbody\nONE\nsecond\n")
		file, diagnostics, err := Parse("missing.sh", source, DialectPOSIX)
		if err != nil || len(diagnostics) != 1 || diagnostics[0].Code != "SHS1006" || diagnostics[0].Primary.Start.Offset != 12 {
			t.Fatalf("parse: %v diagnostics=%#v", err, diagnostics)
		}
		redirections := file.Nodes()[0].Redirections()
		if len(redirections) != 2 || !bytes.Equal(redirections[0].HereDocument().Body(), []byte("body\n")) ||
			!bytes.Equal(redirections[1].HereDocument().Body(), []byte("second\n")) {
			t.Fatalf("redirections=%#v", redirections)
		}
	})
}

func TestForSelectAndTimeProductions(t *testing.T) {
	t.Run("for header words", func(t *testing.T) {
		file, diagnostics, err := Parse("for.sh", []byte("for item in one 'two words'; do printf '%s' \"$item\"; done\n"), DialectPOSIX)
		if err != nil || len(diagnostics) != 0 {
			t.Fatalf("parse: %v diagnostics=%#v", err, diagnostics)
		}
		node := file.Nodes()[0]
		if node.Kind() != NodeFor || node.Operator() != "for" || len(node.Words()) != 3 || len(node.Children()) != 1 || node.Children()[0].Role() != RoleBody {
			t.Fatalf("for node=%#v", node)
		}
	})

	t.Run("implicit positional list", func(t *testing.T) {
		file, diagnostics, err := Parse("for.sh", []byte("for item\ndo printf '%s' \"$item\"\ndone\n"), DialectPOSIX)
		if err != nil || len(diagnostics) != 0 || len(file.Nodes()[0].Words()) != 1 {
			t.Fatalf("parse: %v diagnostics=%#v nodes=%#v", err, diagnostics, file.Nodes())
		}
	})

	t.Run("select dialect contrast", func(t *testing.T) {
		source := []byte("select item in one two; do break; done\n")
		bash, bashDiagnostics, err := Parse("select.sh", source, DialectBash)
		if err != nil || len(bashDiagnostics) != 0 || bash.Nodes()[0].Operator() != "select" {
			t.Fatalf("bash: %v diagnostics=%#v nodes=%#v", err, bashDiagnostics, bash.Nodes())
		}
		_, posixDiagnostics, err := Parse("select.sh", source, DialectPOSIX)
		if err != nil || countCode(posixDiagnostics, "SHD1001") != 1 {
			t.Fatalf("posix: %v diagnostics=%#v", err, posixDiagnostics)
		}
	})

	t.Run("time forms", func(t *testing.T) {
		for _, test := range []struct {
			id      string
			source  string
			dialect Dialect
			wantD   int
		}{
			{id: "posix", source: "time -p left | right\n", dialect: DialectPOSIX},
			{id: "bash", source: "time -- left | right\n", dialect: DialectBash},
			{id: "contrast", source: "time -- left\n", dialect: DialectPOSIX, wantD: 1},
		} {
			t.Run(test.id, func(t *testing.T) {
				file, diagnostics, err := Parse(test.id+".sh", []byte(test.source), test.dialect)
				if err != nil || countCode(diagnostics, "SHD1001") != test.wantD {
					t.Fatalf("parse: %v diagnostics=%#v", err, diagnostics)
				}
				node := file.Nodes()[0]
				if node.Kind() != NodeTime || len(node.Children()) != 1 || node.Children()[0].Role() != RoleBody {
					t.Fatalf("time node=%#v", node)
				}
			})
		}
	})
}

func TestCompoundAndFunctionOwnTrailingRedirections(t *testing.T) {
	for _, test := range []struct {
		id       string
		source   string
		kind     NodeKind
		operator string
	}{
		{id: "brace", source: "{ printf x; } >out\n", kind: NodeBraceGroup, operator: ">"},
		{id: "subshell", source: "(printf x) 2>>err\n", kind: NodeSubshell, operator: ">>"},
		{id: "if", source: "if true; then printf x; fi <in\n", kind: NodeIf, operator: "<"},
		{id: "function", source: "work() { printf x; } >out\n", kind: NodeFunction, operator: ">"},
	} {
		t.Run(test.id, func(t *testing.T) {
			file, diagnostics, err := Parse(test.id+".sh", []byte(test.source), DialectPOSIX)
			if err != nil || len(diagnostics) != 0 {
				t.Fatalf("parse: %v diagnostics=%#v", err, diagnostics)
			}
			node := file.Nodes()[0]
			redirections := node.Redirections()
			if node.Kind() != test.kind || len(redirections) != 1 || redirections[0].Operator() != test.operator || node.Span().End.Offset != len(test.source)-1 {
				t.Fatalf("node=%#v redirections=%#v", node, redirections)
			}
			if node.Kind() == NodeFunction && len(node.Children()[0].Redirections()) != 0 {
				t.Fatalf("function body stole definition redirection: %#v", node.Children()[0])
			}
		})
	}
}

func countCode(diagnostics []Diagnostic, code string) int {
	count := 0
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			count++
		}
	}
	return count
}
