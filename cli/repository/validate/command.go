// Package validate implements repository-wide validation command support.
package validate

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	batchvalidate "dzcli/cli/batch/validate"
	economyvalidate "dzcli/cli/economy/validate"
	aivalidate "dzcli/cli/expansion/ai/validate"
	"dzcli/cli/output"
	"dzcli/cli/validation"
	xmlvalidate "dzcli/cli/xml/validate"
	"dzcli/internal/dayzinit"
	"dzcli/internal/economyconfig"
	"dzcli/internal/expansion"
	"dzcli/internal/gameplayconfig"
	"dzcli/internal/serverconfig"
	"dzcli/internal/weatherconfig"

	"github.com/spf13/cobra"
)

type repositoryResult struct {
	targetPath   string
	textStatuses []validation.TextStatus
	files        []output.ValidationFile
	batchPaths   map[string]bool
}

// NewCommand returns the repository-wide validation command.
func NewCommand(stdout io.Writer) *cobra.Command {
	command := &cobra.Command{
		Use:     "all <repo-or-servers-root>",
		Aliases: []string{"repo"},
		Short:   "Validate a repository or servers root",
		Long: strings.Join([]string{
			"Validate all DayZ server configuration discovered under a repository or servers root.",
			"The command discovers server roots, mission roots, serverDZ.cfg, mission gameplay/weather/init files,",
			"central economy folders with cfgeconomycore.xml, Expansion AI roots, XML trees, and Windows batch files under server roots.",
			"Mission folders without cfgeconomycore.xml skip economy validation while still participating in XML validation.",
		}, " "),
		Example: strings.Join([]string{
			"dzcli validate all ./servers",
			"dzcli validate repo ./dayz-configs",
			"dzcli --output json validate all ./servers",
		}, "\n"),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if output.IsJSON(cmd) {
				return validateRepositoryJSON(args[0], stdout)
			}
			return validateRepositoryWithOptions(args[0], stdout, validation.TextOptionsFromCommand(cmd))
		},
	}
	command.SetOut(stdout)
	return command
}

func validateRepositoryWithOptions(path string, stdout io.Writer, options validation.TextOptions) error {
	result, err := inspectRepository(path)
	if err != nil {
		return fmt.Errorf("repository: failed: %w", err)
	}
	return validation.RenderTextStatuses(stdout, result.textStatuses, options)
}

func validateRepositoryJSON(path string, stdout io.Writer) error {
	result, err := inspectRepository(path)
	if err != nil {
		if writeErr := output.WriteFailure(stdout, fmt.Errorf("repository: failed: %w", err), "repository", path, nil); writeErr != nil {
			return writeErr
		}
		return output.ErrRendered
	}
	if err := output.WriteValidation(stdout, result.targetPath, result.files); err != nil {
		return err
	}
	if hasFailures(result.files) {
		return validation.ErrFailed
	}
	return nil
}

func inspectRepository(path string) (repositoryResult, error) {
	root, err := cleanExistingDir(path)
	if err != nil {
		return repositoryResult{}, err
	}

	serverRoots, err := discoverServerRoots(root)
	if err != nil {
		return repositoryResult{}, err
	}
	if len(serverRoots) == 0 {
		return repositoryResult{}, fmt.Errorf("no repository validation targets found under %s", root)
	}

	result := repositoryResult{targetPath: root}
	for _, serverRoot := range serverRoots {
		result.addServerRoot(serverRoot)
	}
	if len(result.files) == 0 {
		return repositoryResult{}, fmt.Errorf("no repository validation targets found under %s", root)
	}
	return result, nil
}

func (result *repositoryResult) addServerRoot(serverRoot string) {
	if fileExists(filepath.Join(serverRoot, "serverDZ.cfg")) {
		path := filepath.Join(serverRoot, "serverDZ.cfg")
		result.addSimple("server", path, "", serverconfig.ValidateFile(path))
	}

	missionRoots := discoverMissionRoots(serverRoot)
	for _, missionRoot := range missionRoots {
		result.addMissionRoot(missionRoot)
	}

	if !containsPath(missionRoots, serverRoot) {
		result.addExpansionAI(serverRoot)
	}
	result.addXMLTree(serverRoot)
	result.addBatchTree(serverRoot)
}

func (result *repositoryResult) addMissionRoot(missionRoot string) {
	if fileExists(filepath.Join(missionRoot, "cfggameplay.json")) {
		path := filepath.Join(missionRoot, "cfggameplay.json")
		result.addSimple("gameplay", path, "", gameplayconfig.ValidateFile(path))
	}
	if fileExists(filepath.Join(missionRoot, "cfgweather.xml")) {
		path := filepath.Join(missionRoot, "cfgweather.xml")
		result.addSimple("weather", path, "", weatherconfig.ValidateFile(path))
	}
	if fileExists(filepath.Join(missionRoot, "init.c")) {
		path := filepath.Join(missionRoot, "init.c")
		result.addSimple("init", path, "", dayzinit.ValidateFile(path))
	}
	if fileExists(filepath.Join(missionRoot, "cfgeconomycore.xml")) {
		result.addEconomy(missionRoot)
	}
	result.addExpansionAI(missionRoot)
}

func (result *repositoryResult) addEconomy(missionRoot string) {
	statuses, err := economyconfig.InspectEconomy(missionRoot)
	if err != nil {
		result.addSimple("economy", missionRoot, "", err)
		return
	}
	result.textStatuses = append(result.textStatuses, economyvalidate.TextStatuses(statuses)...)
	result.files = append(result.files, output.EconomyValidationFiles(statuses)...)
}

func (result *repositoryResult) addExpansionAI(serverRoot string) {
	statuses, err := expansion.InspectAIPath(serverRoot)
	if err != nil {
		if isNoExpansionAIError(err) {
			return
		}
		result.addSimple("expansion-ai", serverRoot, "", err)
		return
	}
	for _, status := range statuses {
		result.files = append(result.files, output.SimpleValidationFile(status.Kind, status.Path, status.Summary, status.Err))
	}
	result.textStatuses = append(result.textStatuses, aivalidate.TextStatuses(statuses)...)
}

func (result *repositoryResult) addXMLTree(serverRoot string) {
	statuses, err := xmlvalidate.InspectXMLPath(serverRoot)
	if err != nil {
		result.addSimple("xml", serverRoot, "", err)
		return
	}
	for _, status := range statuses {
		result.files = append(result.files, output.SimpleValidationFile(status.Kind, status.Path, "", status.Err))
	}
	result.textStatuses = append(result.textStatuses, xmlvalidate.TextStatuses(statuses)...)
}

func (result *repositoryResult) addBatchTree(serverRoot string) {
	statuses, err := batchvalidate.InspectBatchPath(serverRoot)
	if err != nil {
		result.addSimple("batch", serverRoot, "", err)
		return
	}
	if result.batchPaths == nil {
		result.batchPaths = make(map[string]bool)
	}
	unique := make([]batchvalidate.FileStatus, 0, len(statuses))
	for _, status := range statuses {
		path := filepath.Clean(status.Path)
		if result.batchPaths[path] {
			continue
		}
		result.batchPaths[path] = true
		unique = append(unique, status)
	}
	result.textStatuses = append(result.textStatuses, batchvalidate.TextStatuses(unique)...)
	result.files = append(result.files, batchvalidate.ValidationFiles(unique)...)
}

func (result *repositoryResult) addSimple(kind string, path string, summary string, err error) {
	result.textStatuses = append(result.textStatuses, validation.TextStatus{
		Kind:    kind,
		Path:    filepath.Clean(path),
		Summary: summary,
		Err:     err,
	})
	result.files = append(result.files, output.SimpleValidationFile(kind, filepath.Clean(path), summary, err))
}

func discoverServerRoots(root string) ([]string, error) {
	roots := map[string]bool{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() {
			return nil
		}
		if path != root && shouldSkipDir(entry.Name()) {
			return filepath.SkipDir
		}
		if isServerRoot(path) {
			roots[filepath.Clean(path)] = true
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk %s: %w", root, err)
	}

	if len(roots) == 0 && hasRepositoryTargets(root) {
		roots[filepath.Clean(root)] = true
	}
	return sortedKeys(roots), nil
}

func discoverMissionRoots(serverRoot string) []string {
	roots := map[string]bool{}
	if hasMissionRootFiles(serverRoot) {
		roots[filepath.Clean(serverRoot)] = true
	}

	missionsDir := filepath.Join(serverRoot, "mpmissions")
	entries, err := os.ReadDir(missionsDir)
	if err != nil {
		return sortedKeys(roots)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			roots[filepath.Clean(filepath.Join(missionsDir, entry.Name()))] = true
		}
	}
	return sortedKeys(roots)
}

func cleanExistingDir(path string) (string, error) {
	cleanPath := filepath.Clean(path)
	info, err := os.Stat(cleanPath)
	if err != nil {
		return "", fmt.Errorf("stat %s: %w", cleanPath, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", cleanPath)
	}
	return cleanPath, nil
}

func isServerRoot(path string) bool {
	return fileExists(filepath.Join(path, "serverDZ.cfg")) ||
		dirExists(filepath.Join(path, "mpmissions")) ||
		dirExists(filepath.Join(path, "profiles", "ExpansionMod"))
}

func hasRepositoryTargets(path string) bool {
	if isServerRoot(path) || hasMissionRootFiles(path) {
		return true
	}
	if fileExists(filepath.Join(path, "expansion", "settings", "AIPatrolSettings.json")) ||
		fileExists(filepath.Join(path, "expansion", "settings", "AILocationSettings.json")) {
		return true
	}
	files, err := xmlvalidate.FindXMLFiles(path)
	return err == nil && len(files) > 0
}

func hasMissionRootFiles(path string) bool {
	for _, name := range []string{"cfgeconomycore.xml", "cfggameplay.json", "cfgweather.xml", "init.c"} {
		if fileExists(filepath.Join(path, name)) {
			return true
		}
	}
	return false
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func shouldSkipDir(name string) bool {
	return name == ".git"
}

func sortedKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func containsPath(paths []string, target string) bool {
	cleanTarget := filepath.Clean(target)
	for _, path := range paths {
		if filepath.Clean(path) == cleanTarget {
			return true
		}
	}
	return false
}

func hasFailures(files []output.ValidationFile) bool {
	for _, file := range files {
		if file.Status == output.StatusFailed || len(file.Failures) > 0 {
			return true
		}
	}
	return false
}

func isNoExpansionAIError(err error) bool {
	return strings.Contains(err.Error(), "no Expansion AI config files found under")
}
