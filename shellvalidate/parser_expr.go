package shellvalidate

import "strings"

type expressionToken struct {
	text  string
	start int
	end   int
}

type expressionParser struct {
	source *sourceFile
	tokens []expressionToken
	index  int
}

var arithmeticPrecedence = map[string]int{
	"=": 1, "+=": 1, "-=": 1, "*=": 1, "/=": 1, "%=": 1,
	"?": 2, "||": 3, "&&": 4, "|": 5, "^": 6, "&": 7,
	"==": 8, "!=": 8, "<": 9, "<=": 9, ">": 9, ">=": 9,
	"<<": 10, ">>": 10, "+": 11, "-": 11, "*": 12, "/": 12, "%": 12, "**": 13,
}

func parseArithmeticExpressions(source *sourceFile, start, end int) []Expression {
	parser := expressionParser{source: source, tokens: lexExpression(source.data, start, end)}
	var result []Expression
	for parser.index < len(parser.tokens) {
		result = append(result, parser.parse(1))
		if parser.peek(",") {
			parser.index++
		} else if parser.index < len(parser.tokens) {
			parser.index++
		}
	}
	return result
}

func parseConditionalExpressions(source *sourceFile, start, end int) []Expression {
	tokens := lexExpression(source.data, start, end)
	if len(tokens) == 0 {
		return nil
	}
	parser := expressionParser{source: source, tokens: tokens}
	return []Expression{parser.parse(1)}
}

func (parser *expressionParser) parse(minimum int) Expression {
	if parser.index >= len(parser.tokens) {
		return Expression{}
	}
	token := parser.tokens[parser.index]
	parser.index++
	var left Expression
	if isUnaryOperator(token.text) {
		operand := parser.parse(13)
		left = Expression{kind: ExpressionUnary, operator: token.text, span: parser.source.span(token.start, operand.span.End.Offset), children: []Expression{operand}}
	} else if token.text == "(" {
		left = parser.parse(1)
		if parser.peek(")") {
			left.span = parser.source.span(token.start, parser.tokens[parser.index].end)
			parser.index++
		}
	} else {
		kind := ExpressionName
		if isNumber(token.text) || isQuotedExpression(token.text) {
			kind = ExpressionLiteral
		}
		left = Expression{kind: kind, value: token.text, span: parser.source.span(token.start, token.end)}
	}
	for parser.index < len(parser.tokens) {
		operator := parser.tokens[parser.index]
		precedence, ok := arithmeticPrecedence[operator.text]
		if !ok || precedence < minimum {
			break
		}
		parser.index++
		right := parser.parse(precedence + 1)
		kind := ExpressionBinary
		if strings.HasSuffix(operator.text, "=") && operator.text != "==" && operator.text != "!=" && operator.text != "<=" && operator.text != ">=" {
			kind = ExpressionAssignment
		}
		left = Expression{kind: kind, operator: operator.text, span: parser.source.span(left.span.Start.Offset, right.span.End.Offset), children: []Expression{left, right}}
	}
	return left
}

func (parser *expressionParser) peek(text string) bool {
	return parser.index < len(parser.tokens) && parser.tokens[parser.index].text == text
}

func lexExpression(data []byte, start, end int) []expressionToken {
	var result []expressionToken
	for index := start; index < end; {
		if data[index] == ' ' || data[index] == '\t' || data[index] == '\r' || data[index] == '\n' || data[index] == ';' {
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
		} else if isExpressionOperatorByte(data[index]) {
			index++
			if index < end && isExpressionOperatorPair(data[tokenStart:index+1]) {
				index++
			}
		} else {
			for index < end && !isExpressionDelimiter(data[index]) {
				index++
			}
		}
		result = append(result, expressionToken{text: string(data[tokenStart:index]), start: tokenStart, end: index})
	}
	return result
}

func isExpressionOperatorByte(value byte) bool {
	return strings.ContainsRune("+-*/%<>=!&|^~?:,()", rune(value))
}

func isExpressionDelimiter(value byte) bool {
	return value == ' ' || value == '\t' || value == '\r' || value == '\n' || value == ';' || isExpressionOperatorByte(value)
}

func isExpressionOperatorPair(value []byte) bool {
	switch string(value) {
	case "++", "--", "**", "<<", ">>", "<=", ">=", "==", "!=", "&&", "||", "+=", "-=", "*=", "/=", "%=", "=~":
		return true
	default:
		return false
	}
}

func isUnaryOperator(value string) bool {
	if value == "+" || value == "-" || value == "!" || value == "~" || value == "++" || value == "--" {
		return true
	}
	return len(value) == 2 && value[0] == '-' && value[1] >= 'a' && value[1] <= 'z'
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
