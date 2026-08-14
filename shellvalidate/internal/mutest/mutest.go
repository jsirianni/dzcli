// Package mutest implements the deterministic mutation runner used by the
// shellvalidate conformance suite. It is internal because it is test
// infrastructure, not part of the validator's public API.
package mutest

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Status is the observed outcome of executing one mutant.
type Status string

const (
	StatusKilled     Status = "killed"
	StatusSurvived   Status = "survived"
	StatusEquivalent Status = "equivalent"
	StatusInvalid    Status = "invalid"
	StatusExcluded   Status = "excluded"
)

// Selector pins a replacement to one parsed declaration.
type Selector struct {
	File            string `json:"file"`
	Declaration     string `json:"declaration"`
	DeclarationHash string `json:"declarationHash"`
	Before          string `json:"before"`
	After           string `json:"after"`
}

// Mutant declares one stable mutation obligation.
type Mutant struct {
	ID          string    `json:"id"`
	SourceOrder int       `json:"sourceOrder"`
	Target      string    `json:"target"`
	Critical    bool      `json:"critical"`
	KilledBy    string    `json:"killedBy"`
	Disposition Status    `json:"disposition,omitempty"`
	Rationale   string    `json:"rationale,omitempty"`
	Selector    *Selector `json:"selector,omitempty"`
}

// Result records one mutant without timing or host-specific paths so the
// report is byte-for-byte reproducible.
type Result struct {
	ID          string `json:"id"`
	SourceOrder int    `json:"sourceOrder"`
	Target      string `json:"target"`
	Critical    bool   `json:"critical"`
	KilledBy    string `json:"killedBy"`
	Status      Status `json:"status"`
	Rationale   string `json:"rationale,omitempty"`
	Log         string `json:"log,omitempty"`
}

// Report is the deterministic machine-readable run artifact.
type Report struct {
	Schema  int      `json:"schema"`
	Results []Result `json:"results"`
}

// Config controls a mutation run.
type Config struct {
	Root         string
	ManifestPath string
	OutputDir    string
	Timeout      time.Duration
	runCommand   commandRunner
}

type commandRunner func(context.Context, string, []string, []string) ([]byte, error)

// LoadManifest parses and validates a mutation manifest.
func LoadManifest(path string) ([]Mutant, error) {
	// #nosec G304 -- path is an explicit local manifest selected by the developer or CI workflow.
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var mutants []Mutant
	if err := decoder.Decode(&mutants); err != nil {
		return nil, fmt.Errorf("decode mutation manifest: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(mutants))
	previous := 0
	for index, mutant := range mutants {
		if mutant.ID == "" || mutant.Target == "" || mutant.KilledBy == "" || mutant.SourceOrder <= 0 {
			return nil, fmt.Errorf("mutant %d is incomplete", index)
		}
		if _, exists := seen[mutant.ID]; exists {
			return nil, fmt.Errorf("duplicate mutant %q", mutant.ID)
		}
		seen[mutant.ID] = struct{}{}
		if mutant.SourceOrder <= previous {
			return nil, fmt.Errorf("mutant %q is not in source order", mutant.ID)
		}
		previous = mutant.SourceOrder
		switch mutant.Disposition {
		case "", StatusKilled, StatusSurvived, StatusInvalid:
			if mutant.Selector == nil {
				return nil, fmt.Errorf("mutant %q has no selector", mutant.ID)
			}
		case StatusEquivalent, StatusExcluded:
			if mutant.Rationale == "" {
				return nil, fmt.Errorf("mutant %q requires a rationale", mutant.ID)
			}
		default:
			return nil, fmt.Errorf("mutant %q has invalid disposition %q", mutant.ID, mutant.Disposition)
		}
	}
	return mutants, nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("mutation manifest contains multiple JSON values")
		}
		return fmt.Errorf("decode mutation manifest trailer: %w", err)
	}
	return nil
}

// DeclarationHash returns the canonical SHA-256 for a named declaration.
func DeclarationHash(filename, declaration string) (string, error) {
	// #nosec G304 -- filename is a local Go source selected for manifest auditing.
	data, err := os.ReadFile(filename)
	if err != nil {
		return "", err
	}
	_, node, err := findDeclaration(filename, data, declaration)
	if err != nil {
		return "", err
	}
	var canonical bytes.Buffer
	if err := format.Node(&canonical, token.NewFileSet(), node); err != nil {
		return "", fmt.Errorf("format declaration %q: %w", declaration, err)
	}
	digest := sha256.Sum256(canonical.Bytes())
	return hex.EncodeToString(digest[:]), nil
}

// Apply applies one pinned mutation and formats the changed Go file.
func Apply(root string, mutant Mutant) error {
	if mutant.Selector == nil {
		return fmt.Errorf("mutant %q has no selector", mutant.ID)
	}
	selector := mutant.Selector
	filename, err := containedPath(root, selector.File)
	if err != nil {
		return err
	}
	// #nosec G304 -- filename was resolved and containment-checked by containedPath above.
	data, err := os.ReadFile(filename)
	if err != nil {
		return err
	}
	fset, node, err := findDeclaration(filename, data, selector.Declaration)
	if err != nil {
		return err
	}
	var canonical bytes.Buffer
	if err := format.Node(&canonical, token.NewFileSet(), node); err != nil {
		return err
	}
	digest := sha256.Sum256(canonical.Bytes())
	actualHash := hex.EncodeToString(digest[:])
	if actualHash != selector.DeclarationHash {
		return fmt.Errorf("mutant %q declaration hash is stale: got %s, want %s", mutant.ID, actualHash, selector.DeclarationHash)
	}
	start := fset.Position(node.Pos()).Offset
	end := fset.Position(node.End()).Offset
	if start < 0 || end < start || end > len(data) {
		return fmt.Errorf("mutant %q declaration has invalid offsets", mutant.ID)
	}
	before := []byte(selector.Before)
	count := bytes.Count(data[start:end], before)
	if count != 1 {
		return fmt.Errorf("mutant %q selector matched %d times in declaration %q", mutant.ID, count, selector.Declaration)
	}
	relative := bytes.Index(data[start:end], before)
	absolute := start + relative
	mutated := make([]byte, 0, len(data)-len(before)+len(selector.After))
	mutated = append(mutated, data[:absolute]...)
	mutated = append(mutated, selector.After...)
	mutated = append(mutated, data[absolute+len(before):]...)
	formatted, err := format.Source(mutated)
	if err != nil {
		return fmt.Errorf("mutant %q produces invalid Go syntax: %w", mutant.ID, err)
	}
	if bytes.Equal(formatted, data) {
		return fmt.Errorf("mutant %q does not change its source", mutant.ID)
	}
	// #nosec G703 -- filename was resolved and containment-checked by containedPath above.
	return os.WriteFile(filename, formatted, 0o600)
}

func findDeclaration(filename string, data []byte, name string) (*token.FileSet, ast.Decl, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, data, parser.ParseComments)
	if err != nil {
		return nil, nil, fmt.Errorf("parse %s: %w", filename, err)
	}
	var matches []ast.Decl
	for _, declaration := range file.Decls {
		switch item := declaration.(type) {
		case *ast.FuncDecl:
			declarationName := item.Name.Name
			if item.Recv != nil && len(item.Recv.List) == 1 {
				if receiver := receiverName(item.Recv.List[0].Type); receiver != "" {
					declarationName = receiver + "." + declarationName
				}
			}
			if declarationName == name {
				matches = append(matches, item)
			}
		case *ast.GenDecl:
			for _, spec := range item.Specs {
				value, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for _, identifier := range value.Names {
					if identifier.Name == name {
						matches = append(matches, item)
					}
				}
			}
		}
	}
	if len(matches) != 1 {
		return nil, nil, fmt.Errorf("declaration %q matched %d times in %s", name, len(matches), filename)
	}
	return fset, matches[0], nil
}

func receiverName(expression ast.Expr) string {
	switch item := expression.(type) {
	case *ast.Ident:
		return item.Name
	case *ast.StarExpr:
		return receiverName(item.X)
	case *ast.IndexExpr:
		return receiverName(item.X)
	case *ast.IndexListExpr:
		return receiverName(item.X)
	default:
		return ""
	}
}

// Run executes every manifest entry in source order and writes deterministic
// JSON, Markdown, and per-mutant log artifacts.
func Run(ctx context.Context, config Config) (Report, error) {
	if config.Root == "" {
		config.Root = "."
	}
	root, err := filepath.Abs(config.Root)
	if err != nil {
		return Report{}, err
	}
	if config.ManifestPath == "" {
		config.ManifestPath = filepath.Join(root, "shellvalidate", "testdata", "spec", "mutants.json")
	} else if !filepath.IsAbs(config.ManifestPath) {
		config.ManifestPath = filepath.Join(root, config.ManifestPath)
	}
	if config.OutputDir == "" {
		config.OutputDir = filepath.Join(root, "artifacts", "shellvalidate-mutation")
	} else if !filepath.IsAbs(config.OutputDir) {
		config.OutputDir = filepath.Join(root, config.OutputDir)
	}
	if config.Timeout <= 0 {
		config.Timeout = 30 * time.Second
	}
	if config.runCommand == nil {
		config.runCommand = runCommand
	}
	mutants, err := LoadManifest(config.ManifestPath)
	if err != nil {
		return Report{}, err
	}
	if err := os.MkdirAll(filepath.Join(config.OutputDir, "logs"), 0o750); err != nil {
		return Report{}, err
	}
	report := Report{Schema: 1, Results: make([]Result, 0, len(mutants))}
	for _, mutant := range mutants {
		result := Result{ID: mutant.ID, SourceOrder: mutant.SourceOrder, Target: mutant.Target, Critical: mutant.Critical, KilledBy: mutant.KilledBy}
		if mutant.Disposition == StatusEquivalent || mutant.Disposition == StatusExcluded {
			result.Status, result.Rationale = mutant.Disposition, mutant.Rationale
			report.Results = append(report.Results, result)
			continue
		}
		workspace, makeErr := os.MkdirTemp("", "shellvalidate-mutant-")
		if makeErr != nil {
			return Report{}, makeErr
		}
		logData := []byte(nil)
		status := StatusInvalid
		if copyErr := copyWorkspace(root, workspace); copyErr != nil {
			logData = []byte(copyErr.Error())
		} else if applyErr := Apply(workspace, mutant); applyErr != nil {
			logData = []byte(applyErr.Error())
		} else {
			environment := []string{"CGO_ENABLED=0", "GOWORK=off"}
			buildContext, cancelBuild := context.WithTimeout(ctx, config.Timeout)
			buildOutput, buildErr := config.runCommand(buildContext, workspace, []string{"test", "-mod=readonly", "./shellvalidate", "-run", "^$"}, environment)
			cancelBuild()
			logData = append(logData, []byte("BUILD\n")...)
			logData = append(logData, buildOutput...)
			if buildErr == nil {
				testContext, cancelTest := context.WithTimeout(ctx, config.Timeout)
				pattern := "^" + regexpQuoteTest(mutant.KilledBy) + "$"
				testOutput, testErr := config.runCommand(testContext, workspace, []string{"test", "-mod=readonly", "./shellvalidate", "-run", pattern, "-count=1"}, environment)
				cancelTest()
				logData = append(logData, []byte("\nTEST\n")...)
				logData = append(logData, testOutput...)
				if testErr == nil {
					status = StatusSurvived
				} else if errors.Is(testErr, context.DeadlineExceeded) || errors.Is(testContext.Err(), context.DeadlineExceeded) {
					status = StatusInvalid
					logData = append(logData, []byte("\nmutation test timed out")...)
				} else {
					status = StatusKilled
				}
			} else if errors.Is(buildErr, context.DeadlineExceeded) || errors.Is(buildContext.Err(), context.DeadlineExceeded) {
				logData = append(logData, []byte("\nmutant build timed out")...)
			}
		}
		_ = os.RemoveAll(workspace)
		result.Status = status
		result.Log = filepath.ToSlash(filepath.Join("logs", mutant.ID+".log"))
		logData = normalizeLog(logData, workspace, root)
		if writeErr := os.WriteFile(filepath.Join(config.OutputDir, filepath.FromSlash(result.Log)), logData, 0o600); writeErr != nil {
			return Report{}, writeErr
		}
		report.Results = append(report.Results, result)
	}
	if err := WriteReport(config.OutputDir, report); err != nil {
		return Report{}, err
	}
	return report, nil
}

func regexpQuoteTest(name string) string {
	var result strings.Builder
	for _, value := range name {
		if strings.ContainsRune(`\.+*?()|[]{}^$`, value) {
			result.WriteByte('\\')
		}
		result.WriteRune(value)
	}
	return result.String()
}

func runCommand(ctx context.Context, directory string, arguments, environment []string) ([]byte, error) {
	// #nosec G204 -- executable is the fixed Go tool; arguments are internally constructed test/build flags.
	command := exec.CommandContext(ctx, "go", arguments...)
	command.Dir = directory
	command.Env = append(os.Environ(), environment...)
	output, err := command.CombinedOutput()
	if ctx.Err() != nil {
		return output, ctx.Err()
	}
	return output, err
}

func copyWorkspace(root, target string) error {
	for _, name := range []string{"go.mod", "go.sum"} {
		if err := copyFile(filepath.Join(root, name), filepath.Join(target, name)); err != nil {
			return err
		}
	}
	sourceRoot := filepath.Join(root, "shellvalidate")
	targetRoot := filepath.Join(target, "shellvalidate")
	return filepath.WalkDir(sourceRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(sourceRoot, path)
		if err != nil {
			return err
		}
		destination := filepath.Join(targetRoot, relative)
		if entry.IsDir() {
			return os.MkdirAll(destination, 0o750)
		}
		return copyFile(path, destination)
	})
}

func copyFile(source, target string) error {
	// #nosec G304 -- paths are produced by the contained repository walk and fresh temporary workspace.
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	info, err := os.Stat(source)
	if err != nil {
		return err
	}
	// #nosec G703 -- target is produced by the contained repository walk into a fresh temporary workspace.
	return os.WriteFile(target, data, info.Mode().Perm())
}

func containedPath(root, relative string) (string, error) {
	if filepath.IsAbs(relative) {
		return "", fmt.Errorf("mutation path %q must be relative", relative)
	}
	clean := filepath.Clean(relative)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("mutation path %q escapes the workspace", relative)
	}
	return filepath.Join(root, clean), nil
}

func normalizeLog(data []byte, paths ...string) []byte {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	for _, path := range paths {
		if path == "" {
			continue
		}
		text = strings.ReplaceAll(text, path, "<workspace>")
		text = strings.ReplaceAll(text, filepath.ToSlash(path), "<workspace>")
	}
	return []byte(strings.TrimSpace(text) + "\n")
}

// WriteReport writes canonical JSON and Markdown reports.
func WriteReport(outputDir string, report Report) error {
	sort.SliceStable(report.Results, func(left, right int) bool {
		return report.Results[left].SourceOrder < report.Results[right].SourceOrder
	})
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.WriteFile(filepath.Join(outputDir, "results.json"), data, 0o600); err != nil {
		return err
	}
	var markdown strings.Builder
	markdown.WriteString("# Shell validator mutation results\n\n")
	markdown.WriteString("| Mutant | Status | Critical | Killer | Target |\n")
	markdown.WriteString("| --- | --- | --- | --- | --- |\n")
	for _, result := range report.Results {
		fmt.Fprintf(&markdown, "| %s | %s | %t | `%s` | %s |\n", result.ID, result.Status, result.Critical, result.KilledBy, strings.ReplaceAll(result.Target, "|", "\\|"))
	}
	return os.WriteFile(filepath.Join(outputDir, "results.md"), []byte(markdown.String()), 0o600)
}

// CriticalFailures returns critical survivors and invalid mutants.
func CriticalFailures(report Report) []Result {
	var failures []Result
	for _, result := range report.Results {
		if result.Critical && (result.Status == StatusSurvived || result.Status == StatusInvalid) {
			failures = append(failures, result)
		}
	}
	return failures
}
