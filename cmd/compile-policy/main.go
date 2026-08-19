package main

import (
	"context"
	"fmt"
	"os"

	"github.com/doctrust/doctrust/internal/compiler"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: compile-policy <path/to/POLICY.md>\n")
		os.Exit(1)
	}

	policyPath := os.Args[1]

	if _, err := os.Stat(policyPath); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Error: %s not found\n", policyPath)
		os.Exit(1)
	}

	llmClient := compiler.NewOpenRouterClient()
	comp := compiler.NewCompiler(llmClient)

	fmt.Fprintf(os.Stderr, "Compiling policy from %s...\n", policyPath)
	fmt.Fprintf(os.Stderr, "Calling LLM (%s)...\n", os.Getenv("OPENROUTER_MODEL"))

	result, err := comp.Compile(context.Background(), policyPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nError: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "\nPolicy approved: %s/policy.rego\n", result.OutputDir)
	fmt.Fprintf(os.Stderr, "Extraction schema: %s/extraction.json\n", result.OutputDir)
	fmt.Fprintf(os.Stderr, "Version metadata: %s/policy_version.json\n", result.OutputDir)
	fmt.Fprintf(os.Stderr, "Attempts: %d\n", result.Attempts)

	// Print summary to stdout
	fmt.Printf("Compiled policy: %s\n", result.OutputDir)
	fmt.Printf("Attempts: %d\n", result.Attempts)
	fmt.Printf("Model: %s\n", result.Version.CompilerModel)
}
