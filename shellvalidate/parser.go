package shellvalidate

import (
	"path"
	"strings"
)

type parseFrame struct {
	expected string
	kind     NodeKind
	start    int
}

func parseTokens(source *sourceFile, tokens []token, dialect Dialect) ([]Node, []Diagnostic) {
	var nodes []Node
	var diagnostics []Diagnostic
	var frames []parseFrame
	var words []Word
	commandStart, commandEnd := -1, -1
	atCommandStart := true
	flushCommand := func(incomplete bool) {
		if commandStart < 0 {
			return
		}
		nodes = append(nodes, Node{kind: classifyCommand(words), span: source.span(commandStart, commandEnd), words: append([]Word(nil), words...), incomplete: incomplete})
		words = nil
		commandStart, commandEnd = -1, -1
	}

	for _, item := range tokens {
		if item.kind == tokenEOF {
			break
		}
		if item.kind == tokenComment {
			continue
		}
		if item.kind == tokenNewline || (item.kind == tokenOperator && isCommandSeparator(item.text)) {
			flushCommand(false)
			atCommandStart = true
			continue
		}
		if item.kind == tokenOperator {
			switch item.text {
			case "(", "{", "((", "[[":
				expected := map[string]string{"(": ")", "{": "}", "((": "))", "[[": "]]"}[item.text]
				kind := map[string]NodeKind{"(": NodeSubshell, "{": NodeBraceGroup, "((": NodeArithmetic, "[[": NodeConditional}[item.text]
				if len(frames) >= maxNesting {
					diagnostics = append(diagnostics, source.diagnostic("SHS1005", SeverityError, ConfidenceDefinite, "shell syntax exceeds the nesting safety limit", item.start, item.end))
				} else {
					frames = append(frames, parseFrame{expected: expected, kind: kind, start: item.start})
				}
				atCommandStart = true
				continue
			case ")", "}", "))", "]]":
				if item.text == ")" && len(frames) > 0 && frames[len(frames)-1].expected == "esac" {
					flushCommand(false)
					atCommandStart = true
					continue
				}
				if len(frames) == 0 || frames[len(frames)-1].expected != item.text {
					diagnostics = append(diagnostics, source.diagnostic("SHS1005", SeverityError, ConfidenceDefinite, "unexpected closing token "+item.text, item.start, item.end))
					continue
				}
				flushCommand(false)
				frame := frames[len(frames)-1]
				frames = frames[:len(frames)-1]
				node := Node{kind: frame.kind, span: source.span(frame.start, item.end)}
				if frame.kind == NodeArithmetic {
					node.expressions = parseArithmeticExpressions(source, frame.start+2, item.start)
				} else if frame.kind == NodeConditional {
					node.expressions = parseConditionalExpressions(source, frame.start+2, item.start)
				}
				nodes = append(nodes, node)
				atCommandStart = false
				continue
			}
			if commandStart < 0 {
				commandStart = item.start
			}
			commandEnd = item.end
			continue
		}
		if item.kind != tokenWord {
			continue
		}
		text := item.text
		if atCommandStart {
			if kind, expected, ok := compoundOpen(text); ok {
				if len(frames) >= maxNesting {
					diagnostics = append(diagnostics, source.diagnostic("SHS1005", SeverityError, ConfidenceDefinite, "shell syntax exceeds the nesting safety limit", item.start, item.end))
				} else {
					frames = append(frames, parseFrame{expected: expected, kind: kind, start: item.start})
				}
			}
		}
		if isCompoundClose(text) {
			if len(frames) == 0 || frames[len(frames)-1].expected != text {
				diagnostics = append(diagnostics, source.diagnostic("SHS1005", SeverityError, ConfidenceDefinite, "unexpected reserved word "+text, item.start, item.end))
			} else {
				flushCommand(false)
				frame := frames[len(frames)-1]
				frames = frames[:len(frames)-1]
				nodes = append(nodes, Node{kind: frame.kind, span: source.span(frame.start, item.end)})
			}
			atCommandStart = false
			continue
		}
		if commandStart < 0 {
			commandStart = item.start
		}
		commandEnd = item.end
		words = append(words, wordFromToken(source, item))
		atCommandStart = reservedStartsCommand(text)
	}
	flushCommand(len(frames) != 0)
	for index := len(frames) - 1; index >= 0; index-- {
		frame := frames[index]
		diagnostics = append(diagnostics, source.diagnostic("SHS1004", SeverityError, ConfidenceDefinite, "compound command is missing closing "+frame.expected, frame.start, len(source.data)))
		nodes = append(nodes, Node{kind: frame.kind, span: source.span(frame.start, len(source.data)), incomplete: true})
	}
	sortDiagnostics(diagnostics)
	return nodes, diagnostics
}

func wordFromToken(source *sourceFile, item token) Word {
	parts := append([]WordPart(nil), item.parts...)
	if len(parts) == 0 {
		parts = []WordPart{makePart(source, WordLiteral, QuoteUnquoted, item.start, item.end)}
	}
	return Word{parts: parts, span: source.span(item.start, item.end)}
}

func classifyCommand(words []Word) NodeKind {
	if len(words) == 0 {
		return NodeCommand
	}
	first := string(words[0].parts[0].text)
	switch first {
	case "if":
		return NodeIf
	case "for", "select":
		return NodeFor
	case "while":
		return NodeWhile
	case "until":
		return NodeUntil
	case "case":
		return NodeCase
	case "coproc":
		return NodeCoprocess
	}
	if strings.Contains(first, "=") {
		return NodeAssignment
	}
	return NodeCommand
}

func compoundOpen(text string) (NodeKind, string, bool) {
	switch text {
	case "if":
		return NodeIf, "fi", true
	case "for", "select":
		return NodeFor, "done", true
	case "while":
		return NodeWhile, "done", true
	case "until":
		return NodeUntil, "done", true
	case "case":
		return NodeCase, "esac", true
	}
	return "", "", false
}

func isCompoundClose(text string) bool { return text == "fi" || text == "done" || text == "esac" }

func reservedStartsCommand(text string) bool {
	switch text {
	case "then", "do", "else", "elif", "in", "!", "time":
		return true
	default:
		return false
	}
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
	for _, item := range tokens {
		bashOnly := false
		if item.kind == tokenOperator {
			switch item.text {
			case "[[", "]]", "((", "))", "<<<", "|&", "&>", "&>>", ";;&":
				bashOnly = true
			}
		}
		if item.kind == tokenWord {
			text := item.text
			bashOnly = strings.HasPrefix(text, "<(") || strings.HasPrefix(text, ">(") || strings.Contains(text, "^^}") || strings.Contains(text, ",,}")
			if text == "function" || text == "select" || text == "coproc" {
				bashOnly = true
			}
		}
		if bashOnly {
			diagnostics = append(diagnostics, source.diagnostic("SHD1001", SeverityError, ConfidenceDefinite, "Bash-only syntax is not valid in POSIX shell", item.start, item.end))
		}
	}
	return diagnostics
}
