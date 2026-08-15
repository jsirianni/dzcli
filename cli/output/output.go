package output

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"dzcli/cli/validation"
	"dzcli/internal/economyconfig"

	"github.com/spf13/cobra"
)

const (
	FormatText = "text"
	FormatJSON = "json"

	StatusOK          = "ok"
	StatusFailed      = "failed"
	StatusNotModified = "not_modified"

	jsonCompactWarningThreshold = 3
	jsonCompactWarningItemLimit = 10
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
	Notices     []Diagnostic  `json:"notices,omitempty"`
	Warnings    []Diagnostic  `json:"warnings"`
	Failures    []Diagnostic  `json:"failures"`
	Remediation []Remediation `json:"remediation"`
	Data        any           `json:"data"`
}

type Diagnostic struct {
	Code        string        `json:"code,omitempty"`
	Severity    string        `json:"severity,omitempty"`
	Message     string        `json:"message"`
	Kind        string        `json:"kind"`
	TargetPath  string        `json:"target_path"`
	Span        *SourceSpan   `json:"span,omitempty"`
	Remediation []Remediation `json:"remediation"`
	Group       *WarningGroup `json:"group,omitempty"`

	groupKey   string
	groupTitle string
	itemLabel  string
}

// SourcePosition identifies a byte and its one-based source location.
type SourcePosition struct {
	Offset int `json:"offset"`
	Line   int `json:"line"`
	Column int `json:"column"`
}

// SourceSpan identifies a half-open source range.
type SourceSpan struct {
	Start SourcePosition `json:"start"`
	End   SourcePosition `json:"end"`
}

// WarningGroup describes a compact validation warning group.
type WarningGroup struct {
	Key          string   `json:"key"`
	Title        string   `json:"title"`
	Count        int      `json:"count"`
	Items        []string `json:"items"`
	OmittedItems int      `json:"omitted_items,omitempty"`
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
	Notices     []Diagnostic  `json:"notices,omitempty"`
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
	return WriteValidationWithOptions(w, targetPath, files, validation.DefaultTextOptions())
}

// WriteValidationWithOptions writes validation output using the provided warning mode.
func WriteValidationWithOptions(w io.Writer, targetPath string, files []ValidationFile, options validation.TextOptions) error {
	files = normalizeValidationFiles(files)
	files = applyValidationWarningMode(files, options.WarningMode)
	envelope := Envelope{
		Status:     StatusOK,
		TargetPath: targetPath,
		Data:       ValidationData{Files: files},
	}
	for _, file := range files {
		envelope.Notices = append(envelope.Notices, file.Notices...)
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

func diagnosticForEconomyWarning(status economyconfig.FileStatus, warning economyconfig.WarningDetail, remediation []Remediation) Diagnostic {
	diagnostic := DiagnosticForWarning(status.Kind, status.Path, warning.Message, remediation)
	diagnostic.groupKey = warning.GroupKey
	diagnostic.groupTitle = warning.GroupTitle
	diagnostic.itemLabel = warning.ItemLabel
	return diagnostic
}

func normalizeEnvelope(envelope Envelope) Envelope {
	if envelope.Status == "" {
		envelope.Status = StatusOK
	}
	envelope.Notices = normalizeDiagnostics(envelope.Notices)
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
	file.Notices = normalizeDiagnostics(file.Notices)
	file.Warnings = normalizeDiagnostics(file.Warnings)
	file.Failures = normalizeDiagnostics(file.Failures)
	file.Remediation = normalizeRemediation(file.Remediation)
	return file
}

func applyValidationWarningMode(files []ValidationFile, mode string) []ValidationFile {
	if mode == "" {
		mode = validation.WarningModeCompact
	}
	if mode == validation.WarningModeCompact {
		files = compactValidationWarnings(files)
	}
	for index := range files {
		files[index].Remediation = validationFileRemediation(files[index])
	}
	return files
}

type validationWarningGroup struct {
	Key      string
	Title    string
	Warnings []validationGroupedWarning
}

type validationGroupedWarning struct {
	Diagnostic Diagnostic
}

func compactValidationWarnings(files []ValidationFile) []ValidationFile {
	groups := compactValidationWarningGroups(files)
	if len(groups) == 0 {
		return files
	}
	printedGroups := map[string]bool{}
	for fileIndex := range files {
		warnings := make([]Diagnostic, 0, len(files[fileIndex].Warnings))
		for _, warning := range files[fileIndex].Warnings {
			group, compacted := groups[warning.groupKey]
			if !compacted {
				warnings = append(warnings, warning)
				continue
			}
			if printedGroups[warning.groupKey] {
				continue
			}
			printedGroups[warning.groupKey] = true
			warnings = append(warnings, diagnosticForWarningGroup(group))
		}
		files[fileIndex].Warnings = warnings
	}
	return files
}

func compactValidationWarningGroups(files []ValidationFile) map[string]validationWarningGroup {
	groups := map[string]validationWarningGroup{}
	for _, file := range files {
		for _, warning := range file.Warnings {
			if warning.groupKey == "" {
				continue
			}
			group := groups[warning.groupKey]
			group.Key = warning.groupKey
			if group.Title == "" {
				group.Title = warning.groupTitle
			}
			group.Warnings = append(group.Warnings, validationGroupedWarning{Diagnostic: warning})
			groups[warning.groupKey] = group
		}
	}
	for key, group := range groups {
		if len(group.Warnings) < jsonCompactWarningThreshold {
			delete(groups, key)
		}
	}
	return groups
}

func diagnosticForWarningGroup(group validationWarningGroup) Diagnostic {
	title := group.Title
	if title == "" {
		title = "similar warnings"
	}
	kind, targetPath := warningGroupScope(group)
	items, omittedItems := warningGroupItems(group)
	return Diagnostic{
		Message:     fmt.Sprintf("%d %s", len(group.Warnings), title),
		Kind:        kind,
		TargetPath:  targetPath,
		Remediation: warningGroupRemediation(group),
		Group: &WarningGroup{
			Key:          group.Key,
			Title:        title,
			Count:        len(group.Warnings),
			Items:        items,
			OmittedItems: omittedItems,
		},
	}
}

func warningGroupScope(group validationWarningGroup) (string, string) {
	if len(group.Warnings) == 0 {
		return "validation", ""
	}
	first := group.Warnings[0].Diagnostic
	sameKind := true
	samePath := true
	for _, item := range group.Warnings[1:] {
		if item.Diagnostic.Kind != first.Kind {
			sameKind = false
		}
		if item.Diagnostic.TargetPath != first.TargetPath {
			samePath = false
		}
	}
	if sameKind && samePath {
		return first.Kind, first.TargetPath
	}
	if sameKind {
		return first.Kind, ""
	}
	return "validation", ""
}

func warningGroupItems(group validationWarningGroup) ([]string, int) {
	labels := make([]string, 0, len(group.Warnings))
	for _, item := range group.Warnings {
		if item.Diagnostic.itemLabel == "" {
			continue
		}
		labels = append(labels, item.Diagnostic.itemLabel)
	}
	if len(labels) <= jsonCompactWarningItemLimit {
		return labels, 0
	}
	return labels[:jsonCompactWarningItemLimit], len(labels) - jsonCompactWarningItemLimit
}

func warningGroupRemediation(group validationWarningGroup) []Remediation {
	remediation, shared := sharedWarningGroupRemediation(group)
	if shared {
		return remediation
	}
	return []Remediation{{Detail: "rerun with --warnings full for per-item remediation"}}
}

func sharedWarningGroupRemediation(group validationWarningGroup) ([]Remediation, bool) {
	if len(group.Warnings) == 0 {
		return []Remediation{}, true
	}
	first := group.Warnings[0].Diagnostic.Remediation
	exact := true
	equivalent := true
	for _, item := range group.Warnings[1:] {
		current := item.Diagnostic.Remediation
		if !remediationSlicesEqual(first, current) {
			exact = false
		}
		if !remediationSlicesEquivalent(first, current) {
			equivalent = false
		}
	}
	if exact {
		return append([]Remediation{}, first...), true
	}
	if equivalent {
		return remediationWithoutIDs(first), true
	}
	return nil, false
}

func remediationSlicesEqual(left []Remediation, right []Remediation) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func remediationSlicesEquivalent(left []Remediation, right []Remediation) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		leftItem := left[index]
		rightItem := right[index]
		leftItem.ID = ""
		rightItem.ID = ""
		if leftItem != rightItem {
			return false
		}
	}
	return true
}

func remediationWithoutIDs(remediation []Remediation) []Remediation {
	result := make([]Remediation, 0, len(remediation))
	for _, item := range remediation {
		item.ID = ""
		result = append(result, item)
	}
	return result
}

func validationFileRemediation(file ValidationFile) []Remediation {
	remediation := []Remediation{}
	for _, diagnostic := range file.Warnings {
		remediation = append(remediation, diagnostic.Remediation...)
	}
	for _, diagnostic := range file.Failures {
		remediation = append(remediation, diagnostic.Remediation...)
	}
	return normalizeRemediation(remediation)
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
			diagnostics = append(diagnostics, diagnosticForEconomyWarning(status, warning, remediation))
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
