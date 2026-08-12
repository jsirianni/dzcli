package shellvalidate

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"
)

type normalizedNode struct {
	Kind        NodeKind
	Incomplete  bool
	Words       []normalizedWord
	Children    []normalizedNode
	Expressions []normalizedExpression
}

type normalizedWord struct{ Parts []normalizedPart }
type normalizedPart struct {
	Kind  WordPartKind
	Quote QuoteKind
	Text  string
}
type normalizedExpression struct {
	Kind     ExpressionKind
	Operator string
	Value    string
	Children []normalizedExpression
}

func normalizeFile(file *File) []normalizedNode {
	var result []normalizedNode
	for _, node := range file.Nodes() {
		result = append(result, normalizeNode(node))
	}
	return result
}

func normalizeNode(node Node) normalizedNode {
	result := normalizedNode{Kind: node.Kind(), Incomplete: node.Incomplete()}
	for _, word := range node.Words() {
		item := normalizedWord{}
		for _, part := range word.Parts() {
			item.Parts = append(item.Parts, normalizedPart{Kind: part.Kind(), Quote: part.Quote(), Text: string(part.Text())})
		}
		result.Words = append(result.Words, item)
	}
	for _, child := range node.Children() {
		result.Children = append(result.Children, normalizeNode(child))
	}
	for _, expression := range node.Expressions() {
		result.Expressions = append(result.Expressions, normalizeExpression(expression))
	}
	return result
}

func normalizeExpression(expression Expression) normalizedExpression {
	result := normalizedExpression{Kind: expression.Kind(), Operator: expression.Operator(), Value: expression.Value()}
	for _, child := range expression.Children() {
		result.Children = append(result.Children, normalizeExpression(child))
	}
	return result
}

func deterministicSources(t *testing.T) [][]byte {
	t.Helper()
	result := make([][]byte, 0)
	for _, item := range loadCorpus(t) {
		data, err := osReadFile(item.ScriptPath)
		if err != nil {
			t.Fatal(err)
		}
		result = append(result, data)
	}
	result = append(result,
		[]byte("if true; then printf '%s\\n' \"$value\"; fi\n"),
		[]byte("cat <<'END'\nbody\nEND\n"),
		[]byte("[[ -n $value ]] && ((count += 1))\n"),
	)
	return result
}

var osReadFile = func(name string) ([]byte, error) { return os.ReadFile(name) }

func TestDeterministicPropertyCorpus(t *testing.T) {
	for index, source := range deterministicSources(t) {
		index, source := index, append([]byte(nil), source...)
		t.Run(fmt.Sprintf("serial-%03d", index), func(t *testing.T) {
			first, err := Check(t.Context(), "property.sh", source, Options{Dialect: DialectBash, MaxDiagnostics: 50})
			if err != nil {
				t.Fatal(err)
			}
			second, err := Check(t.Context(), "property.sh", append([]byte(nil), source...), Options{Dialect: DialectBash, MaxDiagnostics: 50})
			if err != nil || !reflect.DeepEqual(first.Diagnostics, second.Diagnostics) || !reflect.DeepEqual(normalizeFile(first.File), normalizeFile(second.File)) {
				t.Fatalf("nondeterministic result: %v", err)
			}
			assertDiagnosticBounds(t, source, first.Diagnostics)
			for _, node := range first.File.Nodes() {
				assertNodeInvariant(t, len(source), node)
			}
			if len(source) > 0 {
				before := first.File.Source()
				source[0] ^= 0xff
				if !bytes.Equal(first.File.Source(), before) {
					t.Fatal("caller mutation changed parsed file")
				}
			}
		})
	}
	for index, source := range deterministicSources(t) {
		index, source := index, append([]byte(nil), source...)
		t.Run(fmt.Sprintf("parallel-%03d", index), func(t *testing.T) {
			t.Parallel()
			result, err := Check(t.Context(), "parallel.sh", source, Options{Dialect: DialectBash, MaxDiagnostics: 50})
			if err != nil {
				t.Fatal(err)
			}
			assertDiagnosticBounds(t, source, result.Diagnostics)
		})
	}
}

func TestLexerStructuralProperties(t *testing.T) {
	for index, source := range deterministicSources(t) {
		file, _, err := Parse(fmt.Sprintf("lexer-%d.sh", index), source, DialectBash)
		if err != nil {
			t.Fatal(err)
		}
		previous := 0
		for tokenIndex, item := range file.tokens {
			if item.start < previous || item.end < item.start || item.end > len(source) {
				t.Fatalf("source %d token %d invalid: %#v", index, tokenIndex, item)
			}
			if item.kind != tokenEOF && item.end == item.start {
				t.Fatalf("source %d token %d made no progress: %#v", index, tokenIndex, item)
			}
			previous = item.end
		}
	}
}

func TestWhitespaceCommentMetamorphisms(t *testing.T) {
	variants := []string{
		"printf '%s\\n' \"$value\"\n",
		"  printf\t'%s\\n'\t\"$value\"  \n",
		"# leading\nprintf '%s\\n' \"$value\" # trailing\n",
	}
	var want []normalizedNode
	for _, source := range variants {
		file, diagnostics, err := Parse("metamorphic.sh", []byte(source), DialectPOSIX)
		if err != nil || len(diagnostics) != 0 {
			t.Fatalf("%q: %v %#v", source, err, diagnostics)
		}
		got := normalizeFile(file)
		if want == nil {
			want = got
		} else if !reflect.DeepEqual(got, want) {
			t.Fatalf("normalized AST changed for %q:\nwant %#v\ngot  %#v", source, want, got)
		}
	}

	semicolon, _, _ := Parse("separator.sh", []byte("echo one; echo two\n"), DialectPOSIX)
	newline, _, _ := Parse("separator.sh", []byte("echo one\necho two\n"), DialectPOSIX)
	if !reflect.DeepEqual(normalizeFile(semicolon), normalizeFile(newline)) {
		t.Fatal("legal semicolon/newline substitution changed normalized structure")
	}
}

type ruleDecisionCase struct {
	code       string
	trigger    []byte
	nonTrigger []byte
	dialect    Dialect
	wantText   []byte
}

func TestRuleDecisionMatrix(t *testing.T) {
	var catalog []struct {
		Code              string `json:"code"`
		DefaultSeverity   string `json:"defaultSeverity"`
		DefaultConfidence string `json:"defaultConfidence"`
	}
	ruleData, err := os.ReadFile("testdata/rules.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(ruleData, &catalog); err != nil {
		t.Fatal(err)
	}
	metadata := make(map[string]struct {
		severity   Severity
		confidence Confidence
	})
	for _, rule := range catalog {
		metadata[rule.Code] = struct {
			severity   Severity
			confidence Confidence
		}{severity: severityByName(rule.DefaultSeverity), confidence: confidenceByName(rule.DefaultConfidence)}
	}
	tests := []ruleDecisionCase{
		{code: "SHS1001", trigger: []byte("echo 'open"), nonTrigger: []byte("echo 'closed'"), dialect: DialectPOSIX, wantText: []byte("'open")},
		{code: "SHS1002", trigger: []byte("echo ${open"), nonTrigger: []byte("echo ${closed}"), dialect: DialectPOSIX, wantText: []byte("${open")},
		{code: "SHS1003", trigger: []byte{'e', 0}, nonTrigger: []byte("echo"), dialect: DialectPOSIX, wantText: []byte{0}},
		{code: "SHS1004", trigger: []byte("if true; then :"), nonTrigger: []byte("if true; then :; fi"), dialect: DialectPOSIX, wantText: []byte("if true; then :")},
		{code: "SHS1005", trigger: []byte(")"), nonTrigger: []byte(":"), dialect: DialectPOSIX, wantText: []byte(")")},
		{code: "SHS1006", trigger: []byte("cat <<END\nbody\n"), nonTrigger: []byte("cat <<END\nbody\nEND\n"), dialect: DialectPOSIX, wantText: []byte("END")},
		{code: "SHD1001", trigger: []byte("[[ x ]]"), nonTrigger: []byte("[ x ]"), dialect: DialectPOSIX, wantText: []byte("[[")},
		{code: "SHD1002", trigger: []byte("#!/usr/bin/env bash\necho ok\n"), nonTrigger: []byte("#!/bin/sh\necho ok\n"), dialect: DialectPOSIX, wantText: []byte("#!/usr/bin/env bash")},
		{code: "SHE1001", trigger: []byte("printf '%s\\n' $value\n"), nonTrigger: []byte("printf '%s\\n' \"$value\"\n"), dialect: DialectPOSIX, wantText: []byte("$value")},
		{code: "SHE1002", trigger: []byte("line=$(printf 'x\\n')\n"), nonTrigger: []byte("line=value\n"), dialect: DialectPOSIX, wantText: []byte("line=$(printf 'x\\n')")},
		{code: "SHV1001", trigger: []byte("set -u\nprintf '%s' \"$missing\"\n"), nonTrigger: []byte("set -u\nmissing=x\nprintf '%s' \"$missing\"\n"), dialect: DialectPOSIX, wantText: []byte("$missing")},
		{code: "SHV1002", trigger: []byte("printf x | read value\n"), nonTrigger: []byte("read value\n"), dialect: DialectPOSIX, wantText: []byte("read value")},
		{code: "SHC1001", trigger: []byte("break\n"), nonTrigger: []byte("while true; do break; done\n"), dialect: DialectPOSIX, wantText: []byte("break")},
		{code: "SHC1002", trigger: []byte("return\n"), nonTrigger: []byte("echo return\n"), dialect: DialectPOSIX, wantText: []byte("return")},
		{code: "SHR1001", trigger: []byte("command 2>&1 >out\n"), nonTrigger: []byte("command >out 2>&1\n"), dialect: DialectPOSIX, wantText: []byte("2>&1")},
		{code: "SHR1002", trigger: []byte("command <data >data\n"), nonTrigger: []byte("command <in >out\n"), dialect: DialectPOSIX, wantText: []byte("data")},
		{code: "SHB1001", trigger: []byte("[ -n value\n"), nonTrigger: []byte("[ -n value ]\n"), dialect: DialectPOSIX, wantText: []byte("[ -n value")},
		{code: "SHB1002", trigger: []byte("printf '%s %s' one\n"), nonTrigger: []byte("printf '%s %s' one two\n"), dialect: DialectPOSIX, wantText: []byte("'%s %s'")},
		{code: "SHP1001", trigger: []byte("echo -n value\n"), nonTrigger: []byte("printf %s value\n"), dialect: DialectPOSIX, wantText: []byte("-n")},
		{code: "SHP1002", trigger: []byte("local value=x\n"), nonTrigger: []byte("value=x\n"), dialect: DialectPOSIX, wantText: []byte("local")},
		{code: "SHX1001", trigger: []byte("eval \"$generated\"\n"), nonTrigger: []byte("eval 'echo ok'\n"), dialect: DialectPOSIX, wantText: []byte("eval \"$generated\"")},
		{code: "SHX1002", trigger: []byte("rm -rf \"$target\"\n"), nonTrigger: []byte("rm -rf ./target\n"), dialect: DialectPOSIX, wantText: []byte("\"$target\"")},
		{code: "SHI1001", trigger: []byte("eval \"$generated\"\n"), nonTrigger: []byte("eval 'echo ok'\n"), dialect: DialectPOSIX, wantText: []byte("eval \"$generated\"")},
	}
	if len(tests) != 23 {
		t.Fatalf("rule decision cases = %d", len(tests))
	}
	for _, item := range tests {
		t.Run(item.code, func(t *testing.T) {
			triggered, err := Check(t.Context(), "trigger.sh", item.trigger, Options{Dialect: item.dialect})
			if err != nil {
				t.Fatal(err)
			}
			diagnostic, ok := diagnosticByCode(triggered.Diagnostics, item.code)
			if !ok {
				t.Fatalf("trigger did not emit %s: %#v", item.code, triggered.Diagnostics)
			}
			want, declared := metadata[item.code]
			if !declared || diagnostic.Severity != want.severity || diagnostic.Confidence != want.confidence {
				t.Fatalf("metadata for %s = severity %v confidence %v; catalog=%#v", item.code, diagnostic.Severity, diagnostic.Confidence, want)
			}
			assertDiagnosticBounds(t, item.trigger, []Diagnostic{diagnostic})
			gotText := item.trigger[diagnostic.Primary.Start.Offset:diagnostic.Primary.End.Offset]
			if !bytes.Equal(gotText, item.wantText) {
				t.Fatalf("primary span text = %q, want %q", gotText, item.wantText)
			}
			notTriggered, err := Check(t.Context(), "non-trigger.sh", item.nonTrigger, Options{Dialect: item.dialect})
			if err != nil {
				t.Fatal(err)
			}
			if hasCode(notTriggered.Diagnostics, item.code) {
				t.Fatalf("nearest non-trigger emitted %s: %#v", item.code, notTriggered.Diagnostics)
			}
		})
	}
}

func severityByName(name string) Severity {
	switch name {
	case "error":
		return SeverityError
	case "warning":
		return SeverityWarning
	case "info":
		return SeverityInfo
	case "style":
		return SeverityStyle
	default:
		return 0
	}
}

func confidenceByName(name string) Confidence {
	switch name {
	case "definite":
		return ConfidenceDefinite
	case "likely":
		return ConfidenceLikely
	case "heuristic":
		return ConfidenceHeuristic
	default:
		return 0
	}
}

func diagnosticByCode(items []Diagnostic, code string) (Diagnostic, bool) {
	for _, item := range items {
		if item.Code == code {
			return item, true
		}
	}
	return Diagnostic{}, false
}

func TestSemanticTruthTables(t *testing.T) {
	t.Run("variables", func(t *testing.T) {
		for _, source := range []string{"set -u\nprintf %s \"$x\"\n", "set -u\nx=value\nprintf %s \"$x\"\n", "printf %s \"$x\"\n"} {
			_, err := Check(t.Context(), "variables.sh", []byte(source), Options{Dialect: DialectPOSIX})
			if err != nil {
				t.Fatal(err)
			}
		}
	})
	t.Run("nounset", func(t *testing.T) {
		off := checkCodes(t, "printf %s \"$missing\"\n", DialectPOSIX)
		on := checkCodes(t, "set -u\nprintf %s \"$missing\"\n", DialectPOSIX, "SHV1001")
		if hasCode(off.Diagnostics, "SHV1001") || !hasCode(on.Diagnostics, "SHV1001") {
			t.Fatal("nounset transition is not reflected")
		}
	})
	t.Run("expansion", func(t *testing.T) {
		for _, item := range []struct {
			source string
			warn   bool
		}{
			{source: "printf %s $value\n", warn: true},
			{source: "printf %s \"$value\"\n", warn: false},
			{source: "printf %s literal\n", warn: false},
		} {
			result, err := Check(t.Context(), "expansion.sh", []byte(item.source), Options{Dialect: DialectPOSIX})
			if err != nil || hasCode(result.Diagnostics, "SHE1001") != item.warn {
				t.Fatalf("%q warn=%v: %v %#v", item.source, item.warn, err, result.Diagnostics)
			}
		}
	})
	t.Run("pipeline", func(t *testing.T) {
		checkCodes(t, "printf x | read value\n", DialectPOSIX, "SHV1002")
		if result := checkCodes(t, "read value\n", DialectPOSIX); hasCode(result.Diagnostics, "SHV1002") {
			t.Fatal("non-pipeline read warned")
		}
	})
	t.Run("control", func(t *testing.T) {
		checkCodes(t, "break\n", DialectPOSIX, "SHC1001")
		checkCodes(t, "return\n", DialectPOSIX, "SHC1002")
	})
	t.Run("redirection", func(t *testing.T) {
		checkCodes(t, "command 2>&1 >out\n", DialectPOSIX, "SHR1001")
		checkCodes(t, "command <data >data\n", DialectPOSIX, "SHR1002")
	})
}

func TestCommandModelDeterminism(t *testing.T) {
	first, second := buildCommandModels(), buildCommandModels()
	if !reflect.DeepEqual(first, second) {
		t.Fatal("command model construction is nondeterministic")
	}
	names := make([]string, 0, len(first))
	for name := range first {
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) != 51 || names[0] != "." || names[len(names)-1] != "xargs" {
		t.Fatalf("command model names=%d first=%q last=%q", len(names), names[0], names[len(names)-1])
	}
}

func TestDiagnosticCapExhaustiveModel(t *testing.T) {
	for _, limit := range []int{defaultMaxDiagnostics - 1, defaultMaxDiagnostics, defaultMaxDiagnostics + 1} {
		source := []byte(strings.Repeat("break;", limit+5))
		result, err := Check(t.Context(), "cap.sh", source, Options{Dialect: DialectPOSIX, MaxDiagnostics: limit})
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Diagnostics) != limit {
			t.Fatalf("limit %d produced %d diagnostics", limit, len(result.Diagnostics))
		}
		assertDiagnosticBounds(t, source, result.Diagnostics)
	}
}

func TestResourceLimitBoundaries(t *testing.T) {
	t.Run("input-size", func(t *testing.T) {
		for _, size := range []int{maxSourceBytes - 1, maxSourceBytes, maxSourceBytes + 1} {
			source := bytes.Repeat([]byte{' '}, size)
			file, diagnostics, err := Parse("size.sh", source, DialectPOSIX)
			if err != nil || file == nil {
				t.Fatalf("size %d: %v", size, err)
			}
			if hasCode(diagnostics, "SHS1005") != (size > maxSourceBytes) {
				t.Fatalf("size %d diagnostics %#v", size, diagnostics)
			}
		}
	})
	t.Run("nesting", func(t *testing.T) {
		for _, depth := range []int{maxNesting - 1, maxNesting, maxNesting + 1} {
			source := []byte(strings.Repeat("( ", depth) + ":" + strings.Repeat(" )", depth))
			_, diagnostics, err := Parse("nesting.sh", source, DialectPOSIX)
			if err != nil {
				t.Fatal(err)
			}
			if hasCode(diagnostics, "SHS1005") != (depth > maxNesting) {
				t.Fatalf("depth %d diagnostics %#v", depth, diagnostics)
			}
		}
	})
}

func TestResolverStateModel(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		result, err := Check(t.Context(), "root.sh", []byte(". child.sh\n"), Options{Dialect: DialectPOSIX})
		if err != nil || !result.AnalysisExact {
			t.Fatalf("disabled source analysis: %v %#v", err, result)
		}
	})
	t.Run("resolved", func(t *testing.T) {
		resolver := mapResolver{files: map[string][]byte{"child.sh": []byte("echo ok\n")}}
		result, err := Check(t.Context(), "root.sh", []byte(". child.sh\n"), Options{Dialect: DialectPOSIX, AnalyzeSourced: true, Resolver: resolver})
		if err != nil || !result.AnalysisExact {
			t.Fatalf("resolved source: %v %#v", err, result)
		}
	})
	t.Run("dynamic", func(t *testing.T) {
		result := checkCodes(t, ". \"$path\"\n", DialectPOSIX, "SHI1001")
		if result.AnalysisExact {
			t.Fatal("dynamic source path reported exact")
		}
	})
	t.Run("cycle", func(t *testing.T) { TestResolverCycleIsBounded(t) })
	t.Run("depth", func(t *testing.T) {
		for _, edges := range []int{maxSourceDepth - 1, maxSourceDepth, maxSourceDepth + 1} {
			files := make(map[string][]byte)
			for index := 0; index <= edges; index++ {
				name := fmt.Sprintf("f%d.sh", index)
				if index == edges {
					files[name] = []byte("echo end\n")
				} else {
					files[name] = []byte(fmt.Sprintf(". f%d.sh\n", index+1))
				}
			}
			result, err := Check(t.Context(), "f0.sh", files["f0.sh"], Options{Dialect: DialectPOSIX, AnalyzeSourced: true, Resolver: mapResolver{files: files}})
			if err != nil {
				t.Fatal(err)
			}
			if result.AnalysisExact != (edges <= maxSourceDepth) {
				t.Fatalf("edges=%d exact=%v diagnostics=%#v", edges, result.AnalysisExact, result.Diagnostics)
			}
		}
	})
}

func TestByteMutationModel(t *testing.T) {
	base := []byte("printf '%s\\n' value\n")
	for _, value := range []byte{0, 0xff} {
		for _, offset := range []int{0, len(base) / 2, len(base)} {
			mutated := append([]byte(nil), base[:offset]...)
			mutated = append(mutated, value)
			mutated = append(mutated, base[offset:]...)
			file, diagnostics, err := Parse("bytes.sh", mutated, DialectPOSIX)
			if err != nil || file == nil || !bytes.Equal(file.Source(), mutated) {
				t.Fatalf("value=%x offset=%d: %v", value, offset, err)
			}
			assertDiagnosticBounds(t, mutated, diagnostics)
			if value == 0 && !hasCode(diagnostics, "SHS1003") {
				t.Fatalf("NUL at %d was not diagnosed", offset)
			}
		}
	}
}

func TestRecoveryMutationModel(t *testing.T) {
	bases := [][]byte{
		[]byte("if true; then echo ok; fi\n"),
		[]byte("( printf '%s\\n' value )\n"),
		[]byte("cat <<END\nbody\nEND\n"),
	}
	for baseIndex, base := range bases {
		for end := 0; end <= len(base); end++ {
			source := base[:end]
			first, firstDiagnostics, firstErr := Parse("truncated.sh", source, DialectPOSIX)
			second, secondDiagnostics, secondErr := Parse("truncated.sh", source, DialectPOSIX)
			if firstErr != nil || secondErr != nil || first == nil || second == nil || !reflect.DeepEqual(firstDiagnostics, secondDiagnostics) {
				t.Fatalf("base=%d end=%d: %v/%v", baseIndex, end, firstErr, secondErr)
			}
			assertDiagnosticBounds(t, source, firstDiagnostics)
		}
	}

	source := []byte("if true; then echo one; echo two; fi\n")
	file, _, err := Parse("delete.sh", source, DialectPOSIX)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range file.tokens {
		if item.kind == tokenEOF || item.kind == tokenComment {
			continue
		}
		mutated := append([]byte(nil), source[:item.start]...)
		mutated = append(mutated, source[item.end:]...)
		_, diagnostics, err := Parse("delete.sh", mutated, DialectPOSIX)
		if err != nil {
			t.Fatal(err)
		}
		assertDiagnosticBounds(t, mutated, diagnostics)
	}
}

type fuzzRegression struct {
	ID      string `json:"id"`
	Feature string `json:"feature"`
	Hex     string `json:"hex"`
}

func TestCommittedFuzzSeedReplay(t *testing.T) {
	data, err := os.ReadFile("testdata/spec/fuzz_regressions.json")
	if err != nil {
		t.Fatal(err)
	}
	var seeds []fuzzRegression
	if err := json.Unmarshal(data, &seeds); err != nil {
		t.Fatal(err)
	}
	if len(seeds) != len(fuzzSeeds) {
		t.Fatalf("committed seeds=%d in-memory seeds=%d", len(seeds), len(fuzzSeeds))
	}
	seen := make(map[string]struct{})
	for index, seed := range seeds {
		if seed.ID == "" || seed.Feature == "" {
			t.Fatalf("invalid seed %#v", seed)
		}
		if _, exists := seen[seed.ID]; exists {
			t.Fatalf("duplicate seed %s", seed.ID)
		}
		seen[seed.ID] = struct{}{}
		source, err := hex.DecodeString(seed.Hex)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(source, fuzzSeeds[index]) {
			t.Fatalf("seed %s differs from native fuzz seed", seed.ID)
		}
		for _, dialect := range []Dialect{DialectPOSIX, DialectBash} {
			verifyParseVector(t, deterministicVector{ID: seed.ID, Dialect: dialect, Source: string(source)})
		}
	}
}

func TestCategoryConfigurationMatrix(t *testing.T) {
	source := []byte("break; printf '%s' $value\n")
	categories := []string{"control", "expansion"}
	for mask := 0; mask < 1<<len(categories); mask++ {
		var enabled []string
		for index, category := range categories {
			if mask&(1<<index) != 0 {
				enabled = append(enabled, category)
			}
		}
		result, err := Check(t.Context(), "categories.sh", source, Options{Dialect: DialectPOSIX, EnableCategories: enabled})
		if err != nil {
			t.Fatal(err)
		}
		if len(enabled) > 0 {
			if hasCode(result.Diagnostics, "SHC1001") != slices.Contains(enabled, "control") || hasCode(result.Diagnostics, "SHE1001") != slices.Contains(enabled, "expansion") {
				t.Fatalf("enabled=%v diagnostics=%#v", enabled, result.Diagnostics)
			}
		}
	}
}
