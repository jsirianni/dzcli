package verbs

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetEconomyTypeDuplicatesAndComparison(t *testing.T) {
	path := filepath.Join(t.TempDir(), "types.xml")
	data := `<types><type name="A"><nominal>0</nominal><usage name="Military" /></type><type name="A"><usage name="Military" /></type><type name="B" /></types>`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := listEconomyTypesAdvanced(path, "", "", true, false, &output); err != nil {
		t.Fatalf("list duplicates: %v", err)
	}
	text := output.String()
	if strings.Count(text, "A") < 2 || strings.Contains(text, "B") || !strings.Contains(text, "canonical") || !strings.Contains(text, "duplicate") {
		t.Fatalf("duplicate output: %s", text)
	}
	output.Reset()
	if err := listEconomyTypesAdvanced(path, "", "A", false, true, &output); err != nil {
		t.Fatalf("compare: %v", err)
	}
	if !strings.Contains(output.String(), "nominal") || !strings.Contains(output.String(), "<absent>") || !strings.Contains(output.String(), "0") {
		t.Fatalf("compare output: %s", output.String())
	}
}
