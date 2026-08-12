package shellvalidate

import (
	"bytes"
	"strings"
)

var shellOperators = []string{
	"&>>", ";;&", "<<<", "<<-", "((", "))", "[[", "]]", "&&", "||", "|&",
	";;", ";&", "<<", ">>", "<&", ">&", "<>", ">|", "&>", "<", ">", "|", "&", ";", "(", ")", "{", "}",
}

type skipRange struct {
	start int
	end   int
}

func lex(source *sourceFile, dialect Dialect) ([]token, []Comment, []Diagnostic) {
	ranges, heredocDiagnostics := findHereDocuments(source)
	var tokens []token
	var comments []Comment
	diagnostics := append([]Diagnostic(nil), heredocDiagnostics...)
	for index, rangeIndex := 0, 0; index < len(source.data); {
		if rangeIndex < len(ranges) && index == ranges[rangeIndex].start {
			index = ranges[rangeIndex].end
			rangeIndex++
			continue
		}
		value := source.data[index]
		if value == 0 {
			diagnostics = append(diagnostics, source.diagnostic("SHS1003", SeverityError, ConfidenceDefinite, "source contains a NUL byte", index, index+1))
			index++
			continue
		}
		if value == ' ' || value == '\t' || value == '\r' {
			index++
			continue
		}
		if value == '\n' {
			tokens = append(tokens, token{kind: tokenNewline, start: index, end: index + 1, text: "\n"})
			index++
			continue
		}
		if value == '#' {
			end := index
			for end < len(source.data) && source.data[end] != '\n' {
				end++
			}
			text := append([]byte(nil), source.data[index:end]...)
			comments = append(comments, Comment{text: text, span: source.span(index, end)})
			tokens = append(tokens, token{kind: tokenComment, start: index, end: end, text: string(text)})
			index = end
			continue
		}
		if operator := operatorAt(source.data, index, dialect); operator != "" {
			end := index + len(operator)
			tokens = append(tokens, token{kind: tokenOperator, start: index, end: end, text: operator})
			index = end
			continue
		}
		item, next, found := scanWord(source, index, dialect)
		tokens = append(tokens, item)
		diagnostics = append(diagnostics, found...)
		if next <= index {
			next = index + 1
		}
		index = next
	}
	tokens = append(tokens, token{kind: tokenEOF, start: len(source.data), end: len(source.data)})
	sortDiagnostics(diagnostics)
	return tokens, comments, diagnostics
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
		if value == 0 || value == '\n' || value == ' ' || value == '\t' || value == '\r' || value == '#' {
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
			open, close := byte('('), byte(')')
			if source.data[index+1] == '{' {
				kind, open, close = WordParameterExpansion, '{', '}'
			} else if index+2 < len(source.data) && source.data[index+2] == '(' {
				kind = WordArithmeticExpansion
			}
			next, closed := scanBalanced(source.data, index+1, open, close)
			parts = append(parts, makePart(source, kind, QuoteUnquoted, index, next))
			if !closed {
				diagnostics = append(diagnostics, source.diagnostic("SHS1002", SeverityError, ConfidenceDefinite, "expansion delimiter is not terminated", index, next))
			}
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
			next, closed := scanBalanced(source.data, index+1, '(', ')')
			parts = append(parts, makePart(source, WordProcessSubstitution, QuoteUnquoted, index, next))
			if !closed {
				diagnostics = append(diagnostics, source.diagnostic("SHS1002", SeverityError, ConfidenceDefinite, "process substitution is not terminated", index, next))
			}
			index = next
		default:
			literalStart := index
			for index < len(source.data) {
				current := source.data[index]
				if current == 0 {
					break
				}
				if bytes.ContainsRune([]byte(" \\t\r\n\\'\"$`#<>|&;(){}"), rune(current)) || operatorAt(source.data, index, dialect) != "" {
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
			open, close := byte('('), byte(')')
			if source.data[index+1] == '{' {
				kind, open, close = WordParameterExpansion, '{', '}'
			} else if index+2 < len(source.data) && source.data[index+2] == '(' {
				kind = WordArithmeticExpansion
			}
			next, _ := scanBalanced(source.data, index+1, open, close)
			part := makePart(source, kind, quote, index, next)
			parts = append(parts, part)
			index, contentStart = next, next
			continue
		}
		if source.data[index] == '\\' && index+1 < len(source.data) {
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

func scanBalanced(data []byte, openOffset int, open, close byte) (int, bool) {
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
		if value == open {
			depth++
		} else if value == close {
			depth--
			if depth == 0 {
				return index + 1, true
			}
		}
	}
	return len(data), false
}

func makePart(source *sourceFile, kind WordPartKind, quote QuoteKind, start, end int) WordPart {
	return WordPart{kind: kind, quote: quote, text: append([]byte(nil), source.data[start:end]...), span: source.span(start, end)}
}

func findHereDocuments(source *sourceFile) ([]skipRange, []Diagnostic) {
	var ranges []skipRange
	var diagnostics []Diagnostic
	for lineStart := 0; lineStart < len(source.data); {
		lineEnd := bytes.IndexByte(source.data[lineStart:], '\n')
		if lineEnd < 0 {
			lineEnd = len(source.data)
		} else {
			lineEnd += lineStart
		}
		line := source.data[lineStart:lineEnd]
		delimiters := heredocDelimiters(line)
		if len(delimiters) == 0 {
			if lineEnd == len(source.data) {
				break
			}
			lineStart = lineEnd + 1
			continue
		}
		bodyStart := lineEnd
		if bodyStart < len(source.data) {
			bodyStart++
		}
		cursor := bodyStart
		for _, delimiter := range delimiters {
			found := false
			for cursor <= len(source.data) {
				end := bytes.IndexByte(source.data[cursor:], '\n')
				if end < 0 {
					end = len(source.data)
				} else {
					end += cursor
				}
				candidate := source.data[cursor:end]
				if delimiter.stripTabs {
					candidate = bytes.TrimLeft(candidate, "\t")
				}
				if string(candidate) == delimiter.text {
					next := end
					if next < len(source.data) {
						next++
					}
					ranges = append(ranges, skipRange{start: bodyStart, end: next})
					cursor, bodyStart, found = next, next, true
					break
				}
				if end == len(source.data) {
					break
				}
				cursor = end + 1
			}
			if !found {
				diagnostics = append(diagnostics, source.diagnostic("SHS1006", SeverityError, ConfidenceDefinite, "here-document terminator is missing", lineStart+delimiter.offset, lineStart+delimiter.offset+len(delimiter.text)))
				break
			}
		}
		if cursor > lineEnd+1 {
			lineStart = cursor
		} else if lineEnd == len(source.data) {
			break
		} else {
			lineStart = lineEnd + 1
		}
	}
	return ranges, diagnostics
}

type heredocDelimiter struct {
	text      string
	stripTabs bool
	offset    int
}

func heredocDelimiters(line []byte) []heredocDelimiter {
	var result []heredocDelimiter
	for index := 0; index+1 < len(line); index++ {
		if line[index] != '<' || line[index+1] != '<' || (index > 0 && line[index-1] == '<') || (index+2 < len(line) && line[index+2] == '<') {
			continue
		}
		stripTabs := index+2 < len(line) && line[index+2] == '-'
		cursor := index + 2
		if stripTabs {
			cursor++
		}
		for cursor < len(line) && (line[cursor] == ' ' || line[cursor] == '\t') {
			cursor++
		}
		start := cursor
		var delimiter strings.Builder
		quote := byte(0)
		for cursor < len(line) {
			value := line[cursor]
			if quote != 0 {
				if value == quote {
					quote = 0
				} else {
					delimiter.WriteByte(value)
				}
				cursor++
				continue
			}
			if value == '\'' || value == '"' {
				quote = value
				cursor++
				continue
			}
			if value == ' ' || value == '\t' || value == ';' || value == '|' || value == '&' {
				break
			}
			delimiter.WriteByte(value)
			cursor++
		}
		if delimiter.Len() > 0 {
			result = append(result, heredocDelimiter{text: delimiter.String(), stripTabs: stripTabs, offset: start})
		}
		index = cursor
	}
	return result
}
