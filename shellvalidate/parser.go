package shellvalidate

import (
	"bytes"
	"path"
	"strconv"
	"strings"
)

type parserStop struct {
	words map[string]bool
	ops   map[string]bool
}

type shellParser struct {
	source      *sourceFile
	tokens      []token
	dialect     Dialect
	index       int
	depth       int
	diagnostics []Diagnostic
	pendingHere []*pendingHereDocument
}

type pendingHereDocument struct {
	document      *HereDocument
	delimiter     []byte
	delimiterSpan Span
}

func parseTokens(source *sourceFile, tokens []token, dialect Dialect) ([]Node, []Diagnostic) {
	parser := &shellParser{source: source, tokens: tokens, dialect: dialect}
	nodes := parser.parseSequence(parserStop{})
	if len(parser.pendingHere) != 0 {
		parser.consumeHereDocuments(len(source.data))
	}
	for !parser.atEOF() {
		before := parser.index
		item := parser.current()
		parser.unexpected(item, "unexpected token "+item.text)
		parser.index++
		if parser.index <= before {
			parser.index = before + 1
		}
	}
	sortDiagnostics(parser.diagnostics)
	return nodes, parser.diagnostics
}

func (parser *shellParser) parseSequence(stop parserStop) []Node {
	var nodes []Node
	for {
		parser.skipComments()
		for parser.current().kind == tokenNewline || parser.isOperator(";") {
			parser.consumeSeparator()
			parser.skipComments()
		}
		if parser.atEOF() || parser.matchesStop(stop) {
			break
		}
		if parser.isOperator(")") || parser.isOperator("}") || parser.isOperator("))") || parser.isOperator("]]", ";;", ";&", ";;&") || parser.isWord("then", "do", "else", "elif", "fi", "done", "esac") {
			break
		}
		before := parser.index
		node := parser.parseAndOr()
		if node.kind != "" {
			nodes = append(nodes, node)
		}
		if parser.index <= before {
			item := parser.current()
			parser.unexpected(item, "parser could not continue at "+item.text)
			parser.index++
		}
		parser.skipComments()
		if parser.atEOF() || parser.matchesStop(stop) {
			break
		}
		if parser.current().kind == tokenNewline || parser.isOperator(";", "&") {
			parser.consumeSeparator()
			continue
		}
		if parser.isOperator(";;", ";&", ";;&") {
			item := parser.current()
			parser.unexpected(item, "case-clause terminator is only valid inside case")
			parser.index++
			continue
		}
	}
	return nodes
}

func (parser *shellParser) parseAndOr() Node {
	left := parser.parsePipeline()
	for parser.isOperator("&&", "||") {
		operator := parser.current()
		parser.index++
		right := parser.parsePipeline()
		if right.kind == "" {
			parser.unexpected(operator, "list operator is missing its right-hand command")
			left.incomplete = true
			break
		}
		left.role, right.role = RoleListElement, RoleListElement
		left = Node{kind: NodeList, operator: operator.text, span: parser.source.span(left.span.Start.Offset, right.span.End.Offset), children: []Node{left, right}}
	}
	return left
}

func (parser *shellParser) parsePipeline() Node {
	if parser.isWord("time") {
		return parser.parseTimePipeline()
	}
	return parser.parsePipelineCore()
}

func (parser *shellParser) parsePipelineCore() Node {
	negated := false
	if parser.isWord("!") {
		negated = true
		parser.index++
	}
	first := parser.parseCommand()
	if first.kind == "" {
		return first
	}
	commands := []Node{first}
	operators := make([]string, 0, 1)
	for parser.isOperator("|", "|&") {
		operator := parser.current()
		parser.index++
		right := parser.parseCommand()
		if right.kind == "" {
			parser.unexpected(operator, "pipeline operator is missing its right-hand command")
			commands[len(commands)-1].incomplete = true
			break
		}
		operators = append(operators, operator.text)
		commands = append(commands, right)
	}
	if len(commands) == 1 && !negated {
		return commands[0]
	}
	for index := range commands {
		commands[index].role = RolePipelineCommand
	}
	operator := strings.Join(operators, " ")
	if negated {
		operator = strings.TrimSpace("! " + operator)
	}
	return Node{kind: NodePipeline, operator: operator, span: parser.source.span(commands[0].span.Start.Offset, commands[len(commands)-1].span.End.Offset), children: commands}
}

func (parser *shellParser) parseTimePipeline() Node {
	opener := parser.current()
	parser.index++
	words := []Word{wordFromToken(parser.source, opener)}
	for parser.current().kind == tokenWord && (parser.current().text == "-p" || parser.current().text == "--") {
		words = append(words, wordFromToken(parser.source, parser.current()))
		parser.index++
	}
	child := parser.parsePipelineCore()
	if child.kind == "" {
		parser.unexpected(opener, "time is missing its pipeline")
		return Node{kind: NodeTime, span: itemSpan(parser.source, opener), words: words, incomplete: true}
	}
	child.role = RoleBody
	return Node{kind: NodeTime, span: parser.source.span(opener.start, child.span.End.Offset), words: words, children: []Node{child}}
}

func (parser *shellParser) parseCommand() Node {
	node := parser.parseCommandPrimary()
	if node.kind != "" && node.kind != NodeCommand && node.kind != NodeAssignment {
		parser.parseTrailingRedirections(&node)
	}
	return node
}

func (parser *shellParser) parseCommandPrimary() Node {
	parser.skipComments()
	if parser.atEOF() {
		return Node{}
	}
	if parser.depth > maxNesting {
		item := parser.current()
		parser.diagnostics = append(parser.diagnostics, parser.source.diagnostic("SHS1005", SeverityError, ConfidenceDefinite, "shell syntax exceeds the nesting safety limit", item.start, item.end))
		parser.index++
		return Node{kind: NodeCommand, span: itemSpan(parser.source, item), incomplete: true}
	}
	switch {
	case parser.isOperator("(("):
		return parser.parseExpressionCommand(NodeArithmetic, "))")
	case parser.isOperator("[["):
		return parser.parseExpressionCommand(NodeConditional, "]]")
	case parser.isOperator("("):
		return parser.parseGrouped(NodeSubshell, ")")
	case parser.isOperator("{"):
		return parser.parseGrouped(NodeBraceGroup, "}")
	case parser.isWord("if"):
		return parser.parseIf()
	case parser.isWord("for"):
		return parser.parseForLoop(false)
	case parser.isWord("select"):
		return parser.parseForLoop(true)
	case parser.isWord("while"):
		return parser.parseWhileLoop(NodeWhile)
	case parser.isWord("until"):
		return parser.parseWhileLoop(NodeUntil)
	case parser.isWord("case"):
		return parser.parseCase()
	case parser.isWord("coproc"):
		return parser.parseCoprocess()
	case parser.isWord("function"):
		return parser.parseFunction(true)
	case parser.functionAhead():
		return parser.parseFunction(false)
	default:
		return parser.parseSimpleCommand()
	}
}

func (parser *shellParser) parseExpressionCommand(kind NodeKind, closer string) Node {
	opener := parser.current()
	parser.index++
	contentStart := opener.end
	depth := 1
	closeIndex := -1
	for index := parser.index; index < len(parser.tokens); index++ {
		item := parser.tokens[index]
		if item.kind != tokenOperator {
			continue
		}
		if kind == NodeArithmetic && item.text == "((" || kind == NodeConditional && item.text == "[[" {
			depth++
		}
		if item.text == closer {
			depth--
			if depth == 0 {
				closeIndex = index
				break
			}
		}
	}
	if closeIndex < 0 {
		parser.index = len(parser.tokens) - 1
		parser.diagnostics = append(parser.diagnostics, parser.source.diagnostic("SHS1004", SeverityError, ConfidenceDefinite, "compound command is missing closing "+closer, opener.start, len(parser.source.data)))
		return Node{kind: kind, span: parser.source.span(opener.start, len(parser.source.data)), incomplete: true}
	}
	closerToken := parser.tokens[closeIndex]
	parser.index = closeIndex + 1
	var expressions []Expression
	var diagnostics []Diagnostic
	if kind == NodeArithmetic {
		expressions, diagnostics = parseArithmeticExpressionSet(parser.source, contentStart, closerToken.start)
	} else {
		expressions, diagnostics = parseConditionalExpressionSet(parser.source, contentStart, closerToken.start)
	}
	parser.diagnostics = append(parser.diagnostics, diagnostics...)
	return Node{kind: kind, span: parser.source.span(opener.start, closerToken.end), expressions: expressions, incomplete: len(diagnostics) != 0}
}

func (parser *shellParser) parseGrouped(kind NodeKind, closer string) Node {
	opener := parser.current()
	parser.index++
	parser.depth++
	children := parser.parseSequence(parserStop{ops: map[string]bool{closer: true}})
	parser.depth--
	end := opener.end
	incomplete := false
	if parser.isOperator(closer) {
		end = parser.current().end
		parser.index++
	} else {
		incomplete = true
		end = parser.sequenceEnd(children, len(parser.source.data))
		parser.diagnostics = append(parser.diagnostics, parser.source.diagnostic("SHS1004", SeverityError, ConfidenceDefinite, "compound command is missing closing "+closer, opener.start, end))
	}
	children = roleChildren(children, RoleBody)
	return Node{kind: kind, span: parser.source.span(opener.start, end), children: children, incomplete: incomplete}
}

func (parser *shellParser) parseIf() Node {
	opener := parser.current()
	parser.index++
	condition := parser.parseSequence(parserStop{words: map[string]bool{"then": true}})
	incomplete := !parser.consumeWord("then")
	if incomplete {
		parser.missingWord(opener, "then")
	}
	body := parser.parseSequence(parserStop{words: map[string]bool{"elif": true, "else": true, "fi": true}})
	children := append(roleChildren(condition, RoleCondition), roleChildren(body, RoleBody)...)
	if parser.isWord("elif") {
		alternate := parser.parseElif()
		alternate.role = RoleAlternate
		children = append(children, alternate)
	} else if parser.consumeWord("else") {
		alternate := parser.parseSequence(parserStop{words: map[string]bool{"fi": true}})
		children = append(children, roleChildren(alternate, RoleAlternate)...)
	}
	end := parser.sequenceEnd(children, len(parser.source.data))
	if parser.isWord("fi") {
		end = parser.current().end
		parser.index++
	} else {
		incomplete = true
		parser.missingWord(opener, "fi")
	}
	return Node{kind: NodeIf, span: parser.source.span(opener.start, end), children: children, incomplete: incomplete}
}

func (parser *shellParser) parseElif() Node {
	opener := parser.current()
	parser.index++
	condition := parser.parseSequence(parserStop{words: map[string]bool{"then": true}})
	incomplete := !parser.consumeWord("then")
	if incomplete {
		parser.missingWord(opener, "then")
	}
	body := parser.parseSequence(parserStop{words: map[string]bool{"elif": true, "else": true, "fi": true}})
	children := append(roleChildren(condition, RoleCondition), roleChildren(body, RoleBody)...)
	if parser.isWord("elif") {
		nested := parser.parseElif()
		nested.role = RoleAlternate
		children = append(children, nested)
	} else if parser.consumeWord("else") {
		children = append(children, roleChildren(parser.parseSequence(parserStop{words: map[string]bool{"fi": true}}), RoleAlternate)...)
	}
	return Node{kind: NodeIf, span: parser.source.span(opener.start, parser.sequenceEnd(children, opener.end)), children: children, incomplete: incomplete}
}

func (parser *shellParser) parseWhileLoop(kind NodeKind) Node {
	opener := parser.current()
	parser.index++
	header := parser.parseSequence(parserStop{words: map[string]bool{"do": true}})
	incomplete := !parser.consumeWord("do")
	if incomplete {
		parser.missingWord(opener, "do")
	}
	body := parser.parseSequence(parserStop{words: map[string]bool{"done": true}})
	children := append(roleChildren(header, RoleCondition), roleChildren(body, RoleBody)...)
	end := parser.sequenceEnd(children, len(parser.source.data))
	if parser.isWord("done") {
		end = parser.current().end
		parser.index++
	} else {
		incomplete = true
		parser.missingWord(opener, "done")
	}
	return Node{kind: kind, span: parser.source.span(opener.start, end), children: children, incomplete: incomplete}
}

func (parser *shellParser) parseForLoop(selectLoop bool) Node {
	opener := parser.current()
	operator := opener.text
	parser.index++
	var words []Word
	incomplete := false
	var header []Node
	if parser.isOperator("((") {
		arithmetic := parser.parseExpressionCommand(NodeArithmetic, "))")
		arithmetic.role = RoleCondition
		header = append(header, arithmetic)
	} else {
		if parser.current().kind != tokenWord || parser.isWord("in", "do") {
			parser.unexpected(parser.current(), operator+" is missing its iteration variable")
			incomplete = true
		} else {
			words = append(words, wordFromToken(parser.source, parser.current()))
			parser.index++
		}
		if parser.consumeWord("in") {
			for !parser.atEOF() && parser.current().kind != tokenNewline && !parser.isOperator(";") {
				if parser.current().kind != tokenWord {
					parser.unexpected(parser.current(), "iteration list requires shell words")
					incomplete = true
					parser.index++
					continue
				}
				words = append(words, wordFromToken(parser.source, parser.current()))
				parser.index++
			}
		}
	}
	if parser.current().kind == tokenNewline || parser.isOperator(";") {
		parser.consumeSeparator()
	} else if !parser.isWord("do") {
		parser.unexpected(parser.current(), operator+" header requires a newline or semicolon before do")
		incomplete = true
	}
	for parser.current().kind == tokenNewline {
		parser.consumeSeparator()
	}
	if !parser.consumeWord("do") {
		parser.missingWord(opener, "do")
		incomplete = true
	}
	body := parser.parseSequence(parserStop{words: map[string]bool{"done": true}})
	children := append(header, roleChildren(body, RoleBody)...)
	end := parser.sequenceEnd(children, opener.end)
	if parser.isWord("done") {
		end = parser.current().end
		parser.index++
	} else {
		parser.missingWord(opener, "done")
		incomplete = true
	}
	_ = selectLoop // select shares the for-clause shape; dialect audit is separate.
	return Node{kind: NodeFor, operator: operator, span: parser.source.span(opener.start, end), words: words, children: children, incomplete: incomplete}
}

func (parser *shellParser) parseCase() Node {
	opener := parser.current()
	parser.index++
	var subject []Word
	if parser.current().kind == tokenWord {
		subject = append(subject, wordFromToken(parser.source, parser.current()))
		parser.index++
	}
	for parser.current().kind == tokenNewline || parser.isOperator(";") {
		parser.consumeSeparator()
	}
	incomplete := !parser.consumeWord("in")
	if incomplete {
		parser.missingWord(opener, "in")
	}
	var arms []Node
	for !parser.atEOF() && !parser.isWord("esac") {
		for parser.current().kind == tokenNewline || parser.isOperator(";") {
			parser.consumeSeparator()
		}
		if parser.isWord("esac") {
			break
		}
		armStart := parser.current().start
		var patterns []Word
		if parser.isOperator("(") {
			parser.index++
		}
		for !parser.atEOF() && !parser.isOperator(")") {
			if parser.current().kind == tokenWord {
				patterns = append(patterns, wordFromToken(parser.source, parser.current()))
			}
			parser.index++
		}
		if !parser.consumeOperator(")") {
			parser.unexpected(parser.current(), "case pattern is missing )")
			incomplete = true
			break
		}
		body := parser.parseSequence(parserStop{words: map[string]bool{"esac": true}, ops: map[string]bool{";;": true, ";&": true, ";;&": true}})
		end := parser.sequenceEnd(body, armStart)
		operator := ""
		if parser.isOperator(";;", ";&", ";;&") {
			operator = parser.current().text
			end = parser.current().end
			parser.index++
		}
		arms = append(arms, Node{kind: NodeList, role: RoleCaseArm, operator: operator, span: parser.source.span(armStart, end), words: patterns, children: roleChildren(body, RoleBody)})
	}
	end := parser.sequenceEnd(arms, len(parser.source.data))
	if parser.isWord("esac") {
		end = parser.current().end
		parser.index++
	} else {
		incomplete = true
		parser.missingWord(opener, "esac")
	}
	return Node{kind: NodeCase, span: parser.source.span(opener.start, end), words: subject, children: arms, incomplete: incomplete}
}

func (parser *shellParser) parseCoprocess() Node {
	opener := parser.current()
	parser.index++
	child := parser.parseCommand()
	if child.kind == "" {
		parser.unexpected(opener, "coproc is missing its command")
		return Node{kind: NodeCoprocess, span: itemSpan(parser.source, opener), incomplete: true}
	}
	child.role = RoleBody
	return Node{kind: NodeCoprocess, span: parser.source.span(opener.start, child.span.End.Offset), children: []Node{child}}
}

func (parser *shellParser) parseFunction(keyword bool) Node {
	start := parser.current().start
	var name token
	if keyword {
		parser.index++
		if parser.current().kind != tokenWord {
			parser.unexpected(parser.current(), "function is missing its name")
			return Node{kind: NodeFunction, span: parser.source.span(start, parser.current().end), incomplete: true}
		}
		name = parser.current()
		parser.index++
		if parser.isOperator("(") && parser.peekOperator(1, ")") {
			parser.index += 2
		}
	} else {
		name = parser.current()
		parser.index += 3
	}
	body := parser.parseCommandPrimary()
	if body.kind == "" {
		parser.unexpected(name, "function is missing its body")
		return Node{kind: NodeFunction, span: parser.source.span(start, name.end), words: []Word{wordFromToken(parser.source, name)}, incomplete: true}
	}
	body.role = RoleFunctionBody
	return Node{kind: NodeFunction, span: parser.source.span(start, body.span.End.Offset), words: []Word{wordFromToken(parser.source, name)}, children: []Node{body}}
}

func (parser *shellParser) parseSimpleCommand() Node {
	start, end := -1, -1
	var words, assignments []Word
	var redirections []Redirection
	incomplete := false
	for !parser.atEOF() {
		item := parser.current()
		if item.kind == tokenComment || item.kind == tokenNewline || item.kind == tokenOperator && !isRedirection(item.text) {
			break
		}
		if item.kind == tokenWord && allDigits(item.text) && parser.peekRedirectionAdjacent() {
			number, _ := strconv.Atoi(item.text)
			parser.index++
			redirection, ok := parser.parseRedirection(number, true, item.start)
			if ok {
				redirections = append(redirections, redirection)
				if start < 0 {
					start = item.start
				}
				end = redirection.span.End.Offset
			} else {
				incomplete = true
			}
			continue
		}
		if item.kind == tokenOperator && isRedirection(item.text) {
			redirection, ok := parser.parseRedirection(0, false, item.start)
			if ok {
				redirections = append(redirections, redirection)
				if start < 0 {
					start = item.start
				}
				end = redirection.span.End.Offset
			} else {
				incomplete = true
			}
			continue
		}
		if item.kind != tokenWord {
			break
		}
		word := wordFromToken(parser.source, item)
		if len(words) == len(assignments) && isAssignmentWord(item.text) {
			assignments = append(assignments, word)
		}
		words = append(words, word)
		if start < 0 {
			start = item.start
		}
		end = item.end
		parser.index++
	}
	if start < 0 {
		return Node{}
	}
	kind := NodeCommand
	if len(assignments) > 0 && len(assignments) == len(words) {
		kind = NodeAssignment
	}
	return Node{kind: kind, span: parser.source.span(start, end), words: words, assignments: assignments, redirections: redirections, incomplete: incomplete}
}

func (parser *shellParser) parseRedirection(number int, hasNumber bool, start int) (Redirection, bool) {
	operator := parser.current()
	parser.index++
	parser.skipComments()
	if parser.current().kind != tokenWord {
		parser.unexpected(operator, "redirection is missing its operand")
		return Redirection{ioNumber: number, hasIONumber: hasNumber, operator: operator.text, span: itemSpan(parser.source, operator)}, false
	}
	targetToken := parser.current()
	parser.index++
	redirection := Redirection{ioNumber: number, hasIONumber: hasNumber, operator: operator.text, target: wordFromToken(parser.source, targetToken), span: parser.source.span(start, targetToken.end)}
	if operator.text == "<<" || operator.text == "<<-" {
		delimiter, quoted := removeHereDocumentQuotes(parser.source.data[targetToken.start:targetToken.end])
		document := &HereDocument{delimiter: wordFromToken(parser.source, targetToken), quoted: quoted, stripTabs: operator.text == "<<-"}
		redirection.hereDocument = document
		parser.pendingHere = append(parser.pendingHere, &pendingHereDocument{document: document, delimiter: delimiter, delimiterSpan: itemSpan(parser.source, targetToken)})
	}
	return redirection, true
}

func (parser *shellParser) parseTrailingRedirections(node *Node) {
	for {
		start := parser.current().start
		number, hasNumber := 0, false
		if parser.current().kind == tokenWord && allDigits(parser.current().text) && parser.peekRedirectionAdjacent() {
			number, _ = strconv.Atoi(parser.current().text)
			hasNumber = true
			parser.index++
		} else if !parser.isOperator("<", ">", ">>", "<<", "<<-", "<<<", "<&", ">&", "<>", ">|", "&>", "&>>") {
			return
		}
		redirection, ok := parser.parseRedirection(number, hasNumber, start)
		if !ok {
			node.incomplete = true
			return
		}
		node.redirections = append(node.redirections, redirection)
		node.span.End = redirection.span.End
	}
}

func (parser *shellParser) consumeSeparator() {
	item := parser.current()
	parser.index++
	if item.kind == tokenNewline && len(parser.pendingHere) != 0 {
		parser.consumeHereDocuments(item.end)
	}
}

func (parser *shellParser) consumeHereDocuments(bodyStart int) {
	cursor := bodyStart
	for _, pending := range parser.pendingHere {
		found := false
		for cursor <= len(parser.source.data) {
			lineStart := cursor
			relativeEnd := bytes.IndexByte(parser.source.data[lineStart:], '\n')
			lineEnd := len(parser.source.data)
			if relativeEnd >= 0 {
				lineEnd = lineStart + relativeEnd
			}
			candidateStart := lineStart
			if pending.document.stripTabs {
				for candidateStart < lineEnd && parser.source.data[candidateStart] == '\t' {
					candidateStart++
				}
			}
			if bytes.Equal(parser.source.data[candidateStart:lineEnd], pending.delimiter) {
				pending.document.body = append([]byte(nil), parser.source.data[bodyStart:lineStart]...)
				pending.document.bodySpan = parser.source.span(bodyStart, lineStart)
				pending.document.terminatorSpan = parser.source.span(candidateStart, lineEnd)
				cursor = lineEnd
				if cursor < len(parser.source.data) {
					cursor++
				}
				found = true
				break
			}
			if lineEnd == len(parser.source.data) {
				cursor = lineEnd
				break
			}
			cursor = lineEnd + 1
		}
		if !found {
			pending.document.body = append([]byte(nil), parser.source.data[bodyStart:]...)
			pending.document.bodySpan = parser.source.span(bodyStart, len(parser.source.data))
			span := pending.delimiterSpan
			parser.diagnostics = append(parser.diagnostics, parser.source.diagnostic("SHS1006", SeverityError, ConfidenceDefinite, "here-document terminator is missing", span.Start.Offset, span.End.Offset))
			cursor = len(parser.source.data)
			break
		}
		bodyStart = cursor
	}
	parser.pendingHere = nil
	for parser.index < len(parser.tokens) && parser.tokens[parser.index].start < cursor {
		parser.index++
	}
}

func (parser *shellParser) matchesStop(stop parserStop) bool {
	item := parser.current()
	return item.kind == tokenWord && stop.words[item.text] || item.kind == tokenOperator && stop.ops[item.text]
}

func (parser *shellParser) current() token {
	if parser.index >= len(parser.tokens) {
		end := len(parser.source.data)
		return token{kind: tokenEOF, start: end, end: end}
	}
	return parser.tokens[parser.index]
}

func (parser *shellParser) atEOF() bool { return parser.current().kind == tokenEOF }

func (parser *shellParser) skipComments() {
	for parser.current().kind == tokenComment {
		parser.index++
	}
}

func (parser *shellParser) isWord(values ...string) bool {
	item := parser.current()
	if item.kind != tokenWord {
		return false
	}
	for _, value := range values {
		if item.text == value {
			return true
		}
	}
	return false
}

func (parser *shellParser) isOperator(values ...string) bool {
	item := parser.current()
	if item.kind != tokenOperator {
		return false
	}
	for _, value := range values {
		if item.text == value {
			return true
		}
	}
	return false
}

func (parser *shellParser) consumeWord(value string) bool {
	if !parser.isWord(value) {
		return false
	}
	parser.index++
	return true
}

func (parser *shellParser) consumeOperator(value string) bool {
	if !parser.isOperator(value) {
		return false
	}
	parser.index++
	return true
}

func (parser *shellParser) peekOperator(offset int, value string) bool {
	index := parser.index + offset
	return index < len(parser.tokens) && parser.tokens[index].kind == tokenOperator && parser.tokens[index].text == value
}

func (parser *shellParser) functionAhead() bool {
	return parser.current().kind == tokenWord && parser.peekOperator(1, "(") && parser.peekOperator(2, ")")
}

func (parser *shellParser) peekRedirectionAdjacent() bool {
	if parser.index+1 >= len(parser.tokens) {
		return false
	}
	current, next := parser.tokens[parser.index], parser.tokens[parser.index+1]
	return next.kind == tokenOperator && isRedirection(next.text) && current.end == next.start
}

func (parser *shellParser) unexpected(item token, message string) {
	parser.diagnostics = append(parser.diagnostics, parser.source.diagnostic("SHS1005", SeverityError, ConfidenceDefinite, message, item.start, item.end))
}

func (parser *shellParser) missingWord(opener token, word string) {
	parser.diagnostics = append(parser.diagnostics, parser.source.diagnostic("SHS1004", SeverityError, ConfidenceDefinite, "compound command is missing closing "+word, opener.start, parser.current().end))
}

func (parser *shellParser) sequenceEnd(nodes []Node, fallback int) int {
	if len(nodes) == 0 {
		return fallback
	}
	return nodes[len(nodes)-1].span.End.Offset
}

func roleChildren(nodes []Node, role NodeRole) []Node {
	result := append([]Node(nil), nodes...)
	for index := range result {
		result[index].role = role
	}
	return result
}

func wordFromToken(source *sourceFile, item token) Word {
	parts := append([]WordPart(nil), item.parts...)
	if len(parts) == 0 {
		parts = []WordPart{makePart(source, WordLiteral, QuoteUnquoted, item.start, item.end)}
	}
	return Word{parts: parts, span: source.span(item.start, item.end)}
}

func isCommandSeparator(text string) bool {
	switch text {
	case ";", "&", "&&", "||", "|", "|&", ";;", ";&", ";;&":
		return true
	default:
		return false
	}
}

func interpreterDirective(data []byte) string {
	if len(data) < 2 || data[0] != '#' || data[1] != '!' {
		return ""
	}
	end := strings.IndexByte(string(data), '\n')
	if end < 0 {
		end = len(data)
	}
	return strings.TrimSpace(string(data[2:end]))
}

func dialectFromInterpreter(interpreter string) Dialect {
	for _, field := range strings.Fields(interpreter) {
		switch path.Base(strings.ReplaceAll(field, "\\", "/")) {
		case "bash":
			return DialectBash
		case "sh":
			return DialectPOSIX
		}
	}
	return DialectPOSIX
}

func bashOnlyDiagnostics(source *sourceFile, tokens []token, dialect Dialect) []Diagnostic {
	if dialect != DialectPOSIX {
		return nil
	}
	var diagnostics []Diagnostic
	for index, item := range tokens {
		bashOnly := false
		if item.kind == tokenOperator {
			switch item.text {
			case "[[", "]]", "((", "))", "<<<", "|&", "&>", "&>>", ";;&", ";&":
				bashOnly = true
			}
		}
		if item.kind == tokenWord {
			text := item.text
			bashOnly = strings.HasPrefix(text, "<(") || strings.HasPrefix(text, ">(") || strings.Contains(text, "^^}") || strings.Contains(text, ",,}")
			if text == "function" || text == "select" || text == "coproc" {
				bashOnly = true
			}
			if text == "--" && index > 0 && tokens[index-1].kind == tokenWord && tokens[index-1].text == "time" {
				bashOnly = true
			}
		}
		if item.kind == tokenOperator && (item.text == "<" || item.text == ">") && index+1 < len(tokens) && tokens[index+1].kind == tokenOperator && tokens[index+1].text == "(" {
			bashOnly = true
		}
		if bashOnly {
			diagnostics = append(diagnostics, source.diagnostic("SHD1001", SeverityError, ConfidenceDefinite, "Bash-only syntax is not valid in POSIX shell", item.start, item.end))
		}
	}
	return diagnostics
}

func allDigits(value string) bool {
	if value == "" {
		return false
	}
	for index := range value {
		if value[index] < '0' || value[index] > '9' {
			return false
		}
	}
	return true
}
