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

const deterministicModelVersion = "shellvalidate-generated-v2"

type deterministicVector struct {
	ID       string                    `json:"id"`
	Dialect  Dialect                   `json:"dialect"`
	Source   string                    `json:"source"`
	Expected *deterministicExpectation `json:"expected,omitempty"`
}

// deterministicExpectation is a test-owned conformance oracle. The values in
// these records are authored from the language specifications; they are never
// copied from lexer or parser tables.
type deterministicExpectation struct {
	SyntaxValid bool     `json:"syntaxValid"`
	Tokens      []string `json:"tokens"`
	AST         string   `json:"ast"`
	Diagnostics []string `json:"diagnostics"`
}

func validVector(id string, dialect Dialect, source string, tokens []string, ast string) deterministicVector {
	return deterministicVector{ID: id, Dialect: dialect, Source: source, Expected: &deterministicExpectation{
		SyntaxValid: true, Tokens: tokens, AST: ast, Diagnostics: []string{},
	}}
}

func invalidVector(id string, dialect Dialect, source string, tokens []string, ast string, diagnostics ...string) deterministicVector {
	return deterministicVector{ID: id, Dialect: dialect, Source: source, Expected: &deterministicExpectation{
		SyntaxValid: false, Tokens: tokens, AST: ast, Diagnostics: append([]string(nil), diagnostics...),
	}}
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
	SchemaVersion   int    `json:"schemaVersion"`
	ModelVersion    string `json:"modelVersion"`
	NormativeDigest string `json:"normativeDigest"`
	ChangeNote      string `json:"changeNote"`
	Models          []struct {
		ID          string         `json:"id"`
		Count       int            `json:"count"`
		SHA256      string         `json:"sha256"`
		Obligations int            `json:"obligations"`
		Executed    int            `json:"executed"`
		Strength    string         `json:"strength"`
		Bounds      map[string]int `json:"bounds,omitempty"`
		Exclusions  []struct {
			Reason string `json:"reason"`
			Count  int    `json:"count"`
		} `json:"exclusions,omitempty"`
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

func TestGeneratedModel_Manifest(t *testing.T) {
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
	if manifest.SchemaVersion != 2 || manifest.ChangeNote == "" {
		t.Fatalf("manifest schema=%d change-note=%q", manifest.SchemaVersion, manifest.ChangeNote)
	}
	digest, err := hex.DecodeString(manifest.NormativeDigest)
	if err != nil || len(digest) != sha256.Size {
		t.Fatalf("invalid normative digest %q", manifest.NormativeDigest)
	}
	previous, total := "", 0
	for _, model := range manifest.Models {
		if model.ID <= previous {
			t.Fatalf("model IDs are not unique and sorted: %q after %q", model.ID, previous)
		}
		if model.Count <= 0 {
			t.Fatalf("model %s count = %d", model.ID, model.Count)
		}
		if model.Obligations <= 0 || model.Executed != model.Count || model.Strength == "" {
			t.Fatalf("model %s obligations=%d executed=%d count=%d strength=%q", model.ID, model.Obligations, model.Executed, model.Count, model.Strength)
		}
		for _, exclusion := range model.Exclusions {
			if exclusion.Reason == "" || exclusion.Count <= 0 {
				t.Fatalf("model %s invalid exclusion %#v", model.ID, exclusion)
			}
		}
		digest, err := hex.DecodeString(model.SHA256)
		if err != nil || len(digest) != sha256.Size {
			t.Fatalf("model %s has invalid SHA-256 %q", model.ID, model.SHA256)
		}
		previous = model.ID
		total += model.Count
	}
	if len(manifest.Models) != 9 || total <= 0 {
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
	if vector.Expected == nil {
		return
	}
	if firstFile == nil {
		t.Fatal("parse returned a nil file")
	}
	if firstFile.syntaxValid != vector.Expected.SyntaxValid {
		t.Fatalf("syntax valid=%v want=%v", firstFile.syntaxValid, vector.Expected.SyntaxValid)
	}
	if got := normalizedTokens(firstFile.tokens); !reflect.DeepEqual(got, vector.Expected.Tokens) {
		t.Fatalf("tokens:\n got %#v\nwant %#v", got, vector.Expected.Tokens)
	}
	if got := normalizedAST(firstFile.nodes); got != vector.Expected.AST {
		t.Fatalf("AST:\n got %s\nwant %s", got, vector.Expected.AST)
	}
	if got := normalizedDiagnostics(firstDiagnostics); !reflect.DeepEqual(got, vector.Expected.Diagnostics) {
		t.Fatalf("diagnostics:\n got %#v\nwant %#v", got, vector.Expected.Diagnostics)
	}
	result, err := Check(t.Context(), vector.ID, []byte(vector.Source), Options{Dialect: vector.Dialect})
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if result.SyntaxValid != vector.Expected.SyntaxValid {
		t.Fatalf("Check syntax valid=%v want=%v", result.SyntaxValid, vector.Expected.SyntaxValid)
	}
}

func normalizedTokens(tokens []token) []string {
	result := make([]string, 0, len(tokens))
	for _, item := range tokens {
		result = append(result, fmt.Sprintf("%d:%s@%d:%d", item.kind, item.text, item.start, item.end))
	}
	return result
}

func normalizedDiagnostics(diagnostics []Diagnostic) []string {
	result := make([]string, len(diagnostics))
	for index, diagnostic := range diagnostics {
		result[index] = fmt.Sprintf("%s@%d:%d:%s", diagnostic.Code, diagnostic.Primary.Start.Offset, diagnostic.Primary.End.Offset, diagnostic.Message)
	}
	return result
}

func normalizedAST(nodes []Node) string {
	var result strings.Builder
	for index, node := range nodes {
		if index != 0 {
			result.WriteByte(';')
		}
		normalizeNode(&result, node)
	}
	return result.String()
}

func normalizeNode(result *strings.Builder, node Node) {
	fmt.Fprintf(result, "%s[%s]", node.kind, node.role)
	if node.operator != "" {
		fmt.Fprintf(result, "<%s>", node.operator)
	}
	if node.incomplete {
		result.WriteString("!")
	}
	result.WriteByte('{')
	for index, word := range node.words {
		if index != 0 {
			result.WriteByte(',')
		}
		result.WriteString(wordSignature(word))
	}
	for _, redirection := range node.redirections {
		io := ""
		if redirection.hasIONumber {
			io = fmt.Sprint(redirection.ioNumber)
		}
		fmt.Fprintf(result, ";r=%s%s:%s", io, redirection.operator, wordSignature(redirection.target))
		if redirection.hereDocument != nil {
			fmt.Fprintf(result, ":h(q=%v,t=%v,b=%q)", redirection.hereDocument.quoted, redirection.hereDocument.stripTabs, redirection.hereDocument.body)
		}
	}
	for _, expression := range node.expressions {
		result.WriteString(";e=")
		normalizeExpression(result, expression)
	}
	result.WriteByte('}')
	if len(node.children) != 0 {
		result.WriteByte('(')
		for index, child := range node.children {
			if index != 0 {
				result.WriteByte(',')
			}
			normalizeNode(result, child)
		}
		result.WriteByte(')')
	}
}

func normalizeExpression(result *strings.Builder, expression Expression) {
	fmt.Fprintf(result, "%d:%s:%s", expression.kind, expression.operator, expression.value)
	if len(expression.children) != 0 {
		result.WriteByte('(')
		for index, child := range expression.children {
			if index != 0 {
				result.WriteByte(',')
			}
			normalizeExpression(result, child)
		}
		result.WriteByte(')')
	}
}

func wordSignature(word Word) string {
	var result strings.Builder
	for _, part := range word.parts {
		fmt.Fprintf(&result, "%d/%d:%s", part.kind, part.quote, part.text)
	}
	return result.String()
}

func simpleCommandAST(words ...string) string {
	return "command[]{" + strings.Join(words, ",") + "}"
}

func simpleTokens(source string, items ...string) []string {
	result := make([]string, 0, len(items)+2)
	search := 0
	for _, item := range items {
		index := strings.Index(source[search:], item)
		if index < 0 {
			panic("test token is absent from source: " + item)
		}
		start := search + index
		kind := tokenWord
		if item == "\n" {
			kind = tokenNewline
		}
		if strings.Contains(" && || | |& ; & ;; ;& ;;& < > >> << <<- <<< <& >& <> >| &> &>> ", " "+item+" ") {
			kind = tokenOperator
		}
		result = append(result, fmt.Sprintf("%d:%s@%d:%d", kind, item, start, start+len(item)))
		search = start + len(item)
	}
	if strings.HasSuffix(source, "\n") {
		result = append(result, fmt.Sprintf("%d:\n@%d:%d", tokenNewline, len(source)-1, len(source)))
	}
	result = append(result, fmt.Sprintf("%d:@%d:%d", tokenEOF, len(source), len(source)))
	return result
}

func expectedTokenKind(text string, dialect Dialect) tokenKind {
	posix := map[string]bool{"&&": true, "||": true, "|": true, "&": true, ";": true, ";;": true, "(": true, ")": true, "{": true, "}": true, "<": true, ">": true, ">>": true, "<<": true, "<<-": true, "<&": true, ">&": true, "<>": true, ">|": true}
	bash := map[string]bool{"&>>": true, ";;&": true, "<<<": true, "((": true, "))": true, "[[": true, "]]": true, "|&": true, ";&": true, "&>": true}
	if posix[text] || dialect == DialectBash && bash[text] {
		return tokenOperator
	}
	return tokenWord
}

func arithmeticExpectedTokens(source, operator string) []string {
	operatorStart := len("((left ")
	result := []string{"2:((@0:2", "1:left@2:6"}
	pieces := []string{operator}
	if strings.HasSuffix(operator, "=") {
		prefix := strings.TrimSuffix(operator, "=")
		if expectedTokenKind(prefix, DialectBash) == tokenOperator {
			pieces = []string{prefix, "="}
		}
	}
	offset := operatorStart
	for _, piece := range pieces {
		result = append(result, fmt.Sprintf("%d:%s@%d:%d", expectedTokenKind(piece, DialectBash), piece, offset, offset+len(piece)))
		offset += len(piece)
	}
	rightStart := operatorStart + len(operator) + 1
	closeStart := rightStart + len("right")
	return append(result, fmt.Sprintf("1:right@%d:%d", rightStart, rightStart+5), fmt.Sprintf("2:))@%d:%d", closeStart, closeStart+2), fmt.Sprintf("3:\n@%d:%d", len(source)-1, len(source)), fmt.Sprintf("5:@%d:%d", len(source), len(source)))
}

func TestGeneratedModel_Operators(t *testing.T) {
	// Normative lexical inventory. This must remain independent of shellOperators.
	operators := []string{"&", "&&", "&>", "&>>", "(", "((", ")", "))", ";", ";&", ";;&", ";;", "<", "<&", "<<", "<<-", "<<<", "<>", ">", ">&", ">>", ">|", "[[", "]]", "{", "|", "|&", "||", "}"}
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

func TestGeneratedModel_Lexical(t *testing.T) {
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

func TestGeneratedModel_DelimiterDeletion(t *testing.T) {
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

func TestGeneratedModel_Separators(t *testing.T) {
	separators := []string{"\n", ";", "&", "&&", "||", "|", "|&", ";;", ";&", ";;&"}
	var vectors []deterministicVector
	for _, separator := range separators {
		dialect := DialectPOSIX
		if separator == "|&" || separator == ";&" || separator == ";;&" {
			dialect = DialectBash
		}
		source := "left" + separator + "right\n"
		tokens := simpleTokens(source, "left", separator, "right")
		ast := "command[]{0/0:left};command[]{0/0:right}"
		var vector deterministicVector
		switch separator {
		case "&&", "||":
			ast = fmt.Sprintf("list[]<%s>{}(command[list-element]{0/0:left},command[list-element]{0/0:right})", separator)
			vector = validVector("separator/"+fmt.Sprintf("%x", separator), dialect, source, tokens, ast)
		case "|", "|&":
			ast = fmt.Sprintf("pipeline[]<%s>{}(command[pipeline-command]{0/0:left},command[pipeline-command]{0/0:right})", separator)
			vector = validVector("separator/"+fmt.Sprintf("%x", separator), dialect, source, tokens, ast)
		case ";;", ";&", ";;&":
			vector = invalidVector("separator/"+fmt.Sprintf("%x", separator), dialect, source, tokens, ast,
				fmt.Sprintf("SHS1005@4:%d:case-clause terminator is only valid inside case", 4+len(separator)))
		default:
			vector = validVector("separator/"+fmt.Sprintf("%x", separator), dialect, source, tokens, ast)
		}
		vectors = append(vectors, vector)
		verifyParseVector(t, vector)
	}
	assertModelFingerprint(t, "separators", vectors)
}

func TestGeneratedModel_Redirections(t *testing.T) {
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

func TestGeneratedModel_HereDocuments(t *testing.T) {
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

func TestGeneratedModel_Arithmetic(t *testing.T) {
	// Bash 5.3 shell-arithmetic binary operators. =~ is intentionally absent.
	operators := []string{"!=", "%", "%=", "&", "&&", "&=", "*", "**", "*=", "+", "+=", ",", "-", "-=", "/", "/=", "<", "<<", "<<=", "<=", "=", "==", ">", ">=", ">>", ">>=", "^", "^=", "|", "|=", "||"}
	var vectors []deterministicVector
	for _, operator := range operators {
		source := "((left " + operator + " right))\n"
		kind := ExpressionBinary
		if strings.Contains(" = *= /= %= += -= <<= >>= &= ^= |= ", " "+operator+" ") {
			kind = ExpressionAssignment
		}
		ast := fmt.Sprintf("arithmetic[]{;e=%d:%s:(1::left,1::right)}", kind, operator)
		vector := validVector("arithmetic/"+operator, DialectBash, source, arithmeticExpectedTokens(source, operator), ast)
		vectors = append(vectors, vector)
		verifyParseVector(t, vector)
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

func TestGeneratedModel_Conditional(t *testing.T) {
	// Bash conditional-command operators, kept separate from arithmetic.
	operators := []string{"!=", "-ef", "-eq", "-ge", "-gt", "-le", "-lt", "-ne", "-nt", "-ot", "<", "=", "==", "=~", ">"}
	var vectors []deterministicVector
	for _, operator := range operators {
		source := "[[ left " + operator + " right ]]\n"
		operatorStart := len("[[ left ")
		rightStart := operatorStart + len(operator) + 1
		closeStart := rightStart + len("right ")
		vector := validVector("conditional/"+operator, DialectBash, source,
			[]string{"2:[[@0:2", "1:left@3:7", fmt.Sprintf("%d:%s@%d:%d", expectedTokenKind(operator, DialectBash), operator, operatorStart, operatorStart+len(operator)), fmt.Sprintf("1:right@%d:%d", rightStart, rightStart+5), fmt.Sprintf("2:]]@%d:%d", closeStart, closeStart+2), fmt.Sprintf("3:\n@%d:%d", len(source)-1, len(source)), fmt.Sprintf("5:@%d:%d", len(source), len(source))},
			fmt.Sprintf("conditional[]{;e=3:%s:(1::left,1::right)}", operator))
		vectors = append(vectors, vector)
		verifyParseVector(t, vector)
	}
	assertModelFingerprint(t, "conditional", vectors)
}

func TestGeneratedModel_ContextRejections(t *testing.T) {
	vectors := []deterministicVector{
		invalidVector("context/regex-is-not-arithmetic", DialectBash, "((left =~ right))\n",
			[]string{"2:((@0:2", "1:left@2:6", "1:=~@7:9", "1:right@10:15", "2:))@15:17", "3:\n@17:18", "5:@18:18"},
			"arithmetic[]!{;e=1::left;e=1::right}", "SHS1005@7:9:operator is not valid in shell arithmetic"),
		invalidVector("context/assignment-is-not-conditional", DialectBash, "[[ left += right ]]\n",
			[]string{"2:[[@0:2", "1:left@3:7", "1:+=@8:10", "1:right@11:16", "2:]]@17:19", "3:\n@19:20", "5:@20:20"},
			"conditional[]!{;e=1::left}", "SHS1005@8:10:token is not valid in a conditional expression", "SHS1005@11:16:token is not valid in a conditional expression"),
	}
	for _, vector := range vectors {
		t.Run(vector.ID, func(t *testing.T) { verifyParseVector(t, vector) })
	}
}

func expressionsForKind(file *File, kind NodeKind) []Expression {
	for _, node := range file.Nodes() {
		if node.Kind() == kind {
			return node.Expressions()
		}
	}
	return nil
}

func TestGeneratedModel_Grammar(t *testing.T) {
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

func TestGeneratedModel_AutoDialect(t *testing.T) {
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

func TestGeneratedModel_DialectInteractions(t *testing.T) {
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

type pairwiseExclusion struct {
	Tuple  interactionVector
	Reason string
}

var pairwiseDimensions = [][]string{
	{"bash", "posix"},
	{"double", "single", "unquoted"},
	{"arithmetic", "command", "literal", "parameter"},
	{"argument", "assignment", "redirection"},
}

func pairwiseConstraint(vector interactionVector) (string, bool) {
	// Single quotes intentionally suppress expansion. Such a tuple cannot both
	// preserve single-quote semantics and expose the requested expansion node.
	if vector[1].Value == "single" && vector[2].Value != "literal" {
		return "single-quote-suppresses-expansion", false
	}
	return "", true
}

func constrainedPairwise() ([]interactionVector, []pairwiseExclusion) {
	var candidates []interactionVector
	var excluded []pairwiseExclusion
	for _, vector := range allInteractionVectors(pairwiseDimensions) {
		if reason, ok := pairwiseConstraint(vector); ok {
			candidates = append(candidates, vector)
		} else {
			excluded = append(excluded, pairwiseExclusion{Tuple: vector, Reason: reason})
		}
	}
	required := make(map[string]struct{})
	for _, candidate := range candidates {
		for _, key := range pairKeys(candidate) {
			required[key] = struct{}{}
		}
	}
	remaining := append([]interactionVector(nil), candidates...)
	var selected []interactionVector
	for len(required) != 0 {
		best, score := -1, -1
		for index, candidate := range remaining {
			candidateScore := 0
			for _, key := range pairKeys(candidate) {
				if _, ok := required[key]; ok {
					candidateScore++
				}
			}
			if candidateScore > score {
				best, score = index, candidateScore
			}
		}
		if best < 0 || score == 0 {
			panic("constrained pairwise generator made no progress")
		}
		selected = append(selected, remaining[best])
		for _, key := range pairKeys(remaining[best]) {
			delete(required, key)
		}
		remaining = append(remaining[:best], remaining[best+1:]...)
	}
	return selected, excluded
}

func renderPairwise(vector interactionVector) (deterministicVector, WordPartKind, QuoteKind, string) {
	dialect := DialectBash
	if vector[0].Value == "posix" {
		dialect = DialectPOSIX
	}
	partText, partKind := "text", WordLiteral
	switch vector[2].Value {
	case "arithmetic":
		partText, partKind = "$((1+2))", WordArithmeticExpansion
	case "command":
		partText, partKind = "$(printf x)", WordCommandSubstitution
	case "parameter":
		partText, partKind = "${value:-safe}", WordParameterExpansion
	}
	quote := QuoteUnquoted
	switch vector[1].Value {
	case "double":
		partText, quote = "\""+partText+"\"", QuoteDouble
	case "single":
		partText, quote = "'"+partText+"'", QuoteSingle
	}
	contextName := vector[3].Value
	source := "printf '%s\\n' " + partText + "\n"
	switch contextName {
	case "assignment":
		source = "value=" + partText + "\nprintf '%s\\n' \"$value\"\n"
	case "redirection":
		source = ": >" + partText + "\n"
	}
	idParts := make([]string, len(vector))
	for index := range vector {
		idParts[index] = vector[index].Value
	}
	oracle := fmt.Sprintf("context=%s;part=%s;quote=%s", contextName, vector[2].Value, vector[1].Value)
	return validVector("pairwise/"+strings.Join(idParts, "/"), dialect, source, nil, oracle), partKind, quote, contextName
}

func diagnosticCodes(items []Diagnostic) []string {
	result := make([]string, len(items))
	for index := range items {
		result[index] = items[index].Code
	}
	return result
}

func wordContainsPart(word Word, kind WordPartKind, quote QuoteKind) bool {
	for _, part := range word.Parts() {
		if part.Kind() == kind && part.Quote() == quote {
			return true
		}
	}
	return false
}

func nodesDepthFirst(nodes []Node) []Node {
	var result []Node
	for _, node := range nodes {
		result = append(result, node)
		result = append(result, nodesDepthFirst(node.Children())...)
	}
	return result
}

func assertPairwiseAST(t *testing.T, file *File, kind WordPartKind, quote QuoteKind, contextName string) {
	t.Helper()
	for _, node := range nodesDepthFirst(file.Nodes()) {
		var words []Word
		switch contextName {
		case "argument":
			words = node.Words()
		case "assignment":
			words = node.Assignments()
		case "redirection":
			for _, redirect := range node.Redirections() {
				words = append(words, redirect.Target())
			}
		}
		for _, word := range words {
			if wordContainsPart(word, kind, quote) {
				return
			}
		}
	}
	t.Fatalf("AST lacks %s part kind=%d quote=%d", contextName, kind, quote)
}

func TestGeneratedModel_PairwiseShellInteractions(t *testing.T) {
	tuples, exclusions := constrainedPairwise()
	if len(exclusions) != 18 {
		t.Fatalf("pairwise exclusions=%d, want 18", len(exclusions))
	}
	vectors := make([]deterministicVector, 0, len(tuples))
	for _, tuple := range tuples {
		vector, partKind, quote, contextName := renderPairwise(tuple)
		vectors = append(vectors, vector)
		result, err := Check(t.Context(), vector.ID, []byte(vector.Source), Options{
			Dialect: vector.Dialect, EnableCategories: []string{"syntax"},
		})
		if err != nil {
			t.Fatalf("%s: %v", vector.ID, err)
		}
		if result.SyntaxValid != vector.Expected.SyntaxValid || !reflect.DeepEqual(diagnosticCodes(result.Diagnostics), vector.Expected.Diagnostics) {
			t.Fatalf("%s: valid=%v diagnostics=%v", vector.ID, result.SyntaxValid, diagnosticCodes(result.Diagnostics))
		}
		assertPairwiseAST(t, result.File, partKind, quote, contextName)
	}
	assertModelFingerprint(t, "pairwise", vectors)
}

func appendSequences(alphabet []string, maximum int, visit func([]string)) {
	var build func([]string)
	build = func(prefix []string) {
		if len(prefix) != 0 {
			visit(append([]string(nil), prefix...))
		}
		if len(prefix) == maximum {
			return
		}
		for _, item := range alphabet {
			build(append(prefix, item))
		}
	}
	build(nil)
}

func checkBoundedVector(t *testing.T, vector deterministicVector) *File {
	t.Helper()
	result, err := Check(t.Context(), vector.ID, []byte(vector.Source), Options{
		Dialect: vector.Dialect, EnableCategories: []string{"syntax"},
	})
	if err != nil || !result.SyntaxValid || len(result.Diagnostics) != 0 {
		t.Fatalf("%s: valid=%v err=%v diagnostics=%v", vector.ID, result.SyntaxValid, err, diagnosticCodes(result.Diagnostics))
	}
	return result.File
}

func TestGeneratedModel_BoundedLocalGrammar(t *testing.T) {
	var vectors []deterministicVector

	// This is exhaustive over every ordered sequence of the three independent
	// word-part classes through length four: 3+9+27+81 = 120 obligations.
	wordParts := []string{"x", "${value:-x}", "$(printf x)"}
	appendSequences(wordParts, 4, func(parts []string) {
		var expectedKinds []WordPartKind
		for _, part := range parts {
			kind := WordLiteral
			switch part {
			case "${value:-x}":
				kind = WordParameterExpansion
			case "$(printf x)":
				kind = WordCommandSubstitution
			}
			if kind == WordLiteral && len(expectedKinds) != 0 && expectedKinds[len(expectedKinds)-1] == WordLiteral {
				continue // the lexer canonically coalesces adjacent literal bytes
			}
			expectedKinds = append(expectedKinds, kind)
		}
		vector := validVector(fmt.Sprintf("bounded/word/%04d", len(vectors)), DialectPOSIX,
			"printf '%s\\n' "+strings.Join(parts, "")+"\n", nil, fmt.Sprintf("word-parts=%v", expectedKinds))
		vectors = append(vectors, vector)
		file := checkBoundedVector(t, vector)
		commands := nodesDepthFirst(file.Nodes())
		found := false
		for _, command := range commands {
			words := command.Words()
			if len(words) >= 3 {
				actualParts := words[len(words)-1].Parts()
				actualKinds := make([]WordPartKind, len(actualParts))
				for index := range actualParts {
					actualKinds[index] = actualParts[index].Kind()
				}
				if reflect.DeepEqual(actualKinds, expectedKinds) {
					found = true
				}
			}
		}
		if !found {
			t.Fatalf("%s: AST lacks ordered word parts %v", vector.ID, expectedKinds)
		}
	})

	// Every ordered heterogeneous pipeline over the three command forms through
	// length three is executed: 3+9+27 = 39 obligations.
	commandForms := []string{":", "printf x", "value=x :"}
	appendSequences(commandForms, 3, func(commands []string) {
		vector := validVector(fmt.Sprintf("bounded/pipeline/%04d", len(vectors)), DialectPOSIX,
			strings.Join(commands, " | ")+"\n", nil, fmt.Sprintf("pipeline-members=%d", len(commands)))
		vectors = append(vectors, vector)
		file := checkBoundedVector(t, vector)
		nodes := file.Nodes()
		if len(commands) > 1 && (len(nodes) != 1 || nodes[0].Kind() != NodePipeline || len(nodes[0].Children()) != len(commands)) {
			t.Fatalf("%s: pipeline AST=%#v", vector.ID, nodes)
		}
	})

	// Redirection location and ownership are a full local Cartesian product.
	redirects := []struct {
		spelling, operator string
		io                 int
		hasIO              bool
	}{
		{spelling: ">out", operator: ">"},
		{spelling: "3>>out", operator: ">>", io: 3, hasIO: true},
		{spelling: "<in", operator: "<"},
	}
	for owner := 0; owner < 2; owner++ {
		for _, position := range []string{"prefix", "suffix"} {
			for _, redirect := range redirects {
				commands := []string{":", ":"}
				if position == "prefix" {
					commands[owner] = redirect.spelling + " :"
				} else {
					commands[owner] = ": " + redirect.spelling
				}
				vector := validVector(fmt.Sprintf("bounded/redirect/o%d/%s/%s", owner, position, redirect.operator),
					DialectPOSIX, strings.Join(commands, " | ")+"\n", nil,
					fmt.Sprintf("redirect-owner=%d;position=%s;operator=%s", owner, position, redirect.operator))
				vectors = append(vectors, vector)
				file := checkBoundedVector(t, vector)
				pipeline := file.Nodes()[0]
				children := pipeline.Children()
				if len(children) != 2 || len(children[owner].Redirections()) != 1 || len(children[1-owner].Redirections()) != 0 {
					t.Fatalf("%s: redirection ownership=%#v", vector.ID, children)
				}
				actual := children[owner].Redirections()[0]
				io, hasIO := actual.IONumber()
				if actual.Operator() != redirect.operator || io != redirect.io || hasIO != redirect.hasIO {
					t.Fatalf("%s: redirection=%#v io=%d/%v", vector.ID, actual, io, hasIO)
				}
			}
		}
	}

	assertModelFingerprint(t, "bounded-recursive", vectors)
}
