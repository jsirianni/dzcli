package validation

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestRenderTextStatusesCompactsWarningGroupsAtThreshold(t *testing.T) {
	var stdout bytes.Buffer

	err := RenderTextStatuses(&stdout, []TextStatus{{
		Kind: "kind",
		Path: "file",
		Warnings: []TextWarning{
			{Message: "first", Remediation: []string{"shared"}, GroupKey: "same", GroupTitle: "similar warnings", ItemLabel: "A"},
			{Message: "second", Remediation: []string{"shared"}, GroupKey: "same", GroupTitle: "similar warnings", ItemLabel: "B"},
			{Message: "third", Remediation: []string{"shared"}, GroupKey: "same", GroupTitle: "similar warnings", ItemLabel: "C"},
		},
	}}, DefaultTextOptions())

	if err != nil {
		t.Fatalf("RenderTextStatuses returned error: %v", err)
	}
	output := stdout.String()
	assertContains(t, output, `kind file warning: 3 similar warnings: "A", "B", "C"`)
	assertContains(t, output, "kind file remediation: shared")
	assertNotContains(t, output, "warning: first")
}

func TestRenderTextStatusesLeavesSmallWarningGroupsExpanded(t *testing.T) {
	var stdout bytes.Buffer

	err := RenderTextStatuses(&stdout, []TextStatus{{
		Kind: "kind",
		Path: "file",
		Warnings: []TextWarning{
			{Message: "first", GroupKey: "same", GroupTitle: "similar warnings", ItemLabel: "A"},
			{Message: "second", GroupKey: "same", GroupTitle: "similar warnings", ItemLabel: "B"},
		},
	}}, DefaultTextOptions())

	if err != nil {
		t.Fatalf("RenderTextStatuses returned error: %v", err)
	}
	output := stdout.String()
	assertContains(t, output, "kind file warning: first")
	assertContains(t, output, "kind file warning: second")
	assertNotContains(t, output, "2 similar warnings")
}

func TestRenderTextStatusesDoesNotCompactStatusLines(t *testing.T) {
	var stdout bytes.Buffer

	err := RenderTextStatuses(&stdout, []TextStatus{
		{Kind: "kind", Path: "ok-a"},
		{Kind: "kind", Path: "ok-b"},
		{Kind: "kind", Path: "bad", Err: errForTest("broken")},
	}, DefaultTextOptions())

	if err != ErrFailed {
		t.Fatalf("RenderTextStatuses err = %v, want ErrFailed", err)
	}
	output := stdout.String()
	assertContains(t, output, "kind ok-a ok")
	assertContains(t, output, "kind ok-b ok")
	assertContains(t, output, "kind bad failed: broken")
}

func TestValidateWarningModeRejectsUnsupportedValue(t *testing.T) {
	var mode string
	command := &cobra.Command{Use: "validate"}
	AddWarningModeFlag(command, &mode)
	if err := command.PersistentFlags().Set("warnings", "loud"); err != nil {
		t.Fatalf("set warnings flag: %v", err)
	}

	err := ValidateWarningMode(command)

	if err == nil {
		t.Fatal("ValidateWarningMode err = nil")
	}
	assertContains(t, err.Error(), `unsupported warning output "loud"`)
}

type errForTest string

func (err errForTest) Error() string {
	return string(err)
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
