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
		result = append(result, propertyNormalizeNode(node))
	}
	return result
}

func propertyNormalizeNode(node Node) normalizedNode {
	result := normalizedNode{Kind: node.Kind(), Incomplete: node.Incomplete()}
	for _, word := range node.Words() {
		item := normalizedWord{}
		for _, part := range word.Parts() {
			item.Parts = append(item.Parts, normalizedPart{Kind: part.Kind(), Quote: part.Quote(), Text: string(part.Text())})
		}
		result.Words = append(result.Words, item)
	}
	for _, child := range node.Children() {
		result.Children = append(result.Children, propertyNormalizeNode(child))
	}
	for _, expression := range node.Expressions() {
		result.Expressions = append(result.Expressions, propertyNormalizeExpression(expression))
	}
	return result
}

func propertyNormalizeExpression(expression Expression) normalizedExpression {
	result := normalizedExpression{Kind: expression.Kind(), Operator: expression.Operator(), Value: expression.Value()}
	for _, child := range expression.Children() {
		result.Children = append(result.Children, propertyNormalizeExpression(child))
	}
	return result
}

type deterministicPropertyCase struct {
	ID       string
	Filename string
	Source   []byte
	Options  Options
}

func deterministicCases(t *testing.T) []deterministicPropertyCase {
	t.Helper()
	result := make([]deterministicPropertyCase, 0)
	for _, item := range loadCorpus(t) {
		data, err := osReadFile(item.ScriptPath)
		if err != nil {
			t.Fatal(err)
		}
		dialect := DialectPOSIX
		if item.Dialect == "bash" {
			dialect = DialectBash
		}
		result = append(result, deterministicPropertyCase{
			ID: item.ID, Filename: item.ScriptPath, Source: data,
			Options: Options{Dialect: dialect, MaxDiagnostics: 50},
		})
	}
	result = append(result,
		deterministicPropertyCase{ID: "property.posix.if", Filename: "property-posix-if.sh", Source: []byte("if true; then printf '%s\\n' \"$value\"; fi\n"), Options: Options{Dialect: DialectPOSIX, MaxDiagnostics: 50}},
		deterministicPropertyCase{ID: "property.posix.heredoc", Filename: "property-posix-heredoc.sh", Source: []byte("cat <<'END'\nbody\nEND\n"), Options: Options{Dialect: DialectPOSIX, MaxDiagnostics: 50}},
		deterministicPropertyCase{ID: "property.bash.compound", Filename: "property-bash-compound.sh", Source: []byte("[[ -n $value ]] && ((count += 1))\n"), Options: Options{Dialect: DialectBash, MaxDiagnostics: 50}},
	)
	return result
}

var osReadFile = func(name string) ([]byte, error) { return os.ReadFile(name) }

func TestDeterministicPropertyCorpus(t *testing.T) {
	cases := deterministicCases(t)
	baselines := make([]Result, len(cases))
	for index, item := range cases {
		index, item := index, item
		t.Run("serial-"+item.ID, func(t *testing.T) {
			source := append([]byte(nil), item.Source...)
			first, err := Check(t.Context(), item.Filename, source, item.Options)
			if err != nil {
				t.Fatal(err)
			}
			second, err := Check(t.Context(), item.Filename, append([]byte(nil), source...), item.Options)
			if err != nil || !reflect.DeepEqual(first, second) {
				t.Fatalf("nondeterministic full result: %v\nfirst=%#v\nsecond=%#v", err, first, second)
			}
			baselines[index] = first
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
	for index, item := range cases {
		index, item := index, item
		t.Run("parallel-"+item.ID, func(t *testing.T) {
			t.Parallel()
			result, err := Check(t.Context(), item.Filename, append([]byte(nil), item.Source...), item.Options)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(result, baselines[index]) {
				t.Fatalf("parallel result differs from serial baseline\nwant=%#v\ngot=%#v", baselines[index], result)
			}
			assertDiagnosticBounds(t, item.Source, result.Diagnostics)
		})
	}
}

func TestLexerStructuralProperties(t *testing.T) {
	for index, item := range deterministicCases(t) {
		source := item.Source
		file, _, err := Parse(fmt.Sprintf("lexer-%d.sh", index), source, item.Options.Dialect)
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
		accounted := make([]bool, len(source))
		for _, token := range file.tokens {
			for offset := token.start; offset < token.end; offset++ {
				accounted[offset] = true
			}
		}
		markHereDocumentBytes(file.Nodes(), accounted)
		for offset, value := range source {
			if accounted[offset] || value == 0 || value == ' ' || value == '\t' || value == '\r' || value == '\n' {
				continue
			}
			t.Fatalf("source %d byte %d (%#x) was not classified by the lexer", index, offset, value)
		}
	}
}

func markHereDocumentBytes(nodes []Node, accounted []bool) {
	for _, node := range nodes {
		for _, redirection := range node.Redirections() {
			document := redirection.HereDocument()
			if document == nil {
				continue
			}
			for _, span := range []Span{document.BodySpan(), document.TerminatorSpan()} {
				for offset := span.Start.Offset; offset < span.End.Offset && offset < len(accounted); offset++ {
					accounted[offset] = true
				}
			}
		}
		markHereDocumentBytes(node.Children(), accounted)
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
	id            string
	source        string
	dialect       Dialect
	analysisExact bool
	want          []ruleDiagnosticExpectation
}

type ruleDiagnosticExpectation struct {
	code       string
	message    string
	spanText   string
	occurrence int
}

type ruleDecisionSpec struct {
	id          string
	code        string
	category    string
	triggers    []ruleDecisionCase
	nonTriggers []ruleDecisionCase
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
	w := func(code, message, span string) ruleDiagnosticExpectation {
		return ruleDiagnosticExpectation{code: code, message: message, spanText: span}
	}
	specs := []ruleDecisionSpec{
		{id: "rule.shs1001", code: "SHS1001", category: "syntax", triggers: []ruleDecisionCase{
			{id: "rule.shs1001.trigger.single", source: "echo 'open", dialect: DialectPOSIX, analysisExact: true, want: []ruleDiagnosticExpectation{w("SHS1001", "quoted text is not terminated", "'open")}},
			{id: "rule.shs1001.trigger.double-interaction", source: "printf x; echo \"open", dialect: DialectPOSIX, analysisExact: true, want: []ruleDiagnosticExpectation{w("SHS1001", "quoted text is not terminated", "\"open")}},
		}, nonTriggers: []ruleDecisionCase{{id: "rule.shs1001.nearest.single", source: "echo 'closed'", dialect: DialectPOSIX, analysisExact: true}, {id: "rule.shs1001.nearest.double", source: "echo \"closed\"", dialect: DialectPOSIX, analysisExact: true}}},
		{id: "rule.shs1002", code: "SHS1002", category: "syntax", triggers: []ruleDecisionCase{
			{id: "rule.shs1002.trigger.parameter", source: "echo ${open", dialect: DialectPOSIX, analysisExact: true, want: []ruleDiagnosticExpectation{w("SHS1002", "expansion delimiter is not terminated", "${open"), w("SHE1001", "unquoted expansion may split into multiple arguments or expand path patterns", "${open")}},
			{id: "rule.shs1002.trigger.command-interaction", source: "printf x; echo $(open", dialect: DialectPOSIX, analysisExact: true, want: []ruleDiagnosticExpectation{w("SHS1002", "expansion delimiter is not terminated", "$(open"), w("SHE1001", "unquoted expansion may split into multiple arguments or expand path patterns", "$(open")}},
		}, nonTriggers: []ruleDecisionCase{{id: "rule.shs1002.nearest.parameter", source: "echo ${closed}", dialect: DialectPOSIX, analysisExact: true, want: []ruleDiagnosticExpectation{w("SHE1001", "unquoted expansion may split into multiple arguments or expand path patterns", "${closed}")}}, {id: "rule.shs1002.nearest.command", source: "echo $(open)", dialect: DialectPOSIX, analysisExact: true, want: []ruleDiagnosticExpectation{w("SHE1001", "unquoted expansion may split into multiple arguments or expand path patterns", "$(open)")}}}},
		{id: "rule.shs1003", code: "SHS1003", category: "syntax", triggers: []ruleDecisionCase{
			{id: "rule.shs1003.trigger.bare", source: "e\x00", dialect: DialectPOSIX, analysisExact: true, want: []ruleDiagnosticExpectation{w("SHS1003", "source contains a NUL byte", "\x00")}},
			{id: "rule.shs1003.trigger.quoted-interaction", source: "printf '%s' \"a\x00b\"", dialect: DialectPOSIX, analysisExact: true, want: []ruleDiagnosticExpectation{w("SHS1003", "source contains a NUL byte", "\x00")}},
		}, nonTriggers: []ruleDecisionCase{{id: "rule.shs1003.nearest.empty", source: "e", dialect: DialectPOSIX, analysisExact: true}, {id: "rule.shs1003.nearest.quoted", source: "printf '%s' \"ab\"", dialect: DialectPOSIX, analysisExact: true}}},
		{id: "rule.shs1004", code: "SHS1004", category: "syntax", triggers: []ruleDecisionCase{
			{id: "rule.shs1004.trigger.if", source: "if true; then :", dialect: DialectPOSIX, analysisExact: false, want: []ruleDiagnosticExpectation{w("SHS1004", "compound command is missing closing fi", "if true; then :"), w("SHI1001", "syntax recovery excluded incomplete commands from analysis", "if true; then :")}},
			{id: "rule.shs1004.trigger.loop-interaction", source: "while true; do printf x", dialect: DialectPOSIX, analysisExact: false, want: []ruleDiagnosticExpectation{w("SHS1004", "compound command is missing closing done", "while true; do printf x"), w("SHI1001", "syntax recovery excluded incomplete commands from analysis", "while true; do printf x")}},
		}, nonTriggers: []ruleDecisionCase{{id: "rule.shs1004.nearest.if", source: "if true; then :; fi", dialect: DialectPOSIX, analysisExact: true}, {id: "rule.shs1004.nearest.loop", source: "while true; do printf x; done", dialect: DialectPOSIX, analysisExact: true}}},
		{id: "rule.shs1005", code: "SHS1005", category: "syntax", triggers: []ruleDecisionCase{
			{id: "rule.shs1005.trigger.paren", source: ")", dialect: DialectPOSIX, analysisExact: true, want: []ruleDiagnosticExpectation{w("SHS1005", "unexpected token )", ")")}},
			{id: "rule.shs1005.trigger.brace-interaction", source: "printf x; }", dialect: DialectPOSIX, analysisExact: true, want: []ruleDiagnosticExpectation{w("SHS1005", "unexpected token }", "}")}},
		}, nonTriggers: []ruleDecisionCase{{id: "rule.shs1005.nearest.subshell", source: "(:)", dialect: DialectPOSIX, analysisExact: true}, {id: "rule.shs1005.nearest.group", source: "{ :; }", dialect: DialectPOSIX, analysisExact: true}}},
		{id: "rule.shs1006", code: "SHS1006", category: "syntax", triggers: []ruleDecisionCase{
			{id: "rule.shs1006.trigger.end", source: "cat <<END\nbody\n", dialect: DialectPOSIX, analysisExact: true, want: []ruleDiagnosticExpectation{w("SHS1006", "here-document terminator is missing", "END")}},
			{id: "rule.shs1006.trigger.quoted-interaction", source: "printf x; cat <<'STOP'\nbody\n", dialect: DialectPOSIX, analysisExact: true, want: []ruleDiagnosticExpectation{w("SHS1006", "here-document terminator is missing", "'STOP'")}},
		}, nonTriggers: []ruleDecisionCase{{id: "rule.shs1006.nearest.end", source: "cat <<END\nbody\nEND\n", dialect: DialectPOSIX, analysisExact: true}, {id: "rule.shs1006.nearest.quoted", source: "cat <<'STOP'\nbody\nSTOP\n", dialect: DialectPOSIX, analysisExact: true}}},
		{id: "rule.shd1001", code: "SHD1001", category: "dialect", triggers: []ruleDecisionCase{
			{id: "rule.shd1001.trigger.conditional", source: "[[ x ]]", dialect: DialectPOSIX, analysisExact: true, want: []ruleDiagnosticExpectation{w("SHD1001", "Bash-only syntax is not valid in POSIX shell", "[["), {code: "SHD1001", message: "Bash-only syntax is not valid in POSIX shell", spanText: "]]", occurrence: 0}}},
			{id: "rule.shd1001.trigger.here-string-interaction", source: "printf x; cat <<<value", dialect: DialectPOSIX, analysisExact: true, want: []ruleDiagnosticExpectation{w("SHD1001", "Bash-only syntax is not valid in POSIX shell", "<<<")}},
		}, nonTriggers: []ruleDecisionCase{{id: "rule.shd1001.nearest.test", source: "[ x ]", dialect: DialectPOSIX, analysisExact: true}, {id: "rule.shd1001.nearest.heredoc", source: "cat <<END\nvalue\nEND\n", dialect: DialectPOSIX, analysisExact: true}}},
		{id: "rule.shd1002", code: "SHD1002", category: "dialect", triggers: []ruleDecisionCase{
			{id: "rule.shd1002.trigger.bash-shebang", source: "#!/usr/bin/env bash\necho ok\n", dialect: DialectPOSIX, analysisExact: true, want: []ruleDiagnosticExpectation{w("SHD1002", "interpreter directive disagrees with the selected shell dialect", "#!/usr/bin/env bash")}},
			{id: "rule.shd1002.trigger.sh-shebang", source: "#!/bin/sh\necho ok\n", dialect: DialectBash, analysisExact: true, want: []ruleDiagnosticExpectation{w("SHD1002", "interpreter directive disagrees with the selected shell dialect", "#!/bin/sh")}},
		}, nonTriggers: []ruleDecisionCase{{id: "rule.shd1002.nearest.posix", source: "#!/bin/sh\necho ok\n", dialect: DialectPOSIX, analysisExact: true}, {id: "rule.shd1002.nearest.bash", source: "#!/usr/bin/env bash\necho ok\n", dialect: DialectBash, analysisExact: true}}},
		{id: "rule.she1001", code: "SHE1001", category: "expansion", triggers: []ruleDecisionCase{
			{id: "rule.she1001.trigger.simple", source: "printf '%s\\n' $value\n", dialect: DialectPOSIX, analysisExact: true, want: []ruleDiagnosticExpectation{w("SHE1001", "unquoted expansion may split into multiple arguments or expand path patterns", "$value")}},
			{id: "rule.she1001.trigger.braced-interaction", source: "if true; then printf %s ${value}; fi\n", dialect: DialectPOSIX, analysisExact: true, want: []ruleDiagnosticExpectation{w("SHE1001", "unquoted expansion may split into multiple arguments or expand path patterns", "${value}")}},
		}, nonTriggers: []ruleDecisionCase{{id: "rule.she1001.nearest.quoted", source: "printf '%s\\n' \"$value\"\n", dialect: DialectPOSIX, analysisExact: true}, {id: "rule.she1001.nearest.literal", source: "printf '%s\\n' value\n", dialect: DialectPOSIX, analysisExact: true}}},
		{id: "rule.she1002", code: "SHE1002", category: "expansion", triggers: []ruleDecisionCase{
			{id: "rule.she1002.trigger.printf", source: "line=$(printf 'x\\n')\n", dialect: DialectPOSIX, analysisExact: true, want: []ruleDiagnosticExpectation{w("SHE1002", "command substitution removes trailing newline bytes from this value", "line=$(printf 'x\\n')")}},
			{id: "rule.she1002.trigger.echo-interaction", source: "before=x\nvalue=$(echo 'y\\n')\n", dialect: DialectPOSIX, analysisExact: true, want: []ruleDiagnosticExpectation{w("SHE1002", "command substitution removes trailing newline bytes from this value", "value=$(echo 'y\\n')")}},
		}, nonTriggers: []ruleDecisionCase{{id: "rule.she1002.nearest.literal", source: "line=value\n", dialect: DialectPOSIX, analysisExact: true}, {id: "rule.she1002.nearest.substitution", source: "line=$(printf x)\n", dialect: DialectPOSIX, analysisExact: true}}},
		{id: "rule.shv1001", code: "SHV1001", category: "variables", triggers: []ruleDecisionCase{
			{id: "rule.shv1001.trigger.argument", source: "set -u\nprintf '%s' \"$missing\"\n", dialect: DialectPOSIX, analysisExact: true, want: []ruleDiagnosticExpectation{w("SHV1001", "variable is read while nounset is enabled and no assignment is visible", "$missing")}},
			{id: "rule.shv1001.trigger.assignment-order", source: "set -u\ncopy=$missing\n", dialect: DialectPOSIX, analysisExact: true, want: []ruleDiagnosticExpectation{w("SHV1001", "variable is read while nounset is enabled and no assignment is visible", "$missing")}},
		}, nonTriggers: []ruleDecisionCase{{id: "rule.shv1001.nearest.assigned", source: "set -u\nmissing=x\nprintf '%s' \"$missing\"\n", dialect: DialectPOSIX, analysisExact: true}, {id: "rule.shv1001.nearest.nounset-off", source: "printf '%s' \"$missing\"\n", dialect: DialectPOSIX, analysisExact: true}}},
		{id: "rule.shv1002", code: "SHV1002", category: "variables", triggers: []ruleDecisionCase{
			{id: "rule.shv1002.trigger.input", source: "printf x | read value\n", dialect: DialectPOSIX, analysisExact: true, want: []ruleDiagnosticExpectation{w("SHV1002", "read runs in a pipeline context, so assigned variables may not reach the parent shell", "read value")}},
			{id: "rule.shv1002.trigger.output", source: "read value | printf x\n", dialect: DialectPOSIX, analysisExact: true, want: []ruleDiagnosticExpectation{w("SHV1002", "read runs in a pipeline context, so assigned variables may not reach the parent shell", "read value")}},
		}, nonTriggers: []ruleDecisionCase{{id: "rule.shv1002.nearest.outside", source: "read value\n", dialect: DialectPOSIX, analysisExact: true}, {id: "rule.shv1002.nearest.no-read", source: "printf x | cat\n", dialect: DialectPOSIX, analysisExact: true}}},
		{id: "rule.shc1001", code: "SHC1001", category: "control", triggers: []ruleDecisionCase{
			{id: "rule.shc1001.trigger.break", source: "break\n", dialect: DialectPOSIX, analysisExact: true, want: []ruleDiagnosticExpectation{w("SHC1001", "break is only valid inside a loop", "break")}},
			{id: "rule.shc1001.trigger.continue-interaction", source: "printf x; continue\n", dialect: DialectPOSIX, analysisExact: true, want: []ruleDiagnosticExpectation{w("SHC1001", "continue is only valid inside a loop", "continue")}},
		}, nonTriggers: []ruleDecisionCase{{id: "rule.shc1001.nearest.break-loop", source: "while true; do break; done\n", dialect: DialectPOSIX, analysisExact: true}, {id: "rule.shc1001.nearest.continue-loop", source: "for x in y; do continue; done\n", dialect: DialectPOSIX, analysisExact: true}}},
		{id: "rule.shc1002", code: "SHC1002", category: "control", triggers: []ruleDecisionCase{
			{id: "rule.shc1002.trigger.bare", source: "return\n", dialect: DialectPOSIX, analysisExact: true, want: []ruleDiagnosticExpectation{w("SHC1002", "return is only valid in a function or sourced file", "return")}},
			{id: "rule.shc1002.trigger.status-interaction", source: "printf x; return 1\n", dialect: DialectPOSIX, analysisExact: true, want: []ruleDiagnosticExpectation{w("SHC1002", "return is only valid in a function or sourced file", "return")}},
		}, nonTriggers: []ruleDecisionCase{{id: "rule.shc1002.nearest.function", source: "f() { return; }\n", dialect: DialectPOSIX, analysisExact: true}, {id: "rule.shc1002.nearest.argument", source: "printf '%s' return\n", dialect: DialectPOSIX, analysisExact: true}}},
		{id: "rule.shr1001", code: "SHR1001", category: "redirection", triggers: []ruleDecisionCase{
			{id: "rule.shr1001.trigger.truncate", source: "command 2>&1 >out\n", dialect: DialectPOSIX, analysisExact: true, want: []ruleDiagnosticExpectation{w("SHR1001", "standard error is duplicated before standard output is redirected", "2>&1")}},
			{id: "rule.shr1001.trigger.append-interaction", source: "printf x; command 2>&1 >>out\n", dialect: DialectPOSIX, analysisExact: true, want: []ruleDiagnosticExpectation{w("SHR1001", "standard error is duplicated before standard output is redirected", "2>&1")}},
		}, nonTriggers: []ruleDecisionCase{{id: "rule.shr1001.nearest.ordered", source: "command >out 2>&1\n", dialect: DialectPOSIX, analysisExact: true}, {id: "rule.shr1001.nearest.dup-only", source: "command 2>&1\n", dialect: DialectPOSIX, analysisExact: true}}},
		{id: "rule.shr1002", code: "SHR1002", category: "redirection", triggers: []ruleDecisionCase{
			{id: "rule.shr1002.trigger.bare", source: "command <data >data\n", dialect: DialectPOSIX, analysisExact: true, want: []ruleDiagnosticExpectation{{code: "SHR1002", message: "input file is also opened for truncating output", spanText: "data", occurrence: 1}}},
			{id: "rule.shr1002.trigger.quoted-interaction", source: "printf x; command <'same' >'same'\n", dialect: DialectPOSIX, analysisExact: true, want: []ruleDiagnosticExpectation{{code: "SHR1002", message: "input file is also opened for truncating output", spanText: "'same'", occurrence: 1}}},
		}, nonTriggers: []ruleDecisionCase{{id: "rule.shr1002.nearest.paths", source: "command <in >out\n", dialect: DialectPOSIX, analysisExact: true}, {id: "rule.shr1002.nearest.append", source: "command <data >>data\n", dialect: DialectPOSIX, analysisExact: true}}},
		{id: "rule.shb1001", code: "SHB1001", category: "commands", triggers: []ruleDecisionCase{
			{id: "rule.shb1001.trigger.expression", source: "[ -n value\n", dialect: DialectPOSIX, analysisExact: true, want: []ruleDiagnosticExpectation{w("SHB1001", "[ command is missing its closing ] argument", "[ -n value")}},
			{id: "rule.shb1001.trigger.empty-interaction", source: "printf x; [\n", dialect: DialectPOSIX, analysisExact: true, want: []ruleDiagnosticExpectation{w("SHB1001", "[ command is missing its closing ] argument", "[")}},
		}, nonTriggers: []ruleDecisionCase{{id: "rule.shb1001.nearest.expression", source: "[ -n value ]\n", dialect: DialectPOSIX, analysisExact: true}, {id: "rule.shb1001.nearest.empty", source: "[ ]\n", dialect: DialectPOSIX, analysisExact: true}}},
		{id: "rule.shb1002", code: "SHB1002", category: "commands", triggers: []ruleDecisionCase{
			{id: "rule.shb1002.trigger.two", source: "printf '%s %s' one\n", dialect: DialectPOSIX, analysisExact: true, want: []ruleDiagnosticExpectation{w("SHB1002", "printf format requires more arguments than the command supplies", "'%s %s'")}},
			{id: "rule.shb1002.trigger.three-interaction", source: "printf x; printf '%s:%s:%s' one two\n", dialect: DialectPOSIX, analysisExact: true, want: []ruleDiagnosticExpectation{w("SHB1002", "printf format requires more arguments than the command supplies", "'%s:%s:%s'")}},
		}, nonTriggers: []ruleDecisionCase{{id: "rule.shb1002.nearest.two", source: "printf '%s %s' one two\n", dialect: DialectPOSIX, analysisExact: true}, {id: "rule.shb1002.nearest.percent", source: "printf '%% %s' one\n", dialect: DialectPOSIX, analysisExact: true}}},
		{id: "rule.shp1001", code: "SHP1001", category: "portability", triggers: []ruleDecisionCase{
			{id: "rule.shp1001.trigger.n", source: "echo -n value\n", dialect: DialectPOSIX, analysisExact: true, want: []ruleDiagnosticExpectation{w("SHP1001", "echo option behavior is not portable; use printf for controlled output", "-n")}},
			{id: "rule.shp1001.trigger.e-interaction", source: "printf x; echo -e value\n", dialect: DialectPOSIX, analysisExact: true, want: []ruleDiagnosticExpectation{w("SHP1001", "echo option behavior is not portable; use printf for controlled output", "-e")}},
		}, nonTriggers: []ruleDecisionCase{{id: "rule.shp1001.nearest.echo", source: "echo value\n", dialect: DialectPOSIX, analysisExact: true}, {id: "rule.shp1001.nearest.printf", source: "printf %s value\n", dialect: DialectPOSIX, analysisExact: true}}},
		{id: "rule.shp1002", code: "SHP1002", category: "portability", triggers: []ruleDecisionCase{
			{id: "rule.shp1002.trigger.assignment", source: "local value=x\n", dialect: DialectPOSIX, analysisExact: true, want: []ruleDiagnosticExpectation{w("SHP1002", "local variable declarations are not specified by POSIX shell", "local")}},
			{id: "rule.shp1002.trigger.multi-interaction", source: "printf x; local first second\n", dialect: DialectPOSIX, analysisExact: true, want: []ruleDiagnosticExpectation{w("SHP1002", "local variable declarations are not specified by POSIX shell", "local")}},
		}, nonTriggers: []ruleDecisionCase{{id: "rule.shp1002.nearest.assignment", source: "value=x\n", dialect: DialectPOSIX, analysisExact: true}, {id: "rule.shp1002.nearest.argument", source: "printf '%s' local\n", dialect: DialectPOSIX, analysisExact: true}}},
		{id: "rule.shx1001", code: "SHX1001", category: "security", triggers: []ruleDecisionCase{
			{id: "rule.shx1001.trigger.parameter", source: "eval \"$generated\"\n", dialect: DialectPOSIX, analysisExact: false, want: []ruleDiagnosticExpectation{w("SHX1001", "dynamic text is evaluated as shell code", "eval \"$generated\""), w("SHI1001", "dynamic eval content prevents exact analysis", "eval \"$generated\"")}},
			{id: "rule.shx1001.trigger.substitution-interaction", source: "printf x; eval \"$(make)\"\n", dialect: DialectPOSIX, analysisExact: false, want: []ruleDiagnosticExpectation{w("SHX1001", "dynamic text is evaluated as shell code", "eval \"$(make)\""), w("SHI1001", "dynamic eval content prevents exact analysis", "eval \"$(make)\"")}},
		}, nonTriggers: []ruleDecisionCase{{id: "rule.shx1001.nearest.quoted", source: "eval 'echo ok'\n", dialect: DialectPOSIX, analysisExact: true}, {id: "rule.shx1001.nearest.arguments", source: "eval echo ok\n", dialect: DialectPOSIX, analysisExact: true}}},
		{id: "rule.shx1002", code: "SHX1002", category: "security", triggers: []ruleDecisionCase{
			{id: "rule.shx1002.trigger.dynamic", source: "rm -rf \"$target\"\n", dialect: DialectPOSIX, analysisExact: false, want: []ruleDiagnosticExpectation{w("SHX1002", "destructive path may become empty or root-like", "\"$target\""), w("SHI1001", "dynamic destructive path prevents exact safety analysis", "\"$target\"")}},
			{id: "rule.shx1002.trigger.root-interaction", source: "printf x; rm -rf /\n", dialect: DialectPOSIX, analysisExact: true, want: []ruleDiagnosticExpectation{w("SHX1002", "destructive path may become empty or root-like", "/")}},
		}, nonTriggers: []ruleDecisionCase{{id: "rule.shx1002.nearest.relative", source: "rm -rf ./target\n", dialect: DialectPOSIX, analysisExact: true}, {id: "rule.shx1002.nearest.file", source: "rm -f file\n", dialect: DialectPOSIX, analysisExact: true}}},
		{id: "rule.shi1001", code: "SHI1001", category: "incomplete", triggers: []ruleDecisionCase{
			{id: "rule.shi1001.trigger.dynamic-source", source: ". \"$path\"\n", dialect: DialectPOSIX, analysisExact: false, want: []ruleDiagnosticExpectation{w("SHI1001", "dynamic source path prevents exact analysis", "\"$path\"")}},
			{id: "rule.shi1001.trigger.unresolved-source", source: ". child.sh\n", dialect: DialectPOSIX, analysisExact: false, want: []ruleDiagnosticExpectation{w("SHI1001", "sourced file was not resolved for analysis", "child.sh")}},
		}, nonTriggers: []ruleDecisionCase{{id: "rule.shi1001.nearest.literal-eval", source: "eval 'echo ok'\n", dialect: DialectPOSIX, analysisExact: true}, {id: "rule.shi1001.nearest.command", source: "echo ok\n", dialect: DialectPOSIX, analysisExact: true}}},
	}
	if len(specs) != 23 {
		t.Fatalf("rule decision specs = %d", len(specs))
	}
	for _, spec := range specs {
		t.Run(spec.id, func(t *testing.T) {
			if len(spec.triggers) != 2 || len(spec.nonTriggers) != 2 {
				t.Fatalf("%s must declare two triggers and two nearest non-triggers", spec.code)
			}
			for _, item := range append(append([]ruleDecisionCase(nil), spec.triggers...), spec.nonTriggers...) {
				t.Run(item.id, func(t *testing.T) { assertRuleDecisionCase(t, item, metadata) })
			}
			// The same trigger is an exact oracle for allowlist and disable-wins
			// behavior. Filtering the independently declared full diagnostic set
			// makes any unrelated false positive fail instead of being ignored.
			trigger := spec.triggers[0]
			t.Run("configuration.allowlist", func(t *testing.T) {
				assertRuleDecisionConfiguration(t, trigger, metadata, []string{spec.category}, nil)
			})
			t.Run("configuration.disable-wins", func(t *testing.T) {
				assertRuleDecisionConfiguration(t, trigger, metadata, []string{spec.category}, []string{spec.category})
			})
		})
	}
}

func assertRuleDecisionCase(t *testing.T, item ruleDecisionCase, metadata map[string]struct {
	severity   Severity
	confidence Confidence
}) {
	t.Helper()
	result, err := Check(t.Context(), item.id+".sh", []byte(item.source), Options{Dialect: item.dialect})
	if err != nil {
		t.Fatal(err)
	}
	if result.AnalysisExact != item.analysisExact {
		t.Fatalf("analysis exact=%v, want %v", result.AnalysisExact, item.analysisExact)
	}
	want := materializeRuleDiagnostics(t, item, metadata)
	if !reflect.DeepEqual(result.Diagnostics, want) {
		t.Fatalf("diagnostics mismatch\n got: %#v\nwant: %#v", result.Diagnostics, want)
	}
}

func assertRuleDecisionConfiguration(t *testing.T, item ruleDecisionCase, metadata map[string]struct {
	severity   Severity
	confidence Confidence
}, enabled, disabled []string) {
	t.Helper()
	result, err := Check(t.Context(), item.id+".sh", []byte(item.source), Options{
		Dialect: item.dialect, EnableCategories: enabled, DisableCategories: disabled,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := materializeRuleDiagnostics(t, item, metadata)
	want = slices.DeleteFunc(want, func(diagnostic Diagnostic) bool {
		category := categoryForCode(diagnostic.Code)
		return len(enabled) > 0 && !slices.Contains(enabled, category) || slices.Contains(disabled, category)
	})
	if !reflect.DeepEqual(result.Diagnostics, want) {
		t.Fatalf("configured diagnostics mismatch\n got: %#v\nwant: %#v", result.Diagnostics, want)
	}
}

func materializeRuleDiagnostics(t *testing.T, item ruleDecisionCase, metadata map[string]struct {
	severity   Severity
	confidence Confidence
}) []Diagnostic {
	t.Helper()
	source := []byte(item.source)
	sourceModel := newSourceFile(item.id+".sh", source)
	result := make([]Diagnostic, 0, len(item.want))
	for _, expectation := range item.want {
		declared, ok := metadata[expectation.code]
		if !ok {
			t.Fatalf("diagnostic %s is absent from catalog", expectation.code)
		}
		start := nthByteIndex(source, []byte(expectation.spanText), expectation.occurrence)
		if start < 0 {
			t.Fatalf("span occurrence %d of %q is absent", expectation.occurrence, expectation.spanText)
		}
		result = append(result, sourceModel.diagnostic(expectation.code, declared.severity, declared.confidence,
			expectation.message, start, start+len(expectation.spanText)))
	}
	sortDiagnostics(result)
	return result
}

func nthByteIndex(source, target []byte, occurrence int) int {
	offset := 0
	for index := 0; index <= occurrence; index++ {
		found := bytes.Index(source[offset:], target)
		if found < 0 {
			return -1
		}
		offset += found
		if index == occurrence {
			return offset
		}
		offset += len(target)
	}
	return -1
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
		if err != nil || result.AnalysisExact || !hasCode(result.Diagnostics, "SHI1001") {
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

func TestByteMutationRobustness(t *testing.T) {
	base := []byte("printf '%s\\n' value\n")
	for _, value := range []byte{0, 0xff} {
		for offset := 0; offset <= len(base); offset++ {
			mutated := append([]byte(nil), base[:offset]...)
			mutated = append(mutated, value)
			mutated = append(mutated, base[offset:]...)
			file, diagnostics, err := Parse("bytes.sh", mutated, DialectPOSIX)
			if err != nil || file == nil || !bytes.Equal(file.Source(), mutated) {
				t.Fatalf("value=%x offset=%d: %v", value, offset, err)
			}
			assertDiagnosticBounds(t, mutated, diagnostics)
			if value == 0 {
				assertSingleNULDiagnostic(t, diagnostics, offset)
			}
		}
	}
}

func TestNULContextsExactlyOnce(t *testing.T) {
	tests := []struct {
		name   string
		source []byte
		offset int
	}{
		{name: "unquoted", source: []byte("printf x\x00y\n"), offset: 8},
		{name: "single-quoted", source: []byte("printf 'x\x00y'\n"), offset: 9},
		{name: "double-quoted", source: []byte("printf \"x\x00y\"\n"), offset: 9},
		{name: "comment", source: []byte("# x\x00y\nprintf ok\n"), offset: 3},
		{name: "here-document-body", source: []byte("cat <<EOF\nx\x00y\nEOF\n"), offset: 11},
		{name: "here-document-terminator", source: []byte("cat <<EOF\nbody\nEO\x00F\n"), offset: 17},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, diagnostics, err := Parse("nul.sh", test.source, DialectPOSIX)
			if err != nil || file == nil || !bytes.Equal(file.Source(), test.source) {
				t.Fatalf("parse: %v file=%#v", err, file)
			}
			assertSingleNULDiagnostic(t, diagnostics, test.offset)
		})
	}
}

func assertSingleNULDiagnostic(t *testing.T, diagnostics []Diagnostic, offset int) {
	t.Helper()
	var found []Diagnostic
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == "SHS1003" {
			found = append(found, diagnostic)
		}
	}
	if len(found) != 1 || found[0].Primary.Start.Offset != offset || found[0].Primary.End.Offset != offset+1 {
		t.Fatalf("NUL at %d: want one SHS1003 on [%d,%d), got %#v", offset, offset, offset+1, found)
	}
}

func TestRecoveryMutationRobustness(t *testing.T) {
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

func TestSyntaxMutationConformance(t *testing.T) {
	tests := []struct {
		name   string
		source string
		codes  []string
	}{
		{name: "delete-fi", source: "if true; then :\n", codes: []string{"SHS1004"}},
		{name: "delete-quote", source: "echo 'unterminated", codes: []string{"SHS1001"}},
		{name: "delete-heredoc-terminator", source: "cat <<EOF\nbody\n", codes: []string{"SHS1006"}},
		{name: "insert-unmatched-closer", source: ")", codes: []string{"SHS1005"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, diagnostics, err := Parse("mutation.sh", []byte(test.source), DialectPOSIX)
			if err != nil {
				t.Fatal(err)
			}
			var codes []string
			for _, diagnostic := range diagnostics {
				codes = append(codes, diagnostic.Code)
			}
			if !reflect.DeepEqual(codes, test.codes) {
				t.Fatalf("diagnostics=%v, want %v: %#v", codes, test.codes, diagnostics)
			}
			assertDiagnosticBounds(t, []byte(test.source), diagnostics)
		})
	}
}

type fuzzSeed struct {
	ID      string `json:"id"`
	Feature string `json:"feature"`
	Hex     string `json:"hex"`
}

type fuzzRegression struct {
	ID            string   `json:"id"`
	ObligationIDs []string `json:"obligationIds"`
	Dialect       string   `json:"dialect"`
	Hex           string   `json:"hex"`
	Expected      struct {
		Diagnostics []caseDiagnostic `json:"diagnostics"`
	} `json:"expected"`
}

type fuzzRegressionFile struct {
	Version     int              `json:"version"`
	Seeds       []fuzzSeed       `json:"seeds"`
	Regressions []fuzzRegression `json:"regressions"`
}

func TestCommittedFuzzSeedReplay(t *testing.T) {
	data, err := os.ReadFile("testdata/spec/fuzz_regressions.json")
	if err != nil {
		t.Fatal(err)
	}
	var corpus fuzzRegressionFile
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&corpus); err != nil {
		t.Fatal(err)
	}
	if corpus.Version != 1 || len(corpus.Seeds) != len(fuzzSeeds) || len(corpus.Regressions) == 0 {
		t.Fatalf("invalid fuzz corpus version/counts: version=%d seeds=%d regressions=%d", corpus.Version, len(corpus.Seeds), len(corpus.Regressions))
	}
	seen := make(map[string]struct{})
	knownObligations := make(map[string]struct{})
	var features []featureCatalogEntry
	var rules []ruleCatalogEntry
	readJSONFile(t, "testdata/features.json", &features)
	readJSONFile(t, "testdata/rules.json", &rules)
	for _, feature := range features {
		knownObligations[feature.ID] = struct{}{}
	}
	for _, rule := range rules {
		knownObligations[rule.Code] = struct{}{}
	}
	for _, entry := range mustLoadCatalog[lexicalEntry](t, "lexical") {
		knownObligations[entry.ID] = struct{}{}
	}
	for _, entry := range mustLoadCatalog[grammarEntry](t, "grammar") {
		knownObligations[entry.ID] = struct{}{}
	}
	for _, entry := range mustLoadCatalog[dialectEntry](t, "dialects") {
		knownObligations[entry.ID] = struct{}{}
	}
	for _, entry := range mustLoadCatalog[semanticEntry](t, "semantics") {
		knownObligations[entry.ID] = struct{}{}
	}
	for _, entry := range mustLoadCatalog[robustnessEntry](t, "robustness") {
		knownObligations[entry.ID] = struct{}{}
	}
	for index, seed := range corpus.Seeds {
		if seed.ID == "" || seed.Feature == "" {
			t.Fatalf("invalid seed %#v", seed)
		}
		if _, exists := seen[seed.ID]; exists {
			t.Fatalf("duplicate seed %s", seed.ID)
		}
		seen[seed.ID] = struct{}{}
		if _, exists := knownObligations[seed.Feature]; !exists {
			t.Fatalf("%s: unknown feature obligation %q", seed.ID, seed.Feature)
		}
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
	for _, regression := range corpus.Regressions {
		if regression.ID == "" || len(regression.ObligationIDs) == 0 || regression.Dialect == "" {
			t.Fatalf("invalid regression %#v", regression)
		}
		if _, exists := seen[regression.ID]; exists {
			t.Fatalf("duplicate fuzz case %s", regression.ID)
		}
		seen[regression.ID] = struct{}{}
		for _, obligationID := range regression.ObligationIDs {
			if _, exists := knownObligations[obligationID]; !exists {
				t.Fatalf("%s: unknown obligation %q", regression.ID, obligationID)
			}
		}
		source, err := hex.DecodeString(regression.Hex)
		if err != nil {
			t.Fatalf("%s: %v", regression.ID, err)
		}
		dialect := DialectPOSIX
		if regression.Dialect == "bash" {
			dialect = DialectBash
		} else if regression.Dialect != "posix" {
			t.Fatalf("%s: invalid dialect %q", regression.ID, regression.Dialect)
		}
		_, diagnostics, err := Parse("fuzz-regression.sh", source, dialect)
		if err != nil {
			t.Fatalf("%s: %v", regression.ID, err)
		}
		if len(diagnostics) != len(regression.Expected.Diagnostics) {
			t.Fatalf("%s: diagnostics=%#v", regression.ID, diagnostics)
		}
		for index, expected := range regression.Expected.Diagnostics {
			got := diagnostics[index]
			if got.Code != expected.Code || expected.StartOffset == nil || expected.EndOffset == nil || got.Primary.Start.Offset != *expected.StartOffset || got.Primary.End.Offset != *expected.EndOffset {
				t.Fatalf("%s diagnostic %d: got %#v want %#v", regression.ID, index, got, expected)
			}
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
