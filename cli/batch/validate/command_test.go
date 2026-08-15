package validate

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dzcli/cli/output"
	"dzcli/cli/validation"
	"dzcli/internal/batchvalidate"
)

func TestNewCommandMetadata(t *testing.T) {
	command := NewCommand(&bytes.Buffer{})
	if command.Use != "batch <file-or-dir>" {
		t.Fatalf("Use = %q", command.Use)
	}
	if command.Short != "Validate Windows batch files" {
		t.Fatalf("Short = %q", command.Short)
	}
}

func TestFindBatchFilesSupportsFilesAndRecursiveDirectories(t *testing.T) {
	root := t.TempDir()
	first := writeBatchTestFile(t, filepath.Join(root, "A.CMD"), "echo first")
	second := writeBatchTestFile(t, filepath.Join(root, "nested", "b.bat"), "echo second")
	writeBatchTestFile(t, filepath.Join(root, "ignored.txt"), "ignored")
	writeBatchTestFile(t, filepath.Join(root, ".git", "ignored.cmd"), "goto missing")

	files, err := FindBatchFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{first, second}
	if len(files) != len(want) {
		t.Fatalf("files = %#v", files)
	}
	for index := range want {
		if files[index] != want[index] {
			t.Fatalf("files[%d] = %q, want %q", index, files[index], want[index])
		}
	}
	skipped, err := FindBatchFiles(filepath.Join(root, ".git"))
	if err != nil || len(skipped) != 0 {
		t.Fatalf(".git files = %#v, %v", skipped, err)
	}

	files, err = FindBatchFiles(first)
	if err != nil || len(files) != 1 || files[0] != first {
		t.Fatalf("single file = %#v, %v", files, err)
	}
}

func TestFindBatchFilesRejectsInvalidPaths(t *testing.T) {
	root := t.TempDir()
	textPath := writeBatchTestFile(t, filepath.Join(root, "script.txt"), "echo text")
	if _, err := FindBatchFiles(textPath); err == nil || !strings.Contains(err.Error(), "is not a .bat or .cmd file") {
		t.Fatalf("non-batch err = %v", err)
	}
	if _, err := FindBatchFiles(filepath.Join(root, "missing.cmd")); err == nil || !strings.Contains(err.Error(), "inspect path") {
		t.Fatalf("missing err = %v", err)
	}
}

func TestValidateBatchPathReportsCompleteAndIncompleteFiles(t *testing.T) {
	root := t.TempDir()
	writeBatchTestFile(t, filepath.Join(root, "complete.cmd"), "@echo off\nsetlocal enableextensions\n:done\ngoto :done\n")
	writeBatchTestFile(t, filepath.Join(root, "opaque.bat"), "vendor-tool.exe --flag\n")
	var stdout bytes.Buffer

	err := ValidateBatchPath(root, &stdout)
	if err != nil {
		t.Fatal(err)
	}
	text := stdout.String()
	assertBatchContains(t, text, "complete.cmd ok (fully validated)")
	assertBatchContains(t, text, "opaque.bat ok")
	if strings.Contains(text, "(analysis incomplete)") {
		t.Fatalf("default output contains incomplete summary: %q", text)
	}
	if strings.Contains(text, "notice:") {
		t.Fatalf("default output contains notice: %q", text)
	}

	stdout.Reset()
	err = ValidateBatchPathWithOptions(root, &stdout, validation.TextOptions{WarningMode: validation.WarningModeCompact, Verbose: true})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stdout.String(), "(analysis incomplete)") {
		t.Fatalf("verbose output contains incomplete summary: %q", stdout.String())
	}
	assertBatchContains(t, stdout.String(), "notice: analysis incomplete: 1 opaque or runtime-dependent region(s)")
}

func TestValidateBatchPathRejectsEmptyDirectory(t *testing.T) {
	err := ValidateBatchPath(t.TempDir(), &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "no .bat or .cmd files found") {
		t.Fatalf("err = %v", err)
	}
}

func TestValidateBatchPathReportsProvenErrors(t *testing.T) {
	path := writeBatchTestFile(t, filepath.Join(t.TempDir(), "invalid.cmd"), "goto missing\n")
	var stdout bytes.Buffer

	err := ValidateBatchPath(path, &stdout)
	if !errors.Is(err, validation.ErrFailed) {
		t.Fatalf("err = %v, want validation.ErrFailed", err)
	}
	assertBatchContains(t, stdout.String(), "failed")
	assertBatchContains(t, stdout.String(), "[BAT6002]")
}

func TestValidateBatchPathJSONPreservesInformationalDiagnostics(t *testing.T) {
	path := writeBatchTestFile(t, filepath.Join(t.TempDir(), "opaque.cmd"), "vendor-tool.exe --flag\n")
	var stdout bytes.Buffer

	err := ValidateBatchPathJSON(path, &stdout)
	if err != nil {
		t.Fatal(err)
	}
	envelope := decodeBatchEnvelope(t, stdout.Bytes())
	if envelope["status"] != "ok" {
		t.Fatalf("status = %#v", envelope["status"])
	}
	notices := envelope["notices"].([]any)
	if len(notices) != 1 {
		t.Fatalf("notices = %#v", notices)
	}
	notice := notices[0].(map[string]any)
	if notice["code"] != "BAT9002" || notice["severity"] != "info" {
		t.Fatalf("notice = %#v", notice)
	}
	start := notice["span"].(map[string]any)["start"].(map[string]any)
	if start["line"] != float64(1) || start["column"] != float64(1) {
		t.Fatalf("start = %#v", start)
	}
	files := envelope["data"].(map[string]any)["files"].([]any)
	file := files[0].(map[string]any)
	if file["summary"] != "analysis incomplete" || len(file["notices"].([]any)) != 1 {
		t.Fatalf("file = %#v", file)
	}
}

func TestValidateBatchPathJSONReportsDiscoveryAndValidationFailures(t *testing.T) {
	var stdout bytes.Buffer
	err := ValidateBatchPathJSON(t.TempDir(), &stdout)
	if !errors.Is(err, output.ErrRendered) {
		t.Fatalf("empty err = %v", err)
	}
	if decodeBatchEnvelope(t, stdout.Bytes())["status"] != "failed" {
		t.Fatalf("empty output = %s", stdout.String())
	}

	stdout.Reset()
	path := writeBatchTestFile(t, filepath.Join(t.TempDir(), "invalid.bat"), "goto missing\n")
	err = ValidateBatchPathJSON(path, &stdout)
	if !errors.Is(err, validation.ErrFailed) {
		t.Fatalf("invalid err = %v", err)
	}
	envelope := decodeBatchEnvelope(t, stdout.Bytes())
	failure := envelope["failures"].([]any)[0].(map[string]any)
	if failure["code"] != "BAT6002" || failure["severity"] != "error" {
		t.Fatalf("failure = %#v", failure)
	}
}

func TestStatusAdaptersPreserveEverySeverityAndReadFailure(t *testing.T) {
	span := batchvalidate.Span{Start: batchvalidate.Position{Line: 2, Column: 3}, End: batchvalidate.Position{Line: 2, Column: 4}}
	status := FileStatus{Path: "mixed.cmd", Result: batchvalidate.Result{
		FullyValidated: false,
		Diagnostics: []batchvalidate.Diagnostic{
			{Code: "I", Severity: batchvalidate.SeverityInfo, Message: "info", Span: span},
			{Code: "W", Severity: batchvalidate.SeverityWarning, Message: "warn", Span: span},
			{Code: "E", Severity: batchvalidate.SeverityError, Message: "error", Span: span},
		},
	}}
	text := TextStatuses([]FileStatus{status})[0]
	if text.Err == nil || len(text.Notices) != 1 || len(text.Warnings) != 1 {
		t.Fatalf("text = %#v", text)
	}
	file := ValidationFiles([]FileStatus{status})[0]
	if file.Status != output.StatusFailed || len(file.Notices) != 1 || len(file.Warnings) != 1 || len(file.Failures) != 1 {
		t.Fatalf("file = %#v", file)
	}
	if !HasFailures([]FileStatus{status}) {
		t.Fatal("HasFailures = false")
	}

	readStatus := FileStatus{Path: "missing.cmd", Err: os.ErrNotExist}
	if TextStatuses([]FileStatus{readStatus})[0].Err == nil || ValidationFiles([]FileStatus{readStatus})[0].Status != output.StatusFailed || !HasFailures([]FileStatus{readStatus}) {
		t.Fatal("read failure was not preserved")
	}
	if HasFailures([]FileStatus{{Path: "ok.cmd", Result: batchvalidate.Result{FullyValidated: true}}}) {
		t.Fatal("valid result failed")
	}
}

func writeBatchTestFile(t *testing.T, path string, content string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return filepath.Clean(path)
}

func decodeBatchEnvelope(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var envelope map[string]any
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatalf("decode %q: %v", data, err)
	}
	return envelope
}

func assertBatchContains(t *testing.T, value string, substring string) {
	t.Helper()
	if !strings.Contains(value, substring) {
		t.Fatalf("%q does not contain %q", value, substring)
	}
}
