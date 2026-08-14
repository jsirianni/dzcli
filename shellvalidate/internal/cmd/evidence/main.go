package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"dzcli/shellvalidate/internal/evidence"
)

func main() {
	root := flag.String("root", ".", "repository root")
	commit := flag.String("source-commit", "", "tested source commit")
	profile := flag.String("coverprofile", "", "Go cover profile")
	mutations := flag.String("mutation-results", "", "mutation runner results JSON")
	out := flag.String("out", "artifacts/shellvalidate-evidence", "output directory")
	flag.Parse()
	report, err := evidence.Build(*root, *commit, *profile, *mutations)
	if err != nil {
		fmt.Fprintf(os.Stderr, "evidence: %v\n", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(*out, 0o750); err != nil {
		fmt.Fprintf(os.Stderr, "evidence: %v\n", err)
		os.Exit(1)
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err == nil {
		data = append(data, '\n')
		err = os.WriteFile(filepath.Join(*out, "evidence.json"), data, 0o600)
	}
	if err == nil {
		err = os.WriteFile(filepath.Join(*out, "evidence.md"), evidence.RenderMarkdown(report), 0o600)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "evidence: %v\n", err)
		os.Exit(1)
	}
}
