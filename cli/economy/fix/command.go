package fix

import (
	"fmt"
	"io"
	"text/tabwriter"

	"dzcli/cli/output"
	"dzcli/cli/validation"
	"dzcli/internal/economyfix"

	"github.com/spf13/cobra"
)

func NewCommand(stdout io.Writer) *cobra.Command {
	var dryRun bool
	var apply bool
	var allowDestructive bool
	command := &cobra.Command{
		Use:   "economy <mission-root|economy-file>",
		Short: "Plan or apply supported economy remediation",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if allowDestructive && !apply {
				return fmt.Errorf("--allow-destructive requires --apply")
			}
			plan, err := economyfix.Build(args[0])
			if err != nil {
				if output.IsJSON(cmd) {
					if writeErr := output.WriteFailure(stdout, fmt.Errorf("economy fix plan: %w", err), "economy-fix", args[0], nil); writeErr != nil {
						return writeErr
					}
					return output.ErrRendered
				}
				return fmt.Errorf("economy fix plan: %w", err)
			}
			if output.IsJSON(cmd) {
				return runFixJSON(stdout, args[0], plan, apply, allowDestructive)
			}
			printPlan(stdout, plan)
			if !apply {
				return nil
			}
			result, err := economyfix.Apply(plan, allowDestructive)
			if err != nil {
				return fmt.Errorf("economy fix apply: %w", err)
			}
			fmt.Fprintf(stdout, "applied %d action(s); skipped %d action(s); wrote %d file(s)\n", len(result.Applied), len(result.Skipped), len(result.Written))
			if len(result.Remaining.Items) > 0 {
				fmt.Fprintf(stdout, "remaining %d action(s) after validation\n", len(result.Remaining.Items))
				printPlan(stdout, result.Remaining)
				return validation.ErrFailed
			}
			fmt.Fprintln(stdout, "economy fix complete")
			return nil
		},
	}
	command.SetOut(stdout)
	command.Flags().BoolVar(&dryRun, "dry-run", false, "print the remediation plan without writing")
	command.Flags().BoolVar(&apply, "apply", false, "apply unambiguous supported actions")
	command.Flags().BoolVar(&allowDestructive, "allow-destructive", false, "allow deterministic planned deletions")
	command.MarkFlagsMutuallyExclusive("dry-run", "apply")
	return command
}

func printPlan(stdout io.Writer, plan economyfix.Plan) {
	if len(plan.Items) == 0 {
		fmt.Fprintln(stdout, "economy fix plan: no findings")
		return
	}
	writer := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(writer, "ORDER\tCLASS\tDESTRUCTIVE\tAPPLY\tACTION")
	for index, item := range plan.Items {
		fmt.Fprintf(writer, "%d\t%s\t%t\t%s\t%s\n", index+1, item.Action.Class, item.Action.Destructive, planApplyState(item), planActionText(item))
		fmt.Fprintf(writer, "\t\t\t\tfinding: %s\n", item.Finding)
	}
	_ = writer.Flush()
}

type fixData struct {
	DryRun           bool          `json:"dry_run"`
	Apply            bool          `json:"apply"`
	AllowDestructive bool          `json:"allow_destructive"`
	Plan             []fixPlanItem `json:"plan"`
	Applied          []fixPlanItem `json:"applied"`
	Skipped          []fixPlanItem `json:"skipped"`
	Remaining        []fixPlanItem `json:"remaining"`
	Written          []string      `json:"written"`
}

type fixPlanItem struct {
	Order       int                `json:"order"`
	Finding     string             `json:"finding"`
	TargetPath  string             `json:"target_path"`
	ApplyState  string             `json:"apply_state"`
	ActionText  string             `json:"action_text"`
	Remediation output.Remediation `json:"remediation"`
}

func runFixJSON(stdout io.Writer, targetPath string, plan economyfix.Plan, apply bool, allowDestructive bool) error {
	data := fixData{
		DryRun:           !apply,
		Apply:            apply,
		AllowDestructive: allowDestructive,
		Plan:             fixPlanItems(plan.Items),
		Applied:          []fixPlanItem{},
		Skipped:          []fixPlanItem{},
		Remaining:        []fixPlanItem{},
		Written:          []string{},
	}
	envelope := output.Envelope{
		Status:      output.StatusOK,
		TargetPath:  targetPath,
		Data:        data,
		Remediation: fixRemediation(plan.Items),
	}
	if !apply {
		return output.Write(stdout, envelope)
	}

	result, err := economyfix.Apply(plan, allowDestructive)
	if err != nil {
		if writeErr := output.WriteFailure(stdout, fmt.Errorf("economy fix apply: %w", err), "economy-fix", targetPath, fixRemediation(plan.Items)); writeErr != nil {
			return writeErr
		}
		return output.ErrRendered
	}
	data.Applied = fixPlanItems(result.Applied)
	data.Skipped = fixPlanItems(result.Skipped)
	data.Written = append([]string{}, result.Written...)
	data.Remaining = fixPlanItems(result.Remaining.Items)
	envelope.Data = data
	envelope.Remediation = fixRemediation(result.Remaining.Items)
	if len(result.Remaining.Items) > 0 {
		envelope.Status = output.StatusFailed
		envelope.Failures = []output.Diagnostic{{
			Message:     fmt.Sprintf("remaining %d action(s) after validation", len(result.Remaining.Items)),
			Kind:        "economy-fix",
			TargetPath:  targetPath,
			Remediation: envelope.Remediation,
		}}
	}
	if err := output.Write(stdout, envelope); err != nil {
		return err
	}
	if len(result.Remaining.Items) > 0 {
		return validation.ErrFailed
	}
	return nil
}

func fixPlanItems(items []economyfix.PlanItem) []fixPlanItem {
	result := make([]fixPlanItem, 0, len(items))
	for index, item := range items {
		result = append(result, fixPlanItem{
			Order:       index + 1,
			Finding:     item.Finding,
			TargetPath:  item.Path,
			ApplyState:  planApplyState(item),
			ActionText:  planActionText(item),
			Remediation: output.FromEconomyAction(item.Action),
		})
	}
	return result
}

func fixRemediation(items []economyfix.PlanItem) []output.Remediation {
	remediation := make([]output.Remediation, 0, len(items))
	for _, item := range items {
		remediation = append(remediation, output.FromEconomyAction(item.Action))
	}
	return remediation
}

func planApplyState(item economyfix.PlanItem) string {
	if item.Action.AutoApply && item.Action.AlternativeGroup == "" {
		if item.Action.Destructive {
			return "requires --allow-destructive"
		}
		return "ready"
	}
	return "skipped"
}

func planActionText(item economyfix.PlanItem) string {
	if item.Action.Command != "" {
		return item.Action.Command
	}
	return item.Action.Detail
}
