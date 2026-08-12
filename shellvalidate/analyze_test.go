package shellvalidate

import (
	"context"
	"errors"
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
