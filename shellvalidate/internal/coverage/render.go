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
	output.WriteString("Generated from `testdata/features.json` and `testdata/rules.json`. Run `go generate ./shellvalidate` after catalog changes.\n\n")
	output.WriteString("## Syntax features\n\n")
	for _, id := range featureIDs {
		fmt.Fprintf(&output, "- `%s`\n", id)
	}
	output.WriteString("\n## Diagnostic rules\n\n")
	for _, code := range ruleCodes {
		fmt.Fprintf(&output, "- `%s`\n", code)
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
