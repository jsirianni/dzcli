package shellvalidate

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestAPIValidation(t *testing.T) {
	if _, _, err := Parse("x.sh", nil, Dialect(99)); err == nil {
		t.Fatal("invalid dialect accepted")
	}
	if _, err := Check(nil, "x.sh", nil, Options{}); !errors.Is(err, errInvalidContext) {
		t.Fatalf("nil context: %v", err)
	}
	if _, _, err := Analyze(context.Background(), nil, Options{}); !errors.Is(err, errInvalidFile) {
		t.Fatalf("nil file: %v", err)
	}
	if _, err := Check(context.Background(), "x.sh", nil, Options{EnableCategories: []string{"missing"}}); err == nil {
		t.Fatal("unknown category accepted")
	}
	if _, err := Check(context.Background(), "x.sh", nil, Options{AnalyzeSourced: true}); err == nil {
		t.Fatal("missing resolver accepted")
	}
}

func TestAutoDialectAndImmutability(t *testing.T) {
	source := []byte("#!/usr/bin/env bash\nprintf '%s\\n' ok\n")
	file, diagnostics, err := Parse("x.sh", source, DialectAuto)
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 0 || file.Dialect() != DialectBash {
		t.Fatalf("dialect=%v diagnostics=%#v", file.Dialect(), diagnostics)
	}
	copySource := file.Source()
	copySource[0] = 'x'
	source[1] = 'x'
	if got := file.Source(); got[0] != '#' || got[1] != '!' {
		t.Fatalf("source aliased: %q", got[:2])
	}
	nodes := file.Nodes()
	if len(nodes) == 0 {
		t.Fatal("no syntax nodes")
	}
	nodes[0] = Node{}
	if reflect.DeepEqual(file.Nodes()[0], Node{}) {
		t.Fatal("nodes aliased")
	}
}

func TestPositionsAreByteOriented(t *testing.T) {
	source := []byte("printf 'é'\n\xFF\x00")
	_, diagnostics, err := Parse("bytes.sh", source, DialectPOSIX)
	if err != nil {
		t.Fatal(err)
	}
	if !hasCode(diagnostics, "SHS1003") {
		t.Fatalf("NUL diagnostic missing: %#v", diagnostics)
	}
	for _, diagnostic := range diagnostics {
		if diagnostic.Primary.Start.Offset < 0 || diagnostic.Primary.End.Offset > len(source) {
			t.Fatalf("span out of bounds: %#v", diagnostic)
		}
	}
}

func TestDiagnosticOrderingAndCap(t *testing.T) {
	source := []byte("break; return; break; return\n")
	result, err := Check(t.Context(), "x.sh", source, Options{Dialect: DialectPOSIX, MaxDiagnostics: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 2 {
		t.Fatalf("diagnostics=%d", len(result.Diagnostics))
	}
	if result.Diagnostics[0].Primary.Start.Offset > result.Diagnostics[1].Primary.Start.Offset {
		t.Fatalf("unsorted: %#v", result.Diagnostics)
	}
}

func TestContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Check(ctx, "x.sh", []byte("echo ok\n"), Options{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
}
