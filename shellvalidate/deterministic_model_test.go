package shellvalidate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
)

const deterministicModelVersion = "shellvalidate-generated-v1"

type deterministicVector struct {
	ID      string  `json:"id"`
	Dialect Dialect `json:"dialect"`
	Source  string  `json:"source"`
}

func modelFingerprint(vectors []deterministicVector) string {
	hash := sha256.New()
	for _, vector := range vectors {
		data, err := json.Marshal(vector)
		if err != nil {
			panic(err)
		}
		_, _ = hash.Write(data)
		_, _ = hash.Write([]byte{'\n'})
	}
	return hex.EncodeToString(hash.Sum(nil))
}

type generatedModelManifest struct {
	ModelVersion string `json:"modelVersion"`
	Models       []struct {
		ID     string         `json:"id"`
		Count  int            `json:"count"`
		SHA256 string         `json:"sha256"`
		Bounds map[string]int `json:"bounds,omitempty"`
	} `json:"models"`
}

func expectedGeneratedModel(t *testing.T, name string) (int, string) {
	t.Helper()
	data, err := os.ReadFile("testdata/spec/generated_models.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest generatedModelManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.ModelVersion != deterministicModelVersion {
		t.Fatalf("manifest version = %q", manifest.ModelVersion)
	}
	for _, model := range manifest.Models {
		if model.ID == name {
			return model.Count, model.SHA256
		}
	}
	t.Fatalf("model %q missing from manifest", name)
	return 0, ""

}

func TestGeneratedModelManifest(t *testing.T) {
	data, err := os.ReadFile("testdata/spec/generated_models.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest generatedModelManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.ModelVersion != deterministicModelVersion {
		t.Fatalf("model version = %q", manifest.ModelVersion)
	}
	previous, total := "", 0
	for _, model := range manifest.Models {
		if model.ID <= previous {
			t.Fatalf("model IDs are not unique and sorted: %q after %q", model.ID, previous)
		}
		if model.Count <= 0 {
			t.Fatalf("model %s count = %d", model.ID, model.Count)
		}
		digest, err := hex.DecodeString(model.SHA256)
		if err != nil || len(digest) != sha256.Size {
			t.Fatalf("model %s has invalid SHA-256 %q", model.ID, model.SHA256)
		}
		previous = model.ID
		total += model.Count
	}
	if len(manifest.Models) != 9 || total != 285 {
		t.Fatalf("manifest models=%d vectors=%d", len(manifest.Models), total)
	}
}

func assertModelFingerprint(t *testing.T, name string, vectors []deterministicVector) {
	t.Helper()
	count, fingerprint := expectedGeneratedModel(t, name)
	if len(vectors) != count || modelFingerprint(vectors) != fingerprint {
		t.Fatalf("model=%s/%s count=%d hash=%s", deterministicModelVersion, name, len(vectors), modelFingerprint(vectors))
	}
}

func verifyParseVector(t *testing.T, vector deterministicVector) {
	t.Helper()
	firstFile, firstDiagnostics, firstErr := Parse(vector.ID, []byte(vector.Source), vector.Dialect)
	secondFile, secondDiagnostics, secondErr := Parse(vector.ID, []byte(vector.Source), vector.Dialect)
	if (firstErr == nil) != (secondErr == nil) || !reflect.DeepEqual(firstDiagnostics, secondDiagnostics) {
		t.Fatalf("nondeterministic parse: %v/%v %#v/%#v", firstErr, secondErr, firstDiagnostics, secondDiagnostics)
	}
	if firstFile != nil && secondFile != nil && !reflect.DeepEqual(firstFile.Nodes(), secondFile.Nodes()) {
		t.Fatal("nondeterministic AST")
	}
	assertDiagnosticBounds(t, []byte(vector.Source), firstDiagnostics)
}

func TestOperatorExhaustiveModel(t *testing.T) {
	operators := append([]string(nil), shellOperators...)
	sort.Strings(operators)
	var vectors []deterministicVector
	for _, dialect := range []Dialect{DialectPOSIX, DialectBash} {
		for _, operator := range operators {
			vectors = append(vectors, deterministicVector{ID: fmt.Sprintf("operator/%d/%s", dialect, operator), Dialect: dialect, Source: operator})
			got := operatorAt([]byte(operator+"suffix"), 0, dialect)
			if got != operator {
				t.Errorf("operatorAt(%q, %d) = %q", operator, dialect, got)
			}
			for length := 1; length < len(operator); length++ {
				prefix := operator[:length]
				got := operatorAt([]byte(prefix), 0, dialect)
				for _, candidate := range shellOperators {
					if strings.HasPrefix(prefix, candidate) && len(candidate) > len(got) {
						t.Fatalf("prefix %q selected %q instead of maximal %q", prefix, got, candidate)
					}
				}
			}
		}
	}
	assertModelFingerprint(t, "operators", vectors)
}

func TestLexicalBoundedModel(t *testing.T) {
	t.Run("backslash", func(t *testing.T) {
		for _, source := range []string{"echo a\\ b\n", "echo a\\\nb\n", "echo \\#not-comment\n"} {
			verifyParseVector(t, deterministicVector{ID: "backslash", Dialect: DialectPOSIX, Source: source})
		}
	})
	t.Run("quotes", func(t *testing.T) {
		tests := []struct {
			source string
			quote  QuoteKind
		}{
			{source: "echo plain\n", quote: QuoteUnquoted},
			{source: "echo 'single'\n", quote: QuoteSingle},
			{source: "echo \"double\"\n", quote: QuoteDouble},
			{source: "echo $'ansi'\n", quote: QuoteANSIC},
			{source: "echo $\"locale\"\n", quote: QuoteLocale},
		}
		for _, item := range tests {
			file, diagnostics, err := Parse("quote.sh", []byte(item.source), DialectBash)
			if err != nil || len(diagnostics) != 0 {
				t.Fatalf("%q: %v %#v", item.source, err, diagnostics)
			}
			parts := file.Nodes()[0].Words()[1].Parts()
			if len(parts) != 1 || parts[0].Quote() != item.quote {
				t.Fatalf("%q quote parts = %#v", item.source, parts)
			}
		}
	})
	t.Run("expansions", func(t *testing.T) {
		tests := []struct {
			source string
			kind   WordPartKind
		}{
			{source: "echo ${name}\n", kind: WordParameterExpansion},
			{source: "echo $(date)\n", kind: WordCommandSubstitution},
			{source: "echo $((1+2))\n", kind: WordArithmeticExpansion},
			{source: "echo <(date)\n", kind: WordProcessSubstitution},
		}
		for _, item := range tests {
			file, diagnostics, err := Parse("expansion.sh", []byte(item.source), DialectBash)
			if err != nil || len(diagnostics) != 0 {
				t.Fatalf("%q: %v %#v", item.source, err, diagnostics)
			}
			parts := file.Nodes()[0].Words()[1].Parts()
			if len(parts) != 1 || parts[0].Kind() != item.kind {
				t.Fatalf("%q parts = %#v", item.source, parts)
			}
		}
	})
}

func TestDelimiterDeletionModel(t *testing.T) {
	vectors := []deterministicVector{
		{ID: "single-quote", Dialect: DialectPOSIX, Source: "echo 'x"},
		{ID: "double-quote", Dialect: DialectPOSIX, Source: "echo \"x"},
		{ID: "parameter", Dialect: DialectPOSIX, Source: "echo ${x"},
		{ID: "command", Dialect: DialectPOSIX, Source: "echo $(date"},
		{ID: "subshell", Dialect: DialectPOSIX, Source: "( echo x"},
		{ID: "brace", Dialect: DialectPOSIX, Source: "{ echo x"},
		{ID: "if", Dialect: DialectPOSIX, Source: "if true; then :"},
		{ID: "arithmetic", Dialect: DialectBash, Source: "((1+2"},
		{ID: "conditional", Dialect: DialectBash, Source: "[[ -n x"},
		{ID: "heredoc", Dialect: DialectPOSIX, Source: "cat <<END\nbody\n"},
	}
	for _, vector := range vectors {
		t.Run(vector.ID, func(t *testing.T) {
			file, diagnostics, err := Parse(vector.ID, []byte(vector.Source), vector.Dialect)
			if err != nil {
				t.Fatal(err)
			}
			if file == nil || len(diagnostics) == 0 || file.syntaxValid {
				t.Fatalf("missing-closer accepted: file=%#v diagnostics=%#v", file, diagnostics)
			}
			assertDiagnosticBounds(t, []byte(vector.Source), diagnostics)
		})
	}
	assertModelFingerprint(t, "delimiter-deletion", vectors)
}

func TestSeparatorExhaustiveModel(t *testing.T) {
	separators := []string{"\n", ";", "&", "&&", "||", "|", "|&", ";;", ";&", ";;&"}
	var vectors []deterministicVector
	for _, separator := range separators {
		dialect := DialectPOSIX
		if separator == "|&" || separator == ";&" || separator == ";;&" {
			dialect = DialectBash
		}
		vector := deterministicVector{ID: "separator/" + fmt.Sprintf("%x", separator), Dialect: dialect, Source: "left" + separator + "right\n"}
		vectors = append(vectors, vector)
		verifyParseVector(t, vector)
	}
	assertModelFingerprint(t, "separators", vectors)
}

func TestRedirectionExhaustiveModel(t *testing.T) {
	operators := []string{"<", ">", ">>", "<<", "<<-", "<<<", "<&", ">&", "<>", ">|", "&>", "&>>"}
	var vectors []deterministicVector
	for _, operator := range operators {
		dialect := DialectPOSIX
		operand := "file"
		if operator == "<<<" || operator == "&>" || operator == "&>>" {
			dialect = DialectBash
		}
		if operator == "<<" || operator == "<<-" {
			operand = "END\nbody\nEND"
		}
		vector := deterministicVector{ID: "redirect/" + operator, Dialect: dialect, Source: "command " + operator + operand + "\n"}
		vectors = append(vectors, vector)
		file, diagnostics, err := Parse(vector.ID, []byte(vector.Source), dialect)
		if err != nil || file == nil || !file.syntaxValid || len(diagnostics) != 0 {
			t.Fatalf("%s: %v %#v", operator, err, diagnostics)
		}
	}
	assertModelFingerprint(t, "redirections", vectors)
}

func TestHereDocumentExhaustiveModel(t *testing.T) {
	delimiters := []string{"END", "'END'", "\"END\""}
	strips := []bool{false, true}
	var vectors []deterministicVector
	for _, delimiter := range delimiters {
		for _, strip := range strips {
			operator, indent := "<<", ""
			if strip {
				operator, indent = "<<-", "\t"
			}
			source := fmt.Sprintf("cat %s%s\n%sbody\n%sEND\n", operator, delimiter, indent, indent)
			vector := deterministicVector{ID: fmt.Sprintf("heredoc/%s/%v", delimiter, strip), Dialect: DialectPOSIX, Source: source}
			vectors = append(vectors, vector)
			file, diagnostics, err := Parse(vector.ID, []byte(source), DialectPOSIX)
			if err != nil || file == nil || !file.syntaxValid || len(diagnostics) != 0 {
				t.Fatalf("%s: %v %#v", vector.ID, err, diagnostics)
			}
		}
	}
	assertModelFingerprint(t, "heredocs", vectors)
}

func TestArithmeticExhaustiveModel(t *testing.T) {
	operators := make([]string, 0, len(arithmeticPrecedence))
	for operator := range arithmeticPrecedence {
		if operator != "?" {
			operators = append(operators, operator)
		}
	}
	sort.Strings(operators)
	var vectors []deterministicVector
	for _, operator := range operators {
		source := "((left " + operator + " right))\n"
		vector := deterministicVector{ID: "arithmetic/" + operator, Dialect: DialectBash, Source: source}
		vectors = append(vectors, vector)
		file, diagnostics, err := Parse(vector.ID, []byte(source), DialectBash)
		if err != nil || len(diagnostics) != 0 {
			t.Fatalf("%s: %v %#v", operator, err, diagnostics)
		}
		expressions := expressionsForKind(file, NodeArithmetic)
		if len(expressions) != 1 || expressions[0].Operator() != operator {
			t.Fatalf("%s expression = %#v", operator, expressions)
		}
	}
	assertModelFingerprint(t, "arithmetic", vectors)

	assertExpressionAssociation(t, "((a-b-c))\n", "-", false)
	assertExpressionAssociation(t, "((a=b=c))\n", "=", true)
	assertExpressionAssociation(t, "((a**b**c))\n", "**", true)
}

func assertExpressionAssociation(t *testing.T, source, operator string, right bool) {
	t.Helper()
	file, diagnostics, err := Parse("association.sh", []byte(source), DialectBash)
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("%q: %v %#v", source, err, diagnostics)
	}
	expressions := expressionsForKind(file, NodeArithmetic)
	if len(expressions) != 1 {
		t.Fatalf("%q expressions = %#v", source, expressions)
	}
	root := expressions[0]
	children := root.Children()
	index := 0
	if right {
		index = 1
	}
	if root.Operator() != operator || len(children) != 2 || children[index].Operator() != operator {
		t.Fatalf("%q association = %#v", source, root)
	}
}

func TestConditionalExhaustiveModel(t *testing.T) {
	operators := []string{"==", "!=", "=~", "<", ">", "&&", "||"}
	var vectors []deterministicVector
	for _, operator := range operators {
		source := "[[ left " + operator + " right ]]\n"
		vector := deterministicVector{ID: "conditional/" + operator, Dialect: DialectBash, Source: source}
		vectors = append(vectors, vector)
		file, diagnostics, err := Parse(vector.ID, []byte(source), DialectBash)
		if err != nil || len(diagnostics) != 0 {
			t.Fatalf("%s: %v %#v", operator, err, diagnostics)
		}
		expressions := expressionsForKind(file, NodeConditional)
		if len(expressions) != 1 || expressions[0].Operator() != operator {
			t.Fatalf("%s expression = %#v", operator, expressions)
		}
	}
	assertModelFingerprint(t, "conditional", vectors)
}

func expressionsForKind(file *File, kind NodeKind) []Expression {
	for _, node := range file.Nodes() {
		if node.Kind() == kind {
			return node.Expressions()
		}
	}
	return nil
}

func TestGrammarHandAuthoredMatrix(t *testing.T) {
	tests := map[string][]string{
		"complete-command": {"echo ok\n", "if true; then :; fi\n"},
		"pipeline":         {"left | right\n", "! left | right\n"},
		"simple-command":   {"echo ok\n", "name=value\n", "echo ok >file\n"},
		"subshell":         {"( echo ok )\n"},
		"brace-group":      {"{ echo ok; }\n"},
		"if":               {"if true; then :; fi\n", "if true; then :; else :; fi\n", "if true; then :; elif false; then :; fi\n"},
		"loop":             {"for x in a b; do :; done\n", "while true; do :; done\n", "until false; do :; done\n"},
		"case":             {"case x in esac\n", "case x in x) :;; esac\n", "case x in x) :;; y) :;; esac\n"},
	}
	names := make([]string, 0, len(tests))
	for name := range tests {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			for index, source := range tests[name] {
				file, diagnostics, err := Parse(fmt.Sprintf("%s-%d.sh", name, index), []byte(source), DialectPOSIX)
				if err != nil || file == nil || !file.syntaxValid || len(diagnostics) != 0 {
					t.Fatalf("%q: %v %#v", source, err, diagnostics)
				}
			}
		})
	}
}

func TestAutoDialectModel(t *testing.T) {
	tests := []struct {
		source string
		want   Dialect
	}{
		{source: "echo ok\n", want: DialectPOSIX},
		{source: "#!/bin/sh\necho ok\n", want: DialectPOSIX},
		{source: "#!/usr/bin/env bash\necho ok\n", want: DialectBash},
		{source: "#!/unknown/shell\necho ok\n", want: DialectPOSIX},
	}
	for _, item := range tests {
		file, _, err := Parse("auto.sh", []byte(item.source), DialectAuto)
		if err != nil || file.Dialect() != item.want {
			t.Fatalf("%q dialect = %v, err = %v", item.source, file.Dialect(), err)
		}
	}
}

func TestDialectInteractionModel(t *testing.T) {
	features := []struct {
		name   string
		source string
	}{
		{name: "arithmetic", source: "((x += 1))\n"},
		{name: "conditional", source: "[[ -n x ]]\n"},
		{name: "process-substitution", source: "cat <(printf x)\n"},
		{name: "select", source: "select x in a; do :; done\n"},
		{name: "coproc", source: "coproc echo x\n"},
	}
	for _, feature := range features {
		t.Run(feature.name, func(t *testing.T) {
			bash, err := Check(t.Context(), feature.name, []byte(feature.source), Options{Dialect: DialectBash})
			if err != nil || !bash.SyntaxValid || hasCode(bash.Diagnostics, "SHD1001") {
				t.Fatalf("bash: %v %#v", err, bash.Diagnostics)
			}
			posix, err := Check(t.Context(), feature.name, []byte(feature.source), Options{Dialect: DialectPOSIX})
			if err != nil || !hasCode(posix.Diagnostics, "SHD1001") {
				t.Fatalf("POSIX contrast: %v %#v", err, posix.Diagnostics)
			}
		})
	}
}

type interactionValue struct {
	Dimension string `json:"dimension"`
	Value     string `json:"value"`
}

type interactionVector []interactionValue

func allInteractionVectors(dimensions [][]string) []interactionVector {
	var result []interactionVector
	var visit func(int, interactionVector)
	visit = func(index int, current interactionVector) {
		if index == len(dimensions) {
			result = append(result, append(interactionVector(nil), current...))
			return
		}
		for _, value := range dimensions[index] {
			visit(index+1, append(current, interactionValue{Dimension: fmt.Sprintf("d%d", index), Value: value}))
		}
	}
	visit(0, nil)
	return result
}

func pairKeys(vector interactionVector) []string {
	var result []string
	for left := 0; left < len(vector); left++ {
		for right := left + 1; right < len(vector); right++ {
			result = append(result, fmt.Sprintf("%d=%s|%d=%s", left, vector[left].Value, right, vector[right].Value))
		}
	}
	return result
}

func deterministicPairwise(dimensions [][]string) []interactionVector {
	candidates := allInteractionVectors(dimensions)
	uncovered := make(map[string]struct{})
	for _, candidate := range candidates {
		for _, key := range pairKeys(candidate) {
			uncovered[key] = struct{}{}
		}
	}
	var selected []interactionVector
	for len(uncovered) > 0 {
		best, bestScore := -1, -1
		for index, candidate := range candidates {
			score := 0
			for _, key := range pairKeys(candidate) {
				if _, ok := uncovered[key]; ok {
					score++
				}
			}
			if score > bestScore {
				best, bestScore = index, score
			}
		}
		if best < 0 || bestScore == 0 {
			panic("pairwise generator made no progress")
		}
		selected = append(selected, candidates[best])
		for _, key := range pairKeys(candidates[best]) {
			delete(uncovered, key)
		}
		candidates = append(candidates[:best], candidates[best+1:]...)
	}
	return selected
}

func TestDeterministicPairwiseModel(t *testing.T) {
	dimensions := [][]string{
		{"bash", "posix"},
		{"double", "single", "unquoted"},
		{"arithmetic", "command", "literal", "parameter"},
		{"argument", "assignment", "redirection"},
	}
	vectors := deterministicPairwise(dimensions)
	data, err := json.Marshal(vectors)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(data)
	count, fingerprint := expectedGeneratedModel(t, "pairwise")
	if len(vectors) != count || hex.EncodeToString(hash[:]) != fingerprint {
		t.Fatalf("model=%s pairwise-count=%d hash=%x", deterministicModelVersion, len(vectors), hash)
	}
	required := make(map[string]struct{})
	for _, vector := range allInteractionVectors(dimensions) {
		for _, key := range pairKeys(vector) {
			required[key] = struct{}{}
		}
	}
	for _, vector := range vectors {
		for _, key := range pairKeys(vector) {
			delete(required, key)
		}
	}
	if len(required) != 0 {
		t.Fatalf("uncovered pairwise tuples: %#v", required)
	}
}

func TestBoundedRecursiveModel(t *testing.T) {
	const (
		maxCompoundDepth  = 3
		maxPipelineLength = 3
		maxWordParts      = 4
		maxRedirections   = 2
	)
	wordParts := []string{"a", "${b}", "$(printf x)", "$((1+2))"}
	var vectors []deterministicVector
	for depth := 0; depth <= maxCompoundDepth; depth++ {
		for pipeline := 1; pipeline <= maxPipelineLength; pipeline++ {
			for partCount := 1; partCount <= maxWordParts; partCount++ {
				for redirections := 0; redirections <= maxRedirections; redirections++ {
					command := "printf '%s\\n' " + strings.Join(wordParts[:partCount], "")
					commands := make([]string, pipeline)
					for index := range commands {
						commands[index] = command
					}
					source := strings.Repeat("( ", depth) + strings.Join(commands, " | ")
					for index := 0; index < redirections; index++ {
						source += fmt.Sprintf(" %d>out%d", index+3, index)
					}
					source += strings.Repeat(" )", depth) + "\n"
					vector := deterministicVector{
						ID:      fmt.Sprintf("recursive/d%d/p%d/w%d/r%d", depth, pipeline, partCount, redirections),
						Dialect: DialectPOSIX,
						Source:  source,
					}
					vectors = append(vectors, vector)
					file, diagnostics, err := Parse(vector.ID, []byte(source), DialectPOSIX)
					if err != nil || file == nil || !file.syntaxValid || len(diagnostics) != 0 {
						t.Fatalf("%s: %v %#v", vector.ID, err, diagnostics)
					}
				}
			}
		}
	}
	assertModelFingerprint(t, "bounded-recursive", vectors)
}
