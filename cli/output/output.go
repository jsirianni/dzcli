package output

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"dzcli/internal/economyconfig"

	"github.com/spf13/cobra"
)

const (
	FormatText = "text"
	FormatJSON = "json"

	StatusOK          = "ok"
	StatusFailed      = "failed"
	StatusNotModified = "not_modified"
)

var ErrRendered = errors.New("structured output rendered")

type InvalidFormatError struct {
	Value string
}

func (err InvalidFormatError) Error() string {
	return fmt.Sprintf("unsupported output format %q (supported: text, json)", err.Value)
}

type Envelope struct {
	Status      string        `json:"status"`
	TargetPath  string        `json:"target_path"`
	Warnings    []Diagnostic  `json:"warnings"`
	Failures    []Diagnostic  `json:"failures"`
	Remediation []Remediation `json:"remediation"`
	Data        any           `json:"data"`
}

type Diagnostic struct {
	Message     string        `json:"message"`
	Kind        string        `json:"kind"`
	TargetPath  string        `json:"target_path"`
	Remediation []Remediation `json:"remediation"`
}

type Remediation struct {
	ID               string `json:"id"`
	Command          string `json:"command"`
	Detail           string `json:"detail"`
	Class            string `json:"class"`
	Destructive      bool   `json:"destructive"`
	AutoApply        bool   `json:"auto_apply"`
	AlternativeGroup string `json:"alternative_group"`
}

type ValidationFile struct {
	Kind        string        `json:"kind"`
	TargetPath  string        `json:"target_path"`
	Status      string        `json:"status"`
	Summary     string        `json:"summary"`
	TypeCount   int           `json:"type_count"`
	Warnings    []Diagnostic  `json:"warnings"`
	Failures    []Diagnostic  `json:"failures"`
	Remediation []Remediation `json:"remediation"`
}

type ValidationData struct {
	Files []ValidationFile `json:"files"`
}

type RowsData struct {
	Rows []map[string]any `json:"rows"`
}

type MutationData struct {
	Kind        string `json:"kind"`
	DryRun      bool   `json:"dry_run"`
	Changed     bool   `json:"changed"`
	ContentType string `json:"content_type"`
	Content     string `json:"content"`
}

func AddFormatFlag(command *cobra.Command, value *string) {
	command.PersistentFlags().StringVarP(value, "output", "o", FormatText, "output format: text or json")
}

func Format(cmd *cobra.Command) string {
	if cmd == nil {
		return FormatText
	}
	root := cmd.Root()
	if root != nil {
		if flag := root.PersistentFlags().Lookup("output"); flag != nil {
			return flag.Value.String()
		}
	}
	value, err := cmd.Flags().GetString("output")
	if err != nil {
		return FormatText
	}
	return value
}

func ValidateFormat(cmd *cobra.Command) error {
	value := Format(cmd)
	if value == FormatText || value == FormatJSON {
		return nil
	}
	return InvalidFormatError{Value: value}
}

func IsJSON(cmd *cobra.Command) bool {
	return Format(cmd) == FormatJSON
}

func ShouldRenderJSONError(cmd *cobra.Command, err error) bool {
	if err == nil || !IsJSON(cmd) {
		return false
	}
	if IsInvalidFormatError(err) || isFlagParseError(err) {
		return false
	}
	return true
}

func IsInvalidFormatError(err error) bool {
	var target InvalidFormatError
	return errors.As(err, &target)
}

func Write(w io.Writer, envelope Envelope) error {
	envelope = normalizeEnvelope(envelope)
	encoder := json.NewEncoder(w)
	return encoder.Encode(envelope)
}

func WriteFailure(w io.Writer, err error, kind string, targetPath string, remediation []Remediation) error {
	message := ""
	if err != nil {
		message = err.Error()
	}
	return Write(w, Envelope{
		Status:     StatusFailed,
		TargetPath: targetPath,
		Failures: []Diagnostic{{
			Message:     message,
			Kind:        kind,
			TargetPath:  targetPath,
			Remediation: normalizeRemediation(remediation),
		}},
		Remediation: remediation,
	})
}

func WriteRows(w io.Writer, targetPath string, rows []map[string]any) error {
	if rows == nil {
		rows = []map[string]any{}
	}
	return Write(w, Envelope{
		Status:     StatusOK,
		TargetPath: targetPath,
		Data:       RowsData{Rows: rows},
	})
}

func WriteTableRows(w io.Writer, targetPath string, headers []string, rows [][]string) error {
	return WriteRows(w, targetPath, TableRows(headers, rows))
}

func TableRows(headers []string, rows [][]string) []map[string]any {
	result := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		item := map[string]any{}
		for index, header := range headers {
			if index >= len(row) {
				continue
			}
			item[headerKey(header)] = row[index]
		}
		result = append(result, item)
	}
	return result
}

func WriteValidation(w io.Writer, targetPath string, files []ValidationFile) error {
	files = normalizeValidationFiles(files)
	envelope := Envelope{
		Status:     StatusOK,
		TargetPath: targetPath,
		Data:       ValidationData{Files: files},
	}
	for _, file := range files {
		envelope.Warnings = append(envelope.Warnings, file.Warnings...)
		envelope.Failures = append(envelope.Failures, file.Failures...)
		envelope.Remediation = append(envelope.Remediation, file.Remediation...)
	}
	if len(envelope.Failures) > 0 {
		envelope.Status = StatusFailed
	}
	return Write(w, envelope)
}

func SimpleValidationFile(kind string, targetPath string, summary string, err error) ValidationFile {
	status := StatusOK
	var failures []Diagnostic
	if err != nil {
		status = StatusFailed
		failures = []Diagnostic{{
			Message:     err.Error(),
			Kind:        kind,
			TargetPath:  targetPath,
			Remediation: []Remediation{},
		}}
	}
	return normalizeValidationFile(ValidationFile{
		Kind:       kind,
		TargetPath: targetPath,
		Status:     status,
		Summary:    summary,
		Failures:   failures,
	})
}

func EconomyValidationFiles(statuses []economyconfig.FileStatus) []ValidationFile {
	files := make([]ValidationFile, 0, len(statuses))
	for _, status := range statuses {
		file := ValidationFile{
			Kind:       status.Kind,
			TargetPath: status.Path,
			Status:     StatusOK,
			TypeCount:  status.TypeCount,
		}
		if status.Err != nil {
			file.Status = StatusFailed
			file.Failures = []Diagnostic{{
				Message:     status.Err.Error(),
				Kind:        status.Kind,
				TargetPath:  status.Path,
				Remediation: []Remediation{},
			}}
		}
		file.Warnings = economyWarnings(status)
		for _, diagnostic := range append(append([]Diagnostic{}, file.Warnings...), file.Failures...) {
			file.Remediation = append(file.Remediation, diagnostic.Remediation...)
		}
		files = append(files, normalizeValidationFile(file))
	}
	return files
}

func WriteMutation(w io.Writer, targetPath string, kind string, changed bool, dryRun bool, contentType string, data []byte, warnings []Diagnostic) error {
	content := ""
	if dryRun {
		content = string(data)
	}
	return Write(w, Envelope{
		Status:     StatusOK,
		TargetPath: targetPath,
		Warnings:   warnings,
		Data: MutationData{
			Kind:        kind,
			DryRun:      dryRun,
			Changed:     changed,
			ContentType: contentType,
			Content:     content,
		},
	})
}

func WriteNotModified(w io.Writer, targetPath string, kind string) error {
	return Write(w, Envelope{
		Status:     StatusNotModified,
		TargetPath: targetPath,
		Data: map[string]any{
			"kind": kind,
		},
	})
}

func InteractiveRemediation() []Remediation {
	return []Remediation{
		{Detail: "rerun with --force to perform the change without an interactive prompt"},
		{Detail: "rerun with --dry-run to inspect the planned output"},
		{Detail: "omit --output json to use the interactive text prompt"},
	}
}

func FromEconomyAction(action economyconfig.RemediationAction) Remediation {
	return Remediation{
		ID:               action.ID,
		Command:          action.Command,
		Detail:           action.Detail,
		Class:            string(action.Class),
		Destructive:      action.Destructive,
		AutoApply:        action.AutoApply,
		AlternativeGroup: action.AlternativeGroup,
	}
}

func FromEconomyActions(actions []economyconfig.RemediationAction) []Remediation {
	remediation := make([]Remediation, 0, len(actions))
	for _, action := range actions {
		remediation = append(remediation, FromEconomyAction(action))
	}
	return remediation
}

func ManualRemediation() []Remediation {
	return []Remediation{{
		Detail: "validation-only; edit the XML manually",
		Class:  string(economyconfig.RemediationSemantic),
	}}
}

func DiagnosticForWarning(kind string, targetPath string, message string, remediation []Remediation) Diagnostic {
	return Diagnostic{
		Message:     message,
		Kind:        kind,
		TargetPath:  targetPath,
		Remediation: normalizeRemediation(remediation),
	}
}

func normalizeEnvelope(envelope Envelope) Envelope {
	if envelope.Status == "" {
		envelope.Status = StatusOK
	}
	envelope.Warnings = normalizeDiagnostics(envelope.Warnings)
	envelope.Failures = normalizeDiagnostics(envelope.Failures)
	envelope.Remediation = normalizeRemediation(envelope.Remediation)
	if envelope.Data == nil {
		envelope.Data = map[string]any{}
	}
	return envelope
}

func normalizeValidationFiles(files []ValidationFile) []ValidationFile {
	if files == nil {
		return []ValidationFile{}
	}
	for index := range files {
		files[index] = normalizeValidationFile(files[index])
	}
	return files
}

func normalizeValidationFile(file ValidationFile) ValidationFile {
	if file.Status == "" {
		file.Status = StatusOK
	}
	file.Warnings = normalizeDiagnostics(file.Warnings)
	file.Failures = normalizeDiagnostics(file.Failures)
	file.Remediation = normalizeRemediation(file.Remediation)
	return file
}

func normalizeDiagnostics(diagnostics []Diagnostic) []Diagnostic {
	if diagnostics == nil {
		return []Diagnostic{}
	}
	for index := range diagnostics {
		diagnostics[index].Remediation = normalizeRemediation(diagnostics[index].Remediation)
	}
	return diagnostics
}

func normalizeRemediation(remediation []Remediation) []Remediation {
	if remediation == nil {
		return []Remediation{}
	}
	return remediation
}

func economyWarnings(status economyconfig.FileStatus) []Diagnostic {
	if len(status.WarningDetails) > 0 {
		diagnostics := make([]Diagnostic, 0, len(status.WarningDetails))
		for _, warning := range status.WarningDetails {
			remediation := FromEconomyActions(warning.Actions)
			if len(remediation) == 0 && warning.ManualOnly {
				remediation = ManualRemediation()
			}
			diagnostics = append(diagnostics, DiagnosticForWarning(status.Kind, status.Path, warning.Message, remediation))
		}
		return diagnostics
	}
	diagnostics := make([]Diagnostic, 0, len(status.Warnings))
	for _, warning := range status.Warnings {
		diagnostics = append(diagnostics, DiagnosticForWarning(status.Kind, status.Path, warning, ManualRemediation()))
	}
	return diagnostics
}

func headerKey(header string) string {
	key := strings.ToLower(strings.TrimSpace(header))
	key = strings.ReplaceAll(key, "-", "_")
	key = strings.ReplaceAll(key, " ", "_")
	return key
}

func isFlagParseError(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	return strings.HasPrefix(message, "unknown flag:") ||
		strings.HasPrefix(message, "unknown shorthand flag:") ||
		strings.HasPrefix(message, "flag needs an argument:")
}
