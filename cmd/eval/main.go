package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/doctrust/doctrust/internal/opa"
)

func main() {
	domain := flag.String("domain", "", "compiled domain (resolves to compiled/<domain>/policy.rego)")
	policy := flag.String("policy", "", "explicit policy.rego path (overrides --domain)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [flags] <snapshot.json>\n\nFlags:\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(1)
	}

	snapshotPath := flag.Arg(0)

	var policyPath string
	if *policy != "" {
		policyPath = *policy
	} else if *domain != "" {
		policyPath = filepath.Join("compiled", *domain, "policy.rego")
	} else {
		fmt.Fprintf(os.Stderr, "Error: specify --domain or --policy\n")
		flag.Usage()
		os.Exit(1)
	}

	snapshotJSON, err := os.ReadFile(snapshotPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading snapshot: %v\n", err)
		os.Exit(1)
	}

	policyRego, err := os.ReadFile(policyPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading policy %s: %v\n", policyPath, err)
		os.Exit(1)
	}

	result, err := opa.Evaluate(context.Background(), snapshotJSON, string(policyRego))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error evaluating policy: %v\n", err)
		os.Exit(1)
	}

	out, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling result: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(string(out))
}
