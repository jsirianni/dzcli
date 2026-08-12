package shellvalidate

import (
	"strings"
	"testing"

	"dzcli/shellvalidate/internal/mutest"
)

func loadImplementationMutants(t *testing.T) []mutest.Mutant {
	t.Helper()
	mutants, err := mutest.LoadManifest("testdata/spec/mutants.json")
	if err != nil {
		t.Fatal(err)
	}
	return mutants
}

func TestMutationManifestAudit(t *testing.T) {
	mutants := loadImplementationMutants(t)
	seen := make(map[string]struct{})
	previous := 0
	for _, mutant := range mutants {
		if mutant.ID == "" || mutant.Target == "" || mutant.KilledBy == "" {
			t.Fatalf("incomplete mutant: %#v", mutant)
		}
		if _, exists := seen[mutant.ID]; exists {
			t.Fatalf("duplicate mutant %s", mutant.ID)
		}
		seen[mutant.ID] = struct{}{}
		if mutant.SourceOrder <= previous {
			t.Fatalf("mutant %s is not in source order", mutant.ID)
		}
		if !mutant.Critical {
			t.Fatalf("mutant %s needs an explicit survivor classification", mutant.ID)
		}
		if mutant.KilledBy != "TestMutationBehavior/"+mutant.ID {
			t.Fatalf("mutant %s kill mapping = %q", mutant.ID, mutant.KilledBy)
		}
		if mutant.Selector == nil {
			t.Fatalf("critical mutant %s has no source selector", mutant.ID)
		}
		filename := strings.TrimPrefix(mutant.Selector.File, "shellvalidate/")
		digest, err := mutest.DeclarationHash(filename, mutant.Selector.Declaration)
		if err != nil {
			t.Fatalf("mutant %s: %v", mutant.ID, err)
		}
		if digest != mutant.Selector.DeclarationHash {
			t.Fatalf("mutant %s declaration hash is stale: got %s want %s", mutant.ID, digest, mutant.Selector.DeclarationHash)
		}
		previous = mutant.SourceOrder
	}
	if len(mutants) != 15 {
		t.Fatalf("critical mutants = %d", len(mutants))
	}
}

func TestMutationBehavior(t *testing.T) {
	mutants := loadImplementationMutants(t)
	for _, mutant := range mutants {
		mutant := mutant
		t.Run(mutant.ID, func(t *testing.T) {
			if !mutationBehaviorHolds(t, mutant.ID) {
				t.Fatalf("behavioral obligation failed: %s (%s)", mutant.ID, mutant.Target)
			}
		})
	}
}

func mutationBehaviorHolds(t *testing.T, id string) bool {
	t.Helper()
	switch id {
	case "LEX001":
		return operatorAt([]byte(";;&"), 0, DialectBash) == ";;&"
	case "LEX002":
		_, diagnostics, err := Parse("nul.sh", []byte("echo 'a\x00b'\n"), DialectPOSIX)
		return err == nil && hasCode(diagnostics, "SHS1003")
	case "PAR001":
		file, diagnostics, err := Parse("closer.sh", []byte("if true; then :"), DialectPOSIX)
		diagnostic, ok := diagnosticByCode(diagnostics, "SHS1004")
		return err == nil && file != nil && !file.syntaxValid && ok && strings.Contains(diagnostic.Message, "closing fi")
	case "PAR002":
		file, diagnostics, err := Parse("assoc.sh", []byte("((a=b=c))\n"), DialectBash)
		if err != nil || len(diagnostics) != 0 {
			return false
		}
		expressions := expressionsForKind(file, NodeArithmetic)
		return len(expressions) == 1 && len(expressions[0].Children()) == 2 && expressions[0].Children()[1].Operator() == "="
	case "PAR003":
		file, diagnostics, err := Parse("precedence.sh", []byte("((a+b*c))\n"), DialectBash)
		if err != nil || len(diagnostics) != 0 {
			return false
		}
		expressions := expressionsForKind(file, NodeArithmetic)
		return len(expressions) == 1 && expressions[0].Operator() == "+" && len(expressions[0].Children()) == 2 && expressions[0].Children()[1].Operator() == "*"
	case "PAR004":
		result, err := Check(t.Context(), "dialect.sh", []byte("[[ x ]]\n"), Options{Dialect: DialectPOSIX})
		return err == nil && hasCode(result.Diagnostics, "SHD1001")
	case "PAR005":
		file, diagnostics, err := Parse("document.sh", []byte("cat <<EOF\nbody\nEOF\n"), DialectPOSIX)
		if err != nil || len(diagnostics) != 0 || len(file.Nodes()) != 1 {
			return false
		}
		redirections := file.Nodes()[0].Redirections()
		return len(redirections) == 1 && redirections[0].HereDocument() != nil && string(redirections[0].HereDocument().Body()) == "body\n"
	case "ANA001":
		result, err := Check(t.Context(), "control.sh", []byte("break\n"), Options{Dialect: DialectPOSIX})
		return err == nil && hasCode(result.Diagnostics, "SHC1001")
	case "ANA002":
		result, err := Check(t.Context(), "quoted.sh", []byte("printf %s \"$value\"\n"), Options{Dialect: DialectPOSIX})
		return err == nil && !hasCode(result.Diagnostics, "SHE1001")
	case "ANA003":
		result, err := Check(t.Context(), "dynamic.sh", []byte("eval \"$generated\"\n"), Options{Dialect: DialectPOSIX})
		return err == nil && !result.AnalysisExact && hasCode(result.Diagnostics, "SHI1001")
	case "DATA001":
		result, err := Check(t.Context(), "join.sh", []byte("set -u\nif maybe; then value=1; fi\nprintf '%s' \"$value\"\n"), Options{Dialect: DialectPOSIX})
		diagnostic, ok := diagnosticByCode(result.Diagnostics, "SHV1001")
		return err == nil && ok && diagnostic.Confidence == ConfidenceLikely
	case "API001":
		result, err := Check(t.Context(), "span.sh", []byte("break\n"), Options{Dialect: DialectPOSIX})
		diagnostic, ok := diagnosticByCode(result.Diagnostics, "SHC1001")
		return err == nil && ok && diagnostic.Primary.Start.Offset == 0 && diagnostic.Primary.End.Offset == len("break")
	case "API002":
		result, err := Check(t.Context(), "cap.sh", []byte(strings.Repeat("break;", 4)), Options{Dialect: DialectPOSIX, MaxDiagnostics: 3})
		return err == nil && len(result.Diagnostics) == 3
	case "REC001":
		source := []byte("echo before;; echo after\n")
		file, diagnostics, err := Parse("recovery.sh", source, DialectPOSIX)
		if err != nil || file == nil || !hasCode(diagnostics, "SHS1005") {
			return false
		}
		for _, node := range file.Nodes() {
			span := node.Span()
			if node.Kind() == NodeCommand && span.Start.Offset >= len("echo before;; ") && string(source[span.Start.Offset:span.End.Offset]) == "echo after" {
				return true
			}
		}
		return false
	case "SRC001":
		resolver := mapResolver{files: map[string][]byte{"a.sh": []byte(". b.sh\n"), "b.sh": []byte(". a.sh\n")}}
		result, err := Check(t.Context(), "a.sh", resolver.files["a.sh"], Options{Dialect: DialectPOSIX, AnalyzeSourced: true, Resolver: resolver})
		return err == nil && !result.AnalysisExact && hasCode(result.Diagnostics, "SHI1001")
	default:
		t.Fatalf("no kill probe for mutant %s", id)
		return false
	}
}
