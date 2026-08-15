package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestWriteValidationAggregatesNotices(t *testing.T) {
	var stdout bytes.Buffer
	diagnostic := Diagnostic{
		Code:       "BAT9002",
		Severity:   "info",
		Message:    "opaque",
		Kind:       "batch",
		TargetPath: "service.cmd",
		Span: &SourceSpan{
			Start: SourcePosition{Offset: 1, Line: 1, Column: 2},
			End:   SourcePosition{Offset: 2, Line: 1, Column: 3},
		},
	}
	err := WriteValidation(&stdout, ".", []ValidationFile{{
		Kind:       "batch",
		TargetPath: "service.cmd",
		Notices:    []Diagnostic{diagnostic},
	}})
	if err != nil {
		t.Fatal(err)
	}
	var envelope map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope["status"] != "ok" || len(envelope["notices"].([]any)) != 1 {
		t.Fatalf("envelope = %#v", envelope)
	}
	file := envelope["data"].(map[string]any)["files"].([]any)[0].(map[string]any)
	if len(file["notices"].([]any)) != 1 {
		t.Fatalf("file = %#v", file)
	}
}

func TestExistingValidationJSONOmitsNotices(t *testing.T) {
	var stdout bytes.Buffer
	if err := WriteValidation(&stdout, "serverDZ.cfg", []ValidationFile{SimpleValidationFile("server", "serverDZ.cfg", "", nil)}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stdout.String(), `"notices"`) {
		t.Fatalf("legacy output unexpectedly contains notices: %s", stdout.String())
	}
}
