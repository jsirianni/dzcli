package coverage

import (
	"strings"
	"testing"
)

func TestParseProfile(t *testing.T) {
	profile, err := ParseProfile(strings.NewReader("mode: atomic\na.go:1.1,2.2 3 1\na.go:3.1,4.2 1 0\n"))
	if err != nil {
		t.Fatal(err)
	}
	if profile.Statements != 4 || profile.CoveredStatements != 3 || profile.Percent() != 75 {
		t.Fatalf("profile = %#v (%.1f%%)", profile, profile.Percent())
	}
}

func TestParseProfileRejectsInvalidInput(t *testing.T) {
	for _, input := range []string{"", "wat\n", "mode: set\n", "mode: set\na.go nope 1\n", "mode: set\na.go 1 nope\n"} {
		if _, err := ParseProfile(strings.NewReader(input)); err == nil {
			t.Fatalf("ParseProfile(%q) succeeded", input)
		}
	}
}
