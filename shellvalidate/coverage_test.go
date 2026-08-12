package shellvalidate

import (
	"bytes"
	"os"
	"testing"

	"dzcli/shellvalidate/internal/coverage"
)

func TestCoverageDocumentCurrent(t *testing.T) {
	want, err := coverage.Render(".")
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile("COVERAGE.md")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("COVERAGE.md is stale; run go generate ./shellvalidate")
	}
}
