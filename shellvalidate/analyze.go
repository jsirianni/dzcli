package shellvalidate

import (
	"context"
	"fmt"
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
	unitID  string
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
	nameKnown    bool
	nameWord     Word
	hasName      bool
	assignments  []Word
	arguments    []Word
	start        int
	end          int
	pipelineIn   bool
	pipelineOut  bool
	backgrounded bool
}

func runAnalysis(ctx context.Context, file *File, options Options, state *analysisState) ([]Diagnostic, bool, error) {
	if options.Dialect != DialectAuto && options.Dialect != file.dialect {
		return nil, false, fmt.Errorf("shellvalidate: analysis dialect %d does not match parsed file dialect %d", options.Dialect, file.dialect)
	}
	if state.unitID == "" {
		state.unitID = "source:" + normalizeSourceIdentity(file.filename)
		state.visited[state.unitID] = struct{}{}
	}
	analysis := &analysisContext{
		ctx:        ctx,
		file:       file,
		source:     newSourceFile(file.filename, file.source),
		options:    options,
		commands:   commandViewsFromFile(file),
		exact:      true,
		state:      state,
		categories: enabledCategories(options),
		assigned:   predefinedVariables(),
	}
	if hasIncompleteNodes(file.nodes) {
		analysis.inexact(0, len(file.source), "syntax recovery excluded incomplete commands from analysis")
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

// commandViewsFromFile derives executable commands from the syntax tree. This
// avoids treating compound-command keywords and here-document bodies as
// ordinary commands, and lets recovery exclude incomplete subtrees.
func commandViewsFromFile(file *File) []commandView {
	var result []commandView
	var walk func(Node, bool, bool, bool)
	walk = func(node Node, pipelineIn, pipelineOut, pipelineContext bool) {
		if node.incomplete {
			return
		}
		switch node.kind {
		case NodeCommand, NodeAssignment:
			view := commandViewFromNode(node)
			view.pipelineIn = pipelineIn || pipelineContext
			view.pipelineOut = pipelineOut || pipelineContext
			result = append(result, view)
		case NodePipeline:
			for index, child := range node.children {
				walk(child, index > 0, index+1 < len(node.children), pipelineContext || len(node.children) > 1)
			}
		default:
			for _, child := range node.children {
				walk(child, pipelineIn, pipelineOut, pipelineContext || pipelineIn || pipelineOut)
			}
		}
	}
	for _, node := range file.nodes {
		walk(node, false, false, false)
	}
	return result
}

func commandViewFromNode(node Node) commandView {
	view := commandView{
		assignments: append([]Word(nil), node.assignments...),
		start:       node.span.Start.Offset,
		end:         node.span.End.Offset,
	}
	commandIndex := len(node.assignments)
	if commandIndex >= len(node.words) {
		return view
	}
	view.hasName = true
	view.nameWord = node.words[commandIndex]
	view.name, view.nameKnown = staticWordValue(view.nameWord)
	view.arguments = append([]Word(nil), node.words[commandIndex+1:]...)
	return view
}

func hasIncompleteNodes(nodes []Node) bool {
	for _, node := range nodes {
		if node.incomplete || hasIncompleteNodes(node.children) {
			return true
		}
	}
	return false
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
	for _, command := range analysis.commands {
		for _, item := range command.assignments {
			name, value, known := assignmentValue(item)
			state := variableState{constant: value, known: known}
			state.array = strings.HasPrefix(value, "(")
			analysis.assigned[name] = state
		}
	}
	return nil
}

func assignmentValue(word Word) (name, value string, known bool) {
	if literal, ok := staticWordValue(word); ok {
		if index := strings.IndexByte(literal, '='); index >= 0 {
			return strings.TrimSuffix(literal[:index], "+"), literal[index+1:], true
		}
	}
	var prefix strings.Builder
	for _, part := range word.parts {
		if part.kind != WordLiteral || part.quote != QuoteUnquoted {
			break
		}
		prefix.Write(part.text)
		if index := strings.IndexByte(prefix.String(), '='); index >= 0 {
			return strings.TrimSuffix(prefix.String()[:index], "+"), "", false
		}
	}
	return "", "", false
}

func wordContainsPartKind(word Word, kind WordPartKind) bool {
	for _, part := range word.parts {
		if part.kind == kind {
			return true
		}
	}
	return false
}

func wordContainsBytes(word Word, needle []byte) bool {
	for _, part := range word.parts {
		if strings.Contains(string(part.text), string(needle)) {
			return true
		}
	}
	return false
}

type expansionPass struct{}

func (expansionPass) ID() passID         { return passExpansion }
func (expansionPass) Requires() []passID { return []passID{passSymbols} }
func (expansionPass) Run(analysis *analysisContext) error {
	for _, command := range analysis.commands {
		for _, assignment := range command.assignments {
			if wordContainsPartKind(assignment, WordCommandSubstitution) && wordContainsBytes(assignment, []byte("\\n")) {
				analysis.add("expansion", analysis.source.diagnostic("SHE1002", SeverityWarning, ConfidenceDefinite, "command substitution removes trailing newline bytes from this value", assignment.span.Start.Offset, assignment.span.End.Offset))
			}
		}
	}
	for _, command := range analysis.commands {
		for argumentIndex, argument := range command.arguments {
			if command.name == "[[" || command.name == "((" || (argumentIndex == 0 && command.name == "case") {
				continue
			}
			for _, occurrence := range unquotedExpansionSpans(argument) {
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
	flow := dataflowAnalyzer{analysis: analysis}
	flow.analyze(analysis.file.nodes, newFlowState(), flowFrame{kind: contextCurrentShell})
	return nil
}

type parameterReference struct {
	name       string
	start, end int
}

func parameterReferences(item Word) []parameterReference {
	var result []parameterReference
	for _, part := range item.parts {
		if part.kind != WordParameterExpansion {
			continue
		}
		for index := 0; index < len(part.text); index++ {
			if part.text[index] != '$' || index+1 >= len(part.text) || part.text[index+1] == '(' {
				continue
			}
			cursor := index + 1
			if part.text[cursor] == '{' {
				cursor++
			}
			nameStart := cursor
			for cursor < len(part.text) && isNameByte(part.text[cursor]) {
				cursor++
			}
			if cursor == nameStart && cursor < len(part.text) && strings.ContainsRune("@*#?$!-0123456789", rune(part.text[cursor])) {
				cursor++
			}
			if cursor > nameStart {
				result = append(result, parameterReference{
					name:  string(part.text[nameStart:cursor]),
					start: part.span.Start.Offset + index,
					end:   part.span.Start.Offset + cursor,
				})
			}
			index = cursor
		}
	}
	return result
}

func unquotedExpansionSpans(item Word) []Span {
	var result []Span
	for _, part := range item.parts {
		if part.quote == QuoteUnquoted &&
			(part.kind == WordParameterExpansion || part.kind == WordCommandSubstitution) &&
			string(part.text) != "$@" {
			result = append(result, part.span)
		}
	}
	return result
}

type controlPass struct{}

func (controlPass) ID() passID         { return passControl }
func (controlPass) Requires() []passID { return []passID{passSymbols} }
func (controlPass) Run(analysis *analysisContext) error {
	for _, node := range analysis.file.nodes {
		analysis.checkControlNode(node, 0, 0)
	}
	return nil
}

func (analysis *analysisContext) checkControlNode(node Node, loopDepth, functionDepth int) {
	if node.incomplete {
		return
	}
	switch node.kind {
	case NodeFor, NodeWhile, NodeUntil:
		loopDepth++
	case NodeFunction:
		functionDepth++
	case NodeCommand, NodeAssignment:
		view := commandViewFromNode(node)
		if !view.hasName || !view.nameKnown {
			break
		}
		switch view.name {
		case "break", "continue":
			if loopDepth == 0 {
				analysis.add("control", analysis.source.diagnostic("SHC1001", SeverityError, ConfidenceDefinite, view.name+" is only valid inside a loop", view.nameWord.span.Start.Offset, view.nameWord.span.End.Offset))
			}
		case "return":
			if functionDepth == 0 && analysis.state.depth == 0 {
				analysis.add("control", analysis.source.diagnostic("SHC1002", SeverityError, ConfidenceDefinite, "return is only valid in a function or sourced file", view.nameWord.span.Start.Offset, view.nameWord.span.End.Offset))
			}
		}
	}
	for _, child := range node.children {
		analysis.checkControlNode(child, loopDepth, functionDepth)
	}
}

type redirectionPass struct{}

func (redirectionPass) ID() passID         { return passRedirection }
func (redirectionPass) Requires() []passID { return []passID{passSymbols} }
func (redirectionPass) Run(analysis *analysisContext) error {
	walkCommandNodes(analysis.file.nodes, func(command Node) {
		var stderrToStdout *Redirection
		inputs := make(map[string]struct{})
		for index := range command.redirections {
			redirect := &command.redirections[index]
			target, literal := staticWordValue(redirect.target)
			fd, explicit := redirect.ioNumber, redirect.hasIONumber
			if !explicit {
				if strings.HasPrefix(redirect.operator, "<") {
					fd = 0
				} else {
					fd = 1
				}
			}
			if redirect.operator == ">&" && fd == 2 && literal && target == "1" {
				stderrToStdout = redirect
				continue
			}
			if fd == 1 && stderrToStdout != nil && (redirect.operator == ">" || redirect.operator == ">|" || redirect.operator == ">>") {
				span := stderrToStdout.span
				analysis.add("redirection", analysis.source.diagnostic("SHR1001", SeverityWarning, ConfidenceLikely, "standard error is duplicated before standard output is redirected", span.Start.Offset, span.End.Offset))
				stderrToStdout = nil
			}
			if !literal || target == "" {
				continue
			}
			if fd == 0 && redirect.operator == "<" {
				inputs[target] = struct{}{}
			}
			if fd == 1 && (redirect.operator == ">" || redirect.operator == ">|") {
				if _, same := inputs[target]; same {
					span := redirect.target.span
					analysis.add("redirection", analysis.source.diagnostic("SHR1002", SeverityError, ConfidenceDefinite, "input file is also opened for truncating output", span.Start.Offset, span.End.Offset))
				}
			}
		}
	})
	return nil
}

func walkCommandNodes(nodes []Node, visit func(Node)) {
	for _, node := range nodes {
		if node.incomplete {
			continue
		}
		if node.kind == NodeCommand || node.kind == NodeAssignment {
			visit(node)
		}
		walkCommandNodes(node.children, visit)
	}
}

func staticWordValue(word Word) (string, bool) {
	if len(word.parts) == 0 {
		return "", false
	}
	var value strings.Builder
	for _, part := range word.parts {
		if part.kind != WordLiteral {
			return "", false
		}
		data := part.text
		switch part.quote {
		case QuoteSingle:
			value.Write(data)
		case QuoteDouble, QuoteLocale:
			for index := 0; index < len(data); index++ {
				if data[index] == '$' || data[index] == '`' {
					return "", false
				}
				if data[index] == '\\' && index+1 < len(data) && strings.ContainsRune("$`\"\\\n", rune(data[index+1])) {
					index++
					if data[index] != '\n' {
						value.WriteByte(data[index])
					}
					continue
				}
				value.WriteByte(data[index])
			}
		case QuoteUnquoted:
			for index := 0; index < len(data); index++ {
				if data[index] == '$' || data[index] == '`' {
					return "", false
				}
				if data[index] == '\\' {
					if index+1 >= len(data) {
						return "", false
					}
					index++
					if data[index] != '\n' {
						value.WriteByte(data[index])
					}
					continue
				}
				value.WriteByte(data[index])
			}
		case QuoteANSIC:
			decoded, ok := decodeANSICLiteral(data)
			if !ok {
				return "", false
			}
			value.WriteString(decoded)
		default:
			return "", false
		}
	}
	return value.String(), true
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
		lastArgument := ""
		if len(command.arguments) > 0 {
			lastArgument, _ = staticWordValue(command.arguments[len(command.arguments)-1])
		}
		if model.bracketTerminator && lastArgument != "]" {
			analysis.add("commands", analysis.source.diagnostic("SHB1001", SeverityError, ConfidenceDefinite, "[ command is missing its closing ] argument", command.nameWord.span.Start.Offset, command.end))
		}
		if model.formatArgument >= 0 && len(command.arguments) > model.formatArgument {
			format, _ := staticWordValue(command.arguments[model.formatArgument])
			if format != "" {
				needed := countPrintfOperands(format)
				provided := len(command.arguments) - model.formatArgument - 1
				if needed > provided && provided > 0 {
					item := command.arguments[model.formatArgument]
					analysis.add("commands", analysis.source.diagnostic("SHB1002", SeverityWarning, ConfidenceDefinite, "printf format requires more arguments than the command supplies", item.span.Start.Offset, item.span.End.Offset))
				}
			}
		}
		firstArgument := ""
		if len(command.arguments) > 0 {
			firstArgument, _ = staticWordValue(command.arguments[0])
		}
		if command.name == "echo" && analysis.file.dialect == DialectPOSIX && strings.HasPrefix(firstArgument, "-") {
			analysis.add("portability", analysis.source.diagnostic("SHP1001", SeverityWarning, ConfidenceDefinite, "echo option behavior is not portable; use printf for controlled output", command.arguments[0].span.Start.Offset, command.arguments[0].span.End.Offset))
		}
		if command.name == "local" && analysis.file.dialect == DialectPOSIX {
			analysis.add("portability", analysis.source.diagnostic("SHP1002", SeverityWarning, ConfidenceDefinite, "local variable declarations are not specified by POSIX shell", command.nameWord.span.Start.Offset, command.nameWord.span.End.Offset))
		}
	}
	return nil
}

func decodeANSICLiteral(data []byte) (string, bool) {
	if len(data) < 3 || data[0] != '$' || data[1] != '\'' || data[len(data)-1] != '\'' {
		return "", false
	}
	content := data[2 : len(data)-1]
	var result strings.Builder
	for index := 0; index < len(content); index++ {
		if content[index] != '\\' {
			result.WriteByte(content[index])
			continue
		}
		index++
		if index >= len(content) {
			return "", false
		}
		switch content[index] {
		case 'a':
			result.WriteByte('\a')
		case 'b':
			result.WriteByte('\b')
		case 'e', 'E':
			result.WriteByte(0x1b)
		case 'f':
			result.WriteByte('\f')
		case 'n':
			result.WriteByte('\n')
		case 'r':
			result.WriteByte('\r')
		case 't':
			result.WriteByte('\t')
		case 'v':
			result.WriteByte('\v')
		case '\\', '\'', '"':
			result.WriteByte(content[index])
		case '\n':
			// A backslash-newline pair is removed.
		default:
			return "", false
		}
	}
	return result.String(), true
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
		if command.hasName && !command.nameKnown {
			analysis.inexact(command.nameWord.span.Start.Offset, command.nameWord.span.End.Offset, "dynamic command name prevents command-specific analysis")
			continue
		}
		switch command.name {
		case "eval":
			constant, ok := constantArguments(command.arguments)
			if !ok {
				analysis.add("security", analysis.source.diagnostic("SHX1001", SeverityWarning, ConfidenceLikely, "dynamic text is evaluated as shell code", command.nameWord.span.Start.Offset, command.end))
				analysis.inexact(command.nameWord.span.Start.Offset, command.end, "dynamic eval content prevents exact analysis")
				continue
			}
			if err := analyzeNestedLiteral(analysis, command, "<eval>", []byte(constant)); err != nil {
				return err
			}
		case ".", "source":
			if len(command.arguments) == 0 {
				continue
			}
			requested, known := staticWordValue(command.arguments[0])
			if !known || requested == "" {
				analysis.inexact(command.arguments[0].span.Start.Offset, command.arguments[0].span.End.Offset, "dynamic source path prevents exact analysis")
				continue
			}
			if !analysis.options.AnalyzeSourced {
				analysis.inexact(command.arguments[0].span.Start.Offset, command.arguments[0].span.End.Offset, "sourced file was not resolved for analysis")
				continue
			}
			resolved, data, err := analysis.options.Resolver.Resolve(analysis.ctx, analysis.file.filename, requested)
			if err != nil {
				return fmt.Errorf("shellvalidate: resolve %s from %s: %w", requested, analysis.file.filename, err)
			}
			if err := analyzeNestedLiteral(analysis, command, resolved, data); err != nil {
				return err
			}
		case "rm":
			for _, argument := range command.arguments {
				value, known := staticWordValue(argument)
				if !known || value == "/" || value == "/*" {
					analysis.add("security", analysis.source.diagnostic("SHX1002", SeverityWarning, ConfidenceLikely, "destructive path may become empty or root-like", argument.span.Start.Offset, argument.span.End.Offset))
					if !known {
						analysis.inexact(argument.span.Start.Offset, argument.span.End.Offset, "dynamic destructive path prevents exact safety analysis")
					}
				}
			}
		}
	}
	return nil
}

func constantArguments(arguments []Word) (string, bool) {
	var parts []string
	for _, item := range arguments {
		value, ok := staticWordValue(item)
		if !ok {
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
	key := "source:" + normalizeSourceIdentity(filename)
	if filename == "<eval>" {
		key = fmt.Sprintf("%s/eval:%d:%d", analysis.state.unitID, command.start, command.end)
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
	nestedState := &analysisState{depth: analysis.state.depth + 1, visited: analysis.state.visited, unitID: key}
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
