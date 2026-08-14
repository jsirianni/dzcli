package main

import (
	"fmt"
	"os"

	"dzcli/shellvalidate/internal/coverage"
)

func main() {
	data, err := coverage.Render(".")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.WriteFile("COVERAGE.md", data, 0o644); err != nil { // #nosec G306 -- generated documentation is intentionally readable.
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
