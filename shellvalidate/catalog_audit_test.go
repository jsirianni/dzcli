package shellvalidate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	gotoken "go/token"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

type auditEntry struct {
	ID           string   `json:"id"`
	Status       string   `json:"status"`
	Reference    string   `json:"reference"`
	Choice       string   `json:"choice"`
	Rationale    string   `json:"rationale"`
	Tests        []string `json:"tests"`
	Alternatives []string `json:"alternatives"`
	Optional     []string `json:"optional"`
	States       []string `json:"states"`
	Boundaries   []string `json:"boundaries"`
	Accepted     []string `json:"accepted"`
	Rejected     []string `json:"rejected"`
	Dialects     []string `json:"dialects"`
	Repetition   bool     `json:"repetition"`
	Bound        int      `json:"bound"`
	Dialect      string   `json:"dialect"`
}

type contractCatalog struct {
	ModelVersion   string             `json:"modelVersion"`
	Baselines      []baseline         `json:"baselines"`
	Configurations []auditEntry       `json:"configurations"`
	Statuses       map[string]string  `json:"statuses"`
	Input          inputContract      `json:"input"`
	Observable     observableContract `json:"observable"`
}

type baseline struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
	Section string `json:"section"`
}

type inputContract struct {
	ArbitraryBytes          bool     `json:"arbitraryBytes"`
	InvalidUTF8             string   `json:"invalidUTF8"`
	NUL                     string   `json:"nul"`
	LineEndings             []string `json:"lineEndings"`
	Positions               string   `json:"positions"`
	Spans                   string   `json:"spans"`
	MaxSourceBytes          int      `json:"maxSourceBytes"`
	MaxNesting              int      `json:"maxNesting"`
	DefaultMaxDiagnostics   int      `json:"defaultMaxDiagnostics"`
	MaxSourceDepth          int      `json:"maxSourceDepth"`
	MaxRecoveryActions      string   `json:"maxRecoveryActions"`
	MaxFixedPointIterations int      `json:"maxFixedPointIterations"`
}

type observableContract struct {
	ChildOrdering      string   `json:"childOrdering"`
	DiagnosticOrdering []string `json:"diagnosticOrdering"`
	IncompleteNodes    string   `json:"incompleteNodes"`
	DynamicUnknown     string   `json:"dynamicUnknown"`
	ResourceLimits     string   `json:"resourceLimits"`
	ParallelSafety     string   `json:"parallelSafety"`
}

var allowedBehaviorStatuses = []string{
	"required", "extension", "implementation-defined", "unspecified", "undefined",
	"unsupported", "dynamic-unknown", "not-applicable", "unimplemented",
}

func loadAuditEntries(filename string) ([]auditEntry, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	var entries []auditEntry
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&entries); err != nil {
		return nil, fmt.Errorf("decode %s: %w", filename, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, fmt.Errorf("decode %s: trailing JSON value", filename)
	}
	return entries, nil
}

func decodeContract(data []byte, target *contractCatalog) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return fmt.Errorf("trailing JSON value")
	}
	return nil
}

func auditCatalog(name string, entries []auditEntry) []string {
	var problems []string
	seen := make(map[string]struct{})
	allowed := make(map[string]struct{}, len(allowedBehaviorStatuses))
	for _, status := range allowedBehaviorStatuses {
		allowed[status] = struct{}{}
	}
	for index, entry := range entries {
		prefix := fmt.Sprintf("%s[%d]", name, index)
		if entry.ID == "" {
			problems = append(problems, prefix+": missing id")
		} else if _, exists := seen[entry.ID]; exists {
			problems = append(problems, prefix+": duplicate id "+entry.ID)
		}
		seen[entry.ID] = struct{}{}
		if _, ok := allowed[entry.Status]; !ok {
			problems = append(problems, prefix+": unknown status "+entry.Status)
		}
		if entry.Status == "unimplemented" {
			problems = append(problems, prefix+": release catalog contains unimplemented behavior")
		}
		if entry.Status == "unsupported" || entry.Status == "not-applicable" {
			if strings.TrimSpace(entry.Rationale) == "" {
				problems = append(problems, prefix+": "+entry.Status+" behavior needs rationale")
			}
		} else if entry.Status == "implementation-defined" && strings.TrimSpace(entry.Choice) == "" {
			problems = append(problems, prefix+": implementation-defined behavior needs choice")
		} else if strings.TrimSpace(entry.Reference) == "" && name != "semantic" && name != "robustness" {
			problems = append(problems, prefix+": missing source reference")
		}
		if len(entry.Tests) == 0 {
			problems = append(problems, prefix+": no deterministic test mapping")
		}
		for _, test := range entry.Tests {
			if !strings.HasPrefix(test, "Test") {
				problems = append(problems, prefix+": invalid test mapping "+test)
			}
		}
		if name == "grammar" && len(entry.Alternatives) == 0 {
			problems = append(problems, prefix+": grammar production has no alternatives")
		}
		if name == "semantic" && entry.Status != "not-applicable" && len(entry.States) < 2 {
			problems = append(problems, prefix+": semantic concept has fewer than two states")
		}
		if name == "robustness" && len(entry.Boundaries) == 0 {
			problems = append(problems, prefix+": robustness behavior has no boundaries")
		}
	}
	sort.Strings(problems)
	return problems
}

func catalogTestFunctions(t *testing.T) map[string]struct{} {
	t.Helper()
	result := make(map[string]struct{})
	files, err := filepath.Glob("*_test.go")
	if err != nil {
		t.Fatal(err)
	}
	set := gotoken.NewFileSet()
	for _, filename := range files {
		file, parseErr := parser.ParseFile(set, filename, nil, 0)
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if ok && function.Recv == nil && strings.HasPrefix(function.Name.Name, "Test") {
				result[function.Name.Name] = struct{}{}
				ast.Inspect(function.Body, func(node ast.Node) bool {
					call, ok := node.(*ast.CallExpr)
					if !ok || len(call.Args) == 0 {
						return true
					}
					selector, ok := call.Fun.(*ast.SelectorExpr)
					name, okName := call.Args[0].(*ast.BasicLit)
					if !ok || !okName || selector.Sel.Name != "Run" || name.Kind != gotoken.STRING {
						return true
					}
					subtest, unquoteErr := strconv.Unquote(name.Value)
					if unquoteErr == nil {
						result[function.Name.Name+"/"+subtest] = struct{}{}
					}
					return true
				})
			}
		}
	}
	return result
}

func TestSpecificationCatalogAudit(t *testing.T) {
	data, err := os.ReadFile("testdata/spec/contract.json")
	if err != nil {
		t.Fatal(err)
	}
	var contract contractCatalog
	if err := decodeContract(data, &contract); err != nil {
		t.Fatal(err)
	}
	if contract.ModelVersion == "" || len(contract.Baselines) != 2 || !contract.Input.ArbitraryBytes || contract.Observable.ChildOrdering == "" || contract.Input.MaxFixedPointIterations <= 0 {
		t.Fatalf("incomplete normative contract: %#v", contract)
	}
	for _, status := range allowedBehaviorStatuses {
		if contract.Statuses[status] == "" {
			t.Errorf("contract omits status %q", status)
		}
	}
	if problems := auditCatalog("configuration", contract.Configurations); len(problems) != 0 {
		t.Fatalf("configuration audit:\n%s", strings.Join(problems, "\n"))
	}
	for _, item := range []struct{ file, kind string }{
		{file: "lexical", kind: "lexical"},
		{file: "grammar", kind: "grammar"},
		{file: "dialects", kind: "dialect"},
		{file: "semantics", kind: "semantic"},
		{file: "robustness", kind: "robustness"},
	} {
		entries, err := loadAuditEntries(filepath.Join("testdata", "spec", item.file+".json"))
		if err != nil {
			t.Fatal(err)
		}
		if problems := auditCatalog(item.kind, entries); len(problems) != 0 {
			t.Fatalf("%s audit:\n%s", item.file, strings.Join(problems, "\n"))
		}
		knownTests := catalogTestFunctions(t)
		for _, entry := range entries {
			for _, mapping := range entry.Tests {
				if _, ok := knownTests[mapping]; !ok {
					t.Errorf("%s %s maps to missing test %s", item.file, entry.ID, mapping)
				}
			}
		}
	}
}

func TestSpecificationCatalogRejectsUnknownFields(t *testing.T) {
	var entries []auditEntry
	decoder := json.NewDecoder(strings.NewReader(`[{"id":"x","status":"required","reference":"r","tests":["TestX"],"mystery":true}]`))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&entries); err == nil {
		t.Fatal("unknown catalog field accepted")
	}
	var contract contractCatalog
	if err := decodeContract([]byte(`{"modelVersion":"v","baselines":[],"configurations":[],"statuses":{},"input":{},"observable":{},"mystery":true}`), &contract); err == nil {
		t.Fatal("unknown contract field accepted")
	}
}

func TestSpecificationCatalogAuditRejectsDefects(t *testing.T) {
	base := auditEntry{ID: "grammar.sample", Status: "required", Reference: "POSIX sample", Tests: []string{"TestSample"}, Alternatives: []string{"one"}}
	tests := []struct {
		name    string
		entries []auditEntry
	}{
		{name: "duplicate id", entries: []auditEntry{base, base}},
		{name: "missing id", entries: []auditEntry{{Status: "required", Reference: "POSIX", Tests: []string{"TestSample"}, Alternatives: []string{"one"}}}},
		{name: "missing reference", entries: []auditEntry{{ID: "x", Status: "required", Tests: []string{"TestSample"}, Alternatives: []string{"one"}}}},
		{name: "unknown status", entries: []auditEntry{{ID: "x", Status: "maybe", Reference: "POSIX", Tests: []string{"TestSample"}, Alternatives: []string{"one"}}}},
		{name: "unimplemented", entries: []auditEntry{{ID: "x", Status: "unimplemented", Reference: "POSIX", Tests: []string{"TestSample"}, Alternatives: []string{"one"}}}},
		{name: "unsupported without rationale", entries: []auditEntry{{ID: "x", Status: "unsupported", Reference: "POSIX", Tests: []string{"TestSample"}, Alternatives: []string{"one"}}}},
		{name: "implementation defined without choice", entries: []auditEntry{{ID: "x", Status: "implementation-defined", Reference: "POSIX", Tests: []string{"TestSample"}, Alternatives: []string{"one"}}}},
		{name: "missing test", entries: []auditEntry{{ID: "x", Status: "required", Reference: "POSIX", Alternatives: []string{"one"}}}},
		{name: "missing alternative", entries: []auditEntry{{ID: "x", Status: "required", Reference: "POSIX", Tests: []string{"TestSample"}}}},
	}
	for _, item := range tests {
		t.Run(item.name, func(t *testing.T) {
			if problems := auditCatalog("grammar", item.entries); len(problems) == 0 {
				t.Fatal("broken catalog passed audit")
			}
		})
	}
}

func TestUnsupportedConfigurationCatalog(t *testing.T) {
	entries, err := loadAuditEntries("testdata/spec/dialects.json")
	if err != nil {
		t.Fatal(err)
	}
	unsupported := 0
	for _, entry := range entries {
		if entry.Status == "unsupported" {
			unsupported++
			if entry.Rationale == "" {
				t.Fatalf("%s has no rationale", entry.ID)
			}
		}
	}
	if unsupported != 3 {
		t.Fatalf("unsupported configurations = %d", unsupported)
	}
}

func TestNotApplicableCatalogRationale(t *testing.T) {
	entries, err := loadAuditEntries("testdata/spec/semantics.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Status == "not-applicable" && entry.Rationale == "" {
			t.Fatalf("%s has no rationale", entry.ID)
		}
	}
}

func TestUnsupportedFixedPointCatalogIsExplicit(t *testing.T) {
	entries, err := loadAuditEntries("testdata/spec/semantics.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.ID != "semantic.fixed-point" {
			continue
		}
		if entry.Status != "unsupported" || !strings.Contains(entry.Rationale, "CFG") {
			t.Fatalf("fixed-point capability is not honestly classified: %#v", entry)
		}
		return
	}
	t.Fatal("semantic.fixed-point entry is missing")
}

func TestHostIndependenceContract(t *testing.T) {
	file, diagnostics, err := Parse(filepath.Join("not", "consulted", "script.sh"), []byte("printf '%s\\n' ok\n"), DialectPOSIX)
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("parse depends on host state: %v %#v", err, diagnostics)
	}
	if file.Filename() != filepath.Join("not", "consulted", "script.sh") {
		t.Fatalf("filename normalized through host: %q", file.Filename())
	}
}
