package shellvalidate

import (
	"bytes"
	"context"
	"reflect"
	"testing"
)

var fuzzSeeds = [][]byte{
	{},
	[]byte("echo ok\n"),
	[]byte("if true; then echo yes; fi\n"),
	[]byte("cat <<EOF\nbody\nEOF\n"),
	[]byte("[[ $x =~ ^a ]]\n"),
	[]byte("for ((i=0;i<3;i++)); do :; done\n"),
	{0xff, '\n'},
}

func addSeeds(f *testing.F) {
	for _, seed := range fuzzSeeds {
		f.Add(seed)
	}
}

func fuzzParse(f *testing.F, dialect Dialect) {
	addSeeds(f)
	f.Fuzz(func(t *testing.T, source []byte) {
		fileA, diagnosticsA, errA := Parse("fuzz.sh", source, dialect)
		fileB, diagnosticsB, errB := Parse("fuzz.sh", source, dialect)
		if (errA == nil) != (errB == nil) || !reflect.DeepEqual(diagnosticsA, diagnosticsB) {
			t.Fatalf("nondeterministic parse: %v/%v %#v/%#v", errA, errB, diagnosticsA, diagnosticsB)
		}
		if fileA != nil && fileB != nil && !reflect.DeepEqual(fileA.Nodes(), fileB.Nodes()) {
			t.Fatal("nondeterministic AST")
		}
		assertDiagnosticBounds(t, source, diagnosticsA)
	})
}

func fuzzAnalyze(f *testing.F, dialect Dialect) {
	addSeeds(f)
	f.Fuzz(func(t *testing.T, source []byte) {
		result, err := Check(context.Background(), "fuzz.sh", source, Options{Dialect: dialect, MaxDiagnostics: 20})
		if err != nil {
			return
		}
		if len(result.Diagnostics) > 20 {
			t.Fatalf("diagnostic cap exceeded: %d", len(result.Diagnostics))
		}
		assertDiagnosticBounds(t, source, result.Diagnostics)
	})
}

func assertDiagnosticBounds(t *testing.T, source []byte, diagnostics []Diagnostic) {
	t.Helper()
	previous := -1
	for _, item := range diagnostics {
		if item.Primary.Start.Offset < 0 || item.Primary.End.Offset < item.Primary.Start.Offset || item.Primary.End.Offset > len(source) {
			t.Fatalf("invalid span for %d bytes: %#v", len(source), item.Primary)
		}
		if item.Primary.Start.Offset < previous {
			t.Fatalf("diagnostics not sorted: %#v", diagnostics)
		}
		previous = item.Primary.Start.Offset
	}
}

func FuzzLex(f *testing.F)                { fuzzParse(f, DialectBash) }
func FuzzParsePOSIX(f *testing.F)         { fuzzParse(f, DialectPOSIX) }
func FuzzParseBash(f *testing.F)          { fuzzParse(f, DialectBash) }
func FuzzCheckPOSIX(f *testing.F)         { fuzzAnalyze(f, DialectPOSIX) }
func FuzzCheckBash(f *testing.F)          { fuzzAnalyze(f, DialectBash) }
func FuzzHereDocuments(f *testing.F)      { fuzzParse(f, DialectBash) }
func FuzzParameterExpansion(f *testing.F) { fuzzParse(f, DialectBash) }
func FuzzArithmetic(f *testing.F)         { fuzzParse(f, DialectBash) }
func FuzzBashConditional(f *testing.F)    { fuzzParse(f, DialectBash) }
func FuzzErrorRecovery(f *testing.F)      { fuzzAnalyze(f, DialectBash) }

func TestParseDeterministicAndParallel(t *testing.T) {
	source := []byte("value=x\nif test -n \"$value\"; then printf '%s\\n' \"$value\"; fi\n")
	want, err := Check(t.Context(), "parallel.sh", source, Options{Dialect: DialectPOSIX})
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 16; index++ {
		t.Run(string(rune('a'+index)), func(t *testing.T) {
			t.Parallel()
			got, checkErr := Check(t.Context(), "parallel.sh", append([]byte(nil), source...), Options{Dialect: DialectPOSIX})
			if checkErr != nil || !reflect.DeepEqual(got.Diagnostics, want.Diagnostics) {
				t.Fatalf("parallel result: %v %#v", checkErr, got.Diagnostics)
			}
		})
	}
}

func TestTruncationNeverPanics(t *testing.T) {
	source := []byte("if true; then printf '%s\\n' \"${value:-$(date)}\"; fi\n")
	for end := range source {
		result, err := Check(t.Context(), "truncated.sh", source[:end], Options{Dialect: DialectPOSIX})
		if err != nil {
			continue
		}
		assertDiagnosticBounds(t, source[:end], result.Diagnostics)
	}
}

func TestInvalidUTF8Preserved(t *testing.T) {
	source := []byte{'e', 'c', 'h', 'o', ' ', 0xff, '\n'}
	file, _, err := Parse("bytes.sh", source, DialectPOSIX)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(file.Source(), source) {
		t.Fatalf("source changed: %x", file.Source())
	}
}

func BenchmarkCheckPOSIX(b *testing.B) {
	source := bytes.Repeat([]byte("name=value; printf '%s\\n' \"$name\"\n"), 100)
	b.ReportAllocs()
	for b.Loop() {
		if _, err := Check(context.Background(), "bench.sh", source, Options{Dialect: DialectPOSIX}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCheckBash(b *testing.B) {
	source := bytes.Repeat([]byte("values=(one two); [[ ${values[0]} == o* ]]\n"), 100)
	b.ReportAllocs()
	for b.Loop() {
		if _, err := Check(context.Background(), "bench.sh", source, Options{Dialect: DialectBash}); err != nil {
			b.Fatal(err)
		}
	}
}
