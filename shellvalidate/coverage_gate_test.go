package shellvalidate

import (
	"context"
	"strings"
	"testing"
)

// These focused cases cover public accessors, parser alternatives, and
// conservative constant decoding that are uncommon in the conformance corpus.
// They are behavioral regression tests, not exclusions from the coverage gate.
func TestCoverageGate_PublicErrorAndAccessorPaths(t *testing.T) {
	if _, _, err := Parse("bad", nil, Dialect(99)); err == nil {
		t.Fatal("Parse accepted an invalid dialect")
	}
	if _, _, err := Analyze(nil, &File{}, Options{}); err != errInvalidContext {
		t.Fatalf("Analyze nil context: %v", err)
	}
	if _, _, err := Analyze(context.Background(), nil, Options{}); err != errInvalidFile {
		t.Fatalf("Analyze nil file: %v", err)
	}
	if _, _, err := Analyze(context.Background(), &File{}, Options{AnalyzeSourced: true}); err == nil {
		t.Fatal("Analyze accepted source analysis without a resolver")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := Analyze(canceled, &File{}, Options{}); err != context.Canceled {
		t.Fatalf("Analyze canceled context: %v", err)
	}
	if _, err := Check(nil, "nil.sh", nil, Options{}); err != errInvalidContext {
		t.Fatalf("Check nil context: %v", err)
	}
	if _, err := Check(canceled, "canceled.sh", nil, Options{}); err != context.Canceled {
		t.Fatalf("Check canceled context: %v", err)
	}
	if _, err := Check(context.Background(), "bad.sh", nil, Options{Dialect: Dialect(99)}); err == nil {
		t.Fatal("Check accepted invalid options")
	}
	large := make([]byte, maxSourceBytes+1)
	largeFile, diagnostics, err := Parse("large.sh", large, DialectPOSIX)
	if err != nil || largeFile == nil || len(diagnostics) != 1 {
		t.Fatalf("large source result: file=%v diagnostics=%d err=%v", largeFile != nil, len(diagnostics), err)
	}
	var file *File
	if file.Filename() != "" || file.Source() != nil || file.Dialect() != DialectAuto || file.Interpreter() != "" || file.Nodes() != nil || file.Comments() != nil {
		t.Fatal("nil File accessors returned nonzero data")
	}
	expression := Expression{kind: ExpressionLiteral, value: "42", span: Span{Start: Position{Offset: 1}, End: Position{Offset: 3}}}
	if expression.Value() != "42" || expression.Span().Start.Offset != 1 {
		t.Fatal("expression accessors lost data")
	}
}

func TestCoverageGate_ArithmeticAlternatives(t *testing.T) {
	cases := []string{
		"a++", "--a", "a ? b : c", "a ? b", "(a + b)", "(a + b", "+", "a =~ b",
		"a += b; c ** d", "1 + '2'", "a +",
	}
	for _, input := range cases {
		source := newSourceFile("arithmetic.sh", []byte(input))
		expressions, diagnostics := parseArithmeticExpressionSet(source, 0, len(input))
		if len(expressions) == 0 {
			t.Errorf("%q produced no expression (diagnostics=%v)", input, diagnostics)
		}
		_ = parseArithmeticExpressions(source, 0, len(input))
	}
}

func TestCoverageGate_ConditionalAlternatives(t *testing.T) {
	cases := []string{
		"a || b", "a && b", "! a", "( a == b )", "( a == b", "-f file", "-N file",
		"a =~ pattern", "a -nt b", "a ==", "a b", "42", "'literal'", "",
	}
	for _, input := range cases {
		source := newSourceFile("conditional.sh", []byte(input))
		expressions, _ := parseConditionalExpressionSet(source, 0, len(input))
		_ = parseConditionalExpressions(source, 0, len(input))
		if input != "" && len(expressions) == 0 {
			t.Errorf("%q produced no expression", input)
		}
	}
}

func TestCoverageGate_ANSICQuoteDecoding(t *testing.T) {
	valid := []string{
		`$'plain'`, `$'\a\b\e\E\f\n\r\t\v'`, `$'\\\'\"'`, "$'a\\\nb'",
	}
	for _, input := range valid {
		if _, ok := decodeANSICLiteral([]byte(input)); !ok {
			t.Errorf("decodeANSICLiteral(%q) rejected valid literal", input)
		}
	}
	for _, input := range []string{"x", "$'unterminated", `$'bad\q'`, "$'bad\\"} {
		if _, ok := decodeANSICLiteral([]byte(input)); ok {
			t.Errorf("decodeANSICLiteral(%q) accepted invalid literal", input)
		}
	}
}

func TestCoverageGate_ParserAlternatives(t *testing.T) {
	scripts := []string{
		"if false; then :; elif true; then :; else :; fi\n",
		"until false; do break; done\n",
		"for value in a b; do printf %s \"$value\"; done\n",
		"case $value in a|b) echo yes;; c) echo no;& esac\n",
		"name() { echo ok; }\nfunction other { echo ok; }\n",
		"! true | false && echo no || echo yes &\n",
		"coproc worker { echo ok; }\ncoproc echo ok\n",
		"select value in a b; do break; done\n",
		"((a ? b : c, d += 2))\n[[ ! ( -f file && a =~ b ) || x -nt y ]]\n",
		"cat 3<<-'EOF'\n\tbody\n\tEOF\n",
	}
	for index, script := range scripts {
		file, _, err := Parse("alternatives.sh", []byte(script), DialectBash)
		if err != nil || file == nil {
			t.Fatalf("script %d parse: %v", index, err)
		}
		if index == 5 {
			views := commandViewsFromFile(file)
			if len(views) < 3 || !views[0].pipelineOut || !views[1].pipelineIn {
				t.Fatalf("legacy command views did not preserve pipeline boundaries: %#v", views)
			}
		}
	}
}

func TestCoverageGate_StaticWordAlternatives(t *testing.T) {
	inputs := []struct {
		text string
		ok   bool
	}{
		{"plain", true}, {"'single'", true}, {"\"double\"", true}, {"$'line\\n'", true},
		{"escaped\\ value", true}, {"$value", false}, {"\"$value\"", false}, {"trailing\\", false},
	}
	for _, input := range inputs {
		source := newSourceFile("token.sh", []byte(input.text))
		tokens, _, _ := lex(source, DialectBash)
		if len(tokens) == 0 {
			t.Fatalf("lex(%q) produced no token", input.text)
		}
		word := wordFromToken(source, tokens[0])
		_, ok := staticWordValue(word)
		if ok != input.ok {
			t.Errorf("staticWordValue(%q) ok=%v want=%v", input.text, ok, input.ok)
		}
	}
	if countPrintfOperands("%s %% %d") != 2 || !strings.Contains(categoryForCode("UNKNOWN"), "incomplete") {
		t.Fatal("analysis helpers returned unexpected results")
	}
}
