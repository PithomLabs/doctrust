package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/doctrust/doctrust/internal/eval"
)

func main() {
	domain := flag.String("domain", "income_verification", "ruleset domain")
	draftPath := flag.String("draft", "", "path to candidate YAML file (promotes directly)")
	dryRun := flag.Bool("dry-run", false, "validate only, do not promote")
	flag.Parse()

	registry := eval.NewRegistry("rulesets")

	var rs eval.Ruleset
	var err error

	if *draftPath != "" {
		// Load from explicit path
		rs, err = eval.LoadRuleset(*draftPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: cannot load draft: %v\n", err)
			os.Exit(1)
		}
	} else {
		// Load from registry working directory.
		// LoadWorking returns Version="draft" BOTH when the file is missing and
		// when a pipeline-written draft carries the "draft" sentinel — so test
		// existence explicitly. Promotion derives the real version number itself
		// (registry.Promote), so the sentinel must not block freezing.
		workingPath := filepath.Join("rulesets", *domain, "working.yaml")
		if _, statErr := os.Stat(workingPath); os.IsNotExist(statErr) {
			fmt.Fprintf(os.Stderr, "Error: no working draft found. Create one first with:\n  echo 'checks: [...]'\n  > rulesets/%s/working.yaml\n", *domain)
			os.Exit(1)
		}
		rs, err = registry.LoadWorking(*domain)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: cannot load working draft for %s: %v\n", *domain, err)
			os.Exit(1)
		}
		if rs.Version == "" {
			fmt.Fprintf(os.Stderr, "Error: working draft for %s has empty version field.\n", *domain)
			os.Exit(1)
		}
	}

	// Always set the domain as the ruleset ID
	rs.ID = *domain

	// Validate
	if err := rs.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "Validation failed: %v\n", err)
		os.Exit(1)
	}

	hash, _ := rs.ComputeHash()

	fmt.Printf("Ruleset: %s\n", rs.ID)
	fmt.Printf("Checks:  %d\n", len(rs.Checks))
	fmt.Printf("Hash:    %s\n", hash)
	for _, c := range rs.Checks {
		fmt.Printf("  - %s v%s", c.ID, c.Version)
		if c.Params != nil {
			fmt.Printf(" params=%v", c.Params)
		}
		fmt.Println()
	}

	if *dryRun {
		fmt.Println("\nDry run — not promoting.")
		os.Exit(0)
	}

	// Promote
	if err := registry.Promote(rs); err != nil {
		fmt.Fprintf(os.Stderr, "Promotion failed: %v\n", err)
		os.Exit(1)
	}

	// Load back to confirm
	promoted, err := registry.LoadPromoted(*domain)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: promoted but cannot reload: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\nPromoted: %s v%s\n", promoted.ID, promoted.Version)
	fmt.Printf("Working draft deleted.\n")
}
