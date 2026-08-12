package shellvalidate

import "sort"

type sourceFile struct {
	name       string
	data       []byte
	lineStarts []int
}

func newSourceFile(name string, data []byte) *sourceFile {
	copyData := append([]byte(nil), data...)
	starts := []int{0}
	for index, value := range copyData {
		if value == '\n' {
			starts = append(starts, index+1)
		}
	}
	return &sourceFile{name: name, data: copyData, lineStarts: starts}
}

func (source *sourceFile) position(offset int) Position {
	if offset < 0 {
		offset = 0
	}
	if offset > len(source.data) {
		offset = len(source.data)
	}
	lineIndex := sort.Search(len(source.lineStarts), func(index int) bool {
		return source.lineStarts[index] > offset
	}) - 1
	if lineIndex < 0 {
		lineIndex = 0
	}
	return Position{Offset: offset, Line: lineIndex + 1, Column: offset - source.lineStarts[lineIndex] + 1}
}

func (source *sourceFile) span(start, end int) Span {
	return Span{Start: source.position(start), End: source.position(end)}
}

func (source *sourceFile) diagnostic(code string, severity Severity, confidence Confidence, message string, start, end int) Diagnostic {
	return Diagnostic{Code: code, Severity: severity, Confidence: confidence, Message: message, Primary: source.span(start, end)}
}

// normalizeSourceIdentity deliberately performs no normalization. The
// resolver-returned filename is the caller's canonical, opaque identity.
func normalizeSourceIdentity(identity string) string { return identity }
