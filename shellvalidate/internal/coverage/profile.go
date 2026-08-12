package coverage

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Profile summarizes statement coverage from a Go cover profile. Statements,
// rather than source regions, are weighted in the same way as go tool cover.
type Profile struct {
	Statements        uint64
	CoveredStatements uint64
}

// Percent returns statement coverage as a percentage.
func (p Profile) Percent() float64 {
	if p.Statements == 0 {
		return 0
	}
	return 100 * float64(p.CoveredStatements) / float64(p.Statements)
}

// ParseProfile parses a Go cover profile and rejects malformed, empty, and
// overflowing input. It is intentionally independent of go tool cover's
// human-readable output.
func ParseProfile(input io.Reader) (Profile, error) {
	scanner := bufio.NewScanner(input)
	if !scanner.Scan() || !strings.HasPrefix(scanner.Text(), "mode: ") {
		return Profile{}, fmt.Errorf("missing cover profile mode")
	}
	var result Profile
	for line := 2; scanner.Scan(); line++ {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 3 {
			return Profile{}, fmt.Errorf("line %d: expected 3 fields", line)
		}
		statements, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return Profile{}, fmt.Errorf("line %d: invalid statement count %q", line, fields[1])
		}
		count, err := strconv.ParseUint(fields[2], 10, 64)
		if err != nil {
			return Profile{}, fmt.Errorf("line %d: invalid execution count %q", line, fields[2])
		}
		if ^uint64(0)-result.Statements < statements {
			return Profile{}, fmt.Errorf("line %d: statement count overflow", line)
		}
		result.Statements += statements
		if count != 0 {
			if ^uint64(0)-result.CoveredStatements < statements {
				return Profile{}, fmt.Errorf("line %d: covered statement count overflow", line)
			}
			result.CoveredStatements += statements
		}
	}
	if err := scanner.Err(); err != nil {
		return Profile{}, fmt.Errorf("read cover profile: %w", err)
	}
	if result.Statements == 0 {
		return Profile{}, fmt.Errorf("cover profile contains no statements")
	}
	return result, nil
}
