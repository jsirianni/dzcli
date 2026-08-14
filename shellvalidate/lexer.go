package shellvalidate

import "bytes"

var shellOperators = []string{
	"&>>", ";;&", "<<<", "<<-", "((", "))", "[[", "]]", "&&", "||", "|&",
	";;", ";&", "<<", ">>", "<&", ">&", "<>", ">|", "&>", "<", ">", "|", "&", ";", "(", ")", "{", "}",
}

type lexicalMode uint8

const (
	modeShell lexicalMode = iota
	modeArithmetic
	modeConditional
)

type shellLexer struct {
	source      *sourceFile
	dialect     Dialect
	index       int
	tokens      []token
	comments    []Comment
	diagnostics []Diagnostic
	modes       []lexicalMode
}

func lex(source *sourceFile, dialect Dialect) ([]token, []Comment, []Diagnostic) {
	return lexSkippingHereDocuments(source, dialect, nil)
}

func lexSkippingHereDocuments(source *sourceFile, dialect Dialect, skipped []Span) ([]token, []Comment, []Diagnostic) {
	lexer := &shellLexer{source: source, dialect: dialect, modes: []lexicalMode{modeShell}}
	// Count each NUL exactly once before lexical modes or here-document skipping
	// can hide it. The parser still preserves every other byte verbatim.
	for index, value := range source.data {
		if value == 0 {
			lexer.diagnostics = append(lexer.diagnostics, source.diagnostic("SHS1003", SeverityError, ConfidenceDefinite, "source contains a NUL byte", index, index+1))
		}
	}
	skipIndex := 0
	for lexer.index < len(source.data) {
		if skipIndex < len(skipped) && lexer.index >= skipped[skipIndex].Start.Offset {
			if lexer.index < skipped[skipIndex].End.Offset {
				lexer.index = skipped[skipIndex].End.Offset
			}
			skipIndex++
			continue
		}
		before := lexer.index
		lexer.scan()
		if lexer.index <= before {
			lexer.index = before + 1
		}
	}
	lexer.tokens = append(lexer.tokens, token{kind: tokenEOF, start: len(source.data), end: len(source.data)})
	sortDiagnostics(lexer.diagnostics)
	return lexer.tokens, lexer.comments, lexer.diagnostics
}

func (lexer *shellLexer) scan() {
	data := lexer.source.data
	value := data[lexer.index]
	if value == 0 {
		lexer.index++
		return
	}
	if value == ' ' || value == '\t' || value == '\r' {
		lexer.index++
		return
	}
	if value == '\n' {
		start := lexer.index
		lexer.index++
		lexer.tokens = append(lexer.tokens, token{kind: tokenNewline, start: start, end: lexer.index, text: "\n"})
		return
	}
	if value == '#' {
		end := lexer.index
		for end < len(data) && data[end] != '\n' {
			end++
		}
		text := append([]byte(nil), data[lexer.index:end]...)
		lexer.comments = append(lexer.comments, Comment{text: text, span: lexer.source.span(lexer.index, end)})
		lexer.tokens = append(lexer.tokens, token{kind: tokenComment, start: lexer.index, end: end, text: string(text)})
		lexer.index = end
		return
	}
	if operator := operatorAt(data, lexer.index, lexer.dialect); operator != "" {
		lexer.scanOperator(operator)
		return
	}
	item, next, diagnostics := scanWord(lexer.source, lexer.index, lexer.dialect)
	lexer.tokens = append(lexer.tokens, item)
	lexer.diagnostics = append(lexer.diagnostics, diagnostics...)
	lexer.index = next
}

func (lexer *shellLexer) scanOperator(operator string) {
	start := lexer.index
	lexer.index += len(operator)
	item := token{kind: tokenOperator, start: start, end: lexer.index, text: operator}
	mode := lexer.mode()
	switch {
	case mode == modeShell && operator == "((":
		lexer.pushMode(modeArithmetic)
	case mode == modeShell && operator == "[[":
		lexer.pushMode(modeConditional)
	case mode == modeArithmetic && operator == "))":
		lexer.popMode()
	case mode == modeConditional && operator == "]]":
		lexer.popMode()
	}
	lexer.tokens = append(lexer.tokens, item)
}

func (lexer *shellLexer) mode() lexicalMode {
	if len(lexer.modes) == 0 {
		return modeShell
	}
	return lexer.modes[len(lexer.modes)-1]
}

func (lexer *shellLexer) pushMode(mode lexicalMode) {
	if len(lexer.modes) < maxNesting {
		lexer.modes = append(lexer.modes, mode)
	}
}

func (lexer *shellLexer) popMode() {
	if len(lexer.modes) > 1 {
		lexer.modes = lexer.modes[:len(lexer.modes)-1]
	}
}

func operatorAt(data []byte, index int, dialect Dialect) string {
	if index+1 < len(data) && (data[index] == '<' || data[index] == '>') && data[index+1] == '(' && dialect == DialectBash {
		return ""
	}
	for _, operator := range shellOperators {
		if len(data)-index >= len(operator) && string(data[index:index+len(operator)]) == operator {
			return operator
		}
	}
	return ""
}

func scanWord(source *sourceFile, start int, dialect Dialect) (token, int, []Diagnostic) {
	index := start
	var parts []WordPart
	var diagnostics []Diagnostic
	quoted := false
	for index < len(source.data) {
		value := source.data[index]
		if value == 0 || value == '\n' || value == ' ' || value == '\t' || value == '\r' {
			break
		}
		if operatorAt(source.data, index, dialect) != "" {
			break
		}
		switch {
		case value == '\\':
			partStart := index
			if index+1 < len(source.data) && source.data[index+1] == '\n' {
				index += 2
				continue
			}
			quoted = true
			index++
			if index < len(source.data) {
				index++
			}
			parts = append(parts, makePart(source, WordLiteral, QuoteUnquoted, partStart, index))
		case value == '\'' || value == '"':
			quote := QuoteSingle
			if value == '"' {
				quote = QuoteDouble
			}
			quoted = true
			next, nestedParts, closed := scanQuoted(source, index, value, quote)
			parts = append(parts, nestedParts...)
			if !closed {
				diagnostics = append(diagnostics, source.diagnostic("SHS1001", SeverityError, ConfidenceDefinite, "quoted text is not terminated", index, len(source.data)))
			}
			index = next
		case value == '$' && index+1 < len(source.data) && (source.data[index+1] == '\'' || source.data[index+1] == '"'):
			quoteByte := source.data[index+1]
			quote := QuoteANSIC
			if quoteByte == '"' {
				quote = QuoteLocale
			}
			quoted = true
			next, nestedParts, closed := scanQuoted(source, index+1, quoteByte, quote)
			if len(nestedParts) > 0 {
				nestedParts[0].span.Start = source.position(index)
				nestedParts[0].text = append([]byte(nil), source.data[index:next]...)
			}
			parts = append(parts, nestedParts...)
			if !closed {
				diagnostics = append(diagnostics, source.diagnostic("SHS1001", SeverityError, ConfidenceDefinite, "quoted text is not terminated", index, len(source.data)))
			}
			index = next
		case value == '$' && index+1 < len(source.data) && (source.data[index+1] == '(' || source.data[index+1] == '{'):
			kind := WordCommandSubstitution
			var next int
			var closed bool
			if source.data[index+1] == '{' {
				kind = WordParameterExpansion
				next, closed = scanParameterExpansion(source.data, index)
			} else if index+2 < len(source.data) && source.data[index+2] == '(' {
				kind = WordArithmeticExpansion
				next, closed = scanParenthesizedExpansion(source.data, index+1)
			} else {
				next, closed = scanParenthesizedExpansion(source.data, index+1)
			}
			parts = append(parts, makePart(source, kind, QuoteUnquoted, index, next))
			if !closed {
				diagnostics = append(diagnostics, source.diagnostic("SHS1002", SeverityError, ConfidenceDefinite, "expansion delimiter is not terminated", index, next))
			}
			index = next
		case value == '$' && index+1 < len(source.data) && parameterByte(source.data[index+1]):
			next := scanUnbracedParameter(source.data, index)
			parts = append(parts, makePart(source, WordParameterExpansion, QuoteUnquoted, index, next))
			index = next
		case value == '`':
			next := index + 1
			for next < len(source.data) && source.data[next] != '`' {
				if source.data[next] == '\\' && next+1 < len(source.data) {
					next += 2
				} else {
					next++
				}
			}
			closed := next < len(source.data)
			if closed {
				next++
			}
			parts = append(parts, makePart(source, WordCommandSubstitution, QuoteUnquoted, index, next))
			if !closed {
				diagnostics = append(diagnostics, source.diagnostic("SHS1002", SeverityError, ConfidenceDefinite, "command substitution is not terminated", index, next))
			}
			index = next
		case dialect == DialectBash && (value == '<' || value == '>') && index+1 < len(source.data) && source.data[index+1] == '(':
			next, closed := scanParenthesizedExpansion(source.data, index+1)
			parts = append(parts, makePart(source, WordProcessSubstitution, QuoteUnquoted, index, next))
			if !closed {
				diagnostics = append(diagnostics, source.diagnostic("SHS1002", SeverityError, ConfidenceDefinite, "process substitution is not terminated", index, next))
			}
			index = next
		default:
			literalStart := index
			for index < len(source.data) {
				current := source.data[index]
				if current == 0 || current == ' ' || current == '\t' || current == '\r' || current == '\n' || current == '\\' || current == '\'' || current == '"' || current == '$' || current == '`' || operatorAt(source.data, index, dialect) != "" {
					break
				}
				index++
			}
			if index == literalStart {
				index++
			}
			parts = append(parts, makePart(source, WordLiteral, QuoteUnquoted, literalStart, index))
		}
	}
	return token{kind: tokenWord, start: start, end: index, text: string(source.data[start:index]), parts: parts, quoted: quoted}, index, diagnostics
}

func scanQuoted(source *sourceFile, quoteOffset int, delimiter byte, quote QuoteKind) (int, []WordPart, bool) {
	index := quoteOffset + 1
	contentStart := index
	var parts []WordPart
	for index < len(source.data) {
		if source.data[index] == delimiter {
			if contentStart < index {
				parts = append(parts, makePart(source, WordLiteral, quote, contentStart, index))
			} else if len(parts) == 0 {
				parts = append(parts, makePart(source, WordLiteral, quote, index, index))
			}
			return index + 1, parts, true
		}
		if delimiter == '"' && source.data[index] == '$' && index+1 < len(source.data) && (source.data[index+1] == '(' || source.data[index+1] == '{') {
			if contentStart < index {
				parts = append(parts, makePart(source, WordLiteral, quote, contentStart, index))
			}
			kind := WordCommandSubstitution
			var next int
			if source.data[index+1] == '{' {
				kind = WordParameterExpansion
				next, _ = scanParameterExpansion(source.data, index)
			} else if index+2 < len(source.data) && source.data[index+2] == '(' {
				kind = WordArithmeticExpansion
				next, _ = scanParenthesizedExpansion(source.data, index+1)
			} else {
				next, _ = scanParenthesizedExpansion(source.data, index+1)
			}
			parts = append(parts, makePart(source, kind, quote, index, next))
			index, contentStart = next, next
			continue
		}
		if delimiter == '"' && source.data[index] == '$' && index+1 < len(source.data) && parameterByte(source.data[index+1]) {
			if contentStart < index {
				parts = append(parts, makePart(source, WordLiteral, quote, contentStart, index))
			}
			next := scanUnbracedParameter(source.data, index)
			parts = append(parts, makePart(source, WordParameterExpansion, quote, index, next))
			index, contentStart = next, next
			continue
		}
		if delimiter == '"' && source.data[index] == '\\' && index+1 < len(source.data) {
			index += 2
		} else {
			index++
		}
	}
	if contentStart < index {
		parts = append(parts, makePart(source, WordLiteral, quote, contentStart, index))
	}
	return index, parts, false
}

func scanParenthesizedExpansion(data []byte, openOffset int) (int, bool) {
	depth := 0
	quote := byte(0)
	for index := openOffset; index < len(data); index++ {
		value := data[index]
		if quote != 0 {
			if value == '\\' && quote == '"' && index+1 < len(data) {
				index++
				continue
			}
			if value == quote {
				quote = 0
			}
			continue
		}
		if value == '\'' || value == '"' {
			quote = value
			continue
		}
		if value == '\\' && index+1 < len(data) {
			index++
			continue
		}
		if value == '(' {
			depth++
		} else if value == ')' {
			depth--
			if depth == 0 {
				return index + 1, true
			}
		}
	}
	return len(data), false
}

func scanParameterExpansion(data []byte, dollarOffset int) (int, bool) {
	depth := 0
	quote := byte(0)
	for index := dollarOffset + 1; index < len(data); index++ {
		value := data[index]
		if quote != 0 {
			if value == '\\' && quote == '"' && index+1 < len(data) {
				index++
				continue
			}
			if value == quote {
				quote = 0
			}
			continue
		}
		if value == '\'' || value == '"' {
			quote = value
			continue
		}
		if value == '\\' && index+1 < len(data) {
			index++
			continue
		}
		if value == '{' {
			depth++
		} else if value == '}' {
			depth--
			if depth == 0 {
				return index + 1, true
			}
		}
	}
	return len(data), false
}

func parameterByte(value byte) bool {
	return isNameStart(value) || value >= '0' && value <= '9' || bytes.ContainsRune([]byte("@*#?$!-"), rune(value))
}

func scanUnbracedParameter(data []byte, dollarOffset int) int {
	index := dollarOffset + 1
	if index >= len(data) {
		return index
	}
	if !isNameStart(data[index]) {
		return index + 1
	}
	index++
	for index < len(data) && isNameByte(data[index]) {
		index++
	}
	return index
}

func makePart(source *sourceFile, kind WordPartKind, quote QuoteKind, start, end int) WordPart {
	return WordPart{kind: kind, quote: quote, text: append([]byte(nil), source.data[start:end]...), span: source.span(start, end)}
}

func removeHereDocumentQuotes(data []byte) ([]byte, bool) {
	result := make([]byte, 0, len(data))
	quote := byte(0)
	quoted := false
	for index := 0; index < len(data); index++ {
		value := data[index]
		if quote != 0 {
			if value == quote {
				quote = 0
				quoted = true
				continue
			}
			if value == '\\' && quote == '"' && index+1 < len(data) {
				quoted = true
				index++
				result = append(result, data[index])
				continue
			}
			result = append(result, value)
			continue
		}
		if value == '\'' || value == '"' {
			quote = value
			quoted = true
			continue
		}
		if value == '\\' && index+1 < len(data) {
			quoted = true
			index++
			result = append(result, data[index])
			continue
		}
		result = append(result, value)
	}
	return result, quoted
}

func itemSpan(source *sourceFile, item token) Span { return source.span(item.start, item.end) }
