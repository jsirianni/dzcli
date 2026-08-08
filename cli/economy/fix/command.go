package fix

import (
	"fmt"
	"io"
	"text/tabwriter"

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
		RunE: func(_ *cobra.Command, args []string) error {
			if allowDestructive && !apply {
				return fmt.Errorf("--allow-destructive requires --apply")
			}
			plan, err := economyfix.Build(args[0])
			if err != nil {
				return fmt.Errorf("economy fix plan: %w", err)
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
		applyState := "skipped"
		if item.Action.AutoApply && item.Action.AlternativeGroup == "" {
			applyState = "ready"
			if item.Action.Destructive {
				applyState = "requires --allow-destructive"
			}
		}
		action := item.Action.Command
		if action == "" {
			action = item.Action.Detail
		}
		fmt.Fprintf(writer, "%d\t%s\t%t\t%s\t%s\n", index+1, item.Action.Class, item.Action.Destructive, applyState, action)
		fmt.Fprintf(writer, "\t\t\t\tfinding: %s\n", item.Finding)
	}
	_ = writer.Flush()
}
