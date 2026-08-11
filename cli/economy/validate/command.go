package validate

import (
	"fmt"
	"io"

	"dzcli/cli/output"
	"dzcli/cli/validation"
	"dzcli/internal/economyconfig"

	"github.com/spf13/cobra"
)

func NewCommand(stdout io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "economy <mission-root|cfgeconomycore.xml|economy-file>",
		Short: "Validate central economy files",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if output.IsJSON(cmd) {
				return validateEconomyJSON(args[0], stdout, validation.TextOptionsFromCommand(cmd))
			}
			return ValidateEconomyWithOptions(args[0], stdout, validation.TextOptionsFromCommand(cmd))
		},
	}
}

func ValidateEconomy(path string, stdout io.Writer) error {
	return ValidateEconomyWithOptions(path, stdout, validation.DefaultTextOptions())
}

func ValidateEconomyWithOptions(path string, stdout io.Writer, options validation.TextOptions) error {
	statuses, err := economyconfig.InspectEconomy(path)
	if err != nil {
		return fmt.Errorf("economy: failed: %w", err)
	}
	return printStatuses(statuses, stdout, options)
}

func ValidateEconomyCore(path string, stdout io.Writer) error {
	return ValidateEconomyCoreWithOptions(path, stdout, validation.DefaultTextOptions())
}

func ValidateEconomyCoreWithOptions(path string, stdout io.Writer, options validation.TextOptions) error {
	statuses, err := economyconfig.InspectEconomyCore(path)
	if err != nil {
		return fmt.Errorf("cfgeconomycore: failed: %w", err)
	}
	return printStatuses(statuses, stdout, options)
}

func printStatuses(statuses []economyconfig.FileStatus, stdout io.Writer, options validation.TextOptions) error {
	return validation.RenderTextStatuses(stdout, economyTextStatuses(statuses), options)
}

func economyTextStatuses(statuses []economyconfig.FileStatus) []validation.TextStatus {
	result := make([]validation.TextStatus, 0, len(statuses))
	for _, status := range statuses {
		summary := ""
		if status.TypeCount > 0 || status.Kind == "base-types" || status.Kind == "types" {
			summary = fmt.Sprintf("%d types", status.TypeCount)
		}
		result = append(result, validation.TextStatus{
			Kind:     status.Kind,
			Path:     status.Path,
			Summary:  summary,
			Err:      status.Err,
			Warnings: economyTextWarnings(status),
		})
	}
	return result
}

func economyTextWarnings(status economyconfig.FileStatus) []validation.TextWarning {
	warnings := []validation.TextWarning{}
	if len(status.WarningDetails) > 0 {
		for _, warning := range status.WarningDetails {
			warnings = append(warnings, validation.TextWarning{
				Message:     warning.Message,
				Remediation: economyWarningRemediation(warning),
				GroupKey:    warning.GroupKey,
				GroupTitle:  warning.GroupTitle,
				ItemLabel:   warning.ItemLabel,
			})
		}
		return warnings
	}
	for _, warning := range status.Warnings {
		warnings = append(warnings, validation.TextWarning{
			Message:     warning,
			Remediation: []string{"validation-only; edit the XML manually"},
		})
	}
	return warnings
}

func economyWarningRemediation(warning economyconfig.WarningDetail) []string {
	remediation := []string{}
	if len(warning.Actions) > 0 {
		for _, action := range warning.Actions {
			if action.Command != "" {
				remediation = append(remediation, action.Command)
			}
			if action.Detail != "" {
				remediation = append(remediation, action.Detail)
			}
		}
	} else {
		remediation = append(remediation, warning.Remediation...)
	}
	if warning.ManualOnly {
		remediation = append(remediation, "validation-only; edit the XML manually")
	}
	return remediation
}

func printWarnings(stdout io.Writer, status economyconfig.FileStatus) {
	for _, warning := range economyTextWarnings(status) {
		fmt.Fprintf(stdout, "%s %s warning: %s\n", status.Kind, status.Path, warning.Message)
		for _, remediation := range warning.Remediation {
			fmt.Fprintf(stdout, "%s %s remediation: %s\n", status.Kind, status.Path, remediation)
		}
	}
}

func validateEconomyJSON(path string, stdout io.Writer, options validation.TextOptions) error {
	statuses, err := economyconfig.InspectEconomy(path)
	if err != nil {
		if writeErr := output.WriteFailure(stdout, fmt.Errorf("economy: failed: %w", err), "economy", path, nil); writeErr != nil {
			return writeErr
		}
		return output.ErrRendered
	}
	files := output.EconomyValidationFiles(statuses)
	if err := output.WriteValidationWithOptions(stdout, path, files, options); err != nil {
		return err
	}
	for _, status := range statuses {
		if status.Err != nil {
			return validation.ErrFailed
		}
	}
	return nil
}
