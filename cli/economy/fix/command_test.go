package fix

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommandPreviewsByDefaultAndRejectsIncompleteApply(t *testing.T) {
	root := t.TempDir()
	fixture := filepath.Join("..", "..", "..", "testdata", "economyremediation")
	if err := os.CopyFS(root, os.DirFS(fixture)); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	command := NewCommand(&output)
	command.SetArgs([]string{root})
	if err := command.Execute(); err != nil {
		t.Fatalf("preview: %v", err)
	}
	for _, expected := range []string{"ORDER", "CLASS", "DESTRUCTIVE", "semantic", "placeholder", "deletion"} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("preview missing %q:\n%s", expected, output.String())
		}
	}
	output.Reset()
	command = NewCommand(&output)
	command.SetArgs([]string{root, "--apply"})
	if err := command.Execute(); err == nil {
		t.Fatal("incomplete apply returned success")
	}
	if !strings.Contains(output.String(), "remaining") {
		t.Fatalf("apply output:\n%s", output.String())
	}
}

func TestCommandRequiresApplyForDestructiveAuthorization(t *testing.T) {
	command := NewCommand(&bytes.Buffer{})
	command.SetArgs([]string{".", "--allow-destructive"})
	if err := command.Execute(); err == nil || !strings.Contains(err.Error(), "requires --apply") {
		t.Fatalf("error = %v", err)
	}
}
