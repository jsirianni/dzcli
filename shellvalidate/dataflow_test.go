package shellvalidate

import "testing"

func TestDataflowControlFlowJoins(t *testing.T) {
	tests := []struct {
		name       string
		source     string
		wantRead   bool
		confidence Confidence
	}{
		{
			name:       "one branch assigns",
			source:     "set -u\nif condition; then value=one; fi\nprintf '%s' \"$value\"\n",
			wantRead:   true,
			confidence: ConfidenceLikely,
		},
		{
			name:     "both branches assign",
			source:   "set -u\nif condition; then value=one; else value=two; fi\nprintf '%s' \"$value\"\n",
			wantRead: false,
		},
		{
			name:       "loop may execute zero times",
			source:     "set -u\nwhile condition; do value=one; done\nprintf '%s' \"$value\"\n",
			wantRead:   true,
			confidence: ConfidenceLikely,
		},
		{
			name:     "literal true branch",
			source:   "set -u\nif true; then value=one; fi\nprintf '%s' \"$value\"\n",
			wantRead: false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := Check(t.Context(), "flow.sh", []byte(test.source), Options{Dialect: DialectPOSIX})
			if err != nil {
				t.Fatal(err)
			}
			var reads []Diagnostic
			for _, diagnostic := range result.Diagnostics {
				if diagnostic.Code == "SHV1001" {
					reads = append(reads, diagnostic)
				}
			}
			if !test.wantRead && len(reads) != 0 {
				t.Fatalf("unexpected nounset diagnostic: %#v", reads)
			}
			if test.wantRead && (len(reads) != 1 || reads[0].Confidence != test.confidence) {
				t.Fatalf("nounset diagnostics = %#v", reads)
			}
		})
	}
}

func TestDataflowExecutionContexts(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		wantRead bool
	}{
		{
			name:     "pipeline read does not escape",
			source:   "set -u\nprintf x | read value\nprintf '%s' \"$value\"\n",
			wantRead: true,
		},
		{
			name:     "subshell assignment does not escape",
			source:   "set -u\n(value=one)\nprintf '%s' \"$value\"\n",
			wantRead: true,
		},
		{
			name:     "brace assignment remains visible",
			source:   "set -u\n{ value=one; }\nprintf '%s' \"$value\"\n",
			wantRead: false,
		},
		{
			name:     "function body does not execute at definition",
			source:   "set -u\nwork() { value=one; }\nprintf '%s' \"$value\"\n",
			wantRead: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := Check(t.Context(), "context.sh", []byte(test.source), Options{Dialect: DialectPOSIX})
			if err != nil {
				t.Fatal(err)
			}
			got := hasCode(result.Diagnostics, "SHV1001")
			if got != test.wantRead {
				t.Fatalf("SHV1001 present=%v, want %v: %#v", got, test.wantRead, result.Diagnostics)
			}
		})
	}
}

func TestDataflowNounsetTriState(t *testing.T) {
	source := "if condition; then set -u; else set +u; fi\nprintf '%s' \"$missing\"\n"
	result, err := Check(t.Context(), "options.sh", []byte(source), Options{Dialect: DialectPOSIX})
	if err != nil {
		t.Fatal(err)
	}
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Code == "SHV1001" {
			if diagnostic.Confidence != ConfidenceLikely {
				t.Fatalf("tri-state nounset confidence = %v", diagnostic.Confidence)
			}
			return
		}
	}
	t.Fatalf("tri-state nounset produced no diagnostic: %#v", result.Diagnostics)
}
