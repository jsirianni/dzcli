package shellvalidate

import "testing"

func TestBashArithmeticExpressionShape(t *testing.T) {
	file, diagnostics, err := Parse("math.sh", []byte("((total += count * 2))\n"), DialectBash)
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("parse: %v %#v", err, diagnostics)
	}
	for _, node := range file.Nodes() {
		if node.Kind() != NodeArithmetic {
			continue
		}
		expressions := node.Expressions()
		if len(expressions) != 1 || expressions[0].Kind() != ExpressionAssignment || expressions[0].Operator() != "+=" {
			t.Fatalf("expression=%#v", expressions)
		}
		return
	}
	t.Fatal("arithmetic node missing")
}

func TestBashConditionalExpressionShape(t *testing.T) {
	file, diagnostics, err := Parse("condition.sh", []byte("[[ -n $value && $value == x* ]]\n"), DialectBash)
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("parse: %v %#v", err, diagnostics)
	}
	for _, node := range file.Nodes() {
		if node.Kind() == NodeConditional && len(node.Expressions()) != 0 {
			return
		}
	}
	t.Fatal("conditional expression missing")
}
