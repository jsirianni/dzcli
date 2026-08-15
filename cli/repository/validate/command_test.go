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

func TestValidateRepositoryIncludesBatchFilesWithNonfatalNotices(t *testing.T) {
	serverRoot := filepath.Join(t.TempDir(), "server")
	if err := os.MkdirAll(filepath.Join(serverRoot, "mpmissions"), 0o755); err != nil {
		t.Fatal(err)
	}
	batchPath := filepath.Join(serverRoot, "service.cmd")
	writeTestFile(t, batchPath, "vendor-tool.exe --flag\n")
	var stdout bytes.Buffer

	err := validateRepositoryWithOptions(serverRoot, &stdout, validation.DefaultTextOptions())

	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, stdout.String(), "batch "+batchPath+" ok (analysis incomplete)")
	assertNotContains(t, stdout.String(), "notice: analysis incomplete: 1 opaque or runtime-dependent region(s)")

	stdout.Reset()
	if err := validateRepositoryWithOptions(serverRoot, &stdout, validation.TextOptions{WarningMode: validation.WarningModeCompact, Verbose: true}); err != nil {
		t.Fatal(err)
	}
	assertContains(t, stdout.String(), "notice: analysis incomplete: 1 opaque or runtime-dependent region(s)")
}

func TestValidateRepositoryBatchFailureReturnsValidationFailure(t *testing.T) {
	serverRoot := filepath.Join(t.TempDir(), "server")
	if err := os.MkdirAll(filepath.Join(serverRoot, "mpmissions"), 0o755); err != nil {
		t.Fatal(err)
	}
	batchPath := filepath.Join(serverRoot, "service.bat")
	writeTestFile(t, batchPath, "goto missing\n")
	var stdout bytes.Buffer

	err := validateRepositoryWithOptions(serverRoot, &stdout, validation.DefaultTextOptions())

	if err != validation.ErrFailed {
		t.Fatalf("err = %v, want validation.ErrFailed", err)
	}
	assertContains(t, stdout.String(), "batch "+batchPath+" failed")
	assertContains(t, stdout.String(), "[BAT6002]")
}

func TestValidateRepositoryJSONIncludesBatchDiagnosticDetails(t *testing.T) {
	serverRoot := filepath.Join(t.TempDir(), "server")
	if err := os.MkdirAll(filepath.Join(serverRoot, "mpmissions"), 0o755); err != nil {
		t.Fatal(err)
	}
	batchPath := filepath.Join(serverRoot, "service.cmd")
	writeTestFile(t, batchPath, "vendor-tool.exe --flag\n")
	var stdout bytes.Buffer

	err := validateRepositoryJSON(serverRoot, &stdout)

	if err != nil {
		t.Fatal(err)
	}
	envelope := decodeJSONEnvelope(t, stdout.String())
	notices := jsonArray(t, envelope["notices"])
	assertEqual(t, len(notices), 1)
	notice := jsonObject(t, notices[0])
	assertEqual(t, notice["code"].(string), "BAT9002")
	assertEqual(t, notice["severity"].(string), "info")
	files := jsonArray(t, jsonObject(t, envelope["data"])["files"])
	assertEqual(t, len(files), 1)
	file := jsonObject(t, files[0])
	assertEqual(t, file["kind"].(string), "batch")
	assertEqual(t, file["target_path"].(string), batchPath)
	assertEqual(t, file["summary"].(string), "analysis incomplete")
}

func TestInspectRepositoryDeduplicatesOverlappingBatchTrees(t *testing.T) {
	root := t.TempDir()
	serverRoot := filepath.Join(root, "server")
	nestedRoot := filepath.Join(serverRoot, "nested")
	if err := os.MkdirAll(filepath.Join(serverRoot, "mpmissions"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(nestedRoot, "mpmissions"), 0o755); err != nil {
		t.Fatal(err)
	}
	batchPath := filepath.Join(nestedRoot, "service.cmd")
	writeTestFile(t, batchPath, "echo ok\n")

	result, err := inspectRepository(root)

	if err != nil {
		t.Fatal(err)
	}
	batchCount := 0
	for _, file := range result.files {
		if file.Kind == "batch" {
			batchCount++
			assertEqual(t, file.TargetPath, batchPath)
		}
	}
	assertEqual(t, batchCount, 1)
}

func TestInspectRepositoryDoesNotScanOutsideServerRootsForBatchFiles(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(root, "outside.cmd")
	writeTestFile(t, outside, "goto missing\n")
	serverRoot := filepath.Join(root, "server")
	if err := os.MkdirAll(filepath.Join(serverRoot, "mpmissions"), 0o755); err != nil {
		t.Fatal(err)
	}
	inside := filepath.Join(serverRoot, "inside.cmd")
	writeTestFile(t, inside, "echo ok\n")

	result, err := inspectRepository(root)

	if err != nil {
		t.Fatal(err)
	}
	for _, file := range result.files {
		if file.TargetPath == outside {
			t.Fatalf("outside batch file was validated: %#v", file)
		}
	}
	assertEqual(t, len(result.files), 1)
	assertEqual(t, result.files[0].TargetPath, inside)
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
