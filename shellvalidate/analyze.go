package shellvalidate

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type passID string

const (
	passDialect     passID = "dialect"
	passSymbols     passID = "symbols"
	passVariables   passID = "variables"
	passExpansion   passID = "expansion"
	passControl     passID = "control"
	passRedirection passID = "redirection"
	passCommands    passID = "commands"
	passSecurity    passID = "security"
)

type analysisPass interface {
	ID() passID
	Requires() []passID
	Run(*analysisContext) error
}

type analysisState struct {
	depth   int
	visited map[string]struct{}
}

type analysisContext struct {
	ctx         context.Context
	file        *File
	source      *sourceFile
	options     Options
	commands    []commandView
	diagnostics []Diagnostic
	exact       bool
	state       *analysisState
	categories  map[string]bool
	assigned    map[string]variableState
}

type variableState struct {
	constant string
	known    bool
	array    bool
	readonly bool
}

type commandView struct {
	name         string
	nameToken    token
	arguments    []token
	tokens       []token
	start        int
	end          int
	pipelineIn   bool
	pipelineOut  bool
	backgrounded bool
}

func runAnalysis(ctx context.Context, file *File, options Options, state *analysisState) ([]Diagnostic, bool, error) {
	analysis := &analysisContext{
		ctx:        ctx,
		file:       file,
		source:     newSourceFile(file.filename, file.source),
		options:    options,
		commands:   commandViews(file.tokens),
		exact:      true,
		state:      state,
		categories: enabledCategories(options),
		assigned:   predefinedVariables(),
	}
	passes := []analysisPass{
		dialectPass{}, symbolPass{}, variablePass{}, expansionPass{}, controlPass{},
		redirectionPass{}, commandPass{}, securityPass{},
	}
	completed := make(map[passID]bool)
	for len(completed) < len(passes) {
		progress := false
		for _, item := range passes {
			if completed[item.ID()] || !requirementsMet(item.Requires(), completed) {
				continue
			}
			if err := ctx.Err(); err != nil {
				return nil, false, err
			}
			if err := item.Run(analysis); err != nil {
				return nil, false, err
			}
			completed[item.ID()] = true
			progress = true
		}
		if !progress {
			return nil, false, fmt.Errorf("shellvalidate: analysis pass dependency cycle")
		}
	}
	analysis.diagnostics = deduplicateDiagnostics(analysis.diagnostics)
	sortDiagnostics(analysis.diagnostics)
	limit := diagnosticLimit(options.MaxDiagnostics)
	if len(analysis.diagnostics) > limit {
		analysis.diagnostics = analysis.diagnostics[:limit]
	}
	return analysis.diagnostics, analysis.exact, nil
}

func requirementsMet(required []passID, completed map[passID]bool) bool {
	for _, item := range required {
		if !completed[item] {
			return false
		}
	}
	return true
}

func enabledCategories(options Options) map[string]bool {
	result := make(map[string]bool)
	if len(options.EnableCategories) == 0 {
		for category := range knownCategories {
			result[category] = true
		}
	} else {
		for _, category := range options.EnableCategories {
			result[category] = true
		}
	}
	for _, category := range options.DisableCategories {
		delete(result, category)
	}
	return result
}

func (analysis *analysisContext) add(category string, diagnostic Diagnostic) {
	if analysis.categories[category] {
		analysis.diagnostics = append(analysis.diagnostics, diagnostic)
	}
}

func (analysis *analysisContext) inexact(start, end int, message string) {
	analysis.exact = false
	analysis.add("incomplete", analysis.source.diagnostic("SHI1001", SeverityInfo, ConfidenceDefinite, message, start, end))
}

func commandViews(tokens []token) []commandView {
	var result []commandView
	var current []token
	pipelineIn := false
	flush := func(pipelineOut, background bool) {
		if len(current) == 0 {
			return
		}
		view := makeCommandView(current)
		view.pipelineIn, view.pipelineOut, view.backgrounded = pipelineIn, pipelineOut, background
		result = append(result, view)
		current = nil
		pipelineIn = pipelineOut
	}
	for _, item := range tokens {
		if item.kind == tokenComment || item.kind == tokenEOF {
			continue
		}
		if item.kind == tokenNewline {
			flush(false, false)
			pipelineIn = false
			continue
		}
		if item.kind == tokenOperator && isCommandSeparator(item.text) {
			isPipeline := item.text == "|" || item.text == "|&"
			flush(isPipeline, item.text == "&")
			if !isPipeline {
				pipelineIn = false
			}
			continue
		}
		current = append(current, item)
	}
	flush(false, false)
	return result
}

func makeCommandView(tokens []token) commandView {
	view := commandView{tokens: append([]token(nil), tokens...), start: tokens[0].start, end: tokens[len(tokens)-1].end}
	redirectionOperand := false
	for _, item := range tokens {
		if item.kind == tokenOperator && isRedirection(item.text) {
			redirectionOperand = true
			continue
		}
		if item.kind != tokenWord {
			continue
		}
		if redirectionOperand {
			redirectionOperand = false
			continue
		}
		if view.name == "" && isAssignmentWord(item.text) {
			continue
		}
		if view.name == "" {
			view.name, view.nameToken = item.text, item
		} else {
			view.arguments = append(view.arguments, item)
		}
	}
	return view
}

func isRedirection(value string) bool {
	switch value {
	case "<", ">", ">>", "<<", "<<-", "<<<", "<&", ">&", "<>", ">|", "&>", "&>>":
		return true
	default:
		return false
	}
}

func isAssignmentWord(value string) bool {
	index := strings.IndexByte(value, '=')
	if index <= 0 {
		return false
	}
	name := strings.TrimSuffix(value[:index], "+")
	return validName(name)
}

func validName(value string) bool {
	if value == "" || !isNameStart(value[0]) {
		return false
	}
	for index := 1; index < len(value); index++ {
		if !isNameByte(value[index]) {
			return false
		}
	}
	return true
}

func isNameStart(value byte) bool {
	return value == '_' || value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z'
}
func isNameByte(value byte) bool { return isNameStart(value) || value >= '0' && value <= '9' }

func predefinedVariables() map[string]variableState {
	result := make(map[string]variableState)
	for _, name := range []string{"IFS", "PATH", "HOME", "PWD", "OLDPWD", "SHELL", "USER", "LOGNAME", "TMPDIR", "RANDOM", "BASH", "BASH_VERSION"} {
		result[name] = variableState{}
	}
	return result
}

func deduplicateDiagnostics(items []Diagnostic) []Diagnostic {
	seen := make(map[string]struct{})
	result := make([]Diagnostic, 0, len(items))
	for _, item := range items {
		key := fmt.Sprintf("%s:%d:%d:%s", item.Code, item.Primary.Start.Offset, item.Primary.End.Offset, item.Message)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, item)
	}
	return result
}

type dialectPass struct{}

func (dialectPass) ID() passID         { return passDialect }
func (dialectPass) Requires() []passID { return nil }
func (dialectPass) Run(analysis *analysisContext) error {
	if analysis.options.Dialect == DialectAuto || analysis.file.interpreter == "" {
		return nil
	}
	interpreterDialect := dialectFromInterpreter(analysis.file.interpreter)
	if interpreterDialect != analysis.options.Dialect {
		end := strings.IndexByte(string(analysis.file.source), '\n')
		if end < 0 {
			end = len(analysis.file.source)
		}
		analysis.add("dialect", analysis.source.diagnostic("SHD1002", SeverityWarning, ConfidenceDefinite, "interpreter directive disagrees with the selected shell dialect", 0, end))
	}
	return nil
}

type symbolPass struct{}

func (symbolPass) ID() passID         { return passSymbols }
func (symbolPass) Requires() []passID { return []passID{passDialect} }
func (symbolPass) Run(analysis *analysisContext) error {
	for _, item := range analysis.file.tokens {
		if item.kind != tokenWord || !isAssignmentWord(item.text) {
			continue
		}
		index := strings.IndexByte(item.text, '=')
		name := strings.TrimSuffix(item.text[:index], "+")
		value := item.text[index+1:]
		state := variableState{constant: value, known: !strings.ContainsAny(value, "$`")}
		state.array = strings.HasPrefix(value, "(")
		analysis.assigned[name] = state
	}
	return nil
}

type expansionPass struct{}

func (expansionPass) ID() passID         { return passExpansion }
func (expansionPass) Requires() []passID { return []passID{passSymbols} }
func (expansionPass) Run(analysis *analysisContext) error {
	for _, item := range analysis.file.tokens {
		if item.kind == tokenWord && isAssignmentWord(item.text) && strings.Contains(item.text, "$(") && strings.Contains(item.text, "\\n") {
			analysis.add("expansion", analysis.source.diagnostic("SHE1002", SeverityWarning, ConfidenceDefinite, "command substitution removes trailing newline bytes from this value", item.start, item.end))
		}
	}
	for _, command := range analysis.commands {
		for argumentIndex, argument := range command.arguments {
			if command.name == "[[" || command.name == "((" || (argumentIndex == 0 && command.name == "case") {
				continue
			}
			for _, occurrence := range unquotedExpansionSpans(analysis.source, argument) {
				if strings.HasPrefix(argument.text, "$((") || argument.text == "$@" {
					continue
				}
				diagnostic := analysis.source.diagnostic("SHE1001", SeverityWarning, ConfidenceLikely, "unquoted expansion may split into multiple arguments or expand path patterns", occurrence.Start.Offset, occurrence.End.Offset)
				analysis.add("expansion", diagnostic)
			}
		}
	}
	return nil
}

type variablePass struct{}

func (variablePass) ID() passID         { return passVariables }
func (variablePass) Requires() []passID { return []passID{passSymbols} }
func (variablePass) Run(analysis *analysisContext) error {
	assigned := predefinedVariables()
	nounset := false
	for _, command := range analysis.commands {
		if command.name == "set" {
			for index, argument := range command.arguments {
				if argument.text == "-u" || argument.text == "-o" && index+1 < len(command.arguments) && command.arguments[index+1].text == "nounset" {
					nounset = true
				}
				if argument.text == "+u" {
					nounset = false
				}
			}
		}
		for _, item := range command.tokens {
			if item.kind == tokenWord && isAssignmentWord(item.text) {
				index := strings.IndexByte(item.text, '=')
				assigned[strings.TrimSuffix(item.text[:index], "+")] = variableState{}
			}
		}
		if command.name == "read" {
			for _, argument := range command.arguments {
				if validName(argument.text) {
					assigned[argument.text] = variableState{}
				}
			}
			if command.pipelineIn || command.pipelineOut {
				analysis.add("variables", analysis.source.diagnostic("SHV1002", SeverityWarning, ConfidenceLikely, "read runs in a pipeline context, so assigned variables may not reach the parent shell", command.nameToken.start, command.end))
			}
		}
		if !nounset {
			continue
		}
		for _, argument := range command.arguments {
			for _, reference := range parameterReferences(analysis.source, argument) {
				if _, ok := assigned[reference.name]; ok || reference.name == "" || reference.name[0] >= '0' && reference.name[0] <= '9' {
					continue
				}
				analysis.add("variables", analysis.source.diagnostic("SHV1001", SeverityWarning, ConfidenceDefinite, "variable is read while nounset is enabled and no assignment is visible", reference.start, reference.end))
			}
		}
	}
	return nil
}

type parameterReference struct {
	name       string
	start, end int
}

func parameterReferences(source *sourceFile, item token) []parameterReference {
	data := source.data[item.start:item.end]
	var result []parameterReference
	quote := byte(0)
	for index := 0; index < len(data); index++ {
		if quote == '\'' {
			if data[index] == '\'' {
				quote = 0
			}
			continue
		}
		if data[index] == '\'' {
			quote = '\''
			continue
		}
		if data[index] != '$' || index+1 >= len(data) || data[index+1] == '(' {
			continue
		}
		cursor := index + 1
		braced := cursor < len(data) && data[cursor] == '{'
		if braced {
			cursor++
		}
		nameStart := cursor
		for cursor < len(data) && isNameByte(data[cursor]) {
			cursor++
		}
		if cursor == nameStart && cursor < len(data) && strings.ContainsRune("@*#?$!-0123456789", rune(data[cursor])) {
			cursor++
		}
		if cursor > nameStart {
			result = append(result, parameterReference{name: string(data[nameStart:cursor]), start: item.start + index, end: item.start + cursor})
		}
		if braced {
			for cursor < len(data) && data[cursor] != '}' {
				cursor++
			}
		}
		index = cursor
	}
	return result
}

func unquotedExpansionSpans(source *sourceFile, item token) []Span {
	var result []Span
	for _, part := range item.parts {
		if part.quote == QuoteUnquoted && (part.kind == WordParameterExpansion || part.kind == WordCommandSubstitution) {
			result = append(result, part.span)
		}
	}
	if len(result) != 0 {
		return result
	}
	data := source.data[item.start:item.end]
	quote := byte(0)
	for index := 0; index < len(data); index++ {
		if quote != 0 {
			if data[index] == quote {
				quote = 0
			} else if data[index] == '\\' && quote == '"' {
				index++
			}
			continue
		}
		if data[index] == '\'' || data[index] == '"' {
			quote = data[index]
			continue
		}
		if data[index] != '$' || index+1 >= len(data) {
			continue
		}
		end := index + 1
		if data[end] == '{' {
			for end < len(data) && data[end] != '}' {
				end++
			}
			if end < len(data) {
				end++
			}
		} else {
			for end < len(data) && (isNameByte(data[end]) || strings.ContainsRune("@*#?$!-", rune(data[end]))) {
				end++
			}
		}
		result = append(result, source.span(item.start+index, item.start+end))
		index = end - 1
	}
	return result
}

type controlPass struct{}

func (controlPass) ID() passID         { return passControl }
func (controlPass) Requires() []passID { return []passID{passSymbols} }
func (controlPass) Run(analysis *analysisContext) error {
	loopDepth, functionDepth := 0, 0
	for _, command := range analysis.commands {
		switch command.name {
		case "for", "while", "until", "select":
			loopDepth++
		case "done":
			if loopDepth > 0 {
				loopDepth--
			}
		case "function":
			functionDepth++
		case "break", "continue":
			if loopDepth == 0 {
				analysis.add("control", analysis.source.diagnostic("SHC1001", SeverityError, ConfidenceDefinite, command.name+" is only valid inside a loop", command.nameToken.start, command.nameToken.end))
			}
		case "return":
			if functionDepth == 0 && analysis.state.depth == 0 {
				analysis.add("control", analysis.source.diagnostic("SHC1002", SeverityError, ConfidenceDefinite, "return is only valid in a function or sourced file", command.nameToken.start, command.nameToken.end))
			}
		}
	}
	return nil
}

type redirectionPass struct{}

func (redirectionPass) ID() passID         { return passRedirection }
func (redirectionPass) Requires() []passID { return []passID{passSymbols} }
func (redirectionPass) Run(analysis *analysisContext) error {
	for _, command := range analysis.commands {
		raw := string(analysis.file.source[command.start:command.end])
		if misplaced := strings.Index(raw, "2>&1"); misplaced >= 0 {
			if output := strings.Index(raw[misplaced+4:], ">"); output >= 0 {
				start := command.start + misplaced
				analysis.add("redirection", analysis.source.diagnostic("SHR1001", SeverityWarning, ConfidenceLikely, "standard error is duplicated before standard output is redirected", start, start+4))
			}
		}
		input, output := literalRedirectPath(raw, "<"), literalRedirectPath(raw, ">")
		if input != "" && input == output {
			start := command.start + strings.LastIndex(raw, output)
			analysis.add("redirection", analysis.source.diagnostic("SHR1002", SeverityError, ConfidenceDefinite, "input file is also opened for truncating output", start, start+len(output)))
		}
	}
	return nil
}

var redirectPathPattern = regexp.MustCompile(`(?:^|[[:space:]])([<>])([[:space:]]*)([^[:space:];&|]+)`)

func literalRedirectPath(raw, operator string) string {
	for _, match := range redirectPathPattern.FindAllStringSubmatch(raw, -1) {
		if match[1] == operator && !strings.ContainsAny(match[3], "$*?[]") {
			return strings.Trim(match[3], "'\"")
		}
	}
	return ""
}

type commandPass struct{}

func (commandPass) ID() passID         { return passCommands }
func (commandPass) Requires() []passID { return []passID{passSymbols, passExpansion} }
func (commandPass) Run(analysis *analysisContext) error {
	for _, command := range analysis.commands {
		model, modeled := commandModels[command.name]
		if !modeled {
			continue
		}
		if model.bracketTerminator && (len(command.arguments) == 0 || command.arguments[len(command.arguments)-1].text != "]") {
			analysis.add("commands", analysis.source.diagnostic("SHB1001", SeverityError, ConfidenceDefinite, "[ command is missing its closing ] argument", command.nameToken.start, command.end))
		}
		if model.formatArgument >= 0 && len(command.arguments) > model.formatArgument {
			format := constantToken(command.arguments[model.formatArgument])
			if format != "" {
				needed := countPrintfOperands(format)
				provided := len(command.arguments) - model.formatArgument - 1
				if needed > provided && provided > 0 {
					item := command.arguments[model.formatArgument]
					analysis.add("commands", analysis.source.diagnostic("SHB1002", SeverityWarning, ConfidenceDefinite, "printf format requires more arguments than the command supplies", item.start, item.end))
				}
			}
		}
		if command.name == "echo" && analysis.file.dialect == DialectPOSIX && len(command.arguments) > 0 && strings.HasPrefix(command.arguments[0].text, "-") {
			analysis.add("portability", analysis.source.diagnostic("SHP1001", SeverityWarning, ConfidenceDefinite, "echo option behavior is not portable; use printf for controlled output", command.arguments[0].start, command.arguments[0].end))
		}
		if command.name == "local" && analysis.file.dialect == DialectPOSIX {
			analysis.add("portability", analysis.source.diagnostic("SHP1002", SeverityWarning, ConfidenceDefinite, "local variable declarations are not specified by POSIX shell", command.nameToken.start, command.nameToken.end))
		}
	}
	return nil
}

func constantToken(item token) string {
	if strings.ContainsAny(item.text, "$`") {
		return ""
	}
	return strings.Trim(item.text, "'\"")
}

func countPrintfOperands(format string) int {
	count := 0
	for index := 0; index < len(format); index++ {
		if format[index] != '%' {
			continue
		}
		if index+1 < len(format) && format[index+1] == '%' {
			index++
			continue
		}
		count++
	}
	return count
}

type securityPass struct{}

func (securityPass) ID() passID         { return passSecurity }
func (securityPass) Requires() []passID { return []passID{passSymbols, passCommands} }
func (securityPass) Run(analysis *analysisContext) error {
	for _, command := range analysis.commands {
		switch command.name {
		case "eval":
			constant, ok := constantArguments(command.arguments)
			if !ok {
				analysis.add("security", analysis.source.diagnostic("SHX1001", SeverityWarning, ConfidenceLikely, "dynamic text is evaluated as shell code", command.nameToken.start, command.end))
				analysis.inexact(command.nameToken.start, command.end, "dynamic eval content prevents exact analysis")
				continue
			}
			if err := analyzeNestedLiteral(analysis, command, "<eval>", []byte(constant)); err != nil {
				return err
			}
		case ".", "source":
			if len(command.arguments) == 0 {
				continue
			}
			requested := constantToken(command.arguments[0])
			if requested == "" {
				analysis.inexact(command.arguments[0].start, command.arguments[0].end, "dynamic source path prevents exact analysis")
				continue
			}
			if analysis.options.AnalyzeSourced {
				resolved, data, err := analysis.options.Resolver.Resolve(analysis.ctx, analysis.file.filename, requested)
				if err != nil {
					return fmt.Errorf("shellvalidate: resolve %s from %s: %w", requested, analysis.file.filename, err)
				}
				if err := analyzeNestedLiteral(analysis, command, resolved, data); err != nil {
					return err
				}
			}
		case "rm":
			for _, argument := range command.arguments {
				if strings.Contains(argument.text, "$") || argument.text == "/" || argument.text == "/*" {
					analysis.add("security", analysis.source.diagnostic("SHX1002", SeverityWarning, ConfidenceLikely, "destructive path may become empty or root-like", argument.start, argument.end))
				}
			}
		}
	}
	return nil
}

func constantArguments(arguments []token) (string, bool) {
	var parts []string
	for _, item := range arguments {
		value := constantToken(item)
		if value == "" && item.text != "''" && item.text != "\"\"" {
			return "", false
		}
		parts = append(parts, value)
	}
	return strings.Join(parts, " "), true
}

func analyzeNestedLiteral(analysis *analysisContext, command commandView, filename string, data []byte) error {
	if analysis.state.depth >= maxSourceDepth {
		analysis.inexact(command.start, command.end, "recursive shell analysis reached its safety limit")
		return nil
	}
	key := filename
	if filename != "<eval>" {
		key = filepath.Clean(filename)
	}
	if _, exists := analysis.state.visited[key]; exists {
		analysis.inexact(command.start, command.end, "sourced-file cycle prevents repeated analysis")
		return nil
	}
	analysis.state.visited[key] = struct{}{}
	defer delete(analysis.state.visited, key)
	file, syntaxDiagnostics, err := Parse(filename, data, analysis.file.dialect)
	if err != nil {
		return err
	}
	nestedState := &analysisState{depth: analysis.state.depth + 1, visited: analysis.state.visited}
	nestedDiagnostics, exact, err := runAnalysis(analysis.ctx, file, analysis.options, nestedState)
	if err != nil {
		return err
	}
	for _, item := range append(syntaxDiagnostics, nestedDiagnostics...) {
		item.Related = append(item.Related, RelatedInformation{Message: "analyzed from here", Span: analysis.source.span(command.start, command.end)})
		analysis.add(categoryForCode(item.Code), item)
	}
	analysis.exact = analysis.exact && exact
	return nil
}

func categoryForCode(code string) string {
	if len(code) < 3 {
		return "incomplete"
	}
	switch code[:3] {
	case "SHS":
		return "syntax"
	case "SHD":
		return "dialect"
	case "SHE":
		return "expansion"
	case "SHV":
		return "variables"
	case "SHC":
		return "control"
	case "SHR":
		return "redirection"
	case "SHB":
		return "commands"
	case "SHP":
		return "portability"
	case "SHX":
		return "security"
	default:
		return "incomplete"
	}
}

func parseStaticDepth(argument token) int {
	value, err := strconv.Atoi(argument.text)
	if err != nil {
		return 0
	}
	return value
}
