package mutest

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestApplyRejectsStaleHashAndNonUniqueSelector(t *testing.T) {
	root := fixtureModule(t, `package sample

func target() bool {
	return true
}
`)
	filename := filepath.Join(root, "shellvalidate", "sample.go")
	digest, err := DeclarationHash(filename, "target")
	if err != nil {
		t.Fatal(err)
	}
	mutant := Mutant{ID: "T001", Selector: &Selector{File: "shellvalidate/sample.go", Declaration: "target", DeclarationHash: "stale", Before: "return true", After: "return false"}}
	if err := Apply(root, mutant); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("stale hash error = %v", err)
	}
	mutant.Selector.DeclarationHash = digest
	mutant.Selector.Before = "true"
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	data = []byte(strings.Replace(string(data), "return true", "if true { return true }; return false", 1))
	if err := os.WriteFile(filename, data, 0o644); err != nil {
		t.Fatal(err)
	}
	digest, err = DeclarationHash(filename, "target")
	if err != nil {
		t.Fatal(err)
	}
	mutant.Selector.DeclarationHash = digest
	if err := Apply(root, mutant); err == nil || !strings.Contains(err.Error(), "matched 2 times") {
		t.Fatalf("non-unique selector error = %v", err)
	}
}

func TestApplyChangesExactlyOneDeclaration(t *testing.T) {
	root := fixtureModule(t, `package sample

func target() bool { return true }
func untouched() bool { return true }
`)
	filename := filepath.Join(root, "shellvalidate", "sample.go")
	digest, err := DeclarationHash(filename, "target")
	if err != nil {
		t.Fatal(err)
	}
	mutant := Mutant{ID: "T001", Selector: &Selector{File: "shellvalidate/sample.go", Declaration: "target", DeclarationHash: digest, Before: "return true", After: "return false"}}
	if err := Apply(root, mutant); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Count(text, "return false") != 1 || strings.Count(text, "return true") != 1 {
		t.Fatalf("unexpected mutation:\n%s", text)
	}
}

func TestRunClassifiesAndOrdersResults(t *testing.T) {
	root := fixtureModule(t, `package sample

func target() bool { return true }
`)
	filename := filepath.Join(root, "shellvalidate", "sample.go")
	digest, err := DeclarationHash(filename, "target")
	if err != nil {
		t.Fatal(err)
	}
	mutants := []Mutant{
		{ID: "KILL", SourceOrder: 1, Target: "killed", Critical: true, KilledBy: "TestBehavior/KILL", Selector: &Selector{File: "shellvalidate/sample.go", Declaration: "target", DeclarationHash: digest, Before: "return true", After: "return false"}},
		{ID: "LIVE", SourceOrder: 2, Target: "survived", Critical: true, KilledBy: "TestBehavior/LIVE", Selector: &Selector{File: "shellvalidate/sample.go", Declaration: "target", DeclarationHash: digest, Before: "return true", After: "return false"}},
		{ID: "BAD", SourceOrder: 3, Target: "invalid", Critical: true, KilledBy: "TestBehavior/BAD", Selector: &Selector{File: "shellvalidate/sample.go", Declaration: "target", DeclarationHash: digest, Before: "return true", After: "return false"}},
		{ID: "SLOW", SourceOrder: 4, Target: "timeout", Critical: true, KilledBy: "TestBehavior/SLOW", Selector: &Selector{File: "shellvalidate/sample.go", Declaration: "target", DeclarationHash: digest, Before: "return true", After: "return false"}},
	}
	manifest := filepath.Join(root, "mutants.json")
	writeJSON(t, manifest, mutants)
	output := filepath.Join(root, "out")
	runner := func(ctx context.Context, _ string, arguments []string, _ []string) ([]byte, error) {
		joined := strings.Join(arguments, " ")
		if strings.Contains(joined, "BAD") && strings.Contains(joined, "-run ^$") {
			return []byte("build failed"), errors.New("exit 1")
		}
		if strings.Contains(joined, "SLOW") {
			<-ctx.Done()
			return nil, ctx.Err()
		}
		if strings.Contains(joined, "KILL") {
			return []byte("test failed"), errors.New("exit 1")
		}
		return []byte("ok"), nil
	}
	// Build commands do not contain the mutant ID, so classify BAD by call
	// sequence and SLOW by their anchored test pattern.
	builds := 0
	runner = func(ctx context.Context, _ string, arguments []string, _ []string) ([]byte, error) {
		joined := strings.Join(arguments, " ")
		if strings.Contains(joined, "-run ^$") {
			builds++
			if builds == 3 {
				return []byte("build failed"), errors.New("exit 1")
			}
			return []byte("build ok"), nil
		}
		if strings.Contains(joined, "SLOW") {
			<-ctx.Done()
			return nil, ctx.Err()
		}
		if strings.Contains(joined, "KILL") {
			return []byte("test failed"), errors.New("exit 1")
		}
		return []byte("test ok"), nil
	}
	report, err := Run(t.Context(), Config{Root: root, ManifestPath: manifest, OutputDir: output, Timeout: time.Millisecond, runCommand: runner})
	if err != nil {
		t.Fatal(err)
	}
	want := []Status{StatusKilled, StatusSurvived, StatusInvalid, StatusInvalid}
	var got []Status
	for _, result := range report.Results {
		got = append(got, result.Status)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("statuses = %v, want %v", got, want)
	}
	if len(CriticalFailures(report)) != 3 {
		t.Fatalf("critical failures = %#v", CriticalFailures(report))
	}
	for _, name := range []string{"results.json", "results.md", "logs/KILL.log", "logs/LIVE.log", "logs/BAD.log", "logs/SLOW.log"} {
		if _, err := os.Stat(filepath.Join(output, filepath.FromSlash(name))); err != nil {
			t.Fatalf("artifact %s: %v", name, err)
		}
	}
}

func TestWriteReportDeterministicOrder(t *testing.T) {
	report := Report{Schema: 1, Results: []Result{{ID: "B", SourceOrder: 2}, {ID: "A", SourceOrder: 1}}}
	first := t.TempDir()
	second := t.TempDir()
	if err := WriteReport(first, report); err != nil {
		t.Fatal(err)
	}
	if err := WriteReport(second, report); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"results.json", "results.md"} {
		left, _ := os.ReadFile(filepath.Join(first, name))
		right, _ := os.ReadFile(filepath.Join(second, name))
		if !reflect.DeepEqual(left, right) {
			t.Fatalf("%s is not deterministic", name)
		}
	}
}

func fixtureModule(t *testing.T, source string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "shellvalidate"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module fixture\n\ngo 1.25\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.sum"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "shellvalidate", "sample.go"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func writeJSON(t *testing.T, filename string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, data, 0o644); err != nil {
		t.Fatal(err)
	}
}
