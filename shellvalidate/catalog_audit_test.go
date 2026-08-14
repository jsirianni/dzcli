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

type catalogBase struct {
	ID        string   `json:"id"`
	Status    string   `json:"status"`
	Reference string   `json:"reference,omitempty"`
	Choice    string   `json:"choice,omitempty"`
	Rationale string   `json:"rationale,omitempty"`
	Tests     []string `json:"tests"`
}

type lexicalEntry struct {
	catalogBase
}

type grammarEntry struct {
	catalogBase
	Dialects     []string `json:"dialects"`
	Alternatives []string `json:"alternatives"`
	Optional     []string `json:"optional,omitempty"`
	Repetition   bool     `json:"repetition,omitempty"`
	Cardinality  []string `json:"cardinality,omitempty"`
}

type dialectEntry struct {
	catalogBase
	Accepted []string `json:"accepted,omitempty"`
	Rejected []string `json:"rejected,omitempty"`
}

type semanticEntry struct {
	catalogBase
	States []string `json:"states"`
}

type robustnessEntry struct {
	catalogBase
	Bound      int      `json:"bound"`
	Boundaries []string `json:"boundaries"`
}

type configurationEntry struct {
	catalogBase
	Dialect string `json:"dialect"`
}

type contractCatalog struct {
	ModelVersion   string               `json:"modelVersion"`
	Baselines      []baseline           `json:"baselines"`
	Configurations []configurationEntry `json:"configurations"`
	Statuses       map[string]string    `json:"statuses"`
	Input          inputContract        `json:"input"`
	Observable     observableContract   `json:"observable"`
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

func decodeStrict[T any](data []byte, target *T) error {
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

func loadStrictCatalog[T any](filename string) ([]T, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	var entries []T
	if err := decodeStrict(data, &entries); err != nil {
		return nil, fmt.Errorf("decode %s: %w", filename, err)
	}
	return entries, nil
}

func validateBase(kind string, base catalogBase) []string {
	var problems []string
	prefix := kind + " " + base.ID
	if strings.TrimSpace(base.ID) == "" {
		problems = append(problems, kind+": missing id")
	}
	if !containsString(allowedBehaviorStatuses, base.Status) {
		problems = append(problems, prefix+": unknown status "+base.Status)
	}
	if base.Status == "unimplemented" {
		problems = append(problems, prefix+": release catalog contains unimplemented behavior")
	}
	if len(base.Tests) == 0 {
		problems = append(problems, prefix+": no deterministic test mapping")
	}
	problems = append(problems, duplicateProblems(prefix+" tests", base.Tests)...)
	for _, test := range base.Tests {
		if !strings.HasPrefix(test, "Test") {
			problems = append(problems, prefix+": invalid test mapping "+test)
		}
	}
	switch base.Status {
	case "required", "extension":
		if strings.TrimSpace(base.Reference) == "" {
			problems = append(problems, prefix+": status requires a source reference")
		}
		if base.Choice != "" || base.Rationale != "" {
			problems = append(problems, prefix+": exact status has contradictory choice or rationale")
		}
	case "implementation-defined":
		if strings.TrimSpace(base.Reference) == "" || strings.TrimSpace(base.Choice) == "" {
			problems = append(problems, prefix+": implementation-defined status requires reference and choice")
		}
	case "unspecified", "undefined":
		if strings.TrimSpace(base.Reference) == "" || strings.TrimSpace(base.Rationale) == "" {
			problems = append(problems, prefix+": status requires reference and conservative rationale")
		}
		if base.Choice != "" {
			problems = append(problems, prefix+": status must not fabricate an implementation choice")
		}
	case "dynamic-unknown":
		if strings.TrimSpace(base.Rationale) == "" {
			problems = append(problems, prefix+": dynamic-unknown status requires an exactness rationale")
		}
	case "unsupported", "not-applicable":
		if strings.TrimSpace(base.Rationale) == "" {
			problems = append(problems, prefix+": "+base.Status+" status requires a rationale")
		}
	}
	return problems
}

func validateLexical(entries []lexicalEntry) []string {
	return validateTypedEntries("lexical", entries, func(entry lexicalEntry) catalogBase { return entry.catalogBase }, nil)
}

func validateGrammar(entries []grammarEntry) []string {
	return validateTypedEntries("grammar", entries, func(entry grammarEntry) catalogBase { return entry.catalogBase }, func(entry grammarEntry) []string {
		prefix := "grammar " + entry.ID
		var problems []string
		if len(entry.Dialects) == 0 {
			problems = append(problems, prefix+": missing dialect")
		}
		for _, dialect := range entry.Dialects {
			if dialect != "posix" && dialect != "bash" {
				problems = append(problems, prefix+": invalid dialect "+dialect)
			}
		}
		problems = append(problems, duplicateProblems(prefix+" dialects", entry.Dialects)...)
		if len(entry.Alternatives) == 0 {
			problems = append(problems, prefix+": production has no alternatives")
		}
		problems = append(problems, duplicateProblems(prefix+" alternatives", entry.Alternatives)...)
		for _, optional := range entry.Optional {
			if !containsString(entry.Alternatives, optional) {
				problems = append(problems, prefix+": unknown optional alternative "+optional)
			}
		}
		if entry.Repetition && !sameStringSet(entry.Cardinality, []string{"zero", "one", "many"}) {
			problems = append(problems, prefix+": repeated production must map zero, one, and many cases")
		}
		if !entry.Repetition && len(entry.Cardinality) != 0 {
			problems = append(problems, prefix+": non-repeated production declares cardinality cases")
		}
		return problems
	})
}

func validateDialects(entries []dialectEntry) []string {
	return validateTypedEntries("dialect", entries, func(entry dialectEntry) catalogBase { return entry.catalogBase }, func(entry dialectEntry) []string {
		prefix := "dialect " + entry.ID
		var problems []string
		problems = append(problems, duplicateProblems(prefix+" accepted", entry.Accepted)...)
		problems = append(problems, duplicateProblems(prefix+" rejected", entry.Rejected)...)
		if entry.ID == "dialect.posix" && len(entry.Rejected) == 0 {
			problems = append(problems, prefix+": POSIX entry has no rejected Bash feature")
		}
		if entry.Status != "unsupported" && len(entry.Accepted) == 0 {
			problems = append(problems, prefix+": entry has no accepted feature")
		}
		return problems
	})
}

func validateSemantics(entries []semanticEntry) []string {
	return validateTypedEntries("semantic", entries, func(entry semanticEntry) catalogBase { return entry.catalogBase }, func(entry semanticEntry) []string {
		prefix := "semantic " + entry.ID
		var problems []string
		if len(entry.States) < 2 {
			problems = append(problems, prefix+": concept has fewer than two states")
		}
		problems = append(problems, duplicateProblems(prefix+" states", entry.States)...)
		if entry.Status == "dynamic-unknown" && (!containsString(entry.States, "exact") || !containsString(entry.States, "inexact")) {
			problems = append(problems, prefix+": dynamic-unknown concept must map exact and inexact states")
		}
		return problems
	})
}

func validateRobustness(entries []robustnessEntry) []string {
	problems := validateTypedEntries("robustness", entries, func(entry robustnessEntry) catalogBase { return entry.catalogBase }, func(entry robustnessEntry) []string {
		prefix := "robustness " + entry.ID
		var result []string
		if len(entry.Boundaries) == 0 {
			result = append(result, prefix+": behavior has no boundaries")
		}
		result = append(result, duplicateProblems(prefix+" boundaries", entry.Boundaries)...)
		return result
	})
	wantBounds := map[string]int{
		"robustness.input-size":   maxSourceBytes,
		"robustness.nesting":      maxNesting,
		"robustness.diagnostics":  defaultMaxDiagnostics,
		"robustness.source-depth": maxSourceDepth,
	}
	for _, entry := range entries {
		if want, ok := wantBounds[entry.ID]; ok && entry.Bound != want {
			problems = append(problems, fmt.Sprintf("robustness %s: bound=%d want=%d", entry.ID, entry.Bound, want))
		}
	}
	sort.Strings(problems)
	return problems
}

func validateConfigurations(entries []configurationEntry) []string {
	return validateTypedEntries("configuration", entries, func(entry configurationEntry) catalogBase { return entry.catalogBase }, func(entry configurationEntry) []string {
		if entry.Dialect != "auto" && entry.Dialect != "posix" && entry.Dialect != "bash" {
			return []string{"configuration " + entry.ID + ": invalid dialect " + entry.Dialect}
		}
		return nil
	})
}

func validateTypedEntries[T any](kind string, entries []T, base func(T) catalogBase, extra func(T) []string) []string {
	var problems []string
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		item := base(entry)
		if _, exists := seen[item.ID]; exists {
			problems = append(problems, kind+": duplicate id "+item.ID)
		}
		seen[item.ID] = struct{}{}
		problems = append(problems, validateBase(kind, item)...)
		if extra != nil {
			problems = append(problems, extra(entry)...)
		}
	}
	sort.Strings(problems)
	return problems
}

func duplicateProblems(prefix string, values []string) []string {
	seen := make(map[string]struct{}, len(values))
	var problems []string
	for _, value := range values {
		if value == "" {
			problems = append(problems, prefix+": empty value")
			continue
		}
		if _, exists := seen[value]; exists {
			problems = append(problems, prefix+": duplicate "+value)
		}
		seen[value] = struct{}{}
	}
	return problems
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func sameStringSet(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for _, value := range right {
		if !containsString(left, value) {
			return false
		}
	}
	return len(duplicateProblems("set", left)) == 0
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
			if !ok || function.Recv != nil || !strings.HasPrefix(function.Name.Name, "Test") {
				continue
			}
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
	return result
}

func validateMappings(t *testing.T, kind string, entries []catalogBase) {
	t.Helper()
	knownTests := catalogTestFunctions(t)
	for _, entry := range entries {
		for _, mapping := range entry.Tests {
			if _, ok := knownTests[mapping]; !ok {
				t.Errorf("%s %s maps to missing test %s", kind, entry.ID, mapping)
			}
		}
	}
}

func bases[T any](entries []T, base func(T) catalogBase) []catalogBase {
	result := make([]catalogBase, 0, len(entries))
	for _, entry := range entries {
		result = append(result, base(entry))
	}
	return result
}

func TestSpecificationCatalogAudit(t *testing.T) {
	data, err := os.ReadFile("testdata/spec/contract.json")
	if err != nil {
		t.Fatal(err)
	}
	var contract contractCatalog
	if err := decodeStrict(data, &contract); err != nil {
		t.Fatal(err)
	}
	if problems := validateContract(contract); len(problems) != 0 {
		t.Fatalf("contract audit:\n%s", strings.Join(problems, "\n"))
	}
	if problems := validateConfigurations(contract.Configurations); len(problems) != 0 {
		t.Fatalf("configuration audit:\n%s", strings.Join(problems, "\n"))
	}
	validateMappings(t, "configuration", bases(contract.Configurations, func(entry configurationEntry) catalogBase { return entry.catalogBase }))

	lexical := mustLoadCatalog[lexicalEntry](t, "lexical")
	grammar := mustLoadCatalog[grammarEntry](t, "grammar")
	dialects := mustLoadCatalog[dialectEntry](t, "dialects")
	semantics := mustLoadCatalog[semanticEntry](t, "semantics")
	robustness := mustLoadCatalog[robustnessEntry](t, "robustness")
	for _, item := range []struct {
		name     string
		problems []string
		entries  []catalogBase
	}{
		{"lexical", validateLexical(lexical), bases(lexical, func(entry lexicalEntry) catalogBase { return entry.catalogBase })},
		{"grammar", validateGrammar(grammar), bases(grammar, func(entry grammarEntry) catalogBase { return entry.catalogBase })},
		{"dialects", validateDialects(dialects), bases(dialects, func(entry dialectEntry) catalogBase { return entry.catalogBase })},
		{"semantics", validateSemantics(semantics), bases(semantics, func(entry semanticEntry) catalogBase { return entry.catalogBase })},
		{"robustness", validateRobustness(robustness), bases(robustness, func(entry robustnessEntry) catalogBase { return entry.catalogBase })},
	} {
		if len(item.problems) != 0 {
			t.Fatalf("%s audit:\n%s", item.name, strings.Join(item.problems, "\n"))
		}
		validateMappings(t, item.name, item.entries)
	}
}

func validateContract(contract contractCatalog) []string {
	var problems []string
	if contract.ModelVersion == "" {
		problems = append(problems, "missing model version")
	}
	if len(contract.Baselines) != 2 || contract.Baselines[0].Name != "POSIX.1-2024" || contract.Baselines[0].Section != "Shell Command Language" || contract.Baselines[1].Name != "Bash" || contract.Baselines[1].Version != "5.3" || contract.Baselines[1].Section != "Bash Reference Manual" {
		problems = append(problems, "baseline versions or sections changed")
	}
	wantLimits := inputContract{
		ArbitraryBytes: true, InvalidUTF8: "preserve", NUL: "diagnostic-SHS1003",
		MaxSourceBytes: maxSourceBytes, MaxNesting: maxNesting,
		DefaultMaxDiagnostics: defaultMaxDiagnostics, MaxSourceDepth: maxSourceDepth,
		MaxFixedPointIterations: maxDataflowIterations,
	}
	if contract.Input.ArbitraryBytes != wantLimits.ArbitraryBytes || contract.Input.InvalidUTF8 != wantLimits.InvalidUTF8 || contract.Input.NUL != wantLimits.NUL || contract.Input.MaxSourceBytes != wantLimits.MaxSourceBytes || contract.Input.MaxNesting != wantLimits.MaxNesting || contract.Input.DefaultMaxDiagnostics != wantLimits.DefaultMaxDiagnostics || contract.Input.MaxSourceDepth != wantLimits.MaxSourceDepth || contract.Input.MaxFixedPointIterations != wantLimits.MaxFixedPointIterations {
		problems = append(problems, "input contract disagrees with production constants")
	}
	if !sameStringSet(contract.Input.LineEndings, []string{"LF", "CRLF", "no-final-newline"}) || contract.Input.Positions != "byte offsets with one-based line and byte column" || contract.Input.Spans != "half-open" || contract.Input.MaxRecoveryActions != "bounded by token progress" {
		problems = append(problems, "input position, span, line-ending, or recovery contract is incomplete")
	}
	if contract.Observable.ChildOrdering == "" || !sameStringSet(contract.Observable.DiagnosticOrdering, []string{"start-offset", "end-offset", "severity", "code"}) || contract.Observable.IncompleteNodes == "" || contract.Observable.DynamicUnknown == "" || contract.Observable.ResourceLimits == "" || contract.Observable.ParallelSafety == "" {
		problems = append(problems, "observable contract is incomplete")
	}
	for _, status := range allowedBehaviorStatuses {
		if strings.TrimSpace(contract.Statuses[status]) == "" {
			problems = append(problems, "contract omits status "+status)
		}
	}
	sort.Strings(problems)
	return problems
}

func mustLoadCatalog[T any](t *testing.T, name string) []T {
	t.Helper()
	entries, err := loadStrictCatalog[T](filepath.Join("testdata", "spec", name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	return entries
}

func TestSpecificationCatalogRejectsUnknownFields(t *testing.T) {
	var lexical []lexicalEntry
	if err := decodeStrict([]byte(`[{"id":"x","status":"required","reference":"r","tests":["TestX"],"mystery":true}]`), &lexical); err == nil {
		t.Fatal("unknown catalog field accepted")
	}
	var contract contractCatalog
	if err := decodeStrict([]byte(`{"modelVersion":"v","baselines":[],"configurations":[],"statuses":{},"input":{},"observable":{},"mystery":true}`), &contract); err == nil {
		t.Fatal("unknown contract field accepted")
	}
}

func TestSpecificationCatalogAuditRejectsDefects(t *testing.T) {
	base := catalogBase{ID: "grammar.sample", Status: "required", Reference: "POSIX sample", Tests: []string{"TestSample"}}
	tests := []struct {
		name    string
		entries []grammarEntry
	}{
		{name: "duplicate id", entries: []grammarEntry{{catalogBase: base, Dialects: []string{"posix"}, Alternatives: []string{"one"}}, {catalogBase: base, Dialects: []string{"posix"}, Alternatives: []string{"one"}}}},
		{name: "missing id", entries: []grammarEntry{{catalogBase: catalogBase{Status: "required", Reference: "POSIX", Tests: []string{"TestSample"}}, Dialects: []string{"posix"}, Alternatives: []string{"one"}}}},
		{name: "missing dialect", entries: []grammarEntry{{catalogBase: base, Alternatives: []string{"one"}}}},
		{name: "duplicate alternatives", entries: []grammarEntry{{catalogBase: base, Dialects: []string{"posix"}, Alternatives: []string{"one", "one"}}}},
		{name: "unknown optional", entries: []grammarEntry{{catalogBase: base, Dialects: []string{"posix"}, Alternatives: []string{"one"}, Optional: []string{"two"}}}},
		{name: "repetition without cardinality", entries: []grammarEntry{{catalogBase: base, Dialects: []string{"posix"}, Alternatives: []string{"one"}, Repetition: true}}},
		{name: "unknown status", entries: []grammarEntry{{catalogBase: catalogBase{ID: "x", Status: "maybe", Reference: "POSIX", Tests: []string{"TestSample"}}, Dialects: []string{"posix"}, Alternatives: []string{"one"}}}},
		{name: "unsupported without rationale", entries: []grammarEntry{{catalogBase: catalogBase{ID: "x", Status: "unsupported", Tests: []string{"TestSample"}}, Dialects: []string{"posix"}, Alternatives: []string{"one"}}}},
	}
	for _, item := range tests {
		t.Run(item.name, func(t *testing.T) {
			if problems := validateGrammar(item.entries); len(problems) == 0 {
				t.Fatal("broken catalog passed audit")
			}
		})
	}
}

func TestSpecificationCatalogRejectsStatusEvidenceDefects(t *testing.T) {
	tests := []struct {
		name string
		base catalogBase
	}{
		{name: "required without reference", base: catalogBase{ID: "x", Status: "required", Tests: []string{"TestSample"}}},
		{name: "extension with rationale", base: catalogBase{ID: "x", Status: "extension", Reference: "Bash", Rationale: "contradiction", Tests: []string{"TestSample"}}},
		{name: "implementation defined without choice", base: catalogBase{ID: "x", Status: "implementation-defined", Reference: "package", Tests: []string{"TestSample"}}},
		{name: "unspecified without rationale", base: catalogBase{ID: "x", Status: "unspecified", Reference: "POSIX", Tests: []string{"TestSample"}}},
		{name: "undefined with fabricated choice", base: catalogBase{ID: "x", Status: "undefined", Reference: "POSIX", Choice: "always succeeds", Rationale: "conservative", Tests: []string{"TestSample"}}},
		{name: "dynamic unknown without rationale", base: catalogBase{ID: "x", Status: "dynamic-unknown", Tests: []string{"TestSample"}}},
		{name: "unimplemented release entry", base: catalogBase{ID: "x", Status: "unimplemented", Tests: []string{"TestSample"}}},
	}
	for _, item := range tests {
		t.Run(item.name, func(t *testing.T) {
			if problems := validateBase("sample", item.base); len(problems) == 0 {
				t.Fatal("invalid status evidence passed audit")
			}
		})
	}

	dynamic := semanticEntry{catalogBase: catalogBase{ID: "semantic.dynamic", Status: "dynamic-unknown", Rationale: "runtime data is unavailable", Tests: []string{"TestSample"}}, States: []string{"known", "unknown"}}
	if problems := validateSemantics([]semanticEntry{dynamic}); len(problems) == 0 {
		t.Fatal("dynamic-unknown semantic without exact/inexact states passed audit")
	}
}

func TestSpecificationContractRejectsDrift(t *testing.T) {
	data, err := os.ReadFile("testdata/spec/contract.json")
	if err != nil {
		t.Fatal(err)
	}
	decode := func(t *testing.T) contractCatalog {
		t.Helper()
		var contract contractCatalog
		if err := decodeStrict(data, &contract); err != nil {
			t.Fatal(err)
		}
		return contract
	}
	tests := []struct {
		name   string
		mutate func(*contractCatalog)
	}{
		{name: "baseline", mutate: func(contract *contractCatalog) { contract.Baselines[0].Name = "older" }},
		{name: "position", mutate: func(contract *contractCatalog) { contract.Input.Positions = "runes" }},
		{name: "span", mutate: func(contract *contractCatalog) { contract.Input.Spans = "closed" }},
		{name: "line endings", mutate: func(contract *contractCatalog) { contract.Input.LineEndings = []string{"LF"} }},
		{name: "diagnostic ordering", mutate: func(contract *contractCatalog) { contract.Observable.DiagnosticOrdering = []string{"code"} }},
		{name: "dynamic exactness", mutate: func(contract *contractCatalog) { contract.Observable.DynamicUnknown = "" }},
		{name: "fixed point bound", mutate: func(contract *contractCatalog) { contract.Input.MaxFixedPointIterations++ }},
		{name: "status taxonomy", mutate: func(contract *contractCatalog) { delete(contract.Statuses, "undefined") }},
	}
	for _, item := range tests {
		t.Run(item.name, func(t *testing.T) {
			contract := decode(t)
			item.mutate(&contract)
			if problems := validateContract(contract); len(problems) == 0 {
				t.Fatal("contract drift passed audit")
			}
		})
	}
}

func TestSpecificationCatalogRejectsBoundDrift(t *testing.T) {
	entries := mustLoadCatalog[robustnessEntry](t, "robustness")
	entries[0].Bound++
	if problems := validateRobustness(entries); len(problems) == 0 {
		t.Fatal("changed production bound passed audit")
	}
}

func TestSpecificationCatalogRejectsMissingDialectRejection(t *testing.T) {
	entries := mustLoadCatalog[dialectEntry](t, "dialects")
	for index := range entries {
		if entries[index].ID == "dialect.posix" {
			entries[index].Rejected = nil
		}
	}
	if problems := validateDialects(entries); len(problems) == 0 {
		t.Fatal("POSIX catalog without rejected Bash behavior passed audit")
	}
}

func TestUnsupportedConfigurationCatalog(t *testing.T) {
	entries := mustLoadCatalog[dialectEntry](t, "dialects")
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

func TestHostIndependenceContract(t *testing.T) {
	file, diagnostics, err := Parse(`opaque\windows/name.sh`, []byte("printf '%s\\n' ok\n"), DialectPOSIX)
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("parse depends on host state: %v %#v", err, diagnostics)
	}
	if file.Filename() != `opaque\windows/name.sh` {
		t.Fatalf("filename normalized through host: %q", file.Filename())
	}

	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	set := gotoken.NewFileSet()
	for _, filename := range files {
		if strings.HasSuffix(filename, "_test.go") || strings.HasPrefix(filename, "doc") {
			continue
		}
		parsed, parseErr := parser.ParseFile(set, filename, nil, parser.ImportsOnly)
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		for _, spec := range parsed.Imports {
			path, unquoteErr := strconv.Unquote(spec.Path.Value)
			if unquoteErr != nil {
				t.Fatal(unquoteErr)
			}
			if path == "os" || path == "path/filepath" {
				t.Errorf("production file %s imports host-dependent package %q", filename, path)
			}
		}
	}
}
