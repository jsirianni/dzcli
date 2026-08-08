package economyfix

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dzcli/internal/economyconfig"
)

func TestPlanSortHonorsDependencies(t *testing.T) {
	plan := Plan{Items: []PlanItem{
		{Path: "a", Action: economyconfig.RemediationAction{ID: "dependent", DependsOn: []string{"prerequisite"}}},
		{Path: "z", Action: economyconfig.RemediationAction{ID: "prerequisite"}},
	}}
	plan.sort()
	if plan.Items[0].Action.ID != "prerequisite" || plan.Items[1].Action.ID != "dependent" {
		t.Fatalf("dependency order = %#v", plan.Items)
	}
}

func TestPlanClassifiesAndAppliesOnlyEligibleActions(t *testing.T) {
	root := t.TempDir()
	fixture := filepath.Join("..", "..", "testdata", "economyremediation")
	if err := os.CopyFS(root, os.DirFS(fixture)); err != nil {
		t.Fatal(err)
	}
	plan, err := Build(root)
	if err != nil {
		t.Fatal(err)
	}
	var sawMechanical, sawDeletion, sawSemantic, sawPlaceholder bool
	for _, item := range plan.Items {
		switch item.Action.Class {
		case "mechanical":
			sawMechanical = true
		case "deletion":
			sawDeletion = true
		case "semantic":
			sawSemantic = true
		case "placeholder":
			sawPlaceholder = true
		}
		if strings.Contains(item.Action.Command, "'x,z'") {
			t.Fatalf("non-executable coordinates remained: %s", item.Action.Command)
		}
	}
	if !sawMechanical || !sawDeletion || !sawSemantic || !sawPlaceholder {
		t.Fatalf("classes mechanical=%t deletion=%t semantic=%t placeholder=%t", sawMechanical, sawDeletion, sawSemantic, sawPlaceholder)
	}
	result, err := Apply(plan, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Applied) == 0 {
		t.Fatal("no safe action applied")
	}
	environment, _ := os.ReadFile(filepath.Join(root, "cfgenvironment.xml"))
	if !strings.Contains(string(environment), "path=\"env/hare.xml\"") {
		t.Fatalf("safe environment registration not applied:\n%s", environment)
	}
	duplicate, _ := os.ReadFile(filepath.Join(root, "mods", "duplicate_types.xml"))
	if !strings.Contains(string(duplicate), "name=\"WoodenLog\"") {
		t.Fatal("destructive action applied without authorization")
	}
	plan, err = Build(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(plan, true); err != nil {
		t.Fatal(err)
	}
	duplicate, _ = os.ReadFile(filepath.Join(root, "mods", "duplicate_types.xml"))
	if strings.Contains(string(duplicate), "name=\"WoodenLog\"") {
		t.Fatal("authorized deterministic deletion was not applied")
	}
}

func TestPlanOffersDeleteForCoveredInvalidEmptyEvent(t *testing.T) {
	root := t.TempDir()
	write := func(relative, data string) {
		t.Helper()
		path := filepath.Join(root, relative)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("cfgeconomycore.xml", "<economycore />")
	write("cfglimitsdefinition.xml", "<lists><categories /><tags /><usageflags /><valueflags /></lists>")
	write("db/types.xml", "<types />")
	write("db/events.xml", "<events><event name=\"AmbientHare\"><position>fixed</position><active>1</active></event></events>")
	write("cfgeventspawns.xml", "<eventposdef><event name=\"AmbientHare\" /></eventposdef>")
	write("cfgenvironment.xml", "<env><territories><file path=\"env/hare.xml\" /><territory type=\"Ambient\" name=\"AmbientHare\"><file usable=\"hare\" /></territory></territories></env>")
	write("env/hare.xml", "<territory-type><territory><zone name=\"Hare\" smin=\"0\" smax=\"0\" dmin=\"0\" dmax=\"2\" x=\"1\" z=\"2\" r=\"50\" /></territory></territory-type>")
	plan, err := Build(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range plan.Items {
		if strings.Contains(item.Action.Command, "delete economy event-spawns") && item.Action.AutoApply && item.Action.Destructive {
			return
		}
	}
	t.Fatalf("covered invalid event deletion missing: %#v", plan.Items)
}
