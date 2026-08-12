package shellvalidate

import (
	"bytes"
	"testing"
)

func TestArithmeticAndConditionalOperatorsAreContextual(t *testing.T) {
	t.Run("regex rejected by arithmetic", func(t *testing.T) {
		file, diagnostics, err := Parse("arithmetic.sh", []byte("((left =~ right))\n"), DialectBash)
		if err != nil {
			t.Fatal(err)
		}
		if file == nil || file.syntaxValid || !hasCode(diagnostics, "SHS1005") {
			t.Fatalf("arithmetic regex accepted: file=%#v diagnostics=%#v", file, diagnostics)
		}
	})

	t.Run("regex accepted by conditional", func(t *testing.T) {
		file, diagnostics, err := Parse("conditional.sh", []byte("[[ left =~ right ]]\n"), DialectBash)
		if err != nil || len(diagnostics) != 0 {
			t.Fatalf("parse: %v %#v", err, diagnostics)
		}
		expression := firstExpression(t, file, NodeConditional)
		if expression.Kind() != ExpressionBinary || expression.Operator() != "=~" {
			t.Fatalf("expression=%#v", expression)
		}
	})

	t.Run("pattern equality is comparison", func(t *testing.T) {
		file, diagnostics, err := Parse("conditional.sh", []byte("[[ left = p* ]]\n"), DialectBash)
		if err != nil || len(diagnostics) != 0 {
			t.Fatalf("parse: %v %#v", err, diagnostics)
		}
		expression := firstExpression(t, file, NodeConditional)
		if expression.Kind() != ExpressionBinary || expression.Operator() != "=" {
			t.Fatalf("expression=%#v", expression)
		}
	})

	t.Run("arithmetic precedence and associativity", func(t *testing.T) {
		file, diagnostics, err := Parse("arithmetic.sh", []byte("((a=b=c+2*3**4))\n"), DialectBash)
		if err != nil || len(diagnostics) != 0 {
			t.Fatalf("parse: %v %#v", err, diagnostics)
		}
		root := firstExpression(t, file, NodeArithmetic)
		if root.Kind() != ExpressionAssignment || root.Operator() != "=" {
			t.Fatalf("root=%#v", root)
		}
		right := root.Children()[1]
		if right.Kind() != ExpressionAssignment || right.Operator() != "=" {
			t.Fatalf("right=%#v", right)
		}
		addition := right.Children()[1]
		if addition.Operator() != "+" || addition.Children()[1].Operator() != "*" || addition.Children()[1].Children()[1].Operator() != "**" {
			t.Fatalf("precedence=%#v", root)
		}
	})
}

func TestParserBuildsHierarchicalASTAndRedirections(t *testing.T) {
	t.Run("pipeline", func(t *testing.T) {
		file, diagnostics, err := Parse("pipeline.sh", []byte("left | right\n"), DialectPOSIX)
		if err != nil || len(diagnostics) != 0 {
			t.Fatalf("parse: %v %#v", err, diagnostics)
		}
		nodes := file.Nodes()
		if len(nodes) != 1 || nodes[0].Kind() != NodePipeline || nodes[0].Operator() != "|" {
			t.Fatalf("nodes=%#v", nodes)
		}
		children := nodes[0].Children()
		if len(children) != 2 || children[0].Role() != RolePipelineCommand || children[1].Role() != RolePipelineCommand {
			t.Fatalf("children=%#v", children)
		}
	})

	t.Run("if roles", func(t *testing.T) {
		file, diagnostics, err := Parse("if.sh", []byte("if check; then pass; else fail; fi\n"), DialectPOSIX)
		if err != nil || len(diagnostics) != 0 {
			t.Fatalf("parse: %v %#v", err, diagnostics)
		}
		node := file.Nodes()[0]
		if node.Kind() != NodeIf {
			t.Fatalf("node=%#v", node)
		}
		children := node.Children()
		if len(children) != 3 || children[0].Role() != RoleCondition || children[1].Role() != RoleBody || children[2].Role() != RoleAlternate {
			t.Fatalf("children=%#v", children)
		}
	})

	t.Run("io number and ownership", func(t *testing.T) {
		file, diagnostics, err := Parse("redirect.sh", []byte("command 3>file arg\n"), DialectPOSIX)
		if err != nil || len(diagnostics) != 0 {
			t.Fatalf("parse: %v %#v", err, diagnostics)
		}
		node := file.Nodes()[0]
		redirects := node.Redirections()
		if len(redirects) != 1 || redirects[0].Operator() != ">" {
			t.Fatalf("redirects=%#v", redirects)
		}
		if number, ok := redirects[0].IONumber(); !ok || number != 3 {
			t.Fatalf("io number=%d/%v", number, ok)
		}
		if got := string(redirects[0].Target().Parts()[0].Text()); got != "file" {
			t.Fatalf("target=%q", got)
		}
		if len(node.Words()) != 2 {
			t.Fatalf("words=%#v", node.Words())
		}
	})
}

func TestHereDocumentsAreRecognizedFromRedirections(t *testing.T) {
	t.Run("comment does not enqueue", func(t *testing.T) {
		file, diagnostics, err := Parse("comment.sh", []byte("echo ok # <<EOF\nnext\n"), DialectPOSIX)
		if err != nil || !file.syntaxValid || hasCode(diagnostics, "SHS1006") {
			t.Fatalf("comment created heredoc: %v %#v", err, diagnostics)
		}
	})

	t.Run("arithmetic shift does not enqueue", func(t *testing.T) {
		file, diagnostics, err := Parse("shift.sh", []byte("((value\n << 2))\n"), DialectBash)
		if err != nil || !file.syntaxValid || hasCode(diagnostics, "SHS1006") {
			t.Fatalf("shift created heredoc: %v %#v", err, diagnostics)
		}
	})

	t.Run("metadata and fifo ownership", func(t *testing.T) {
		source := []byte("cat 3<<'ONE' 4<<-TWO\n$literal\nONE\n\tsecond\n\tTWO\n")
		file, diagnostics, err := Parse("heredoc.sh", source, DialectPOSIX)
		if err != nil || len(diagnostics) != 0 {
			t.Fatalf("parse: %v %#v", err, diagnostics)
		}
		redirects := file.Nodes()[0].Redirections()
		if len(redirects) != 2 {
			t.Fatalf("redirects=%#v", redirects)
		}
		first, second := redirects[0].HereDocument(), redirects[1].HereDocument()
		if first == nil || !first.Quoted() || first.StripTabs() || !bytes.Equal(first.Body(), []byte("$literal\n")) {
			t.Fatalf("first=%#v", first)
		}
		if second == nil || second.Quoted() || !second.StripTabs() || !bytes.Equal(second.Body(), []byte("\tsecond\n")) {
			t.Fatalf("second=%#v", second)
		}
	})
}

func TestNULDiagnosticsAreExactAcrossLexicalContexts(t *testing.T) {
	tests := []struct {
		name   string
		source []byte
	}{
		{name: "plain", source: []byte{'e', 'c', 'h', 'o', ' ', 0, '\n'}},
		{name: "quoted", source: []byte{'e', 'c', 'h', 'o', ' ', '\'', 'a', 0, 'b', '\'', '\n'}},
		{name: "comment", source: []byte{'#', ' ', 0, '\n'}},
		{name: "heredoc", source: []byte{'c', 'a', 't', ' ', '<', '<', 'E', '\n', 'a', 0, 'b', '\n', 'E', '\n'}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, diagnostics, err := Parse(test.name+".sh", test.source, DialectPOSIX)
			if err != nil {
				t.Fatal(err)
			}
			var nul []Diagnostic
			for _, diagnostic := range diagnostics {
				if diagnostic.Code == "SHS1003" {
					nul = append(nul, diagnostic)
				}
			}
			if len(nul) != 1 || nul[0].Primary.End.Offset-nul[0].Primary.Start.Offset != 1 || test.source[nul[0].Primary.Start.Offset] != 0 {
				t.Fatalf("NUL diagnostics=%#v", nul)
			}
		})
	}
}

func TestLexerPreservesParameterPartsAndSingleQuotedBackslashes(t *testing.T) {
	source := []byte("printf '%s' '$name\\still-literal' \"$name:$1:$?:$@\" $plain\n")
	file, diagnostics, err := Parse("parts.sh", source, DialectPOSIX)
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("parse: %v %#v", err, diagnostics)
	}
	words := file.Nodes()[0].Words()
	if got := string(words[2].Parts()[0].Text()); got != "$name\\still-literal" {
		t.Fatalf("single-quoted content=%q", got)
	}
	var parameters []string
	for _, word := range words {
		for _, part := range word.Parts() {
			if part.Kind() == WordParameterExpansion {
				parameters = append(parameters, string(part.Text()))
			}
		}
	}
	want := []string{"$name", "$1", "$?", "$@", "$plain"}
	if len(parameters) != len(want) {
		t.Fatalf("parameters=%q want=%q", parameters, want)
	}
	for index := range want {
		if parameters[index] != want[index] {
			t.Fatalf("parameters=%q want=%q", parameters, want)
		}
	}
}

func TestNestedExpansionDelimiters(t *testing.T) {
	source := []byte("printf '%s' \"$((1 + (2 * 3)))\" \"$(printf '%s' \"${value:-x}\")\"\n")
	file, diagnostics, err := Parse("nested.sh", source, DialectPOSIX)
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("parse: %v %#v", err, diagnostics)
	}
	words := file.Nodes()[0].Words()
	if len(words) != 4 || words[2].Parts()[0].Kind() != WordArithmeticExpansion || words[3].Parts()[0].Kind() != WordCommandSubstitution {
		t.Fatalf("words=%#v", words)
	}
}

func firstExpression(t *testing.T, file *File, kind NodeKind) Expression {
	t.Helper()
	for _, node := range file.Nodes() {
		if node.Kind() == kind && len(node.Expressions()) != 0 {
			return node.Expressions()[0]
		}
	}
	t.Fatalf("node %s with expression missing", kind)
	return Expression{}
}
