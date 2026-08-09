package validate

import (
	"io"

	"dzcli/cli/output"
	"dzcli/cli/validation"
	"dzcli/internal/dayzinit"

	"github.com/spf13/cobra"
)

func NewCommand(stdout io.Writer) *cobra.Command {
	command := &cobra.Command{
		Use:   "init <init.c>",
		Short: "Validate DayZ mission init.c",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if output.IsJSON(cmd) {
				return validateInitJSON(args[0], stdout)
			}
			return ValidateInit(args[0], stdout)
		},
	}
	command.SetOut(stdout)
	return command
}

func ValidateInit(path string, stdout io.Writer) error {
	err := dayzinit.ValidateFile(path)
	return validation.RenderTextStatuses(stdout, []validation.TextStatus{{
		Kind: "init",
		Path: path,
		Err:  err,
	}}, validation.DefaultTextOptions())
}

func validateInitJSON(path string, stdout io.Writer) error {
	err := dayzinit.ValidateFile(path)
	if writeErr := output.WriteValidation(stdout, path, []output.ValidationFile{
		output.SimpleValidationFile("init", path, "", err),
	}); writeErr != nil {
		return writeErr
	}
	if err != nil {
		return validation.ErrFailed
	}
	return nil
}
