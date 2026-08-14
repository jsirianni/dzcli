package shellvalidate

import "strings"

type expressionToken struct {
	text  string
	start int
	end   int
}

type binaryOperator struct {
	precedence int
	right      bool
	kind       ExpressionKind
}

// Arithmetic and conditional operators intentionally have separate grammars.
// In particular, =~ and the test primaries are never arithmetic operators.
var arithmeticPrecedence = map[string]int{
	",": 1,
	"=": 2, "*=": 2, "/=": 2, "%=": 2, "+=": 2, "-=": 2,
	"<<=": 2, ">>=": 2, "&=": 2, "^=": 2, "|=": 2,
	"?": 3, "||": 4, "&&": 5, "|": 6, "^": 7, "&": 8,
	"==": 9, "!=": 9, "<": 10, "<=": 10, ">": 10, ">=": 10,
	"<<": 11, ">>": 11, "+": 12, "-": 12, "*": 13, "/": 13, "%": 13, "**": 14,
}

var arithmeticBinary = func() map[string]binaryOperator {
	result := make(map[string]binaryOperator, len(arithmeticPrecedence))
	for operator, precedence := range arithmeticPrecedence {
		kind := ExpressionBinary
		right := operator == "**" || precedence == 2
		if precedence == 2 {
			kind = ExpressionAssignment
		}
		result[operator] = binaryOperator{precedence: precedence, right: right, kind: kind}
	}
	return result
}()

type arithmeticParser struct {
	source      *sourceFile
	tokens      []expressionToken
	index       int
	diagnostics []Diagnostic
}

func parseArithmeticExpressions(source *sourceFile, start, end int) []Expression {
	expressions, _ := parseArithmeticExpressionSet(source, start, end)
	return expressions
}

func parseArithmeticExpressionSet(source *sourceFile, start, end int) ([]Expression, []Diagnostic) {
	parser := arithmeticParser{source: source, tokens: lexArithmeticExpression(source.data, start, end)}
	if len(parser.tokens) == 0 {
		return nil, nil
	}
	var expressions []Expression
	for parser.index < len(parser.tokens) {
		expressions = append(expressions, parser.parse(1))
		if parser.peek(";") {
			parser.index++
			continue
		}
		if parser.index >= len(parser.tokens) {
			break
		}
		item := parser.tokens[parser.index]
		parser.diagnostics = append(parser.diagnostics, source.diagnostic("SHS1005", SeverityError, ConfidenceDefinite, "operator is not valid in shell arithmetic", item.start, item.end))
		parser.index++
	}
	return expressions, parser.diagnostics
}

func (parser *arithmeticParser) parse(minimum int) Expression {
	left := parser.prefix()
	for parser.index < len(parser.tokens) {
		item := parser.tokens[parser.index]
		if item.text == "++" || item.text == "--" {
			parser.index++
			left = Expression{kind: ExpressionUnary, operator: "post" + item.text, span: parser.source.span(left.span.Start.Offset, item.end), children: []Expression{left}}
			continue
		}
		if item.text == "?" {
			info := arithmeticBinary["?"]
			if info.precedence < minimum {
				break
			}
			parser.index++
			middle := parser.parse(1)
			if !parser.peek(":") {
				parser.diagnostics = append(parser.diagnostics, parser.source.diagnostic("SHS1005", SeverityError, ConfidenceDefinite, "conditional arithmetic expression is missing :", item.start, item.end))
				return left
			}
			parser.index++
			right := parser.parse(info.precedence)
			left = Expression{kind: ExpressionConditional, operator: "?:", span: parser.source.span(left.span.Start.Offset, right.span.End.Offset), children: []Expression{left, middle, right}}
			continue
		}
		info, ok := arithmeticBinary[item.text]
		if !ok || info.precedence < minimum || item.text == "?" {
			break
		}
		parser.index++
		nextMinimum := info.precedence + 1
		if info.right {
			nextMinimum = info.precedence
		}
		right := parser.parse(nextMinimum)
		end := item.end
		if right.span.End.Offset > end {
			end = right.span.End.Offset
		}
		left = Expression{kind: info.kind, operator: item.text, span: parser.source.span(left.span.Start.Offset, end), children: []Expression{left, right}}
	}
	return left
}

func (parser *arithmeticParser) prefix() Expression {
	if parser.index >= len(parser.tokens) {
		return Expression{span: parser.source.span(len(parser.source.data), len(parser.source.data))}
	}
	item := parser.tokens[parser.index]
	parser.index++
	if isArithmeticUnary(item.text) {
		operand := parser.parse(15)
		return Expression{kind: ExpressionUnary, operator: item.text, span: parser.source.span(item.start, operand.span.End.Offset), children: []Expression{operand}}
	}
	if item.text == "(" {
		expression := parser.parse(1)
		if parser.peek(")") {
			end := parser.tokens[parser.index].end
			parser.index++
			expression.span = parser.source.span(item.start, end)
		} else {
			parser.diagnostics = append(parser.diagnostics, parser.source.diagnostic("SHS1005", SeverityError, ConfidenceDefinite, "arithmetic grouping is missing )", item.start, item.end))
		}
		return expression
	}
	if _, invalid := arithmeticBinary[item.text]; invalid || item.text == ")" || item.text == ":" || item.text == "=~" {
		parser.diagnostics = append(parser.diagnostics, parser.source.diagnostic("SHS1005", SeverityError, ConfidenceDefinite, "arithmetic operator is missing an operand", item.start, item.end))
		return Expression{kind: ExpressionLiteral, span: parser.source.span(item.start, item.end)}
	}
	kind := ExpressionName
	if isNumber(item.text) || isQuotedExpression(item.text) {
		kind = ExpressionLiteral
	}
	return Expression{kind: kind, value: item.text, span: parser.source.span(item.start, item.end)}
}

func (parser *arithmeticParser) peek(text string) bool {
	return parser.index < len(parser.tokens) && parser.tokens[parser.index].text == text
}

type conditionalParser struct {
	source      *sourceFile
	tokens      []expressionToken
	index       int
	diagnostics []Diagnostic
}

func parseConditionalExpressions(source *sourceFile, start, end int) []Expression {
	expressions, _ := parseConditionalExpressionSet(source, start, end)
	return expressions
}

func parseConditionalExpressionSet(source *sourceFile, start, end int) ([]Expression, []Diagnostic) {
	parser := conditionalParser{source: source, tokens: lexConditionalExpression(source.data, start, end)}
	if len(parser.tokens) == 0 {
		return nil, []Diagnostic{source.diagnostic("SHS1005", SeverityError, ConfidenceDefinite, "conditional command requires an expression", start, end)}
	}
	root := parser.logicalOr()
	for parser.index < len(parser.tokens) {
		item := parser.tokens[parser.index]
		parser.diagnostics = append(parser.diagnostics, source.diagnostic("SHS1005", SeverityError, ConfidenceDefinite, "token is not valid in a conditional expression", item.start, item.end))
		parser.index++
	}
	return []Expression{root}, parser.diagnostics
}

func (parser *conditionalParser) logicalOr() Expression {
	left := parser.logicalAnd()
	for parser.peek("||") {
		parser.index++
		right := parser.logicalAnd()
		left = Expression{kind: ExpressionBinary, operator: "||", span: parser.source.span(left.span.Start.Offset, right.span.End.Offset), children: []Expression{left, right}}
	}
	return left
}

func (parser *conditionalParser) logicalAnd() Expression {
	left := parser.primary()
	for parser.peek("&&") {
		parser.index++
		right := parser.primary()
		left = Expression{kind: ExpressionBinary, operator: "&&", span: parser.source.span(left.span.Start.Offset, right.span.End.Offset), children: []Expression{left, right}}
	}
	return left
}

func (parser *conditionalParser) primary() Expression {
	if parser.index >= len(parser.tokens) {
		return Expression{span: parser.source.span(len(parser.source.data), len(parser.source.data))}
	}
	item := parser.tokens[parser.index]
	parser.index++
	if item.text == "!" {
		operand := parser.primary()
		return Expression{kind: ExpressionUnary, operator: "!", span: parser.source.span(item.start, operand.span.End.Offset), children: []Expression{operand}}
	}
	if item.text == "(" {
		expression := parser.logicalOr()
		if parser.peek(")") {
			end := parser.tokens[parser.index].end
			parser.index++
			expression.span = parser.source.span(item.start, end)
		} else {
			parser.diagnostics = append(parser.diagnostics, parser.source.diagnostic("SHS1005", SeverityError, ConfidenceDefinite, "conditional grouping is missing )", item.start, item.end))
		}
		return expression
	}
	if isConditionalUnary(item.text) {
		operand := parser.conditionalOperand()
		return Expression{kind: ExpressionUnary, operator: item.text, span: parser.source.span(item.start, operand.span.End.Offset), children: []Expression{operand}}
	}
	left := expressionAtom(parser.source, item)
	if parser.index < len(parser.tokens) && isConditionalBinary(parser.tokens[parser.index].text) {
		operator := parser.tokens[parser.index]
		parser.index++
		right := parser.conditionalOperand()
		return Expression{kind: ExpressionBinary, operator: operator.text, span: parser.source.span(left.span.Start.Offset, right.span.End.Offset), children: []Expression{left, right}}
	}
	return left
}

func (parser *conditionalParser) conditionalOperand() Expression {
	if parser.index >= len(parser.tokens) {
		end := len(parser.source.data)
		parser.diagnostics = append(parser.diagnostics, parser.source.diagnostic("SHS1005", SeverityError, ConfidenceDefinite, "conditional operator is missing an operand", end, end))
		return Expression{span: parser.source.span(end, end)}
	}
	item := parser.tokens[parser.index]
	parser.index++
	return expressionAtom(parser.source, item)
}

func (parser *conditionalParser) peek(text string) bool {
	return parser.index < len(parser.tokens) && parser.tokens[parser.index].text == text
}

func expressionAtom(source *sourceFile, item expressionToken) Expression {
	kind := ExpressionName
	if isNumber(item.text) || isQuotedExpression(item.text) {
		kind = ExpressionLiteral
	}
	return Expression{kind: kind, value: item.text, span: source.span(item.start, item.end)}
}

var arithmeticOperatorSpellings = []string{
	">>=", "<<=", "++", "--", "**", "<<", ">>", "<=", ">=", "==", "!=", "&&", "||",
	"*=", "/=", "%=", "+=", "-=", "&=", "^=", "|=", "=~",
	"+", "-", "*", "/", "%", "<", ">", "=", "!", "&", "|", "^", "~", "?", ":", ",", ";", "(", ")",
}

func lexArithmeticExpression(data []byte, start, end int) []expressionToken {
	return lexExpressionWithOperators(data, start, end, arithmeticOperatorSpellings, true)
}

var conditionalOperatorSpellings = []string{"&&", "||", "==", "!=", "=~", "<", ">", "=", "!", "(", ")"}

func lexConditionalExpression(data []byte, start, end int) []expressionToken {
	return lexExpressionWithOperators(data, start, end, conditionalOperatorSpellings, false)
}

func lexExpressionWithOperators(data []byte, start, end int, operators []string, splitAdjacent bool) []expressionToken {
	var result []expressionToken
	for index := start; index < end; {
		if isExpressionSpace(data[index]) || data[index] == ';' && expressionOperatorAt(data, index, end, operators) == "" {
			index++
			continue
		}
		tokenStart := index
		if data[index] == '\'' || data[index] == '"' {
			delimiter := data[index]
			index++
			for index < end && data[index] != delimiter {
				if data[index] == '\\' && index+1 < end {
					index += 2
				} else {
					index++
				}
			}
			if index < end {
				index++
			}
		} else if operator := expressionOperatorAt(data, index, end, operators); operator != "" {
			index += len(operator)
		} else {
			for index < end && !isExpressionSpace(data[index]) && (data[index] != ';' || expressionOperatorAt(data, index, end, operators) != "") {
				if splitAdjacent && expressionOperatorAt(data, index, end, operators) != "" {
					break
				}
				if !splitAdjacent && (data[index] == '(' || data[index] == ')') {
					break
				}
				index++
			}
		}
		if index == tokenStart {
			index++
		}
		result = append(result, expressionToken{text: string(data[tokenStart:index]), start: tokenStart, end: index})
	}
	return result
}

func expressionOperatorAt(data []byte, index, end int, operators []string) string {
	for _, operator := range operators {
		if index+len(operator) <= end && string(data[index:index+len(operator)]) == operator {
			return operator
		}
	}
	return ""
}

func isExpressionSpace(value byte) bool {
	return value == ' ' || value == '\t' || value == '\r' || value == '\n'
}

func isArithmeticUnary(value string) bool {
	return value == "+" || value == "-" || value == "!" || value == "~" || value == "++" || value == "--"
}

func isConditionalUnary(value string) bool {
	if value == "-N" || value == "-O" || value == "-G" || value == "-L" || value == "-S" || value == "-R" || value == "-v" {
		return true
	}
	return len(value) == 2 && value[0] == '-' && strings.ContainsRune("abcdefghknoprstuwxz", rune(value[1]))
}

func isConditionalBinary(value string) bool {
	switch value {
	case "=", "==", "!=", "=~", "<", ">", "-ef", "-nt", "-ot", "-eq", "-ne", "-lt", "-le", "-gt", "-ge":
		return true
	default:
		return false
	}
}

func isNumber(value string) bool {
	if value == "" {
		return false
	}
	for _, item := range value {
		if (item < '0' || item > '9') && item != '#' {
			return false
		}
	}
	return true
}

func isQuotedExpression(value string) bool {
	return len(value) >= 2 && ((value[0] == '\'' && value[len(value)-1] == '\'') || (value[0] == '"' && value[len(value)-1] == '"'))
}
