package validate

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"dzcli/cli/validation"
	"dzcli/internal/economyconfig"
)

func TestNewCommandMetadata(t *testing.T) {
	command := NewCommand(&bytes.Buffer{})

	assertEqual(t, command.Use, "economy <mission-root|cfgeconomycore.xml|economy-file>")
	assertEqual(t, command.Short, "Validate central economy files")
}

func TestNewCommandExecutesAggregateValidation(t *testing.T) {
	var stdout bytes.Buffer
	command := NewCommand(&stdout)
	command.SetArgs([]string{fixturePath(t, "mission")})

	err := command.Execute()

	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	assertContains(t, stdout.String(), "cfgeconomycore.xml ok")
	assertContains(t, stdout.String(), "cfglimitsdefinition.xml ok")
	assertContains(t, stdout.String(), filepath.Join("db", "types.xml")+" ok (2 types)")
}

func TestValidateEconomyReturnsConfigError(t *testing.T) {
	var stdout bytes.Buffer

	err := ValidateEconomy(fixturePath(t, "mission", "mods", "valid_types.xml"), &stdout)

	if err == nil {
		t.Fatal("err = nil, want config error")
	}
	assertContains(t, err.Error(), "economy: failed")
}

func TestValidateEconomyReturnsValidationError(t *testing.T) {
	var stdout bytes.Buffer

	err := ValidateEconomy(fixturePath(t, "mission", "cfgeconomycore_bad_ref.xml"), &stdout)

	if err != validation.ErrFailed {
		t.Fatalf("err = %v, want validation.ErrFailed", err)
	}
	assertContains(t, stdout.String(), "failed")
}

func TestValidateEconomyCoreWrapper(t *testing.T) {
	var stdout bytes.Buffer

	err := ValidateEconomyCore(fixturePath(t, "mission", "cfgeconomycore.xml"), &stdout)

	if err != nil {
		t.Fatalf("ValidateEconomyCore returned error: %v", err)
	}
	assertContains(t, stdout.String(), "cfgeconomycore.xml ok")
}

func TestValidateEconomyCoreWrapperReturnsConfigError(t *testing.T) {
	var stdout bytes.Buffer

	err := ValidateEconomyCore(fixturePath(t, "mission", "missing.xml"), &stdout)

	if err == nil {
		t.Fatal("err = nil, want config error")
	}
	assertContains(t, err.Error(), "cfgeconomycore: failed")
}

func TestPrintWarningsWritesEachWarning(t *testing.T) {
	var stdout bytes.Buffer

	printWarnings(&stdout, economyconfig.FileStatus{
		Kind:     "types",
		Path:     "types.xml",
		Warnings: []string{"first", "second"},
	})

	assertContains(t, stdout.String(), "types types.xml warning: first")
	assertContains(t, stdout.String(), "types types.xml warning: second")
}

func TestValidateEconomyPrintsActionableAndManualRemediation(t *testing.T) {
	var stdout bytes.Buffer
	if err := ValidateEconomy(fixturePath(t, "economyremediation"), &stdout); err != nil {
		t.Fatalf("ValidateEconomy returned error: %v", err)
	}
	output := stdout.String()
	assertContains(t, output, "remediation: dzcli delete economy types 'WoodenLog'")
	assertContains(t, output, "remediation: input required: provide --pos coordinates or --copy-zone-from a valid event")
	assertContains(t, output, "remediation: validation-only; edit the XML manually")
}

func TestValidateEconomyCompactsSimilarWarningsAtThreshold(t *testing.T) {
	root := economyRemediationRootWithEvents(t, "StaticTruck")
	var stdout bytes.Buffer

	if err := ValidateEconomy(root, &stdout); err != nil {
		t.Fatalf("ValidateEconomy returned error: %v", err)
	}

	eventsPath := filepath.Join(root, "db", "events.xml")
	output := stdout.String()
	assertContains(t, output, "events "+eventsPath+" ok")
	assertContains(t, output, `events `+eventsPath+` warning: 3 fixed events have no matching cfgeventspawns.xml event: "AmbientBear", "AnimalCow", "StaticTruck"`)
	assertContains(t, output, "events "+eventsPath+" remediation: input required: provide --pos coordinates or --copy-zone-from a valid event")
	assertNotContains(t, output, `warning: fixed event "AmbientBear" has no matching cfgeventspawns.xml event`)
}

func TestValidateEconomyLeavesSmallWarningGroupsExpanded(t *testing.T) {
	var stdout bytes.Buffer

	if err := ValidateEconomy(fixturePath(t, "economyremediation"), &stdout); err != nil {
		t.Fatalf("ValidateEconomy returned error: %v", err)
	}

	output := stdout.String()
	assertContains(t, output, `warning: fixed event "AmbientBear" has no matching cfgeventspawns.xml event`)
	assertContains(t, output, `warning: fixed event "AnimalCow" has no matching cfgeventspawns.xml event`)
	assertNotContains(t, output, "2 fixed events have no matching")
}

func TestValidateEconomyFullWarningsPreservesExpandedOutput(t *testing.T) {
	root := economyRemediationRootWithEvents(t, "StaticTruck")
	var stdout bytes.Buffer

	if err := ValidateEconomyWithOptions(root, &stdout, validation.TextOptions{WarningMode: validation.WarningModeFull}); err != nil {
		t.Fatalf("ValidateEconomy returned error: %v", err)
	}

	output := stdout.String()
	assertContains(t, output, `warning: fixed event "AmbientBear" has no matching cfgeventspawns.xml event`)
	assertContains(t, output, `warning: fixed event "AnimalCow" has no matching cfgeventspawns.xml event`)
	assertContains(t, output, `warning: fixed event "StaticTruck" has no matching cfgeventspawns.xml event`)
	assertNotContains(t, output, "3 fixed events have no matching")
}

func economyRemediationRootWithEvents(t *testing.T, names ...string) string {
	t.Helper()

	root := t.TempDir()
	if err := os.CopyFS(root, os.DirFS(fixturePath(t, "economyremediation"))); err != nil {
		t.Fatalf("copy fixture: %v", err)
	}
	eventsPath := filepath.Join(root, "db", "events.xml")
	data, err := os.ReadFile(eventsPath)
	if err != nil {
		t.Fatalf("read events fixture: %v", err)
	}
	var extra strings.Builder
	for _, name := range names {
		fmt.Fprintf(&extra, "  <event name=%q><position>fixed</position><active>1</active></event>\n", name)
	}
	updated := strings.Replace(string(data), "</events>", extra.String()+"</events>", 1)
	if err := os.WriteFile(eventsPath, []byte(updated), 0o600); err != nil {
		t.Fatalf("write events fixture: %v", err)
	}
	return root
}

func fixturePath(t *testing.T, parts ...string) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}

	args := append([]string{filepath.Dir(file), "..", "..", "..", "testdata"}, parts...)
	return filepath.Clean(filepath.Join(args...))
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
