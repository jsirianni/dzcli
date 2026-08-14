package evidence

import (
	"strings"
	"testing"
)

func TestCatalogObligationCount(t *testing.T) {
	tests := []struct {
		name  string
		entry catalogEntry
		want  int
	}{
		{name: "lexical", entry: catalogEntry{ID: "lex"}, want: 1},
		{name: "grammar", entry: catalogEntry{Dialects: []string{"posix", "bash"}, Alternatives: []string{"one", "two"}, Optional: []string{"two"}, Cardinality: []string{"zero", "one", "many"}}, want: 9},
		{name: "dialects", entry: catalogEntry{Accepted: []string{"one", "two"}, Rejected: []string{"three"}}, want: 3},
		{name: "semantics", entry: catalogEntry{States: []string{"off", "maybe", "on"}}, want: 3},
		{name: "robustness", entry: catalogEntry{Boundaries: []string{"N-1", "N", "N+1"}}, want: 3},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := catalogObligationCount(test.name, test.entry); got != test.want {
				t.Fatalf("count=%d want=%d", got, test.want)
			}
		})
	}
}

func TestRenderMarkdownIncludesObservedCatalogs(t *testing.T) {
	report := Report{SourceCommit: "abc", ModelVersion: "v1", Catalogs: []Catalog{{Name: "grammar", Entries: 2, Obligations: 7, Satisfied: 7}}}
	text := string(RenderMarkdown(report))
	for _, fragment := range []string{"Audited specification obligations", "`grammar`", "| 2 | 7 | 7 |"} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("markdown omitted %q:\n%s", fragment, text)
		}
	}
}
