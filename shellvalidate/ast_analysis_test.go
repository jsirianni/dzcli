package shellvalidate

import (
	"strings"
	"testing"
)

func TestCommandViewsUseExecutableASTOwnership(t *testing.T) {
	source := []byte("eval() { printf '%s\\n' ok; }\ncase \"$kind\" in\n eval) printf match ;;\nesac\ncat <<'EOF'\neval \"$payload\"\nEOF\n")
	file, diagnostics, err := Parse("ownership.sh", source, DialectPOSIX)
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 0 {
		t.Fatalf("parse diagnostics: %#v", diagnostics)
	}

	views := commandViewsFromFile(file)
	names := make([]string, 0, len(views))
	for _, view := range views {
		if view.hasName {
			names = append(names, view.name)
		}
	}
	if got, want := strings.Join(names, ","), "printf,printf,cat"; got != want {
		t.Fatalf("executable command names = %q, want %q", got, want)
	}

	result, err := Check(t.Context(), "ownership.sh", source, Options{Dialect: DialectPOSIX})
	if err != nil {
		t.Fatal(err)
	}
	if !result.AnalysisExact || hasCode(result.Diagnostics, "SHX1001") || hasCode(result.Diagnostics, "SHI1001") {
		t.Fatalf("function name, case pattern, or here-document body became executable semantics: %#v", result)
	}
}

func TestNestedCompoundRetainsPipelineOwnership(t *testing.T) {
	result, err := Check(t.Context(), "pipeline.sh", []byte("printf x | { read value; }\n"), Options{Dialect: DialectPOSIX})
	if err != nil {
		t.Fatal(err)
	}
	if !hasCode(result.Diagnostics, "SHV1002") {
		t.Fatalf("read nested in a pipeline compound lost pipeline ownership: %#v", result.Diagnostics)
	}
}

func TestStructuredWordsPreserveCommandAndArgumentQuotes(t *testing.T) {
	evaluated, err := Check(t.Context(), "quoted-command.sh", []byte("prefix=ok 'eval' 'break'\n"), Options{Dialect: DialectPOSIX})
	if err != nil {
		t.Fatal(err)
	}
	if !evaluated.AnalysisExact || !hasCode(evaluated.Diagnostics, "SHC1001") || hasCode(evaluated.Diagnostics, "SHX1001") {
		t.Fatalf("quoted static command was not interpreted through structured word parts: %#v", evaluated)
	}

	source := "set -u\nprintf '%s' '$missing' \"$missing\"\n"
	result, err := Check(t.Context(), "quotes.sh", []byte(source), Options{Dialect: DialectPOSIX})
	if err != nil {
		t.Fatal(err)
	}
	var reads []Diagnostic
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Code == "SHV1001" {
			reads = append(reads, diagnostic)
		}
	}
	wantStart := strings.LastIndex(source, "$missing")
	if len(reads) != 1 || reads[0].Primary.Start.Offset != wantStart {
		t.Fatalf("nounset reads = %#v, want one read at byte %d", reads, wantStart)
	}
}
