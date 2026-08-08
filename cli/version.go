package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

var version = "unknown"

func newVersionCommand(stdout io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print dzcli version",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			_, err := fmt.Fprintln(stdout, version)
			return err
		},
	}
}
