// Package coverage renders the shell validation catalog summary.
package coverage

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

type feature struct {
	ID string `json:"id"`
}

type rule struct {
	Code string `json:"code"`
}

type specificationEntry struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

type generatedModels struct {
	ModelVersion string `json:"modelVersion"`
	Models       []struct {
		ID          string         `json:"id"`
		Count       int            `json:"count"`
		SHA256      string         `json:"sha256"`
		Bounds      map[string]int `json:"bounds"`
		Obligations int            `json:"obligations"`
		Executed    int            `json:"executed"`
		Strength    string         `json:"strength"`
		Exclusions  []struct {
			Reason string `json:"reason"`
			Count  int    `json:"count"`
		} `json:"exclusions"`
	} `json:"models"`
}

type mutant struct {
	ID       string `json:"id"`
	Target   string `json:"target"`
	Critical bool   `json:"critical"`
	KilledBy string `json:"killedBy"`
}

// Render returns the canonical COVERAGE.md bytes for a shellvalidate root.
func Render(root string) ([]byte, error) {
	var features []feature
	var rules []rule
	scope, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer scope.Close()
	if err := read(scope, "testdata/features.json", &features); err != nil {
		return nil, err
	}
	if err := read(scope, "testdata/rules.json", &rules); err != nil {
		return nil, err
	}
	catalogNames := []string{"lexical", "grammar", "dialects", "semantics", "robustness"}
	catalogs := make(map[string][]specificationEntry, len(catalogNames))
	for _, name := range catalogNames {
		var entries []specificationEntry
		if err := read(scope, "testdata/spec/"+name+".json", &entries); err != nil {
			return nil, err
		}
		catalogs[name] = entries
	}
	var models generatedModels
	if err := read(scope, "testdata/spec/generated_models.json", &models); err != nil {
		return nil, err
	}
	var mutants []mutant
	if err := read(scope, "testdata/spec/mutants.json", &mutants); err != nil {
		return nil, err
	}
	featureIDs := make([]string, 0, len(features))
	ruleCodes := make([]string, 0, len(rules))
	for _, item := range features {
		featureIDs = append(featureIDs, item.ID)
	}
	for _, item := range rules {
		ruleCodes = append(ruleCodes, item.Code)
	}
	sort.Strings(featureIDs)
	sort.Strings(ruleCodes)
	var output strings.Builder
	output.WriteString("# Coverage Catalog\n\n")
	output.WriteString("Generated from the machine-readable files under `testdata/`. Run `go generate ./shellvalidate` after catalog changes.\n\n")
	output.WriteString("## Specification catalogs\n\n")
	output.WriteString("| Catalog | Entries | Required | Extensions | Classified |\n")
	output.WriteString("|---|---:|---:|---:|---:|\n")
	for _, name := range catalogNames {
		required, extensions := 0, 0
		for _, entry := range catalogs[name] {
			switch entry.Status {
			case "required":
				required++
			case "extension":
				extensions++
			}
		}
		fmt.Fprintf(&output, "| %s | %d | %d | %d | %d |\n", name, len(catalogs[name]), required, extensions, len(catalogs[name])-required-extensions)
	}
	output.WriteString("\n")
	output.WriteString("## Syntax features\n\n")
	for _, id := range featureIDs {
		fmt.Fprintf(&output, "- `%s`\n", id)
	}
	output.WriteString("\n## Diagnostic rules\n\n")
	for _, code := range ruleCodes {
		fmt.Fprintf(&output, "- `%s`\n", code)
	}
	output.WriteString("\n## Deterministic generated models\n\n")
	fmt.Fprintf(&output, "Model version: `%s`.\n\n", models.ModelVersion)
	output.WriteString("This is the expected deterministic-test contract. Runtime CI evidence, including observed coverage and mutation statuses, is published as workflow artifacts.\n\n")
	output.WriteString("| Model | Vectors | Obligations | SHA-256 | Strength and bounds |\n")
	output.WriteString("|---|---:|---:|---|---|\n")
	totalVectors := 0
	for _, model := range models.Models {
		bounds := "-"
		if len(model.Bounds) != 0 {
			keys := make([]string, 0, len(model.Bounds))
			for key := range model.Bounds {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			parts := make([]string, 0, len(keys))
			for _, key := range keys {
				parts = append(parts, fmt.Sprintf("%s=%d", key, model.Bounds[key]))
			}
			bounds = strings.Join(parts, ", ")
		}
		detail := model.Strength
		if bounds != "-" {
			detail += "; " + bounds
		}
		if len(model.Exclusions) != 0 {
			parts := make([]string, 0, len(model.Exclusions))
			for _, exclusion := range model.Exclusions {
				parts = append(parts, fmt.Sprintf("%s=%d", exclusion.Reason, exclusion.Count))
			}
			detail += "; excluded: " + strings.Join(parts, ", ")
		}
		fmt.Fprintf(&output, "| `%s` | %d | %d | `%s` | %s |\n", model.ID, model.Count, model.Obligations, model.SHA256, detail)
		totalVectors += model.Count
	}
	fmt.Fprintf(&output, "\nTotal generated vectors: **%d**.\n", totalVectors)
	output.WriteString("\n## Expected critical mutation contract\n\n")
	output.WriteString("These declarations identify the required source mutations and their anchored tests. They do not assert that a mutation was killed; only a mutation-runner result artifact is observed evidence.\n\n")
	for _, item := range mutants {
		classification := "non-critical"
		if item.Critical {
			classification = "critical"
		}
		fmt.Fprintf(&output, "- `%s` (%s): %s; required test anchor `%s`\n", item.ID, classification, item.Target, item.KilledBy)
	}
	return []byte(output.String()), nil
}

func read(root *os.Root, name string, target any) error {
	file, err := root.Open(name)
	if err != nil {
		return err
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("decode %s: %w", name, err)
	}
	return nil
}
