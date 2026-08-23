package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/doctrust/doctrust/internal/eval"
)

func main() {
	domain := flag.String("domain", "", "ruleset domain to verify (required)")
	rulesetsDir := flag.String("rulesets-dir", "rulesets", "path to rulesets directory")
	expectVersion := flag.String("expect-version", "", "expected ruleset version (optional)")
	expectHash := flag.String("expect-hash", "", "expected ruleset hash (optional)")
	flag.Parse()

	if *domain == "" {
		fmt.Fprintln(os.Stderr, "Error: --domain is required")
		flag.Usage()
		os.Exit(1)
	}

	registry := eval.NewRegistry(*rulesetsDir)

	// Load promoted ruleset
	rs, err := registry.LoadPromoted(*domain)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot load promoted ruleset for domain %s: %v\n", *domain, err)
		os.Exit(1)
	}

	// Verify version if expected
	if *expectVersion != "" && rs.Version != *expectVersion {
		fmt.Fprintf(os.Stderr, "Error: ruleset version mismatch: expected %s, got %s\n", *expectVersion, rs.Version)
		os.Exit(1)
	}

	// Verify hash if expected
	if *expectHash != "" {
		actualHash, err := rs.ComputeHash()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: cannot compute ruleset hash: %v\n", err)
			os.Exit(1)
		}
		if actualHash != *expectHash {
			fmt.Fprintf(os.Stderr, "Error: ruleset hash mismatch: expected %s, got %s\n", *expectHash, actualHash)
			os.Exit(1)
		}
	}

	// Verify all CheckRefs have registered checks
	checkRegistry := eval.DefaultRegistry()
	var missing []string
	for _, ref := range rs.Checks {
		if _, err := checkRegistry.Get(ref.ID); err != nil {
			missing = append(missing, ref.ID)
		}
	}

	if len(missing) > 0 {
		fmt.Fprintf(os.Stderr, "Error: %d check(s) in ruleset %s are not registered:\n", len(missing), *domain)
		for _, id := range missing {
			fmt.Fprintf(os.Stderr, "  - %s\n", id)
		}
		os.Exit(1)
	}

	// Print ruleset info
	fmt.Printf("Ruleset %s (version %s) verified successfully\n", rs.ID, rs.Version)
	fmt.Printf("  Checks: %d\n", len(rs.Checks))
	for _, ref := range rs.Checks {
		fmt.Printf("    - %s v%s\n", ref.ID, ref.Version)
	}
	if *expectVersion != "" {
		fmt.Printf("  Version match: PASS\n")
	}
	if *expectHash != "" {
		fmt.Printf("  Hash match: PASS\n")
	}
}
