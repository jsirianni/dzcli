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
