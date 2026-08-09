package validate

import (
	"fmt"
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
	if err := dayzinit.ValidateFile(path); err != nil {
		fmt.Fprintf(stdout, "init %s failed: %v\n", path, err)
		return validation.ErrFailed
	}
	fmt.Fprintf(stdout, "init %s ok\n", path)
	return nil
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
