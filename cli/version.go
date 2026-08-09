package cli

import (
	"fmt"
	"io"

	"dzcli/cli/output"

	"github.com/spf13/cobra"
)

var version = "unknown"

func newVersionCommand(stdout io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print dzcli version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if output.IsJSON(cmd) {
				return output.Write(stdout, output.Envelope{
					Status: output.StatusOK,
					Data: map[string]any{
						"version": version,
					},
				})
			}
			_, err := fmt.Fprintln(stdout, version)
			return err
		},
	}
}
