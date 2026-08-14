package coverage

import "testing"

func TestSpecificationObligationCount(t *testing.T) {
	entry := specificationEntry{
		Dialects: []string{"posix", "bash"}, Alternatives: []string{"one", "two"},
		Optional: []string{"two"}, Cardinality: []string{"zero", "one", "many"},
	}
	if got := specificationObligationCount("grammar", entry); got != 9 {
		t.Fatalf("grammar obligations=%d want=9", got)
	}
	if got := specificationObligationCount("lexical", specificationEntry{}); got != 1 {
		t.Fatalf("lexical obligations=%d want=1", got)
	}
}
