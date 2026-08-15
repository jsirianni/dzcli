// Package validate implements Windows batch-file validation commands.
package validate

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"dzcli/cli/output"
	"dzcli/cli/validation"
	"dzcli/internal/batchvalidate"

	"github.com/spf13/cobra"
)

const kindBatch = "batch"

// FileStatus is one batch file's validation outcome.
type FileStatus struct {
	Path   string
	Result batchvalidate.Result
	Err    error
}

// NewCommand returns the batch-file validation command.
func NewCommand(stdout io.Writer) *cobra.Command {
	command := &cobra.Command{
		Use:   "batch <file-or-dir>",
		Short: "Validate Windows batch files",
		Long:  "Validate one .bat or .cmd file, or discover Windows batch files recursively under a directory, without executing them.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if output.IsJSON(cmd) {
				return ValidateBatchPathJSON(args[0], stdout)
			}
			return ValidateBatchPathWithOptions(args[0], stdout, validation.TextOptionsFromCommand(cmd))
		},
	}
	command.SetOut(stdout)
	return command
}

// ValidateBatchPath validates a file or directory using default text options.
func ValidateBatchPath(path string, stdout io.Writer) error {
	return ValidateBatchPathWithOptions(path, stdout, validation.DefaultTextOptions())
}

// ValidateBatchPathWithOptions validates a file or directory and renders text.
func ValidateBatchPathWithOptions(path string, stdout io.Writer, options validation.TextOptions) error {
	statuses, err := InspectBatchPath(path)
	if err != nil {
		return fmt.Errorf("batch: failed: %w", err)
	}
	if len(statuses) == 0 {
		return fmt.Errorf("batch: failed: no .bat or .cmd files found under %s", filepath.Clean(path))
	}
	return validation.RenderTextStatuses(stdout, TextStatuses(statuses), options)
}

// ValidateBatchPathJSON validates a file or directory and renders JSON.
func ValidateBatchPathJSON(path string, stdout io.Writer) error {
	statuses, err := InspectBatchPath(path)
	if err == nil && len(statuses) == 0 {
		err = fmt.Errorf("no .bat or .cmd files found under %s", filepath.Clean(path))
	}
	if err != nil {
		if writeErr := output.WriteFailure(stdout, fmt.Errorf("batch: failed: %w", err), kindBatch, path, nil); writeErr != nil {
			return writeErr
		}
		return output.ErrRendered
	}
	files := ValidationFiles(statuses)
	if err := output.WriteValidation(stdout, path, files); err != nil {
		return err
	}
	if HasFailures(statuses) {
		return validation.ErrFailed
	}
	return nil
}

// InspectBatchPath discovers and validates batch files without rendering them.
func InspectBatchPath(path string) ([]FileStatus, error) {
	files, err := FindBatchFiles(path)
	if err != nil {
		return nil, err
	}
	statuses := make([]FileStatus, 0, len(files))
	options := batchvalidate.Options{
		InitialCommandExtensions: batchvalidate.FeatureUnknown,
		InitialDelayedExpansion:  batchvalidate.FeatureUnknown,
		ReportUnsupported:        true,
	}
	for _, file := range files {
		result, err := batchvalidate.ValidateFile(file, options)
		statuses = append(statuses, FileStatus{Path: file, Result: result, Err: err})
	}
	return statuses, nil
}

// FindBatchFiles returns deterministic batch-file paths for a file or tree.
func FindBatchFiles(path string) ([]string, error) {
	cleanPath := filepath.Clean(path)
	info, err := os.Stat(cleanPath)
	if err != nil {
		return nil, fmt.Errorf("inspect path %s: %w", cleanPath, err)
	}
	if !info.IsDir() {
		if !isBatchPath(cleanPath) {
			return nil, fmt.Errorf("%s is not a .bat or .cmd file", cleanPath)
		}
		return []string{cleanPath}, nil
	}

	var files []string
	err = filepath.WalkDir(cleanPath, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if strings.EqualFold(entry.Name(), ".git") {
				return filepath.SkipDir
			}
			return nil
		}
		if isBatchPath(entry.Name()) {
			files = append(files, current)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk %s: %w", cleanPath, err)
	}
	sort.Strings(files)
	return files, nil
}

// TextStatuses converts batch validation outcomes to shared text statuses.
func TextStatuses(statuses []FileStatus) []validation.TextStatus {
	result := make([]validation.TextStatus, 0, len(statuses))
	for _, status := range statuses {
		textStatus := validation.TextStatus{
			Kind: kindBatch,
			Path: status.Path,
		}
		if status.Err != nil {
			textStatus.Err = status.Err
			result = append(result, textStatus)
			continue
		}
		textStatus.Summary = textSummary(status.Result)
		textStatus.Err = statusError(status)
		infoCount := 0
		for _, diagnostic := range status.Result.Diagnostics {
			switch diagnostic.Severity {
			case batchvalidate.SeverityInfo:
				infoCount++
			case batchvalidate.SeverityWarning:
				textStatus.Warnings = append(textStatus.Warnings, validation.TextWarning{Message: formatDiagnostic(diagnostic)})
			}
		}
		if !status.Result.FullyValidated {
			textStatus.Notices = []string{fmt.Sprintf("analysis incomplete: %d opaque or runtime-dependent region(s); use --output json for details", infoCount)}
		}
		result = append(result, textStatus)
	}
	return result
}

// ValidationFiles converts batch validation outcomes to structured output.
func ValidationFiles(statuses []FileStatus) []output.ValidationFile {
	files := make([]output.ValidationFile, 0, len(statuses))
	for _, status := range statuses {
		file := output.ValidationFile{
			Kind:       kindBatch,
			TargetPath: status.Path,
			Status:     output.StatusOK,
		}
		if status.Err != nil {
			file.Status = output.StatusFailed
			file.Failures = []output.Diagnostic{{Message: status.Err.Error(), Kind: kindBatch, TargetPath: status.Path}}
		} else {
			file.Summary = summary(status.Result)
			for _, diagnostic := range status.Result.Diagnostics {
				item := outputDiagnostic(status.Path, diagnostic)
				switch diagnostic.Severity {
				case batchvalidate.SeverityError:
					file.Status = output.StatusFailed
					file.Failures = append(file.Failures, item)
				case batchvalidate.SeverityWarning:
					file.Warnings = append(file.Warnings, item)
				case batchvalidate.SeverityInfo:
					file.Notices = append(file.Notices, item)
				}
			}
		}
		files = append(files, file)
	}
	return files
}

// HasFailures reports whether any file could not be read or was invalid.
func HasFailures(statuses []FileStatus) bool {
	for _, status := range statuses {
		if status.Err != nil || status.Result.HasErrors() {
			return true
		}
	}
	return false
}

type diagnosticsError struct {
	path        string
	diagnostics []batchvalidate.Diagnostic
}

func (err diagnosticsError) Error() string {
	lines := make([]string, 0, len(err.diagnostics))
	for _, diagnostic := range err.diagnostics {
		lines = append(lines, fmt.Sprintf("%s:%d:%d [%s] %s", err.path, diagnostic.Span.Start.Line, diagnostic.Span.Start.Column, diagnostic.Code, diagnostic.Message))
	}
	return strings.Join(lines, "\n")
}

func statusError(status FileStatus) error {
	if status.Err != nil {
		return status.Err
	}
	var diagnostics []batchvalidate.Diagnostic
	for _, diagnostic := range status.Result.Diagnostics {
		if diagnostic.Severity == batchvalidate.SeverityError {
			diagnostics = append(diagnostics, diagnostic)
		}
	}
	if len(diagnostics) == 0 {
		return nil
	}
	return diagnosticsError{path: status.Path, diagnostics: diagnostics}
}

func summary(result batchvalidate.Result) string {
	if result.HasErrors() {
		if result.FullyValidated {
			return "validation failed"
		}
		return "validation failed; analysis incomplete"
	}
	if result.FullyValidated {
		return "fully validated"
	}
	return "analysis incomplete"
}

func textSummary(result batchvalidate.Result) string {
	if result.FullyValidated {
		return "fully validated"
	}
	return ""
}

func formatDiagnostic(diagnostic batchvalidate.Diagnostic) string {
	return fmt.Sprintf("%d:%d [%s] %s", diagnostic.Span.Start.Line, diagnostic.Span.Start.Column, diagnostic.Code, diagnostic.Message)
}

func outputDiagnostic(path string, diagnostic batchvalidate.Diagnostic) output.Diagnostic {
	return output.Diagnostic{
		Code:       diagnostic.Code,
		Severity:   severityName(diagnostic.Severity),
		Message:    diagnostic.Message,
		Kind:       kindBatch,
		TargetPath: path,
		Span: &output.SourceSpan{
			Start: output.SourcePosition{Offset: diagnostic.Span.Start.Offset, Line: diagnostic.Span.Start.Line, Column: diagnostic.Span.Start.Column},
			End:   output.SourcePosition{Offset: diagnostic.Span.End.Offset, Line: diagnostic.Span.End.Line, Column: diagnostic.Span.End.Column},
		},
	}
}

func severityName(severity batchvalidate.Severity) string {
	switch severity {
	case batchvalidate.SeverityError:
		return "error"
	case batchvalidate.SeverityWarning:
		return "warning"
	default:
		return "info"
	}
}

func isBatchPath(path string) bool {
	extension := filepath.Ext(path)
	return strings.EqualFold(extension, ".bat") || strings.EqualFold(extension, ".cmd")
}
