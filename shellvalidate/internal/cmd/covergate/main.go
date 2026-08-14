package main

import (
	"flag"
	"fmt"
	"os"

	"dzcli/shellvalidate/internal/coverage"
)

func main() {
	profileName := flag.String("profile", "", "Go cover profile to inspect")
	minimum := flag.Float64("min", 90, "minimum statement coverage percentage")
	flag.Parse()
	if *profileName == "" || *minimum < 0 || *minimum > 100 {
		fmt.Fprintln(os.Stderr, "covergate: -profile is required and -min must be between 0 and 100")
		os.Exit(2)
	}
	file, err := os.Open(*profileName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "covergate: %v\n", err)
		os.Exit(2)
	}
	defer file.Close()
	profile, err := coverage.ParseProfile(file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "covergate: %v\n", err)
		os.Exit(2)
	}
	fmt.Printf("shellvalidate statement coverage: %.2f%% (%d/%d)\n", profile.Percent(), profile.CoveredStatements, profile.Statements)
	if profile.Percent()+1e-9 < *minimum {
		fmt.Fprintf(os.Stderr, "covergate: statement coverage %.2f%% is below %.2f%%\n", profile.Percent(), *minimum)
		os.Exit(1)
	}
}
