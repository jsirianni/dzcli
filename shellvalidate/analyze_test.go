package shellvalidate

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func checkCodes(t *testing.T, source string, dialect Dialect, expected ...string) Result {
	t.Helper()
	result, err := Check(t.Context(), "script.sh", []byte(source), Options{Dialect: dialect})
	if err != nil {
		t.Fatal(err)
	}
	for _, code := range expected {
		if !hasCode(result.Diagnostics, code) {
			t.Errorf("missing %s in %#v", code, result.Diagnostics)
		}
	}
	return result
}

func TestAnalysisRuleFamilies(t *testing.T) {
	tests := []struct {
		name   string
		source string
		codes  []string
	}{
		{name: "nounset", source: "set -u\nprintf '%s\\n' \"$missing\"\n", codes: []string{"SHV1001"}},
		{name: "pipeline scope", source: "printf x | read value\n", codes: []string{"SHV1002"}},
		{name: "trailing newline", source: "line=$(printf 'x\\n')\n", codes: []string{"SHE1002"}},
		{name: "redirection order", source: "command 2>&1 >out\n", codes: []string{"SHR1001"}},
		{name: "same path", source: "command <data >data\n", codes: []string{"SHR1002"}},
		{name: "bracket", source: "[ -n value\n", codes: []string{"SHB1001"}},
		{name: "destructive", source: "rm -rf \"$target\"\n", codes: []string{"SHX1002"}},
	}
	for _, item := range tests {
		t.Run(item.name, func(t *testing.T) {
			checkCodes(t, item.source, DialectPOSIX, item.codes...)
		})
	}
}

func TestDialectMismatch(t *testing.T) {
	checkCodes(t, "#!/usr/bin/env bash\necho ok\n", DialectPOSIX, "SHD1002")
}

func TestCategorySelectionAndDisablePrecedence(t *testing.T) {
	source := []byte("break\nprintf '%s\\n' $value\n")
	result, err := Check(t.Context(), "script.sh", source, Options{
		Dialect:           DialectPOSIX,
		EnableCategories:  []string{"control", "expansion"},
		DisableCategories: []string{"expansion"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !hasCode(result.Diagnostics, "SHC1001") || hasCode(result.Diagnostics, "SHE1001") {
		t.Fatalf("category filtering: %#v", result.Diagnostics)
	}
}

type mapResolver struct {
	files map[string][]byte
	err   error
}

func (resolver mapResolver) Resolve(_ context.Context, _, requestedPath string) (string, []byte, error) {
	if resolver.err != nil {
		return "", nil, resolver.err
	}
	data, ok := resolver.files[requestedPath]
	if !ok {
		return "", nil, errors.New("not found")
	}
	return requestedPath, append([]byte(nil), data...), nil
}

func TestSourceResolverAndLiteralEval(t *testing.T) {
	resolver := mapResolver{files: map[string][]byte{"child.sh": []byte("break\n")}}
	result, err := Check(t.Context(), "parent.sh", []byte(". 'child.sh'\n"), Options{Dialect: DialectPOSIX, AnalyzeSourced: true, Resolver: resolver})
	if err != nil {
		t.Fatal(err)
	}
	if !hasCode(result.Diagnostics, "SHC1001") {
		t.Fatalf("sourced diagnostics: %#v", result.Diagnostics)
	}
	for _, item := range result.Diagnostics {
		if item.Code == "SHC1001" && len(item.Related) == 0 {
			t.Fatal("sourced diagnostic has no origin")
		}
	}

	evaluated := checkCodes(t, "eval 'break'\n", DialectPOSIX, "SHC1001")
	if !evaluated.AnalysisExact {
		t.Fatal("literal eval should remain exact")
	}
}

func TestResolverFailureIsGoError(t *testing.T) {
	want := errors.New("resolver unavailable")
	_, err := Check(t.Context(), "parent.sh", []byte(". child.sh\n"), Options{Dialect: DialectPOSIX, AnalyzeSourced: true, Resolver: mapResolver{err: want}})
	if !errors.Is(err, want) || !strings.Contains(err.Error(), "child.sh") {
		t.Fatalf("resolver error: %v", err)
	}
}

func TestDynamicAnalysisIsInexact(t *testing.T) {
	result := checkCodes(t, "eval \"$generated\"\n", DialectPOSIX, "SHX1001", "SHI1001")
	if result.AnalysisExact {
		t.Fatal("dynamic eval reported exact")
	}
}

func TestAnalyzeRejectsDialectMismatch(t *testing.T) {
	file, diagnostics, err := Parse("script.sh", []byte("echo ok\n"), DialectPOSIX)
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("parse: %v %#v", err, diagnostics)
	}
	if _, _, err := Analyze(t.Context(), file, Options{Dialect: DialectBash}); err == nil || !strings.Contains(err.Error(), "parsed file dialect") {
		t.Fatalf("dialect mismatch error = %v", err)
	}
	if _, _, err := Analyze(t.Context(), file, Options{Dialect: DialectAuto}); err != nil {
		t.Fatalf("auto dialect should accept parsed dialect: %v", err)
	}
}

func TestLiteralEvalUsesStructuredQuoteRemoval(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{name: "single-quoted-dollar", source: "eval 'printf %s $value'\n"},
		{name: "concatenated-fragments", source: "eval 'bre'\"ak\"\n"},
		{name: "escaped-word", source: "eval bre\\ak\n"},
		{name: "ansi-c-word", source: "eval $'break'\n"},
		{name: "finite-nesting", source: "eval 'eval \"break\"'\n"},
	}
	for _, item := range tests {
		t.Run(item.name, func(t *testing.T) {
			result, err := Check(t.Context(), "eval.sh", []byte(item.source), Options{Dialect: DialectPOSIX})
			if err != nil {
				t.Fatal(err)
			}
			if hasCode(result.Diagnostics, "SHX1001") || hasCode(result.Diagnostics, "SHI1001") || !result.AnalysisExact {
				t.Fatalf("static eval classified dynamic: %#v", result)
			}
			if item.name != "single-quoted-dollar" && !hasCode(result.Diagnostics, "SHC1001") {
				t.Fatalf("literal eval was not analyzed: %#v", result.Diagnostics)
			}
		})
	}
}

func TestLocaleQuotedEvalIsConservativelyDynamic(t *testing.T) {
	result, err := Check(t.Context(), "eval.sh", []byte("eval $\"break\"\n"), Options{Dialect: DialectBash})
	if err != nil {
		t.Fatal(err)
	}
	if result.AnalysisExact || !hasCode(result.Diagnostics, "SHX1001") || !hasCode(result.Diagnostics, "SHI1001") {
		t.Fatalf("locale-translated eval classified exact: %#v", result)
	}
}

type identityResolver struct {
	requests []string
}

func (resolver *identityResolver) Resolve(_ context.Context, _, requestedPath string) (string, []byte, error) {
	resolver.requests = append(resolver.requests, requestedPath)
	switch requestedPath {
	case "first":
		return `opaque\child`, []byte(". second\n"), nil
	case "second":
		return "opaque/child", []byte("break\n"), nil
	default:
		return "", nil, errors.New("unexpected request")
	}
}

func TestResolverIdentityIsOpaque(t *testing.T) {
	resolver := &identityResolver{}
	result, err := Check(t.Context(), "root.sh", []byte(". first\n"), Options{Dialect: DialectPOSIX, AnalyzeSourced: true, Resolver: resolver})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(resolver.requests, []string{"first", "second"}) {
		t.Fatalf("requests = %#v", resolver.requests)
	}
	if !result.AnalysisExact || hasCode(result.Diagnostics, "SHI1001") || !hasCode(result.Diagnostics, "SHC1001") {
		t.Fatalf("distinct opaque identities were conflated: %#v", result)
	}
}

func TestSourceUncertaintyPropagates(t *testing.T) {
	result, err := Check(t.Context(), "root.sh", []byte(". known.sh\n"), Options{Dialect: DialectPOSIX})
	if err != nil {
		t.Fatal(err)
	}
	if result.AnalysisExact || !hasCode(result.Diagnostics, "SHI1001") {
		t.Fatalf("unanalyzed source reported exact: %#v", result)
	}

	resolver := mapResolver{files: map[string][]byte{"child.sh": []byte("eval \"$generated\"\n")}}
	result, err = Check(t.Context(), "root.sh", []byte(". child.sh\n"), Options{Dialect: DialectPOSIX, AnalyzeSourced: true, Resolver: resolver})
	if err != nil {
		t.Fatal(err)
	}
	if result.AnalysisExact || !hasCode(result.Diagnostics, "SHX1001") || !hasCode(result.Diagnostics, "SHI1001") {
		t.Fatalf("nested uncertainty did not propagate: %#v", result)
	}
}

func TestMaterialCommandAndPathUncertainty(t *testing.T) {
	tests := []struct {
		name   string
		source string
		code   string
	}{
		{name: "dynamic-command-name", source: "\"$command\" argument\n", code: "SHI1001"},
		{name: "dynamic-destructive-path", source: "rm -rf \"$target\"\n", code: "SHX1002"},
		{name: "computed-destructive-path", source: "rm -rf \"$(compute_path)\"\n", code: "SHX1002"},
	}
	for _, item := range tests {
		t.Run(item.name, func(t *testing.T) {
			result, err := Check(t.Context(), "uncertain.sh", []byte(item.source), Options{Dialect: DialectPOSIX})
			if err != nil {
				t.Fatal(err)
			}
			if result.AnalysisExact || !hasCode(result.Diagnostics, "SHI1001") || !hasCode(result.Diagnostics, item.code) {
				t.Fatalf("material uncertainty reported exact: %#v", result)
			}
		})
	}

	result, err := Check(t.Context(), "literal.sh", []byte("vendor-tool literal\n"), Options{Dialect: DialectPOSIX})
	if err != nil {
		t.Fatal(err)
	}
	if !result.AnalysisExact || hasCode(result.Diagnostics, "SHI1001") {
		t.Fatalf("literal external command caused blanket uncertainty: %#v", result)
	}
}

func TestControlDiagnosticsUseASTScope(t *testing.T) {
	tests := []struct {
		name   string
		source string
		code   string
		want   bool
	}{
		{name: "top-level break", source: "break\n", code: "SHC1001", want: true},
		{name: "loop break", source: "while ready; do break; done\n", code: "SHC1001", want: false},
		{name: "top-level return", source: "return\n", code: "SHC1002", want: true},
		{name: "function return", source: "work() { return; }\n", code: "SHC1002", want: false},
	}
	for _, item := range tests {
		t.Run(item.name, func(t *testing.T) {
			result, err := Check(t.Context(), "scope.sh", []byte(item.source), Options{Dialect: DialectPOSIX})
			if err != nil {
				t.Fatal(err)
			}
			if got := hasCode(result.Diagnostics, item.code); got != item.want {
				t.Fatalf("%s present=%v, want %v: %#v", item.code, got, item.want, result.Diagnostics)
			}
		})
	}
}

func TestIncompleteSubtreeDoesNotCascade(t *testing.T) {
	result, err := Check(t.Context(), "recovery.sh", []byte("if true; then break\nprintf ok\n"), Options{Dialect: DialectPOSIX})
	if err != nil {
		t.Fatal(err)
	}
	if result.AnalysisExact || !hasCode(result.Diagnostics, "SHS1004") || !hasCode(result.Diagnostics, "SHI1001") {
		t.Fatalf("recovery state: %#v", result)
	}
	if hasCode(result.Diagnostics, "SHC1001") {
		t.Fatalf("incomplete subtree produced semantic cascade: %#v", result.Diagnostics)
	}
}

func TestRedirectionAnalysisUsesASTOwnership(t *testing.T) {
	result, err := Check(t.Context(), "redirect.sh", []byte("printf '%s' '2>&1 >out'\n"), Options{Dialect: DialectPOSIX})
	if err != nil {
		t.Fatal(err)
	}
	if hasCode(result.Diagnostics, "SHR1001") || hasCode(result.Diagnostics, "SHR1002") {
		t.Fatalf("quoted text was interpreted as redirection: %#v", result.Diagnostics)
	}

	result, err = Check(t.Context(), "redirect.sh", []byte("command 2>&1 >out <data >data\n"), Options{Dialect: DialectPOSIX})
	if err != nil {
		t.Fatal(err)
	}
	if !hasCode(result.Diagnostics, "SHR1001") || !hasCode(result.Diagnostics, "SHR1002") {
		t.Fatalf("ordered redirections not analyzed: %#v", result.Diagnostics)
	}
}

func TestAssignmentOrderingAndCommandArguments(t *testing.T) {
	result, err := Check(t.Context(), "assign.sh", []byte("set -u\nx=$x\nprintf '%s' y=value\nprintf '%s' \"$y\"\n"), Options{Dialect: DialectPOSIX})
	if err != nil {
		t.Fatal(err)
	}
	var reads []Diagnostic
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Code == "SHV1001" {
			reads = append(reads, diagnostic)
		}
	}
	if len(reads) != 2 {
		t.Fatalf("nounset reads=%#v, all=%#v", reads, result.Diagnostics)
	}
}
