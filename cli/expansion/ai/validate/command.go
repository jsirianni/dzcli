package validate

import (
	"fmt"
	"io"

	"dzcli/cli/output"
	"dzcli/cli/validation"
	"dzcli/internal/expansion"

	"github.com/spf13/cobra"
)

func NewCommand(stdout io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "validate [path]",
		Short: "Validate DayZ Expansion AI patrol and loadout files",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "."
			if len(args) == 1 {
				path = args[0]
			}
			if output.IsJSON(cmd) {
				return ValidateAIPathJSON(path, stdout)
			}
			return ValidateAIPath(path, stdout)
		},
	}
}

func ValidateAIPath(path string, stdout io.Writer) error {
	statuses, err := expansion.InspectAIPath(path)
	if err != nil {
		return fmt.Errorf("expansion ai: failed: %w", err)
	}

	allOK := true
	for _, status := range statuses {
		if status.Err != nil {
			allOK = false
			fmt.Fprintf(stdout, "%s %s failed: %v\n", status.Kind, status.Path, status.Err)
			continue
		}
		if status.Summary == "" {
			fmt.Fprintf(stdout, "%s %s ok\n", status.Kind, status.Path)
			continue
		}
		fmt.Fprintf(stdout, "%s %s ok (%s)\n", status.Kind, status.Path, status.Summary)
	}

	if !allOK {
		return validation.ErrFailed
	}
	return nil
}

func ValidateAIPathJSON(path string, stdout io.Writer) error {
	statuses, err := expansion.InspectAIPath(path)
	if err != nil {
		if writeErr := output.WriteFailure(stdout, fmt.Errorf("expansion ai: failed: %w", err), "expansion-ai", path, nil); writeErr != nil {
			return writeErr
		}
		return output.ErrRendered
	}

	files := make([]output.ValidationFile, 0, len(statuses))
	allOK := true
	for _, status := range statuses {
		if status.Err != nil {
			allOK = false
		}
		files = append(files, output.SimpleValidationFile(status.Kind, status.Path, status.Summary, status.Err))
	}

	if err := output.WriteValidation(stdout, path, files); err != nil {
		return err
	}
	if !allOK {
		return validation.ErrFailed
	}
	return nil
}
