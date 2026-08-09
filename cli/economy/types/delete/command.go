package delete

import (
	"fmt"
	"io"

	"dzcli/cli/output"
	"dzcli/internal/economy"

	"github.com/spf13/cobra"
)

var writeFileMutation = economy.WriteFileMutation

func NewCommand(stdout io.Writer) *cobra.Command {
	var file string
	var occurrence int
	var dryRun bool
	command := &cobra.Command{
		Use:   "types <type-name>",
		Short: "Delete one type entry from a types XML file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if file == "" {
				return fmt.Errorf("--file is required")
			}
			options := economy.TypeDeleteOptions{TypeName: args[0], Occurrence: occurrence, OccurrenceSet: cmd.Flags().Changed("occurrence")}
			mutation, err := economy.DeleteTypeFile(file, options)
			if err != nil {
				return err
			}
			if output.IsJSON(cmd) {
				return output.WriteMutation(stdout, file, "types", mutation.Changed, dryRun, "application/xml", mutation.Data, nil)
			}
			if dryRun {
				_, err = stdout.Write(mutation.Data)
				return err
			}
			if err := writeFileMutation(file, mutation); err != nil {
				return err
			}
			fmt.Fprintf(stdout, "types %s ok\n", file)
			return nil
		},
	}
	command.SetOut(stdout)
	command.Flags().StringVar(&file, "file", "", "types.xml path")
	command.Flags().IntVar(&occurrence, "occurrence", 0, "select a duplicate type occurrence")
	command.Flags().BoolVar(&dryRun, "dry-run", false, "print modified XML without writing")
	return command
}
