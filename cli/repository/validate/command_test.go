package validate

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"dzcli/cli/validation"
)

func TestNewCommandMetadata(t *testing.T) {
	command := NewCommand(&bytes.Buffer{})

	assertEqual(t, command.Use, "all <repo-or-servers-root>")
	assertEqual(t, command.Short, "Validate a repository or servers root")
	assertContains(t, strings.Join(command.Aliases, ","), "repo")
}

func TestValidateRepositorySkipsPartialEconomyAndValidatesXML(t *testing.T) {
	serverRoot := filepath.Join(t.TempDir(), "server")
	missionRoot := filepath.Join(serverRoot, "mpmissions", "partial")
	writeTestFile(t, filepath.Join(missionRoot, "db", "types.xml"), `<types><type name="Apple" /></types>`)
	copyTestFile(t,
		fixturePath(t, "expansionai", "valid", "empty.deerisle", "expansion", "settings", "AILocationSettings.json"),
		filepath.Join(missionRoot, "expansion", "settings", "AILocationSettings.json"),
	)
	var stdout bytes.Buffer

	err := validateRepositoryWithOptions(serverRoot, &stdout, validation.DefaultTextOptions())

	if err != nil {
		t.Fatalf("validateRepositoryWithOptions returned error: %v", err)
	}
	output := stdout.String()
	assertContains(t, output, "xml "+filepath.Join(missionRoot, "db", "types.xml")+" ok")
	assertContains(t, output, "expansion-ai-locations "+filepath.Join(missionRoot, "expansion", "settings", "AILocationSettings.json")+" ok (1 locations)")
	assertNotContains(t, output, "economy")
	assertNotContains(t, output, "cfgeconomycore")
}

func TestValidateRepositoryJSONReportsXMLFailure(t *testing.T) {
	serverRoot := filepath.Join(t.TempDir(), "server")
	missionRoot := filepath.Join(serverRoot, "mpmissions", "partial")
	writeTestFile(t, filepath.Join(missionRoot, "broken.xml"), `<broken>`)
	var stdout bytes.Buffer

	err := validateRepositoryJSON(serverRoot, &stdout)

	if err != validation.ErrFailed {
		t.Fatalf("err = %v, want validation.ErrFailed", err)
	}
	envelope := decodeJSONEnvelope(t, stdout.String())
	assertEqual(t, envelope["status"].(string), "failed")
	files := jsonArray(t, jsonObject(t, envelope["data"])["files"])
	assertEqual(t, len(files), 1)
	file := jsonObject(t, files[0])
	assertEqual(t, file["kind"].(string), "xml")
	assertEqual(t, file["status"].(string), "failed")
	assertContains(t, file["target_path"].(string), "broken.xml")
}

func writeTestFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create fixture directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write fixture %s: %v", path, err)
	}
}

func copyTestFile(t *testing.T, source string, destination string) {
	t.Helper()
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatalf("read fixture %s: %v", source, err)
	}
	writeTestFile(t, destination, string(data))
}

func fixturePath(t *testing.T, parts ...string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
	return filepath.Join(append([]string{root, "testdata"}, parts...)...)
}

func decodeJSONEnvelope(t *testing.T, raw string) map[string]any {
	t.Helper()
	var envelope map[string]any
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		t.Fatalf("json.Unmarshal(%q): %v", raw, err)
	}
	return envelope
}

func jsonObject(t *testing.T, value any) map[string]any {
	t.Helper()
	object, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("value = %#v, want JSON object", value)
	}
	return object
}

func jsonArray(t *testing.T, value any) []any {
	t.Helper()
	array, ok := value.([]any)
	if !ok {
		t.Fatalf("value = %#v, want JSON array", value)
	}
	return array
}

func assertContains(t *testing.T, haystack string, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Fatalf("%q does not contain %q", haystack, needle)
	}
}

func assertNotContains(t *testing.T, haystack string, needle string) {
	t.Helper()
	if strings.Contains(haystack, needle) {
		t.Fatalf("%q contains %q", haystack, needle)
	}
}

func assertEqual[T comparable](t *testing.T, got T, want T) {
	t.Helper()
	if got != want {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}
