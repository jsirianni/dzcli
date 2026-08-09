package validate

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"dzcli/cli/output"
	"dzcli/cli/validation"

	"github.com/spf13/cobra"
)

type FileStatus struct {
	Path string
	Kind string
	Err  error
}

func NewCommand(stdout io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "validate [path]",
		Short: "Validate XML files recursively",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "."
			if len(args) == 1 {
				path = args[0]
			}
			if output.IsJSON(cmd) {
				return ValidateXMLPathJSON(path, stdout)
			}
			return ValidateXMLPathWithOptions(path, stdout, validation.TextOptionsFromCommand(cmd))
		},
	}
}

func ValidateXMLPath(path string, stdout io.Writer) error {
	return ValidateXMLPathWithOptions(path, stdout, validation.DefaultTextOptions())
}

func ValidateXMLPathWithOptions(path string, stdout io.Writer, options validation.TextOptions) error {
	statuses, err := InspectXMLPath(path)
	if err != nil {
		return fmt.Errorf("xml: failed: %w", err)
	}
	return validation.RenderTextStatuses(stdout, xmlTextStatuses(statuses), options)
}

func InspectXMLPath(path string) ([]FileStatus, error) {
	files, err := FindXMLFiles(path)
	if err != nil {
		return nil, err
	}

	statuses := make([]FileStatus, 0, len(files))
	for _, file := range files {
		status := FileStatus{
			Path: file,
			Kind: "xml",
		}
		if err := ParseGenericXMLFile(file); err != nil {
			status.Err = err
		}
		statuses = append(statuses, status)
	}
	return statuses, nil
}

func FindXMLFiles(path string) ([]string, error) {
	var files []string
	dirs := []string{filepath.Clean(path)}

	for len(dirs) > 0 {
		dir := dirs[0]
		dirs = dirs[1:]

		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil, fmt.Errorf("read directory %s: %w", dir, err)
		}

		for _, entry := range entries {
			entryPath := filepath.Join(dir, entry.Name())
			if entry.IsDir() {
				dirs = append(dirs, entryPath)
				continue
			}
			if strings.EqualFold(filepath.Ext(entry.Name()), ".xml") {
				files = append(files, entryPath)
			}
		}
	}

	sort.Strings(files)
	return files, nil
}

func ParseGenericXMLFile(path string) error {
	file, err := os.Open(path) // #nosec G304 -- dzcli intentionally reads user-provided XML paths.
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	defer file.Close()

	decoder := xml.NewDecoder(file)
	for {
		var token any
		token, err = decoder.Token()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		_ = token
	}
}

func ValidateXMLPathJSON(path string, stdout io.Writer) error {
	statuses, err := InspectXMLPath(path)
	if err != nil {
		if writeErr := output.WriteFailure(stdout, fmt.Errorf("xml: failed: %w", err), "xml", path, nil); writeErr != nil {
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
		files = append(files, output.SimpleValidationFile(status.Kind, status.Path, "", status.Err))
	}
	if err := output.WriteValidation(stdout, path, files); err != nil {
		return err
	}
	if !allOK {
		return validation.ErrFailed
	}
	return nil
}

func xmlTextStatuses(statuses []FileStatus) []validation.TextStatus {
	result := make([]validation.TextStatus, 0, len(statuses))
	for _, status := range statuses {
		result = append(result, validation.TextStatus{
			Kind: status.Kind,
			Path: status.Path,
			Err:  status.Err,
		})
	}
	return result
}
