package delete

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeleteTypeCommandDryRunOccurrenceAndWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "types.xml")
	data := `<types><type name="A"><nominal>1</nominal></type><type name="A"><nominal>2</nominal></type></types>`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	command := NewCommand(&bytes.Buffer{})
	command.SetArgs([]string{"A", "--file", path})
	if err := command.Execute(); err == nil || !strings.Contains(err.Error(), "--occurrence") {
		t.Fatalf("ambiguity error = %v", err)
	}
	var output bytes.Buffer
	command = NewCommand(&output)
	command.SetArgs([]string{"A", "--file", path, "--occurrence", "2", "--dry-run"})
	if err := command.Execute(); err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if strings.Contains(output.String(), "<nominal>2</nominal>") {
		t.Fatalf("dry-run output retained target: %s", output.String())
	}
	original, _ := os.ReadFile(path)
	if string(original) != data {
		t.Fatal("dry run wrote the file")
	}
	command = NewCommand(&bytes.Buffer{})
	command.SetArgs([]string{"A", "--file", path, "--occurrence", "2"})
	if err := command.Execute(); err != nil {
		t.Fatalf("write: %v", err)
	}
	written, _ := os.ReadFile(path)
	if strings.Contains(string(written), "<nominal>2</nominal>") {
		t.Fatalf("written file retained target: %s", written)
	}
}

func TestDeleteTypeCommandRequiresReadableFile(t *testing.T) {
	command := NewCommand(&bytes.Buffer{})
	command.SetArgs([]string{"A", "--file", filepath.Join(t.TempDir(), "missing.xml")})
	if err := command.Execute(); err == nil {
		t.Fatal("missing file error = nil")
	}
}
