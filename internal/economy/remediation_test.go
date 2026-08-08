package economy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dzcli/internal/economyconfig"
)

func TestDeleteTypeXMLSelectsDuplicateOccurrence(t *testing.T) {
	data := []byte("<types>\r\n  <type name=\"A\"><nominal>1</nominal></type>\r\n  <type name=\"A\"><nominal>2</nominal></type>\r\n</types>\r\n")
	if _, _, err := DeleteTypeXML(data, TypeDeleteOptions{TypeName: "A"}); err == nil || !strings.Contains(err.Error(), "--occurrence") {
		t.Fatalf("ambiguous delete error = %v", err)
	}
	updated, changed, err := DeleteTypeXML(data, TypeDeleteOptions{TypeName: "A", Occurrence: 2, OccurrenceSet: true})
	if err != nil {
		t.Fatalf("delete occurrence: %v", err)
	}
	if !changed || strings.Contains(string(updated), "<nominal>2</nominal>") || !strings.Contains(string(updated), "\r\n") {
		t.Fatalf("unexpected update: %s", updated)
	}
	if _, _, err := DeleteTypeXML(data, TypeDeleteOptions{TypeName: "A", Occurrence: 3, OccurrenceSet: true}); err == nil {
		t.Fatal("out-of-range occurrence error = nil")
	}
}

func TestEventSpawnMutationsPreserveUnknownContentAndCRLF(t *testing.T) {
	data := []byte("<?xml version=\"1.0\"?>\r\n<eventposdef>\r\n  <!-- keep -->\r\n  <event name=\"A\" custom=\"yes\">\r\n    <unknown value=\"keep\"><pos x=\"99\" z=\"99\" /></unknown>\r\n    <pos x=\"1\" z=\"2\" />\r\n  </event>\r\n</eventposdef>\r\n")
	position, _ := ParseEventSpawnPosition("3,4,90,5")
	zone, _ := ParseEventSpawnZone("0,1,2,3,4")
	updated, changed, err := UpdateEventSpawnXML(data, EventSpawnUpdateOptions{Name: "A", Rename: "B", AddPositions: []EventSpawnPosition{position}, RemovePositions: []int{1}, SetZone: &zone})
	if err != nil {
		t.Fatalf("update event spawn: %v", err)
	}
	text := string(updated)
	for _, expected := range []string{"<!-- keep -->", `custom="yes"`, `<unknown value="keep"><pos x="99" z="99" /></unknown>`, `name="B"`, `x="3" z="4" a="90" y="5"`, `smin="0"`, "\r\n    <pos"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("updated XML missing %q:\n%s", expected, text)
		}
	}
	if !changed {
		t.Fatal("changed = false")
	}
	entries, err := ParseEventSpawnEntriesData(updated, "test.xml")
	if err != nil || len(entries) != 1 || len(entries[0].Positions) != 1 || len(entries[0].Zones) != 1 {
		t.Fatalf("parsed entries = %#v, err = %v", entries, err)
	}
}

func TestEventSpawnDiscoveryAndMutationIgnoreNestedUnknownEvents(t *testing.T) {
	data := []byte(`<eventposdef><unknown><event name="Nested"><pos x="9" z="9" /></event></unknown><event name="Direct"><pos x="1" z="2" /></event></eventposdef>`)
	entries, err := ParseEventSpawnEntriesData(data, "spawns.xml")
	if err != nil {
		t.Fatalf("parse entries: %v", err)
	}
	if len(entries) != 1 || entries[0].Name != "Direct" {
		t.Fatalf("entries = %#v, want only direct event", entries)
	}
	if _, _, err := UpdateEventSpawnXML(data, EventSpawnUpdateOptions{Name: "Nested", Rename: "Changed"}); err == nil {
		t.Fatal("nested event update error = nil")
	}
	position, _ := ParseEventSpawnPosition("3,4")
	updated, _, err := UpdateEventSpawnXML(data, EventSpawnUpdateOptions{Name: "Direct", AddPositions: []EventSpawnPosition{position}})
	if err != nil {
		t.Fatalf("update direct event: %v", err)
	}
	if !strings.Contains(string(updated), `<unknown><event name="Nested"><pos x="9" z="9" /></event></unknown>`) {
		t.Fatalf("nested unknown event was changed: %s", updated)
	}
	deleted, _, err := DeleteEventSpawnXML(updated, "Direct", 0, false)
	if err != nil {
		t.Fatalf("delete direct event: %v", err)
	}
	if !strings.Contains(string(deleted), `name="Nested"`) || strings.Contains(string(deleted), `name="Direct"`) {
		t.Fatalf("delete selected wrong event: %s", deleted)
	}
}

func TestEventSpawnCreateDeleteAndEmptyPrevention(t *testing.T) {
	data := []byte("<eventposdef>\n</eventposdef>\n")
	position, _ := ParseEventSpawnPosition("1,2")
	created, _, err := CreateEventSpawnXML(data, "A", []EventSpawnPosition{position}, nil)
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	if _, _, err := CreateEventSpawnXML(created, "A", []EventSpawnPosition{position}, nil); err == nil {
		t.Fatal("duplicate create error = nil")
	}
	if _, _, err := UpdateEventSpawnXML(created, EventSpawnUpdateOptions{Name: "A", SetPositionsSet: true}); err == nil || !strings.Contains(err.Error(), "cannot be empty") {
		t.Fatalf("empty update error = %v", err)
	}
	deleted, _, err := DeleteEventSpawnXML(created, "A", 0, false)
	if err != nil || strings.Contains(string(deleted), `name="A"`) {
		t.Fatalf("delete event: %v\n%s", err, deleted)
	}
}

func TestEventSpawnInspectionAndRemediationAllowInvalidSiblings(t *testing.T) {
	data := []byte("<eventposdef>\n  <event name=\"Empty\" />\n  <event name=\"Broken\"><pos x=\"bad\" /></event>\n</eventposdef>\n")
	entries, err := ParseEventSpawnEntriesData(data, "spawns.xml")
	if err != nil {
		t.Fatalf("inspect invalid entries: %v", err)
	}
	if len(entries) != 2 || len(entries[0].Issues) == 0 || len(entries[1].Issues) == 0 {
		t.Fatalf("entries = %#v", entries)
	}
	deleted, _, err := DeleteEventSpawnXML(data, "Empty", 0, false)
	if err != nil || strings.Contains(string(deleted), `name="Empty"`) || !strings.Contains(string(deleted), `name="Broken"`) {
		t.Fatalf("delete invalid entry: %v\n%s", err, deleted)
	}
	position, _ := ParseEventSpawnPosition("1,2")
	created, _, err := CreateEventSpawnXML(deleted, "Valid", []EventSpawnPosition{position}, nil)
	if err != nil || !strings.Contains(string(created), `name="Valid"`) {
		t.Fatalf("create beside invalid entry: %v\n%s", err, created)
	}
}

func TestEventSpawnStructuralValidationStillRejectsUnsafeDocuments(t *testing.T) {
	for _, data := range []string{`<wrong><event name="A" /></wrong>`, `<eventposdef><event name="A"></eventposdef>`} {
		if _, err := ParseEventSpawnEntriesData([]byte(data), "spawns.xml"); err == nil {
			t.Fatalf("structural error = nil for %q", data)
		}
	}
}

func TestEventSpawnZoneCopyAndPlaceholderDetection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cfgeventspawns.xml")
	data := `<eventposdef><event name="Source"><zone smin="0" smax="1" dmin="2" dmax="3" r="40" /><zone smin="1" smax="2" dmin="3" dmax="4" r="50" /></event></eventposdef>`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	zones, err := EventSpawnZonesFile(path, "Source", 0, false)
	if err != nil || len(zones) != 2 {
		t.Fatalf("zones = %#v, err = %v", zones, err)
	}
	created, _, err := CreateEventSpawnXMLWithOptions([]byte(`<eventposdef />`), EventSpawnCreateOptions{Name: "Target", Zones: zones})
	if err != nil || strings.Count(string(created), "<zone ") != 2 {
		t.Fatalf("copied zones: %v\n%s", err, created)
	}
	zero, _ := ParseEventSpawnZone("0,0.0,-0,0,0")
	if !IsPlaceholderEventSpawnZone(zero) {
		t.Fatal("zero zone not detected")
	}
}

func TestChangedEconomyXMLIsWhitespaceClean(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cfgeventspawns.xml")
	data := []byte("<eventposdef>  \r\n  <event name=\"A\">\r\n    <pos x=\"1\" z=\"2\" />   \r\n  </event>\t\r\n</eventposdef>   ")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	mutation, err := DeleteEventSpawnFile(path, "A", 0, false)
	if err != nil {
		t.Fatal(err)
	}
	text := string(mutation.Data)
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		if strings.HasSuffix(line, " ") || strings.HasSuffix(line, "\t") {
			t.Fatalf("trailing whitespace remains in %q", line)
		}
	}
	if !strings.HasSuffix(text, "\r\n") || strings.HasSuffix(text, "\r\n\r\n") {
		t.Fatalf("terminal newline = %q", text)
	}
}

func TestEventAndEnvironmentNoOpUpdates(t *testing.T) {
	eventData := []byte(`<eventposdef><event name="A"><pos x="1" z="2" /></event></eventposdef>`)
	updated, changed, err := UpdateEventSpawnXML(eventData, EventSpawnUpdateOptions{Name: "A"})
	if err != nil || changed || string(updated) != string(eventData) {
		t.Fatalf("event no-op changed=%t err=%v", changed, err)
	}
	environmentData := []byte(`<env><territories><file path="env/a.xml" /></territories></env>`)
	updated, changed, err = UpdateEnvironmentXML(environmentData, EnvironmentUpdate, EnvironmentReferenceOptions{Kind: "path", Value: "env/a.xml", Replacement: "env/a.xml"})
	if err != nil || changed || string(updated) != string(environmentData) {
		t.Fatalf("environment no-op changed=%t err=%v", changed, err)
	}
}

func TestEventSpawnInputValidation(t *testing.T) {
	for _, value := range []string{"1", "1,x", "1,NaN", "1,2,3,4,5"} {
		if _, err := ParseEventSpawnPosition(value); err == nil {
			t.Fatalf("position %q error = nil", value)
		}
	}
	for _, value := range []string{"1,2", "1,2,3,4,x", "1,2,3,4,+Inf"} {
		if _, err := ParseEventSpawnZone(value); err == nil {
			t.Fatalf("zone %q error = nil", value)
		}
	}
}

func TestEnvironmentReferenceMutationsAndScaffoldContainment(t *testing.T) {
	data := []byte("<env>\n  <territories>\n    <territory name=\"Bear\" custom=\"keep\">\n      <!-- keep -->\n      <file usable=\"old\" />\n    </territory>\n  </territories>\n</env>\n")
	withPath, _, err := UpdateEnvironmentXML(data, EnvironmentCreate, EnvironmentReferenceOptions{Kind: "path", Value: "env/bear.xml"})
	if err != nil {
		t.Fatalf("create path: %v", err)
	}
	withUsable, _, err := UpdateEnvironmentXML(withPath, EnvironmentCreate, EnvironmentReferenceOptions{Kind: "usable", Territory: "Bear", Value: "bear"})
	if err != nil {
		t.Fatalf("create usable: %v", err)
	}
	renamed, _, err := UpdateEnvironmentXML(withUsable, EnvironmentUpdate, EnvironmentReferenceOptions{Kind: "usable", Territory: "Bear", Value: "old", Replacement: "legacy"})
	if err != nil {
		t.Fatalf("rename usable: %v", err)
	}
	text := string(renamed)
	for _, expected := range []string{`path="env/bear.xml"`, `usable="bear"`, `usable="legacy"`, `custom="keep"`, "<!-- keep -->"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("environment XML missing %q:\n%s", expected, text)
		}
	}

	root := t.TempDir()
	for _, value := range []string{"../escape.xml", `..\escape.xml`, "env/../escape.xml", `env\..\escape.xml`, `env/..\escape.xml`} {
		if _, err := ValidateEnvironmentRelativePath(root, value); err == nil {
			t.Fatalf("traversal path %q error = nil", value)
		}
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "linked")); err == nil {
		if _, err := ValidateEnvironmentRelativePath(root, "linked/escape.xml"); err == nil {
			t.Fatal("symlink escape error = nil")
		}
	}
	if err := ScaffoldTerritoryFile(root, "env/bear.xml"); err != nil {
		t.Fatalf("scaffold: %v", err)
	}
	created, err := os.ReadFile(filepath.Join(root, "env", "bear.xml"))
	if err != nil || !strings.Contains(string(created), "<territory-type />") {
		t.Fatalf("scaffold data = %q, err = %v", created, err)
	}
}

func TestTerritoryTemplateAndLiveZoneScaffolds(t *testing.T) {
	root := t.TempDir()
	zone, err := ParseTerritoryZone("Zone_Hare,0,0,0,2,100,200,50")
	if err != nil {
		t.Fatal(err)
	}
	_, created, err := ScaffoldTerritoryFileWithOptions(root, "env/hare.xml", TerritoryScaffoldOptions{Template: true, OwnerName: "AmbientHare", OwnerType: "Ambient", SuggestedName: "Zone_Hare", Zones: []TerritoryZone{zone}})
	if err != nil || !created {
		t.Fatalf("live scaffold: created=%t err=%v", created, err)
	}
	data, _ := os.ReadFile(filepath.Join(root, "env", "hare.xml"))
	for _, expected := range []string{"AmbientHare", `<territory>`, `x="100"`, `r="50"`} {
		if !strings.Contains(string(data), expected) {
			t.Fatalf("scaffold missing %q:\n%s", expected, data)
		}
	}
	if _, err := ParseTerritoryZone("Zone,0,0,0,2,1,2,0"); err == nil {
		t.Fatal("zero radius accepted")
	}
}

func TestEnvironmentDuplicateTargeting(t *testing.T) {
	data := []byte(`<env><territories><file path="env/a.xml" /><file path="env/a.xml" /><territory name="A"><file usable="u" /></territory><territory name="A"><file usable="u" /></territory></territories></env>`)
	if _, _, err := UpdateEnvironmentXML(data, EnvironmentDelete, EnvironmentReferenceOptions{Kind: "path", Value: "env/a.xml"}); err == nil || !strings.Contains(err.Error(), "--occurrence") {
		t.Fatalf("path ambiguity error = %v", err)
	}
	if _, _, err := UpdateEnvironmentXML(data, EnvironmentDelete, EnvironmentReferenceOptions{Kind: "usable", Territory: "A", Value: "u"}); err == nil || !strings.Contains(err.Error(), "--territory-occurrence") {
		t.Fatalf("territory ambiguity error = %v", err)
	}
}

func TestEconomyRemediationIntegration(t *testing.T) {
	fixture := filepath.Join("..", "..", "testdata", "economyremediation")
	root := t.TempDir()
	if err := os.CopyFS(root, os.DirFS(fixture)); err != nil {
		t.Fatalf("copy integration fixture: %v", err)
	}
	statuses, err := economyconfig.InspectEconomy(root)
	if err != nil {
		t.Fatalf("initial validation: %v", err)
	}
	initial := joinedEconomyWarnings(statuses)
	for _, expected := range []string{"duplicates a type", "min greater than nominal", "MissingUsage", "AnimalCow", "missing.xml", `usable file "hare"`, "ManualPreset"} {
		if !strings.Contains(initial, expected) {
			t.Fatalf("initial warnings missing %q:\n%s", expected, initial)
		}
	}
	for _, status := range statuses {
		if len(status.Warnings) != len(status.WarningDetails) {
			t.Fatalf("%s lacks structured warning details", status.Kind)
		}
		for _, warning := range status.WarningDetails {
			if len(warning.Remediation) == 0 && len(warning.Actions) == 0 && !warning.ManualOnly {
				t.Fatalf("warning lacks a command or manual marker: %#v", warning)
			}
		}
	}
	var remediationCommands []string
	for _, status := range statuses {
		for _, warning := range status.WarningDetails {
			remediationCommands = append(remediationCommands, warning.Remediation...)
			for _, action := range warning.Actions {
				if action.Command != "" {
					remediationCommands = append(remediationCommands, action.Command)
				}
			}
		}
	}
	commandText := strings.Join(remediationCommands, "\n")
	for _, expected := range []string{"delete economy types 'WoodenLog'", "update economy types 'NeedsRepair'", "create economy limits 'usage' 'MissingUsage'", "delete economy environment path 'env/missing.xml'", "create economy environment path 'env/hare.xml'"} {
		if !strings.Contains(commandText, expected) {
			t.Fatalf("remediation commands missing %q:\n%s", expected, commandText)
		}
	}

	duplicatePath := filepath.Join(root, "mods", "duplicate_types.xml")
	writeTestMutation(t, duplicatePath, DeleteTypeFile, TypeDeleteOptions{TypeName: "WoodenLog"})
	typesPath := filepath.Join(root, "db", "types.xml")
	typeMutation, err := UpdateTypesFile(typesPath, TypeUpdateOptions{TypeName: "NeedsRepair", Scalars: map[string]int{"nominal": 2}})
	if err != nil || WriteFileMutation(typesPath, typeMutation) != nil {
		t.Fatalf("repair relationship: %v", err)
	}
	limitsPath := filepath.Join(root, "cfglimitsdefinition.xml")
	limitMutation, err := UpdateLimitsFile(limitsPath, "usage", "MissingUsage", LimitAdd)
	if err != nil || WriteFileMutation(limitsPath, limitMutation) != nil {
		t.Fatalf("add missing limit: %v", err)
	}
	position, _ := ParseEventSpawnPosition("10,20")
	spawnsPath := filepath.Join(root, "cfgeventspawns.xml")
	spawnMutation, err := CreateEventSpawnFile(spawnsPath, "AnimalCow", []EventSpawnPosition{position}, nil)
	if err != nil || WriteFileMutation(spawnsPath, spawnMutation) != nil {
		t.Fatalf("create event spawn: %v", err)
	}
	environmentPath := filepath.Join(root, "cfgenvironment.xml")
	deleteMissing, err := UpdateEnvironmentFile(environmentPath, EnvironmentDelete, EnvironmentReferenceOptions{Kind: "path", Value: "env/missing.xml"})
	if err != nil || WriteFileMutation(environmentPath, deleteMissing) != nil {
		t.Fatalf("delete stale environment path: %v", err)
	}
	registerHare, err := UpdateEnvironmentFile(environmentPath, EnvironmentCreate, EnvironmentReferenceOptions{Kind: "path", Value: "env/hare.xml"})
	if err != nil || WriteFileMutation(environmentPath, registerHare) != nil {
		t.Fatalf("register environment path: %v", err)
	}

	statuses, err = economyconfig.InspectEconomy(root)
	if err != nil {
		t.Fatalf("post-remediation validation: %v", err)
	}
	remaining := joinedEconomyWarnings(statuses)
	if !strings.Contains(remaining, "ManualPreset") {
		t.Fatalf("manual-only warning missing:\n%s", remaining)
	}
	for _, resolved := range []string{"duplicates a type", "min greater than nominal", "MissingUsage", "AnimalCow", "missing.xml", `usable file "hare"`} {
		if strings.Contains(remaining, resolved) {
			t.Fatalf("warning %q remained:\n%s", resolved, remaining)
		}
	}
	for _, path := range []string{duplicatePath, spawnsPath} {
		data, readErr := os.ReadFile(path)
		if readErr != nil || !strings.Contains(string(data), "unrelated content survives") {
			t.Fatalf("unrelated XML was not preserved in %s: %v\n%s", path, readErr, data)
		}
	}
}

func writeTestMutation(t *testing.T, path string, mutate func(string, TypeDeleteOptions) (FileMutation, error), options TypeDeleteOptions) {
	t.Helper()
	mutation, err := mutate(path, options)
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteFileMutation(path, mutation); err != nil {
		t.Fatal(err)
	}
}

func joinedEconomyWarnings(statuses []economyconfig.FileStatus) string {
	var warnings []string
	for _, status := range statuses {
		warnings = append(warnings, status.Warnings...)
	}
	return strings.Join(warnings, "\n")
}
