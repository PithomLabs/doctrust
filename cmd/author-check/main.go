package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/doctrust/doctrust/internal/compiler"
	"github.com/doctrust/doctrust/internal/eval"
)

func main() {
	domain := flag.String("domain", "income_verification", "ruleset domain")
	intent := flag.String("intent", "", "natural language requirement (required)")
	candidatesDir := flag.String("dir", "candidates", "candidates base directory")
	flag.Parse()

	if *intent == "" {
		fmt.Fprintln(os.Stderr, "Error: --intent is required")
		flag.Usage()
		os.Exit(1)
	}

	// Load existing checks
	registry := eval.DefaultRegistry()

	// Build facts schema (hardcoded for income_verification domain)
	// In a production system, this would be loaded from the domain's configuration.
	factsSchema := map[string][]string{
		"base_salary":              {"paystub.base_salary_ytd"},
		"gross_income_projected":   {"paystub.annualized_gross_ytd"},
		"gross_income_taxable":     {"w2.wages_tips_other_compensation"},
		"bonus_compensation":       {"paystub.bonus_ytd"},
		"net_cash_flow":            {"bank_statement.total_deposits"},
	}
	_ = domain // used for future domain-specific schema loading

	// Build prompt
	prompt := compiler.BuildCheckPrompt(*intent, registry, factsSchema)

	// Call LLM
	client := compiler.NewOpenRouterClient()
	ctx := context.Background()

	fmt.Println("Generating candidate check from LLM...")
	raw, err := client.GenerateWithSystem(ctx, compiler.GoCheckSystemPrompt(), prompt)
	if err != nil {
		fmt.Fprintf(os.Stderr, "LLM error: %v\n", err)
		os.Exit(1)
	}

	// Parse output
	candidate, err := compiler.ParseCheckOutput(raw)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Parse error: %v\n", err)
		os.Exit(1)
	}

	// Stage candidate
	dir, err := compiler.StageCandidate(candidate, *candidatesDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Stage error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Candidate generated: %s\n\n", dir)
	fmt.Printf("  Check ID:   %s\n", candidate.CheckID)
	fmt.Printf("  Version:    %s\n", candidate.Version)
	fmt.Printf("  Description: %s\n", candidate.Description)
	fmt.Printf("  Scenarios:  %d\n", len(candidate.Scenarios))
	fmt.Printf("\nNext steps:\n")
	fmt.Printf("  1. Review:  bin/review-check %s\n", dir)
	fmt.Printf("  2. Author adversarial scenario: edit %s/adversarial.yaml\n", dir)
	fmt.Printf("  3. Approve: bin/review-check %s\n", dir)
}
