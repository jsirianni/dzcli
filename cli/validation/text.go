package validation

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

const (
	WarningModeCompact = "compact"
	WarningModeFull    = "full"

	compactWarningThreshold = 3
	compactWarningItemLimit = 10
)

type TextOptions struct {
	WarningMode string
}

type TextStatus struct {
	Kind     string
	Path     string
	Summary  string
	Err      error
	Warnings []TextWarning
}

type TextWarning struct {
	Message     string
	Remediation []string
	GroupKey    string
	GroupTitle  string
	ItemLabel   string
}

func AddWarningModeFlag(command *cobra.Command, value *string) {
	command.PersistentFlags().StringVar(value, "warnings", WarningModeCompact, "warning output: compact or full")
}

func WarningMode(cmd *cobra.Command) string {
	if cmd == nil {
		return WarningModeCompact
	}
	flag := cmd.Flag("warnings")
	if flag == nil {
		return WarningModeCompact
	}
	return flag.Value.String()
}

func ValidateWarningMode(cmd *cobra.Command) error {
	value := WarningMode(cmd)
	if value == WarningModeCompact || value == WarningModeFull {
		return nil
	}
	return fmt.Errorf("unsupported warning output %q (supported: compact, full)", value)
}

func TextOptionsFromCommand(cmd *cobra.Command) TextOptions {
	return TextOptions{WarningMode: WarningMode(cmd)}
}

func DefaultTextOptions() TextOptions {
	return TextOptions{WarningMode: WarningModeCompact}
}

func RenderTextStatuses(stdout io.Writer, statuses []TextStatus, options TextOptions) error {
	if options.WarningMode == "" {
		options.WarningMode = WarningModeCompact
	}
	groups := compactWarningGroups(statuses, options.WarningMode)
	printedGroups := map[string]bool{}
	allOK := true
	for _, status := range statuses {
		if status.Err != nil {
			allOK = false
			fmt.Fprintf(stdout, "%s %s failed: %v\n", status.Kind, status.Path, status.Err)
		} else if status.Summary != "" {
			fmt.Fprintf(stdout, "%s %s ok (%s)\n", status.Kind, status.Path, status.Summary)
		} else {
			fmt.Fprintf(stdout, "%s %s ok\n", status.Kind, status.Path)
		}
		printTextWarnings(stdout, status, groups, printedGroups)
	}
	if !allOK {
		return ErrFailed
	}
	return nil
}

type warningGroup struct {
	Key      string
	Title    string
	Warnings []groupedWarning
}

type groupedWarning struct {
	Status  TextStatus
	Warning TextWarning
}

func compactWarningGroups(statuses []TextStatus, mode string) map[string]warningGroup {
	groups := map[string]warningGroup{}
	if mode != WarningModeCompact {
		return groups
	}
	for _, status := range statuses {
		for _, warning := range status.Warnings {
			if warning.GroupKey == "" {
				continue
			}
			group := groups[warning.GroupKey]
			group.Key = warning.GroupKey
			if group.Title == "" {
				group.Title = warning.GroupTitle
			}
			group.Warnings = append(group.Warnings, groupedWarning{Status: status, Warning: warning})
			groups[warning.GroupKey] = group
		}
	}
	for key, group := range groups {
		if len(group.Warnings) < compactWarningThreshold {
			delete(groups, key)
		}
	}
	return groups
}

func printTextWarnings(stdout io.Writer, status TextStatus, groups map[string]warningGroup, printedGroups map[string]bool) {
	for _, warning := range status.Warnings {
		group, compacted := groups[warning.GroupKey]
		if compacted {
			if printedGroups[warning.GroupKey] {
				continue
			}
			printedGroups[warning.GroupKey] = true
			printWarningGroup(stdout, group)
			continue
		}
		printWarning(stdout, status, warning)
	}
}

func printWarning(stdout io.Writer, status TextStatus, warning TextWarning) {
	fmt.Fprintf(stdout, "%s %s warning: %s\n", status.Kind, status.Path, warning.Message)
	for _, remediation := range warning.Remediation {
		fmt.Fprintf(stdout, "%s %s remediation: %s\n", status.Kind, status.Path, remediation)
	}
}

func printWarningGroup(stdout io.Writer, group warningGroup) {
	prefix := groupPrefix(group)
	title := group.Title
	if title == "" {
		title = "similar warnings"
	}
	labels := groupLabels(group)
	if labels == "" {
		fmt.Fprintf(stdout, "%s warning: %d %s\n", prefix, len(group.Warnings), title)
	} else {
		fmt.Fprintf(stdout, "%s warning: %d %s: %s\n", prefix, len(group.Warnings), title, labels)
	}
	remediation, shared := sharedRemediation(group)
	if shared {
		for _, item := range remediation {
			fmt.Fprintf(stdout, "%s remediation: %s\n", prefix, item)
		}
		return
	}
	fmt.Fprintf(stdout, "%s remediation: rerun with --warnings full for per-item remediation\n", prefix)
}

func groupPrefix(group warningGroup) string {
	if len(group.Warnings) == 0 {
		return "validation"
	}
	first := group.Warnings[0].Status
	sameKind := true
	samePath := true
	for _, item := range group.Warnings[1:] {
		if item.Status.Kind != first.Kind {
			sameKind = false
		}
		if item.Status.Path != first.Path {
			samePath = false
		}
	}
	if sameKind && samePath {
		return first.Kind + " " + first.Path
	}
	if sameKind {
		return first.Kind
	}
	return "validation"
}

func groupLabels(group warningGroup) string {
	labels := make([]string, 0, len(group.Warnings))
	for _, item := range group.Warnings {
		if item.Warning.ItemLabel == "" {
			continue
		}
		labels = append(labels, strconv.Quote(item.Warning.ItemLabel))
	}
	if len(labels) == 0 {
		return ""
	}
	visible := labels
	if len(visible) > compactWarningItemLimit {
		visible = visible[:compactWarningItemLimit]
	}
	parts := append([]string{}, visible...)
	if remaining := len(labels) - len(visible); remaining > 0 {
		parts = append(parts, fmt.Sprintf("+%d more", remaining))
	}
	return strings.Join(parts, ", ")
}

func sharedRemediation(group warningGroup) ([]string, bool) {
	if len(group.Warnings) == 0 {
		return nil, true
	}
	first := group.Warnings[0].Warning.Remediation
	for _, item := range group.Warnings[1:] {
		if !stringSlicesEqual(first, item.Warning.Remediation) {
			return nil, false
		}
	}
	return first, true
}

func stringSlicesEqual(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
