package economyfix

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"dzcli/internal/economy"
	"dzcli/internal/economyconfig"
)

type PlanItem struct {
	Finding string
	Path    string
	Action  economyconfig.RemediationAction
}

type Plan struct {
	MissionRoot string
	Items       []PlanItem
	Statuses    []economyconfig.FileStatus
}

type ApplyResult struct {
	Applied   []PlanItem
	Skipped   []PlanItem
	Remaining Plan
	Written   []string
}

func Build(path string) (Plan, error) {
	root, err := economyconfig.ResolveMissionRoot(path)
	if err != nil {
		return Plan{}, err
	}
	statuses, err := economyconfig.InspectEconomy(path)
	if err != nil {
		return Plan{}, err
	}
	plan := Plan{MissionRoot: root, Statuses: statuses}
	for _, status := range statuses {
		if status.Err != nil {
			plan.Items = append(plan.Items, PlanItem{Finding: fmt.Sprintf("%s validation failed: %v", status.Kind, status.Err), Path: status.Path, Action: economyconfig.RemediationAction{ID: "validation-failed-" + status.Kind, Detail: "repair the structural or semantic validation errors listed above", Class: economyconfig.RemediationSemantic}})
		}
		for _, warning := range status.WarningDetails {
			if len(warning.Actions) == 0 && warning.ManualOnly {
				plan.Items = append(plan.Items, PlanItem{Finding: warning.Message, Path: status.Path, Action: economyconfig.RemediationAction{
					ID: "manual-" + status.Kind, Detail: "validation-only; edit the XML manually", Class: economyconfig.RemediationSemantic,
				}})
			}
			for _, action := range warning.Actions {
				plan.Items = append(plan.Items, PlanItem{Finding: warning.Message, Path: status.Path, Action: action})
			}
		}
	}
	plan.addInvalidEventSpawnActions()
	plan.sort()
	return plan, nil
}

func (plan *Plan) addInvalidEventSpawnActions() {
	spawnsPath := filepath.Join(plan.MissionRoot, "cfgeventspawns.xml")
	entries, err := economy.ListEventSpawnsFile(spawnsPath)
	if err != nil {
		return
	}
	events, _ := economy.ListEconomyEventsFile(filepath.Join(plan.MissionRoot, "db", "events.xml"))
	active := map[string]bool{}
	for _, event := range events {
		if event.ActivePresent {
			active[event.Name] = event.Active != 0
		} else {
			active[event.Name] = true
		}
	}
	totals := map[string]int{}
	for _, entry := range entries {
		totals[entry.Name]++
	}
	for _, entry := range entries {
		if len(entry.Issues) == 0 {
			continue
		}
		finding := fmt.Sprintf("event spawn %q is invalid: %s", entry.Name, strings.Join(entry.Issues, "; "))
		if entry.Name == "" {
			plan.Items = append(plan.Items, PlanItem{Finding: finding, Path: spawnsPath, Action: economyconfig.RemediationAction{ID: "event-spawn-missing-name", Detail: "input required: add a name or remove the unnamed entry manually", Class: economyconfig.RemediationSemantic}})
			continue
		}
		if len(entry.Positions) == 0 && len(entry.Zones) == 0 {
			occurrenceFlag := ""
			if totals[entry.Name] > 1 {
				occurrenceFlag = fmt.Sprintf(" --occurrence %d", entry.Occurrence)
			}
			covered := economyconfig.EnvironmentBacksEvent(plan.MissionRoot, entry.Name)
			if !active[entry.Name] || covered {
				reason := "event is inactive"
				if covered {
					reason = "event is covered by a valid environment territory"
				}
				plan.Items = append(plan.Items, PlanItem{Finding: finding + "; " + reason, Path: spawnsPath, Action: economyconfig.RemediationAction{
					ID:      fmt.Sprintf("delete-invalid-event-spawn-%s-%d", entry.Name, entry.Occurrence),
					Command: fmt.Sprintf("dzcli delete economy event-spawns %s --file %s%s", quotePowerShell(entry.Name), quotePowerShell(spawnsPath), occurrenceFlag),
					Class:   economyconfig.RemediationDeletion, Destructive: true, AutoApply: true,
					Operation: economyconfig.RemediationDeleteEventSpawn, File: spawnsPath, Name: entry.Name, Occurrence: entry.Occurrence, OccurrenceSet: totals[entry.Name] > 1,
				}})
				continue
			}
			group := fmt.Sprintf("invalid-event-spawn-%s-%d", entry.Name, entry.Occurrence)
			plan.Items = append(plan.Items,
				PlanItem{Finding: finding, Path: spawnsPath, Action: economyconfig.RemediationAction{ID: group + "-copy", Detail: "input required: choose a valid source event and run update with --copy-zone-from", Class: economyconfig.RemediationSemantic, AlternativeGroup: group}},
				PlanItem{Finding: finding, Path: spawnsPath, Action: economyconfig.RemediationAction{ID: group + "-placeholder", Command: fmt.Sprintf("dzcli update economy event-spawns %s --file %s%s --set-zone '0,0,0,0,0' --scaffold-placeholder", quotePowerShell(entry.Name), quotePowerShell(spawnsPath), occurrenceFlag), Class: economyconfig.RemediationPlaceholder, AlternativeGroup: group}},
			)
			continue
		}
		plan.Items = append(plan.Items, PlanItem{Finding: finding, Path: spawnsPath, Action: economyconfig.RemediationAction{ID: fmt.Sprintf("repair-event-spawn-%s-%d", entry.Name, entry.Occurrence), Detail: "input required: repair the listed position or zone attributes", Class: economyconfig.RemediationSemantic}})
	}
}

func (plan *Plan) sort() {
	sort.SliceStable(plan.Items, func(left, right int) bool {
		a := plan.Items[left]
		b := plan.Items[right]
		if a.Action.Destructive != b.Action.Destructive {
			return !a.Action.Destructive
		}
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		if a.Action.Name == b.Action.Name && a.Action.Occurrence != b.Action.Occurrence && a.Action.Destructive {
			return a.Action.Occurrence > b.Action.Occurrence
		}
		return a.Action.ID < b.Action.ID
	})
	ordered := make([]PlanItem, 0, len(plan.Items))
	done := map[string]bool{}
	remaining := append([]PlanItem(nil), plan.Items...)
	for len(remaining) > 0 {
		progress := false
		for index := 0; index < len(remaining); {
			ready := true
			for _, dependency := range remaining[index].Action.DependsOn {
				if !done[dependency] {
					ready = false
					break
				}
			}
			if !ready {
				index++
				continue
			}
			item := remaining[index]
			ordered = append(ordered, item)
			done[item.Action.ID] = true
			remaining = append(remaining[:index], remaining[index+1:]...)
			progress = true
		}
		if !progress {
			ordered = append(ordered, remaining...)
			break
		}
	}
	plan.Items = ordered
}

func Apply(plan Plan, allowDestructive bool) (ApplyResult, error) {
	result := ApplyResult{}
	type stagedFile struct {
		data []byte
		mode fs.FileMode
	}
	staged := map[string]stagedFile{}
	for _, item := range plan.Items {
		if !item.Action.AutoApply || item.Action.Destructive && !allowDestructive || item.Action.AlternativeGroup != "" {
			result.Skipped = append(result.Skipped, item)
			continue
		}
		file := item.Action.File
		state, ok := staged[file]
		if !ok {
			info, err := os.Stat(file)
			if err != nil {
				return result, fmt.Errorf("preflight %s: %w", file, err)
			}
			data, err := os.ReadFile(file) // #nosec G304 -- the plan contains inspected mission files only.
			if err != nil {
				return result, fmt.Errorf("preflight %s: %w", file, err)
			}
			state = stagedFile{data: data, mode: info.Mode().Perm()}
		}
		updated, changed, err := applyAction(state.data, item.Action)
		if err != nil {
			return result, fmt.Errorf("preflight %s: %w", item.Action.ID, err)
		}
		if changed {
			state.data = economy.NormalizeXMLMutationData(updated)
			staged[file] = state
		}
		result.Applied = append(result.Applied, item)
	}
	paths := make([]string, 0, len(staged))
	for path := range staged {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		state := staged[path]
		if err := economy.WriteFileMutation(path, economy.FileMutation{Data: state.data, Mode: state.mode, Changed: true}); err != nil {
			return result, fmt.Errorf("write %s after writing %s: %w", path, strings.Join(result.Written, ", "), err)
		}
		result.Written = append(result.Written, path)
	}
	remaining, err := Build(plan.MissionRoot)
	if err != nil {
		return result, err
	}
	result.Remaining = remaining
	return result, nil
}

func applyAction(data []byte, action economyconfig.RemediationAction) ([]byte, bool, error) {
	switch action.Operation {
	case economyconfig.RemediationDeleteType:
		return economy.DeleteTypeXML(data, economy.TypeDeleteOptions{TypeName: action.Name, Occurrence: action.Occurrence, OccurrenceSet: true})
	case economyconfig.RemediationCreateEnvironment:
		return economy.UpdateEnvironmentXML(data, economy.EnvironmentCreate, economy.EnvironmentReferenceOptions{Kind: "path", Value: action.Name})
	case economyconfig.RemediationDeleteEventSpawn:
		return economy.DeleteEventSpawnXML(data, action.Name, action.Occurrence, action.OccurrenceSet)
	default:
		return nil, false, fmt.Errorf("unsupported remediation operation %q", action.Operation)
	}
}

func quotePowerShell(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}
