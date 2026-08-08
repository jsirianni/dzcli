package remediation

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dzcli/internal/economy"
)

func TestEventSpawnCommandsDryRunAndWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cfgeventspawns.xml")
	if err := os.WriteFile(path, []byte("<eventposdef>\n</eventposdef>\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	create := NewCreateEventSpawnsCommand(&output)
	create.SetArgs([]string{"A", "--file", path, "--pos", "1,2", "--dry-run"})
	if err := create.Execute(); err != nil {
		t.Fatalf("create dry run: %v", err)
	}
	if !strings.Contains(output.String(), `name="A"`) {
		t.Fatalf("dry-run output: %s", output.String())
	}
	original, _ := os.ReadFile(path)
	if strings.Contains(string(original), `name="A"`) {
		t.Fatal("dry run wrote file")
	}

	output.Reset()
	create = NewCreateEventSpawnsCommand(&output)
	create.SetArgs([]string{"A", "--file", path, "--pos", "1,2"})
	if err := create.Execute(); err != nil {
		t.Fatalf("create write: %v", err)
	}
	update := NewUpdateEventSpawnsCommand(&output)
	update.SetArgs([]string{"A", "--file", path, "--add-pos", "3,4", "--set-zone", "0,1,0,1,5"})
	if err := update.Execute(); err != nil {
		t.Fatalf("update write: %v", err)
	}
	getOutput := bytes.Buffer{}
	get := NewGetEventSpawnsCommand(&getOutput)
	get.SetArgs([]string{"A", "--file", path})
	if err := get.Execute(); err != nil {
		t.Fatalf("get: %v", err)
	}
	if !strings.Contains(getOutput.String(), "1,2;3,4") || !strings.Contains(getOutput.String(), "0,1,0,1,5") {
		t.Fatalf("get output: %s", getOutput.String())
	}
	deleteCommand := NewDeleteEventSpawnsCommand(&output)
	deleteCommand.SetArgs([]string{"A", "--file", path})
	if err := deleteCommand.Execute(); err != nil {
		t.Fatalf("delete: %v", err)
	}
}

func TestGetEventsShowsPositionActivityAndOccurrence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.xml")
	data := `<events><event name="A"><position>fixed</position><active>0</active></event><event name="A"><position>player</position></event></events>`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	command := NewGetEventsCommand(&output)
	command.SetArgs([]string{"A", "--file", path})
	if err := command.Execute(); err != nil {
		t.Fatalf("get events: %v", err)
	}
	text := output.String()
	for _, expected := range []string{"fixed", "player", "<absent>", "0", "2"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("events output missing %q:\n%s", expected, text)
		}
	}
}

func TestEventSpawnCommandRejectsInvalidInputs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cfgeventspawns.xml")
	_ = os.WriteFile(path, []byte("<eventposdef />"), 0o600)
	command := NewCreateEventSpawnsCommand(&bytes.Buffer{})
	command.SetArgs([]string{"A", "--file", path, "--pos", "bad"})
	if err := command.Execute(); err == nil {
		t.Fatal("invalid coordinate error = nil")
	}
}

func TestEnvironmentCommandsScaffoldAndNeverDeletePhysicalFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "cfgenvironment.xml")
	if err := os.WriteFile(path, []byte(`<env><territories><territory name="Bear" /></territories></env>`), 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	createPath := NewCreateEnvironmentCommand(&output)
	createPath.SetArgs([]string{"path", "env/bear.xml", "--file", path, "--scaffold"})
	if err := createPath.Execute(); err != nil {
		t.Fatalf("create path: %v", err)
	}
	physical := filepath.Join(root, "env", "bear.xml")
	if _, err := os.Stat(physical); err != nil {
		t.Fatalf("scaffold missing: %v", err)
	}
	createUsable := NewCreateEnvironmentCommand(&output)
	createUsable.SetArgs([]string{"usable", "Bear", "bear", "--file", path})
	if err := createUsable.Execute(); err != nil {
		t.Fatalf("create usable: %v", err)
	}
	getOutput := bytes.Buffer{}
	get := NewGetEnvironmentCommand(&getOutput)
	get.SetArgs([]string{"Bear", "--file", path})
	if err := get.Execute(); err != nil || !strings.Contains(getOutput.String(), "usable") || !strings.Contains(getOutput.String(), "true") {
		t.Fatalf("get environment: %v\n%s", err, getOutput.String())
	}
	deletePath := NewDeleteEnvironmentCommand(&output)
	deletePath.SetArgs([]string{"path", "env/bear.xml", "--file", path})
	if err := deletePath.Execute(); err != nil {
		t.Fatalf("delete reference: %v", err)
	}
	if _, err := os.Stat(physical); err != nil {
		t.Fatalf("physical file was deleted: %v", err)
	}
}

func TestEnvironmentScaffoldRollsBackWhenEnvironmentWriteFails(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "cfgenvironment.xml")
	original := []byte(`<env><territories /></env>`)
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	originalWriter := writeFileMutation
	writeFileMutation = func(string, economy.FileMutation) error { return errors.New("forced write failure") }
	t.Cleanup(func() { writeFileMutation = originalWriter })
	command := NewCreateEnvironmentCommand(&bytes.Buffer{})
	command.SetArgs([]string{"path", "env/new.xml", "--file", path, "--scaffold"})
	err := command.Execute()
	if err == nil || !strings.Contains(err.Error(), "forced write failure") {
		t.Fatalf("write failure error = %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, "env", "new.xml")); !os.IsNotExist(statErr) {
		t.Fatalf("scaffold was not rolled back: %v", statErr)
	}
	written, readErr := os.ReadFile(path)
	if readErr != nil || !bytes.Equal(written, original) {
		t.Fatalf("environment file changed: %v %q", readErr, written)
	}
}

func TestEnvironmentCommandRejectsMissingAndTraversalPaths(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "cfgenvironment.xml")
	_ = os.WriteFile(path, []byte(`<env><territories /></env>`), 0o600)
	for _, value := range []string{"env/missing.xml", "../escape.xml"} {
		command := NewCreateEnvironmentCommand(&bytes.Buffer{})
		command.SetArgs([]string{"path", value, "--file", path})
		if err := command.Execute(); err == nil {
			t.Fatalf("path %q error = nil", value)
		}
	}
}

func TestRemediationCommandsReportConfigIOFailures(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.xml")
	eventCommand := NewGetEventSpawnsCommand(&bytes.Buffer{})
	eventCommand.SetArgs([]string{"--file", missing})
	if err := eventCommand.Execute(); err == nil {
		t.Fatal("event-spawn I/O error = nil")
	}
	environmentCommand := NewGetEnvironmentCommand(&bytes.Buffer{})
	environmentCommand.SetArgs([]string{"--file", missing})
	if err := environmentCommand.Execute(); err == nil {
		t.Fatal("environment I/O error = nil")
	}
}
