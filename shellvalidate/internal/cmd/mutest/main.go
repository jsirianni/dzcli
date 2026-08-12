package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"dzcli/shellvalidate/internal/mutest"
)

func main() {
	root := flag.String("root", ".", "module root")
	manifest := flag.String("manifest", "shellvalidate/testdata/spec/mutants.json", "mutation manifest")
	output := flag.String("out", "artifacts/shellvalidate-mutation", "artifact directory")
	timeout := flag.Duration("timeout", 30*time.Second, "per-command timeout")
	hashFile := flag.String("hash-file", "", "print the canonical hash of a declaration in this file")
	hashDeclaration := flag.String("hash-declaration", "", "declaration to hash")
	flag.Parse()
	if *hashFile != "" || *hashDeclaration != "" {
		if *hashFile == "" || *hashDeclaration == "" {
			fmt.Fprintln(os.Stderr, "-hash-file and -hash-declaration must be used together")
			os.Exit(2)
		}
		digest, err := mutest.DeclarationHash(*hashFile, *hashDeclaration)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		fmt.Println(digest)
		return
	}

	report, err := mutest.Run(context.Background(), mutest.Config{
		Root: *root, ManifestPath: *manifest, OutputDir: *output, Timeout: *timeout,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	failures := mutest.CriticalFailures(report)
	for _, result := range report.Results {
		fmt.Printf("%s: %s\n", result.ID, result.Status)
	}
	if len(failures) != 0 {
		fmt.Fprintf(os.Stderr, "%d critical mutants survived or were invalid\n", len(failures))
		os.Exit(1)
	}
}
