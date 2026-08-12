package shellvalidate

import (
	"context"
	"errors"
	"fmt"
	"sort"
)

const (
	defaultMaxDiagnostics = 100
	maxSourceBytes        = 8 << 20
	maxNesting            = 256
	maxSourceDepth        = 16
)

// Dialect selects the shell language accepted by the parser.
type Dialect uint8

const (
	DialectAuto Dialect = iota
	DialectPOSIX
	DialectBash
)

// Severity describes the impact of a diagnostic.
type Severity uint8

const (
	SeverityError Severity = iota + 1
	SeverityWarning
	SeverityInfo
	SeverityStyle
)

// Confidence describes how strongly static evidence supports a diagnostic.
type Confidence uint8

const (
	ConfidenceDefinite Confidence = iota + 1
	ConfidenceLikely
	ConfidenceHeuristic
)

// Position identifies a byte and its one-based line and byte column.
type Position struct {
	Offset int
	Line   int
	Column int
}

// Span identifies a half-open source range.
type Span struct {
	Start Position
	End   Position
}

// RelatedInformation points to another location relevant to a diagnostic.
type RelatedInformation struct {
	Message string
	Span    Span
}

// Edit describes one source replacement.
type Edit struct {
	Span    Span
	NewText string
}

// Fix is a behavior-preserving collection of non-overlapping edits.
type Fix struct {
	Description string
	Edits       []Edit
}

// Diagnostic describes one independently actionable script defect.
type Diagnostic struct {
	Code       string
	Severity   Severity
	Confidence Confidence
	Message    string
	Primary    Span
	Related    []RelatedInformation
	Fixes      []Fix
}

// Options configures static analysis.
type Options struct {
	Dialect           Dialect
	MaxDiagnostics    int
	EnableCategories  []string
	DisableCategories []string
	AnalyzeSourced    bool
	Resolver          SourceResolver
}

// SourceResolver resolves an explicitly requested sourced shell file.
type SourceResolver interface {
	Resolve(ctx context.Context, fromFilename, requestedPath string) (resolvedFilename string, source []byte, err error)
}

// Result combines parsing and analysis output.
type Result struct {
	File          *File
	Diagnostics   []Diagnostic
	SyntaxValid   bool
	Valid         bool
	AnalysisExact bool
}

var (
	errInvalidContext = errors.New("shellvalidate: nil context")
	errInvalidFile    = errors.New("shellvalidate: nil file")
)

var knownCategories = map[string]struct{}{
	"syntax": {}, "dialect": {}, "expansion": {}, "variables": {}, "control": {},
	"redirection": {}, "commands": {}, "portability": {}, "security": {},
	"incomplete": {},
}

// Parse parses source according to dialect. Script errors are diagnostics.
func Parse(filename string, source []byte, dialect Dialect) (*File, []Diagnostic, error) {
	if !validDialect(dialect) {
		return nil, nil, fmt.Errorf("shellvalidate: invalid dialect %d", dialect)
	}
	sourceModel := newSourceFile(filename, source)
	interpreter := interpreterDirective(sourceModel.data)
	effectiveDialect := dialect
	if effectiveDialect == DialectAuto {
		effectiveDialect = dialectFromInterpreter(interpreter)
	}
	if len(sourceModel.data) > maxSourceBytes {
		diagnostic := sourceModel.diagnostic("SHS1005", SeverityError, ConfidenceDefinite, "source exceeds the 8 MiB safety limit", 0, 0)
		file := &File{filename: filename, source: sourceModel.data, dialect: effectiveDialect, interpreter: interpreter}
		return file, []Diagnostic{diagnostic}, nil
	}
	tokens, comments, lexicalDiagnostics := lex(sourceModel, effectiveDialect)
	nodes, parseDiagnostics := parseTokens(sourceModel, tokens, effectiveDialect)
	diagnostics := append(append([]Diagnostic(nil), lexicalDiagnostics...), parseDiagnostics...)
	diagnostics = append(diagnostics, bashOnlyDiagnostics(sourceModel, tokens, effectiveDialect)...)
	sortDiagnostics(diagnostics)
	syntaxValid := true
	for _, item := range diagnostics {
		if item.Severity == SeverityError {
			syntaxValid = false
			break
		}
	}
	file := &File{filename: filename, source: sourceModel.data, dialect: effectiveDialect, interpreter: interpreter, nodes: nodes, comments: comments, tokens: cloneTokens(tokens), syntaxValid: syntaxValid}
	return file, diagnostics, nil
}

// Analyze performs semantic and static analysis over a parsed file.
func Analyze(ctx context.Context, file *File, options Options) ([]Diagnostic, bool, error) {
	if ctx == nil {
		return nil, false, errInvalidContext
	}
	if file == nil {
		return nil, false, errInvalidFile
	}
	if err := validateOptions(options); err != nil {
		return nil, false, err
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	return nil, true, nil
}

// Check parses and analyzes one source buffer.
func Check(ctx context.Context, filename string, source []byte, options Options) (Result, error) {
	if ctx == nil {
		return Result{}, errInvalidContext
	}
	if err := validateOptions(options); err != nil {
		return Result{}, err
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	file, parseDiagnostics, err := Parse(filename, source, options.Dialect)
	if err != nil {
		return Result{}, err
	}
	analysisDiagnostics, exact, err := Analyze(ctx, file, options)
	if err != nil {
		return Result{}, err
	}
	diagnostics := append(append([]Diagnostic(nil), parseDiagnostics...), analysisDiagnostics...)
	sortDiagnostics(diagnostics)
	limit := diagnosticLimit(options.MaxDiagnostics)
	if len(diagnostics) > limit {
		diagnostics = diagnostics[:limit]
	}
	valid := true
	for _, item := range diagnostics {
		if item.Severity == SeverityError {
			valid = false
			break
		}
	}
	return Result{File: file, Diagnostics: diagnostics, SyntaxValid: file.syntaxValid, Valid: valid, AnalysisExact: exact}, nil
}

func validDialect(dialect Dialect) bool {
	return dialect == DialectAuto || dialect == DialectPOSIX || dialect == DialectBash
}

func validateOptions(options Options) error {
	if !validDialect(options.Dialect) {
		return fmt.Errorf("shellvalidate: invalid dialect %d", options.Dialect)
	}
	for _, names := range [][]string{options.EnableCategories, options.DisableCategories} {
		for _, name := range names {
			if _, ok := knownCategories[name]; !ok {
				return fmt.Errorf("shellvalidate: unknown diagnostic category %q", name)
			}
		}
	}
	if options.AnalyzeSourced && options.Resolver == nil {
		return errors.New("shellvalidate: AnalyzeSourced requires a resolver")
	}
	return nil
}

func diagnosticLimit(value int) int {
	if value <= 0 {
		return defaultMaxDiagnostics
	}
	return value
}

func sortDiagnostics(items []Diagnostic) {
	sort.SliceStable(items, func(left, right int) bool {
		a, b := items[left], items[right]
		if a.Primary.Start.Offset != b.Primary.Start.Offset {
			return a.Primary.Start.Offset < b.Primary.Start.Offset
		}
		if a.Primary.End.Offset != b.Primary.End.Offset {
			return a.Primary.End.Offset < b.Primary.End.Offset
		}
		if a.Severity != b.Severity {
			return a.Severity < b.Severity
		}
		return a.Code < b.Code
	})
}
