package shellvalidate

type tokenKind uint8

const (
	tokenInvalid tokenKind = iota
	tokenWord
	tokenOperator
	tokenNewline
	tokenComment
	tokenEOF
)

type token struct {
	kind   tokenKind
	start  int
	end    int
	text   string
	parts  []WordPart
	quoted bool
}

func cloneTokens(items []token) []token {
	result := append([]token(nil), items...)
	for index := range result {
		result[index].parts = append([]WordPart(nil), result[index].parts...)
	}
	return result
}
