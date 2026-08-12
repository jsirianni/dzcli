package shellvalidate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

type catalogReference struct {
	Source  string `json:"source"`
	Section string `json:"section"`
}

type featureCatalogEntry struct {
	ID           string             `json:"id"`
	References   []catalogReference `json:"references"`
	Requirements map[string]int     `json:"requirements"`
}

type ruleCatalogEntry struct {
	Code       string             `json:"code"`
	Category   string             `json:"category"`
	References []catalogReference `json:"references"`
}

type caseDiagnostic struct {
	Code        string `json:"code"`
	StartOffset *int   `json:"startOffset,omitempty"`
	EndOffset   *int   `json:"endOffset,omitempty"`
}

type caseMetadata struct {
	ID             string   `json:"id"`
	Dialect        string   `json:"dialect"`
	Phase          string   `json:"phase"`
	Classification string   `json:"classification"`
	Features       []string `json:"features"`
	Rules          []string `json:"rules"`
	Expected       struct {
		SyntaxValid   bool             `json:"syntaxValid"`
		Valid         bool             `json:"valid"`
		AnalysisExact bool             `json:"analysisExact"`
		Diagnostics   []caseDiagnostic `json:"diagnostics"`
	} `json:"expected"`
	References []catalogReference `json:"references"`
	Tags       []string           `json:"tags"`
	ScriptPath string             `json:"-"`
}

func readJSONFile(t *testing.T, name string, target any) {
	t.Helper()
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		t.Fatalf("decode %s: %v", name, err)
	}
}

func loadCorpus(t *testing.T) []caseMetadata {
	t.Helper()
	var result []caseMetadata
	err := filepath.Walk("testdata/cases", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || filepath.Ext(path) != ".json" {
			return nil
		}
		var item caseMetadata
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if decodeErr := json.Unmarshal(data, &item); decodeErr != nil {
			return fmt.Errorf("decode %s: %w", path, decodeErr)
		}
		item.ScriptPath = strings.TrimSuffix(path, ".json") + ".sh"
		result = append(result, item)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func TestCorpusMetadata(t *testing.T) {
	var features []featureCatalogEntry
	var rules []ruleCatalogEntry
	readJSONFile(t, "testdata/features.json", &features)
	readJSONFile(t, "testdata/rules.json", &rules)
	featureSet := make(map[string]struct{})
	ruleSet := make(map[string]struct{})
	categorySet := make(map[string]struct{})
	for _, item := range features {
		if item.ID == "" || len(item.References) == 0 {
			t.Fatalf("invalid feature entry: %#v", item)
		}
		if _, exists := featureSet[item.ID]; exists {
			t.Fatalf("duplicate feature %s", item.ID)
		}
		featureSet[item.ID] = struct{}{}
	}
	for _, item := range rules {
		if item.Code == "" || item.Category == "" || len(item.References) == 0 {
			t.Fatalf("invalid rule entry: %#v", item)
		}
		if _, exists := ruleSet[item.Code]; exists {
			t.Fatalf("duplicate rule %s", item.Code)
		}
		ruleSet[item.Code] = struct{}{}
		categorySet[item.Category] = struct{}{}
	}
	for category := range categorySet {
		if _, ok := knownCategories[category]; !ok {
			t.Fatalf("catalog has unknown category %q", category)
		}
	}

	caseIDs := make(map[string]struct{})
	featureCases := make(map[string]map[string]int)
	ruleCases := make(map[string]int)
	for _, item := range loadCorpus(t) {
		if item.ID == "" || item.Dialect == "" || item.Phase == "" || item.Classification == "" || len(item.References) == 0 {
			t.Fatalf("invalid case metadata: %#v", item)
		}
		if _, exists := caseIDs[item.ID]; exists {
			t.Fatalf("duplicate case %s", item.ID)
		}
		caseIDs[item.ID] = struct{}{}
		if _, err := os.Stat(item.ScriptPath); err != nil {
			t.Fatalf("case %s has no script: %v", item.ID, err)
		}
		for _, feature := range item.Features {
			if _, ok := featureSet[feature]; !ok {
				t.Fatalf("case %s references unknown feature %s", item.ID, feature)
			}
			if featureCases[feature] == nil {
				featureCases[feature] = make(map[string]int)
			}
			featureCases[feature][item.Classification]++
		}
		for _, code := range item.Rules {
			if _, ok := ruleSet[code]; !ok {
				t.Fatalf("case %s references unknown rule %s", item.ID, code)
			}
			ruleCases[code]++
		}
		for _, diagnostic := range item.Expected.Diagnostics {
			if _, ok := ruleSet[diagnostic.Code]; !ok {
				t.Fatalf("case %s expects undeclared code %s", item.ID, diagnostic.Code)
			}
		}
	}
	for _, feature := range features {
		if len(featureCases[feature.ID]) == 0 {
			t.Errorf("feature %s has no case", feature.ID)
		}
		for class, count := range feature.Requirements {
			name := requirementClass(class)
			if featureCases[feature.ID][name] < count {
				t.Errorf("feature %s needs %d %s cases; got %d", feature.ID, count, name, featureCases[feature.ID][name])
			}
		}
	}
}

func requirementClass(name string) string {
	var result strings.Builder
	for index, r := range name {
		if r >= 'A' && r <= 'Z' {
			if index > 0 {
				result.WriteByte('-')
			}
			result.WriteRune(r + ('a' - 'A'))
		} else {
			result.WriteRune(r)
		}
	}
	return result.String()
}

func TestCorpusBehavior(t *testing.T) {
	for _, item := range loadCorpus(t) {
		item := item
		t.Run(item.ID, func(t *testing.T) {
			source, err := os.ReadFile(item.ScriptPath)
			if err != nil {
				t.Fatal(err)
			}
			dialect := DialectPOSIX
			if item.Dialect == "bash" {
				dialect = DialectBash
			}
			result, err := Check(t.Context(), item.ScriptPath, source, Options{Dialect: dialect})
			if err != nil {
				t.Fatal(err)
			}
			if result.SyntaxValid != item.Expected.SyntaxValid || result.Valid != item.Expected.Valid || result.AnalysisExact != item.Expected.AnalysisExact {
				t.Fatalf("status = syntax:%v valid:%v exact:%v diagnostics=%#v", result.SyntaxValid, result.Valid, result.AnalysisExact, result.Diagnostics)
			}
			for _, expected := range item.Expected.Diagnostics {
				if !hasCode(result.Diagnostics, expected.Code) {
					t.Errorf("missing %s in %#v", expected.Code, result.Diagnostics)
				}
			}
		})
	}
}

func hasCode(items []Diagnostic, code string) bool {
	for _, item := range items {
		if item.Code == code {
			return true
		}
	}
	return false
}
