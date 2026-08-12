package shellvalidate

import (
	"encoding/json"
	"testing"
)

type snapshotFile struct {
	Dialect     Dialect        `json:"dialect"`
	Interpreter string         `json:"interpreter,omitempty"`
	Nodes       []snapshotNode `json:"nodes"`
	Comments    []snapshotText `json:"comments,omitempty"`
}

type snapshotNode struct {
	Kind        NodeKind             `json:"kind"`
	Span        [2]int               `json:"span"`
	Incomplete  bool                 `json:"incomplete,omitempty"`
	Words       []snapshotWord       `json:"words,omitempty"`
	Children    []snapshotNode       `json:"children,omitempty"`
	Expressions []snapshotExpression `json:"expressions,omitempty"`
}

type snapshotWord struct {
	Span  [2]int         `json:"span"`
	Parts []snapshotPart `json:"parts"`
}

type snapshotPart struct {
	Kind  WordPartKind `json:"kind"`
	Quote QuoteKind    `json:"quote"`
	Text  string       `json:"text"`
	Span  [2]int       `json:"span"`
}

type snapshotExpression struct {
	Kind     ExpressionKind       `json:"kind"`
	Operator string               `json:"operator,omitempty"`
	Value    string               `json:"value,omitempty"`
	Span     [2]int               `json:"span"`
	Children []snapshotExpression `json:"children,omitempty"`
}

type snapshotText struct {
	Text string `json:"text"`
	Span [2]int `json:"span"`
}

func snapshotAST(file *File) ([]byte, error) {
	result := snapshotFile{Dialect: file.Dialect(), Interpreter: file.Interpreter()}
	for _, node := range file.Nodes() {
		result.Nodes = append(result.Nodes, snapshotNodeValue(node))
	}
	for _, comment := range file.Comments() {
		result.Comments = append(result.Comments, snapshotText{Text: string(comment.Text()), Span: offsets(comment.Span())})
	}
	return json.MarshalIndent(result, "", "  ")
}

func snapshotNodeValue(node Node) snapshotNode {
	result := snapshotNode{Kind: node.Kind(), Span: offsets(node.Span()), Incomplete: node.Incomplete()}
	for _, word := range node.Words() {
		item := snapshotWord{Span: offsets(word.Span())}
		for _, part := range word.Parts() {
			item.Parts = append(item.Parts, snapshotPart{Kind: part.Kind(), Quote: part.Quote(), Text: string(part.Text()), Span: offsets(part.Span())})
		}
		result.Words = append(result.Words, item)
	}
	for _, child := range node.Children() {
		result.Children = append(result.Children, snapshotNodeValue(child))
	}
	for _, expression := range node.Expressions() {
		result.Expressions = append(result.Expressions, snapshotExpressionValue(expression))
	}
	return result
}

func snapshotExpressionValue(expression Expression) snapshotExpression {
	result := snapshotExpression{Kind: expression.Kind(), Operator: expression.Operator(), Value: expression.Value(), Span: offsets(expression.Span())}
	for _, child := range expression.Children() {
		result.Children = append(result.Children, snapshotExpressionValue(child))
	}
	return result
}

func offsets(span Span) [2]int { return [2]int{span.Start.Offset, span.End.Offset} }

func TestASTSnapshotContract(t *testing.T) {
	source := []byte("# heading\nname='value'; printf \"%s\\n\" \"$name\"\n")
	file, diagnostics, err := Parse("snapshot.sh", source, DialectPOSIX)
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("parse: %v %#v", err, diagnostics)
	}
	first, err := snapshotAST(file)
	if err != nil {
		t.Fatal(err)
	}
	second, err := snapshotAST(file)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatal("AST serializer is nondeterministic")
	}
	const want = `{
  "dialect": 1,
  "nodes": [
    {
      "kind": "assignment",
      "span": [
        10,
        22
      ],
      "words": [
        {
          "span": [
            10,
            22
          ],
          "parts": [
            {
              "kind": 0,
              "quote": 0,
              "text": "name=",
              "span": [
                10,
                15
              ]
            },
            {
              "kind": 0,
              "quote": 1,
              "text": "value",
              "span": [
                16,
                21
              ]
            }
          ]
        }
      ]
    },
    {
      "kind": "command",
      "span": [
        24,
        45
      ],
      "words": [
        {
          "span": [
            24,
            30
          ],
          "parts": [
            {
              "kind": 0,
              "quote": 0,
              "text": "printf",
              "span": [
                24,
                30
              ]
            }
          ]
        },
        {
          "span": [
            31,
            37
          ],
          "parts": [
            {
              "kind": 0,
              "quote": 2,
              "text": "%s\\n",
              "span": [
                32,
                36
              ]
            }
          ]
        },
        {
          "span": [
            38,
            45
          ],
          "parts": [
            {
              "kind": 0,
              "quote": 2,
              "text": "$name",
              "span": [
                39,
                44
              ]
            }
          ]
        }
      ]
    }
  ],
  "comments": [
    {
      "text": "# heading",
      "span": [
        0,
        9
      ]
    }
  ]
}`
	if string(first) != want {
		t.Fatalf("snapshot changed:\n%s", first)
	}
}
