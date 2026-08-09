package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestMainExitsOnFailure(t *testing.T) {
	var exitCode int
	Main([]string{"unknown"}, &bytes.Buffer{}, &bytes.Buffer{}, func(code int) {
		exitCode = code
	})

	if exitCode != FailureExitCode {
		t.Fatalf("exit code = %d, want %d", exitCode, FailureExitCode)
	}
}

func TestRunWithoutCommandShowsRootHelp(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := Run([]string{}, &stdout, &stderr)

	assertEqual(t, code, SuccessExitCode)
	assertEqual(t, stderr.String(), "")
	assertContains(t, stdout.String(), "Tools for DayZ server configuration")
	assertContains(t, stdout.String(), "Available Commands:")
	assertContains(t, stdout.String(), "create")
	assertContains(t, stdout.String(), "delete")
	assertContains(t, stdout.String(), "get")
	assertContains(t, stdout.String(), "update")
	assertContains(t, stdout.String(), "validate")
	assertContains(t, stdout.String(), "version")
}

func TestRunReturnsUnknownCommandFailure(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := Run([]string{"unknown"}, &stdout, &stderr)

	assertEqual(t, code, FailureExitCode)
	assertEqual(t, stdout.String(), "")
	assertContains(t, stderr.String(), "unknown command")
}

func TestRunReturnsMissingValidateArgumentFailure(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := Run([]string{"validate", "economy"}, &stdout, &stderr)

	assertEqual(t, code, FailureExitCode)
	assertEqual(t, stdout.String(), "")
	assertContains(t, stderr.String(), "accepts 1 arg(s), received 0")
}

func TestRunVersionCommandPrintsDefaultVersion(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := Run([]string{"version"}, &stdout, &stderr)

	assertEqual(t, code, SuccessExitCode)
	assertEqual(t, stdout.String(), "unknown\n")
	assertEqual(t, stderr.String(), "")
}

func TestRunVersionCommandPrintsInjectedVersion(t *testing.T) {
	originalVersion := version
	t.Cleanup(func() { version = originalVersion })
	version = "1.2.3"
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := Run([]string{"version"}, &stdout, &stderr)

	assertEqual(t, code, SuccessExitCode)
	assertEqual(t, stdout.String(), "1.2.3\n")
	assertEqual(t, stderr.String(), "")
}

func TestRunVersionCommandPrintsJSON(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := Run([]string{"--output", "json", "version"}, &stdout, &stderr)

	assertEqual(t, code, SuccessExitCode)
	assertEqual(t, stderr.String(), "")
	envelope := decodeJSONEnvelope(t, stdout.String())
	assertEqual(t, envelope["status"].(string), "ok")
	assertEqual(t, jsonObject(t, envelope["data"])["version"].(string), "unknown")
}

func TestRunJSONValidationFailureWritesEnvelopeToStdout(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	dir := mixedXMLDir(t)

	code := Run([]string{"--output", "json", "validate", "xml", dir}, &stdout, &stderr)

	assertEqual(t, code, FailureExitCode)
	assertEqual(t, stderr.String(), "")
	envelope := decodeJSONEnvelope(t, stdout.String())
	assertEqual(t, envelope["status"].(string), "failed")
	assertEqual(t, len(jsonArray(t, envelope["failures"])), 1)
	files := jsonArray(t, jsonObject(t, envelope["data"])["files"])
	assertEqual(t, len(files), 2)
}

func TestRunJSONXMLValidationReportsSingleFile(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	path := fixturePath(t, "xml", "valid", "root.xml")

	code := Run([]string{"--output", "json", "validate", "xml", path}, &stdout, &stderr)

	assertEqual(t, code, SuccessExitCode)
	assertEqual(t, stderr.String(), "")
	envelope := decodeJSONEnvelope(t, stdout.String())
	assertEqual(t, envelope["status"].(string), "ok")
	assertEqual(t, envelope["target_path"].(string), path)
	files := jsonArray(t, jsonObject(t, envelope["data"])["files"])
	assertEqual(t, len(files), 1)
	file := jsonObject(t, files[0])
	assertEqual(t, file["kind"].(string), "xml")
	assertEqual(t, file["target_path"].(string), path)
	assertEqual(t, file["status"].(string), "ok")
}

func TestRunJSONXMLValidationRejectsNonXMLFile(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	path := fixturePath(t, "xml", "valid", "ignored.txt")

	code := Run([]string{"--output", "json", "validate", "xml", path}, &stdout, &stderr)

	assertEqual(t, code, FailureExitCode)
	assertEqual(t, stderr.String(), "")
	envelope := decodeJSONEnvelope(t, stdout.String())
	assertEqual(t, envelope["status"].(string), "failed")
	assertEqual(t, envelope["target_path"].(string), path)
	failure := jsonObject(t, jsonArray(t, envelope["failures"])[0])
	assertEqual(t, failure["kind"].(string), "xml")
	assertEqual(t, failure["target_path"].(string), path)
	assertContains(t, failure["message"].(string), "is not an XML file")
}

func TestRunJSONCommandErrorWritesFailureEnvelope(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := Run([]string{"--output", "json", "get", "server"}, &stdout, &stderr)

	assertEqual(t, code, FailureExitCode)
	assertEqual(t, stderr.String(), "")
	envelope := decodeJSONEnvelope(t, stdout.String())
	assertEqual(t, envelope["status"].(string), "failed")
	assertContains(t, jsonObject(t, jsonArray(t, envelope["failures"])[0])["message"].(string), "--file is required")
}

func TestRunInvalidOutputFormatUsesTextError(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := Run([]string{"--output", "yaml", "version"}, &stdout, &stderr)

	assertEqual(t, code, FailureExitCode)
	assertEqual(t, stdout.String(), "")
	assertContains(t, stderr.String(), "unsupported output format")
}

func TestRunInvalidWarningsModeUsesTextError(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := Run([]string{"validate", "--warnings", "loud", "xml", fixturePath(t, "xml", "valid")}, &stdout, &stderr)

	assertEqual(t, code, FailureExitCode)
	assertEqual(t, stdout.String(), "")
	assertContains(t, stderr.String(), "unsupported warning output")
}

func TestRunJSONPromptBlockedIncludesRemediation(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	path := writeTempFile(t, `hostname = "Old";`)

	code := Run([]string{"--output", "json", "update", "server", "hostname", "--file", path, "--value", "New"}, &stdout, &stderr)

	assertEqual(t, code, FailureExitCode)
	assertEqual(t, stderr.String(), "")
	envelope := decodeJSONEnvelope(t, stdout.String())
	assertEqual(t, envelope["status"].(string), "failed")
	failure := jsonObject(t, jsonArray(t, envelope["failures"])[0])
	assertContains(t, failure["message"].(string), "interactive confirmation")
	assertEqual(t, len(jsonArray(t, failure["remediation"])), 3)
}

func TestRunJSONDryRunNestsContent(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	path := writeTempFile(t, `hostname = "Old";`)

	code := Run([]string{"--output", "json", "update", "server", "hostname", "--file", path, "--value", "New", "--dry-run"}, &stdout, &stderr)

	assertEqual(t, code, SuccessExitCode)
	assertEqual(t, stderr.String(), "")
	envelope := decodeJSONEnvelope(t, stdout.String())
	data := jsonObject(t, envelope["data"])
	assertEqual(t, envelope["status"].(string), "ok")
	assertEqual(t, data["dry_run"].(bool), true)
	assertContains(t, data["content"].(string), "New")
}

func TestRunJSONEconomyValidationIncludesRemediation(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := Run([]string{"--output", "json", "validate", "economy", fixturePath(t, "economyremediation")}, &stdout, &stderr)

	assertEqual(t, code, SuccessExitCode)
	assertEqual(t, stderr.String(), "")
	envelope := decodeJSONEnvelope(t, stdout.String())
	assertEqual(t, envelope["status"].(string), "ok")
	if len(jsonArray(t, envelope["warnings"])) == 0 {
		t.Fatal("warnings = 0, want remediation warnings")
	}
	if len(jsonArray(t, envelope["remediation"])) == 0 {
		t.Fatal("remediation = 0, want remediation actions")
	}
}

func TestRunJSONEconomyValidationCompactsWarningsByDefault(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	extraEvents := []string{
		"StaticTruck00",
		"StaticTruck01",
		"StaticTruck02",
		"StaticTruck03",
		"StaticTruck04",
		"StaticTruck05",
		"StaticTruck06",
		"StaticTruck07",
		"StaticTruck08",
		"StaticTruck09",
		"StaticTruck10",
	}
	root := economyRemediationRootWithEvents(t, extraEvents...)

	code := Run([]string{"--output", "json", "validate", "economy", root}, &stdout, &stderr)

	assertEqual(t, code, SuccessExitCode)
	assertEqual(t, stderr.String(), "")
	envelope := decodeJSONEnvelope(t, stdout.String())
	warning := findWarningGroup(t, jsonArray(t, envelope["warnings"]), "economy.fixed-event-missing-spawn")
	assertEqual(t, warning["message"].(string), "13 fixed events have no matching cfgeventspawns.xml event")
	group := jsonObject(t, warning["group"])
	assertEqual(t, int(group["count"].(float64)), 13)
	assertEqual(t, len(jsonArray(t, group["items"])), 10)
	assertEqual(t, int(group["omitted_items"].(float64)), 3)
	remediation := jsonObject(t, jsonArray(t, warning["remediation"])[0])
	assertContains(t, remediation["detail"].(string), "input required")
	assertNotContains(t, stdout.String(), "event-spawn-create-AmbientBear")
	assertNotContains(t, stdout.String(), `fixed event \"AmbientBear\"`)
}

func TestRunJSONEconomyValidationFullWarningsPreservesExpandedOutput(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	root := economyRemediationRootWithEvents(t, "StaticTruck")

	code := Run([]string{"--output", "json", "validate", "--warnings", "full", "economy", root}, &stdout, &stderr)

	assertEqual(t, code, SuccessExitCode)
	assertEqual(t, stderr.String(), "")
	envelope := decodeJSONEnvelope(t, stdout.String())
	warnings := jsonArray(t, envelope["warnings"])
	assertHasWarningMessage(t, warnings, `fixed event "AmbientBear" has no matching cfgeventspawns.xml event`)
	assertHasWarningMessage(t, warnings, `fixed event "AnimalCow" has no matching cfgeventspawns.xml event`)
	assertHasWarningMessage(t, warnings, `fixed event "StaticTruck" has no matching cfgeventspawns.xml event`)
	for _, item := range warnings {
		warning := jsonObject(t, item)
		if _, exists := warning["group"]; exists {
			t.Fatalf("warning group = %#v, want no compact group in full mode", warning["group"])
		}
	}
	assertContains(t, stdout.String(), "event-spawn-create-AmbientBear")
	assertContains(t, stdout.String(), "event-spawn-create-StaticTruck")
}

func TestRunJSONGetRows(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := Run([]string{"--output", "json", "get", "server", "--file", fixturePath(t, "serverconfig", "valid.cfg")}, &stdout, &stderr)

	assertEqual(t, code, SuccessExitCode)
	assertEqual(t, stderr.String(), "")
	envelope := decodeJSONEnvelope(t, stdout.String())
	rows := jsonArray(t, jsonObject(t, envelope["data"])["rows"])
	if len(rows) == 0 {
		t.Fatal("rows = 0, want server config rows")
	}
}

func TestRunJSONFixEconomyPlan(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := Run([]string{"--output", "json", "fix", "economy", fixturePath(t, "economyremediation")}, &stdout, &stderr)

	assertEqual(t, code, SuccessExitCode)
	assertEqual(t, stderr.String(), "")
	envelope := decodeJSONEnvelope(t, stdout.String())
	data := jsonObject(t, envelope["data"])
	if len(jsonArray(t, data["plan"])) == 0 {
		t.Fatal("plan = 0, want remediation plan items")
	}
	assertEqual(t, len(jsonArray(t, data["applied"])), 0)
	assertEqual(t, len(jsonArray(t, data["written"])), 0)
	if len(jsonArray(t, envelope["remediation"])) == 0 {
		t.Fatal("remediation = 0, want fix remediation actions")
	}
}

func TestRunJSONLoadoutDeleteDryRunPlan(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	loadout := fixturePath(t, "expansionai", "valid", "profiles", "ExpansionMod", "Loadouts", "TestLoadout.json")

	code := Run([]string{"--output", "json", "delete", "expansion", "ai", "loadouts", "TestLoadout", "--file", loadout, "--dry-run"}, &stdout, &stderr)

	assertEqual(t, code, SuccessExitCode)
	assertEqual(t, stderr.String(), "")
	envelope := decodeJSONEnvelope(t, stdout.String())
	data := jsonObject(t, envelope["data"])
	assertEqual(t, envelope["target_path"].(string), loadout)
	assertEqual(t, data["dry_run"].(bool), true)
	assertEqual(t, data["deleted"].(bool), false)
}

func TestRunReportsEconomyCoreParseFailure(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := Run([]string{"validate", "economy", fixturePath(t, "mission", "mods", "valid_types.xml")}, &stdout, &stderr)

	assertEqual(t, code, FailureExitCode)
	assertEqual(t, stdout.String(), "")
	assertContains(t, stderr.String(), "expected <economycore> root")
}

func TestRunReportsMissingEconomyCoreDiagnostic(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	dir := partialEconomyDir(t)

	code := Run([]string{"validate", "economy", dir}, &stdout, &stderr)

	assertEqual(t, code, FailureExitCode)
	assertEqual(t, stdout.String(), "")
	assertContains(t, stderr.String(), "full mission-root economy validation requires cfgeconomycore.xml")
	assertContains(t, stderr.String(), "dzcli validate xml <file-or-dir>")
}

func TestRunJSONReportsMissingEconomyCoreDiagnostic(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	dir := partialEconomyDir(t)

	code := Run([]string{"--output", "json", "validate", "economy", dir}, &stdout, &stderr)

	assertEqual(t, code, FailureExitCode)
	assertEqual(t, stderr.String(), "")
	envelope := decodeJSONEnvelope(t, stdout.String())
	assertEqual(t, envelope["status"].(string), "failed")
	failure := jsonObject(t, jsonArray(t, envelope["failures"])[0])
	assertEqual(t, failure["kind"].(string), "economy")
	assertEqual(t, failure["target_path"].(string), dir)
	assertContains(t, failure["message"].(string), "full mission-root economy validation requires cfgeconomycore.xml")
	assertContains(t, failure["message"].(string), "dzcli validate xml <file-or-dir>")
}

func TestRunReportsEachTypesFile(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := Run([]string{"validate", "economy", fixturePath(t, "mission", "cfgeconomycore.xml")}, &stdout, &stderr)

	assertEqual(t, code, SuccessExitCode)
	assertEqual(t, stderr.String(), "")
	assertContains(t, stdout.String(), "cfgeconomycore")
	assertContains(t, stdout.String(), "cfgeconomycore.xml ok")
	assertContains(t, stdout.String(), "base-types")
	assertContains(t, stdout.String(), filepath.Join("db", "types.xml")+" ok (2 types)")
	assertContains(t, stdout.String(), filepath.Join("mods", "valid_types.xml")+" ok (1 types)")
	assertNotContains(t, stdout.String(), "spawnabletypes")
}

func TestRunReturnsFailureWhenAReferencedTypesFileFails(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := Run([]string{"validate", "economy", fixturePath(t, "mission", "cfgeconomycore_bad_ref.xml")}, &stdout, &stderr)

	assertEqual(t, code, FailureExitCode)
	assertEqual(t, stderr.String(), "")
	assertContains(t, stdout.String(), filepath.Join("badmods", "invalid_types.badxml")+" failed")
	assertContains(t, stdout.String(), "XML syntax error")
}

func TestRunReportsEachGenericXMLFile(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := Run([]string{"validate", "xml", fixturePath(t, "xml", "valid")}, &stdout, &stderr)

	assertEqual(t, code, SuccessExitCode)
	assertEqual(t, stderr.String(), "")
	assertContains(t, stdout.String(), "xml "+filepath.Join(fixturePath(t, "xml", "valid"), "nested", "second.XML")+" ok")
	assertContains(t, stdout.String(), "xml "+filepath.Join(fixturePath(t, "xml", "valid"), "root.xml")+" ok")
	assertNotContains(t, stdout.String(), "ignored.txt")
}

func TestRunReportsSingleGenericXMLFile(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	path := fixturePath(t, "xml", "valid", "root.xml")

	code := Run([]string{"validate", "xml", path}, &stdout, &stderr)

	assertEqual(t, code, SuccessExitCode)
	assertEqual(t, stderr.String(), "")
	assertContains(t, stdout.String(), "xml "+path+" ok")
	assertNotContains(t, stdout.String(), "second.XML")
}

func TestRunReportsInitValidation(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := Run([]string{"validate", "init", fixturePath(t, "dayzinit", "valid", "init.c")}, &stdout, &stderr)

	assertEqual(t, code, SuccessExitCode)
	assertEqual(t, stderr.String(), "")
	assertContains(t, stdout.String(), "init ")
	assertContains(t, stdout.String(), " ok")

	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"validate", "init", fixturePath(t, "dayzinit", "invalid", "init.c")}, &stdout, &stderr)

	assertEqual(t, code, FailureExitCode)
	assertEqual(t, stderr.String(), "")
	assertContains(t, stdout.String(), "failed")
	assertContains(t, stdout.String(), "DZI")
}

func TestRunReportsGameplayValidation(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := Run([]string{"validate", "gameplay", fixturePath(t, "gameplayconfig", "valid.json")}, &stdout, &stderr)

	assertEqual(t, code, SuccessExitCode)
	assertEqual(t, stderr.String(), "")
	assertContains(t, stdout.String(), "gameplay")
	assertContains(t, stdout.String(), " ok")

	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"validate", "gameplay", fixturePath(t, "gameplayconfig", "unknown.json")}, &stdout, &stderr)

	assertEqual(t, code, FailureExitCode)
	assertEqual(t, stderr.String(), "")
	assertContains(t, stdout.String(), "failed")
	assertContains(t, stdout.String(), "unknown field")
}

func TestRunReportsWeatherValidation(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := Run([]string{"validate", "weather", fixturePath(t, "weatherconfig", "valid-modern", "cfgweather.xml")}, &stdout, &stderr)

	assertEqual(t, code, SuccessExitCode)
	assertEqual(t, stderr.String(), "")
	assertContains(t, stdout.String(), "weather")
	assertContains(t, stdout.String(), " ok")

	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"validate", "weather", fixturePath(t, "weatherconfig", "invalid-range", "cfgweather.xml")}, &stdout, &stderr)

	assertEqual(t, code, FailureExitCode)
	assertEqual(t, stderr.String(), "")
	assertContains(t, stdout.String(), "failed")
	assertContains(t, stdout.String(), "less than or equal")
}

func TestRunValidateAllReportsRepositoryValidation(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	repo := t.TempDir()
	serverRoot := filepath.Join(repo, "server-one")
	if err := os.MkdirAll(filepath.Join(serverRoot, "mpmissions"), 0o755); err != nil {
		t.Fatalf("create server fixture: %v", err)
	}
	serverConfig, err := os.ReadFile(fixturePath(t, "serverconfig", "valid.cfg"))
	if err != nil {
		t.Fatalf("read server config fixture: %v", err)
	}
	writeTestFile(t, filepath.Join(serverRoot, "serverDZ.cfg"), string(serverConfig))
	missionRoot := filepath.Join(serverRoot, "mpmissions", "dayzOffline.test")
	if err := os.CopyFS(missionRoot, os.DirFS(fixturePath(t, "mission"))); err != nil {
		t.Fatalf("copy mission fixture: %v", err)
	}

	code := Run([]string{"validate", "all", repo}, &stdout, &stderr)

	assertEqual(t, code, SuccessExitCode)
	assertEqual(t, stderr.String(), "")
	assertContains(t, stdout.String(), "server "+filepath.Join(serverRoot, "serverDZ.cfg")+" ok")
	assertContains(t, stdout.String(), "cfgeconomycore "+filepath.Join(missionRoot, "cfgeconomycore.xml")+" ok")
	assertContains(t, stdout.String(), "xml "+filepath.Join(missionRoot, "cfgeconomycore.xml")+" ok")
}

func TestRunValidateRepoAliasWritesJSONEnvelope(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	repo := t.TempDir()
	brokenXML := filepath.Join(repo, "server-one", "mpmissions", "partial", "broken.xml")
	if err := os.MkdirAll(filepath.Dir(brokenXML), 0o755); err != nil {
		t.Fatalf("create XML fixture directory: %v", err)
	}
	writeTestFile(t, brokenXML, `<broken>`)

	code := Run([]string{"--output", "json", "validate", "repo", repo}, &stdout, &stderr)

	assertEqual(t, code, FailureExitCode)
	assertEqual(t, stderr.String(), "")
	envelope := decodeJSONEnvelope(t, stdout.String())
	assertEqual(t, envelope["status"].(string), "failed")
	assertEqual(t, envelope["target_path"].(string), repo)
	files := jsonArray(t, jsonObject(t, envelope["data"])["files"])
	assertEqual(t, len(files), 1)
	file := jsonObject(t, files[0])
	assertEqual(t, file["kind"].(string), "xml")
	assertEqual(t, file["status"].(string), "failed")
	assertContains(t, file["target_path"].(string), "broken.xml")
}

func TestRunXMLValidateReturnsFailureForInvalidXML(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	dir := mixedXMLDir(t)

	code := Run([]string{"validate", "xml", dir}, &stdout, &stderr)

	assertEqual(t, code, FailureExitCode)
	assertEqual(t, stderr.String(), "")
	assertContains(t, stdout.String(), "bad.xml failed")
	assertContains(t, stdout.String(), "good.xml ok")
}

func TestNewRootCommandMetadata(t *testing.T) {
	command := NewRootCommand(&bytes.Buffer{}, &bytes.Buffer{})

	assertEqual(t, command.Use, "dzcli")
	if len(command.Commands()) != 7 {
		t.Fatalf("root command count = %d, want 7", len(command.Commands()))
	}
	uses := commandUses(command.Commands())
	assertContains(t, uses, "create")
	assertContains(t, uses, "delete")
	assertContains(t, uses, "get")
	assertContains(t, uses, "fix")
	assertContains(t, uses, "update")
	assertContains(t, uses, "validate")
	assertContains(t, uses, "version")
}

func TestOldNounFirstCommandsFail(t *testing.T) {
	for _, args := range [][]string{
		{"economy", "types", "update"},
		{"expansion", "ai", "validate"},
		{"xml", "validate"},
	} {
		var stdout bytes.Buffer
		var stderr bytes.Buffer

		code := Run(args, &stdout, &stderr)

		assertEqual(t, code, FailureExitCode)
		assertEqual(t, stdout.String(), "")
		assertContains(t, stderr.String(), "unknown command")
	}
}

func fixturePath(t *testing.T, parts ...string) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}

	args := append([]string{filepath.Dir(file), "..", "testdata"}, parts...)
	return filepath.Clean(filepath.Join(args...))
}

func mixedXMLDir(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "bad.xml"), `<?xml version="1.0" encoding="UTF-8"?><bad>`)
	writeTestFile(t, filepath.Join(dir, "good.xml"), `<?xml version="1.0" encoding="UTF-8"?><good />`)
	return dir
}

func partialEconomyDir(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "db"), 0o700); err != nil {
		t.Fatalf("mkdir partial economy db: %v", err)
	}
	writeTestFile(t, filepath.Join(dir, "db", "types.xml"), `<?xml version="1.0" encoding="UTF-8"?><types />`)
	return dir
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

func writeTestFile(t *testing.T, path string, content string) {
	t.Helper()

	if err := osWriteFile(path, []byte(content)); err != nil {
		t.Fatalf("write test file %s: %v", path, err)
	}
}

func writeTempFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "file.cfg")
	writeTestFile(t, path, content)
	return path
}

var osWriteFile = func(path string, data []byte) error {
	return os.WriteFile(path, data, 0o600)
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

func findWarningGroup(t *testing.T, warnings []any, key string) map[string]any {
	t.Helper()
	for _, item := range warnings {
		warning := jsonObject(t, item)
		groupValue, exists := warning["group"]
		if !exists {
			continue
		}
		group := jsonObject(t, groupValue)
		if group["key"] == key {
			return warning
		}
	}
	t.Fatalf("group %q not found in warnings %#v", key, warnings)
	return nil
}

func assertHasWarningMessage(t *testing.T, warnings []any, message string) {
	t.Helper()
	for _, item := range warnings {
		warning := jsonObject(t, item)
		if warning["message"] == message {
			return
		}
	}
	t.Fatalf("warning message %q not found in %#v", message, warnings)
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

func commandUses(commands []*cobra.Command) string {
	uses := make([]string, 0, len(commands))
	for _, command := range commands {
		uses = append(uses, command.Use)
	}
	return strings.Join(uses, ",")
}
