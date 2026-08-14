package shellvalidate

import (
	"context"
	"strings"
	"testing"
)

func TestASTInvariants(t *testing.T) {
	sources := []struct {
		dialect Dialect
		text    string
	}{
		{DialectPOSIX, "if true; then (printf '%s\\n' \"$value\"); fi\n"},
		{DialectPOSIX, "cat <<'END'\nbody\nEND\n"},
		{DialectBash, "[[ -n $value ]] && ((count += 1))\n"},
	}
	for _, item := range sources {
		file, diagnostics, err := Parse("invariant.sh", []byte(item.text), item.dialect)
		if err != nil || len(diagnostics) != 0 {
			t.Fatalf("parse: %v %#v", err, diagnostics)
		}
		for _, node := range file.Nodes() {
			assertNodeInvariant(t, len(item.text), node)
		}
	}
}

func assertNodeInvariant(t *testing.T, sourceLength int, node Node) {
	t.Helper()
	span := node.Span()
	if span.Start.Offset < 0 || span.End.Offset < span.Start.Offset || span.End.Offset > sourceLength {
		t.Fatalf("node span out of bounds: %#v", span)
	}
	for _, word := range node.Words() {
		if word.Span().Start.Offset < span.Start.Offset || word.Span().End.Offset > span.End.Offset {
			t.Fatalf("word %#v outside node %#v", word.Span(), span)
		}
		for _, part := range word.Parts() {
			if part.Span().Start.Offset < word.Span().Start.Offset || part.Span().End.Offset > word.Span().End.Offset {
				t.Fatalf("part %#v outside word %#v", part.Span(), word.Span())
			}
		}
	}
	for _, child := range node.Children() {
		assertNodeInvariant(t, sourceLength, child)
	}
}

func TestRecoveryCollectsIndependentErrors(t *testing.T) {
	source := []byte("if true; then echo x\n)\nprintf '%s' 'open\n")
	file, diagnostics, err := Parse("broken.sh", source, DialectPOSIX)
	if err != nil {
		t.Fatal(err)
	}
	if file == nil || len(diagnostics) < 2 || file.syntaxValid {
		t.Fatalf("recovery: file=%#v diagnostics=%#v", file, diagnostics)
	}
	assertDiagnosticBounds(t, source, diagnostics)
}

func TestSyntaxDiagnosticFamilies(t *testing.T) {
	tests := []struct {
		name   string
		source []byte
		code   string
	}{
		{name: "quote", source: []byte("echo 'open"), code: "SHS1001"},
		{name: "expansion", source: []byte("echo $(open"), code: "SHS1002"},
		{name: "nul", source: []byte{'e', 'c', 'h', 'o', 0}, code: "SHS1003"},
		{name: "compound", source: []byte("if true; then :"), code: "SHS1004"},
		{name: "unexpected", source: []byte(")"), code: "SHS1005"},
		{name: "heredoc", source: []byte("cat <<END\nbody\n"), code: "SHS1006"},
	}
	for _, item := range tests {
		t.Run(item.name, func(t *testing.T) {
			_, diagnostics, err := Parse("broken.sh", item.source, DialectPOSIX)
			if err != nil {
				t.Fatal(err)
			}
			if !hasCode(diagnostics, item.code) {
				t.Fatalf("missing %s in %#v", item.code, diagnostics)
			}
		})
	}
}

func TestNestingSafetyLimit(t *testing.T) {
	source := []byte(strings.Repeat("( ", maxNesting+10) + ":" + strings.Repeat(" )", maxNesting+10))
	_, diagnostics, err := Parse("deep.sh", source, DialectPOSIX)
	if err != nil {
		t.Fatal(err)
	}
	if !hasCode(diagnostics, "SHS1005") {
		t.Fatalf("nesting diagnostic missing: %d diagnostics", len(diagnostics))
	}
}

func TestSourceSizeLimit(t *testing.T) {
	source := make([]byte, maxSourceBytes+1)
	file, diagnostics, err := Parse("large.sh", source, DialectPOSIX)
	if err != nil {
		t.Fatal(err)
	}
	if file == nil || file.syntaxValid || !hasCode(diagnostics, "SHS1005") {
		t.Fatalf("size limit: file=%#v diagnostics=%#v", file, diagnostics)
	}
}

func TestResolverCycleIsBounded(t *testing.T) {
	resolver := mapResolver{files: map[string][]byte{
		"a.sh": []byte(". b.sh\n"),
		"b.sh": []byte(". a.sh\n"),
	}}
	result, err := Check(context.Background(), "a.sh", resolver.files["a.sh"], Options{Dialect: DialectPOSIX, AnalyzeSourced: true, Resolver: resolver})
	if err != nil {
		t.Fatal(err)
	}
	if result.AnalysisExact || !hasCode(result.Diagnostics, "SHI1001") {
		t.Fatalf("cycle result: %#v", result)
	}
}

func TestWhitespaceAndNewlineVariants(t *testing.T) {
	variants := []string{
		"if true; then echo ok; fi\n",
		"if true\nthen\n echo ok\nfi\n",
		"if\ttrue;then\techo\tok;fi\n",
	}
	for _, source := range variants {
		result, err := Check(t.Context(), "variant.sh", []byte(source), Options{Dialect: DialectPOSIX})
		if err != nil || !result.SyntaxValid {
			t.Fatalf("variant %q: %v %#v", source, err, result.Diagnostics)
		}
	}
}
