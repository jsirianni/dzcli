package economyconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEventSpawnValidationIncludesZonesAndY(t *testing.T) {
	data := []byte(`<eventposdef><event name="A"><pos x="1" z="2" y="bad" /><zone smin="0" smax="1" dmin="0" dmax="bad" /></event></eventposdef>`)
	_, err := ParseEventSpawnsData(data, "spawns.xml")
	if err == nil || !strings.Contains(err.Error(), "pos y expected float") || !strings.Contains(err.Error(), "zone dmax expected float") || !strings.Contains(err.Error(), "zone missing r") {
		t.Fatalf("validation error = %v", err)
	}
}

func TestEventSpawnValidationRejectsEmptyEvent(t *testing.T) {
	_, err := ParseEventSpawnsData([]byte(`<eventposdef><event name="A" /></eventposdef>`), "spawns.xml")
	if err == nil || !strings.Contains(err.Error(), "requires at least one pos or zone") {
		t.Fatalf("empty event error = %v", err)
	}
}

func TestMissingFixedSpawnUsesActiveAndEnvironmentTerritoryContext(t *testing.T) {
	root := t.TempDir()
	writeRemediationFixture(t, filepath.Join(root, "cfgeconomycore.xml"), `<economycore />`)
	writeRemediationFixture(t, filepath.Join(root, "cfglimitsdefinition.xml"), `<lists><categories /><tags /><usageflags /><valueflags /></lists>`)
	writeRemediationFixture(t, filepath.Join(root, "db", "types.xml"), `<types />`)
	writeRemediationFixture(t, filepath.Join(root, "db", "events.xml"), `<events>
<event name="AmbientBear"><position>fixed</position><active>1</active></event>
<event name="AnimalCow"><position>fixed</position><active>1</active></event>
<event name="StaticDisabled"><position>fixed</position><active>0</active></event>
<event name="StaticSpawned"><position>fixed</position></event>
</events>`)
	writeRemediationFixture(t, filepath.Join(root, "cfgeventspawns.xml"), `<eventposdef><event name="StaticSpawned"><pos x="1" z="2" /></event></eventposdef>`)
	writeRemediationFixture(t, filepath.Join(root, "cfgenvironment.xml"), `<env><territories><file path="env/bear.xml" /><territory name="Bear"><file usable="bear" /></territory></territories></env>`)
	writeRemediationFixture(t, filepath.Join(root, "env", "bear.xml"), `<territory-type />`)

	statuses, err := InspectEconomy(root)
	if err != nil {
		t.Fatalf("inspect economy: %v", err)
	}
	warnings := remediationWarnings(statuses)
	if strings.Contains(warnings, "AmbientBear") || strings.Contains(warnings, "StaticDisabled") || strings.Contains(warnings, "StaticSpawned") {
		t.Fatalf("unexpected warning suppression result:\n%s", warnings)
	}
	if !strings.Contains(warnings, `fixed event "AnimalCow" has no matching`) {
		t.Fatalf("missing AnimalCow warning:\n%s", warnings)
	}
}

func TestEveryEconomyWarningHasStructuredRemediation(t *testing.T) {
	statuses, err := InspectEconomy(filepath.Join("..", "..", "testdata", "economyconfig", "full"))
	if err != nil {
		t.Fatalf("inspect fixture: %v", err)
	}
	for _, status := range statuses {
		if len(status.Warnings) != len(status.WarningDetails) {
			t.Fatalf("%s warning details = %d, warnings = %d", status.Kind, len(status.WarningDetails), len(status.Warnings))
		}
		for _, warning := range status.WarningDetails {
			if len(warning.Remediation) == 0 && !warning.ManualOnly {
				t.Fatalf("warning lacks remediation: %#v", warning)
			}
		}
	}
}

func TestNormalizedTypeFieldsDistinguishesAbsentAndZero(t *testing.T) {
	absent, err := ParseTypesData([]byte(`<types><type name="A" /></types>`), "types.xml")
	if err != nil {
		t.Fatal(err)
	}
	zero, err := ParseTypesData([]byte(`<types><type name="A"><nominal>0</nominal></type></types>`), "types.xml")
	if err != nil {
		t.Fatal(err)
	}
	if NormalizedTypeFields(absent.Types[0])["nominal"] != "<absent>" || NormalizedTypeFields(zero.Types[0])["nominal"] != "0" {
		t.Fatal("normalized fields did not preserve absent-versus-zero")
	}
}

func TestRemediationCommandsPowerShellQuoteValuesAndPathsWithSpaces(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Mission & Space's")
	fixture := filepath.Join("..", "..", "testdata", "economyremediation")
	if err := os.CopyFS(root, os.DirFS(fixture)); err != nil {
		t.Fatalf("copy fixture: %v", err)
	}
	typesPath := filepath.Join(root, "db", "types.xml")
	writeRemediationFixture(t, typesPath, `<types><type name="Odd Item's"><nominal>1</nominal><min>2</min><usage name="Missing &amp; Tier" /></type><type name="WoodenLog" /></types>`)
	statuses, err := InspectEconomy(root)
	if err != nil {
		t.Fatalf("inspect economy: %v", err)
	}
	var commands []string
	for _, status := range statuses {
		for _, warning := range status.WarningDetails {
			commands = append(commands, warning.Remediation...)
		}
	}
	if len(commands) == 0 {
		t.Fatal("no remediation commands generated")
	}
	for _, command := range commands {
		if strings.Contains(command, "--file ") && !strings.Contains(command, "--file '") {
			t.Fatalf("--file path is not PowerShell quoted: %s", command)
		}
		if strings.Contains(command, "--file ") && !strings.Contains(command, "Mission & Space''s") {
			t.Fatalf("quoted mission path missing escaped apostrophe: %s", command)
		}
	}
	commandText := strings.Join(commands, "\n")
	for _, expected := range []string{"types 'Odd Item''s'", "'Missing & Tier'", "types 'WoodenLog'", "event-spawns 'AnimalCow'", "path 'env/missing.xml'"} {
		if !strings.Contains(commandText, expected) {
			t.Fatalf("commands missing quoted value %q:\n%s", expected, commandText)
		}
	}
}

func writeRemediationFixture(t *testing.T, path string, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}

func remediationWarnings(statuses []FileStatus) string {
	var warnings []string
	for _, status := range statuses {
		warnings = append(warnings, status.Warnings...)
	}
	return strings.Join(warnings, "\n")
}
