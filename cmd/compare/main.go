package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/PithomLabs/doctrust/internal/eval"
	"github.com/PithomLabs/doctrust/internal/evidence"
	"github.com/PithomLabs/doctrust/internal/opa"
	"github.com/PithomLabs/doctrust/internal/service"
)

type comparisonResult struct {
	decisionMatch   bool
	findingMismatch []string
}

func main() {
	snapshotPath := "demo/income_verification/evidence_snapshot.json"
	if len(os.Args) > 1 {
		snapshotPath = os.Args[1]
	}

	snapshotJSON, err := os.ReadFile(snapshotPath)
	if err != nil {
		fmt.Printf("Error reading snapshot: %v\n", err)
		os.Exit(1)
	}

	var snapshot evidence.EvidenceGraph
	if err := json.Unmarshal(snapshotJSON, &snapshot); err != nil {
		fmt.Printf("Error unmarshaling snapshot: %v\n", err)
		os.Exit(1)
	}

	// OPA evaluation
	policyRegoPath := "policies/income_verification/policy.rego"
	policyData, err := os.ReadFile(policyRegoPath)
	if err != nil {
		fmt.Printf("Warning: cannot read OPA policy at %s: %v\n", policyRegoPath, err)
		fmt.Println("Skipping OPA evaluation — no programmatic comparison possible.")
		os.Exit(0)
	}

	opaResult, err := opa.Evaluate(context.Background(), snapshotJSON, string(policyData))
	if err != nil {
		fmt.Printf("OPA evaluation error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("=== OPA Result ===\n")
	fmt.Printf("Decision: %s\n", opaResult.Decision)
	fmt.Printf("Findings: %d\n", len(opaResult.Findings))
	for _, f := range opaResult.Findings {
		fmt.Printf("  - Rule: %s, Severity: %s, ClaimA: %s, ClaimB: %s\n", f.Rule, f.Severity, f.ClaimA, f.ClaimB)
	}

	// internal/eval evaluation
	registry := eval.NewRegistry("rulesets")
	rs, err := registry.LoadPromoted("income_verification")
	if err != nil {
		fmt.Printf("Error loading promoted ruleset: %v\n", err)
		os.Exit(1)
	}

	checks := map[string]eval.Check{
		"gross_income_consistency":     &eval.GrossIncomeConsistencyCheck{},
		"required_documents":           &eval.RequiredDocumentsCheck{},
		"net_vs_gross_incomparability": &eval.NetVsGrossIncomparabilityCheck{},
	}
	runner := eval.NewRunner(checks)

	// Build Facts from snapshot using canonical builder
	f, err := service.BuildFactsFromSnapshot(&snapshot)
	if err != nil {
		fmt.Printf("BuildFactsFromSnapshot error: %v\n", err)
		os.Exit(1)
	}

	decision, err := runner.Evaluate(context.Background(), rs, f)
	if err != nil {
		fmt.Printf("internal/eval error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\n=== internal/eval Result ===\n")
	fmt.Printf("Decision: %s\n", decision.Status)
	fmt.Printf("Findings: %d\n", len(decision.Results))
	for _, res := range decision.Results {
		fmt.Printf("  - Rule: %s, Severity: %s, Status: %s, Reason: %s\n", res.CheckID, res.Severity, res.Status, res.Reason)
		for _, ev := range res.Evidence {
			fmt.Printf("    Evidence: field=%s, sourceDoc=%s, sourceSpan=%s, confidence=%.2f\n", ev.Field, ev.SourceDoc, ev.SourceSpan, ev.Confidence)
		}
	}

	// Programmatic comparison
	result := compareResults(opaResult, string(decision.Status), decision.Results)

	fmt.Printf("\n=== Comparison ===\n")
	if result.decisionMatch {
		fmt.Println("Decision: MATCH")
	} else {
		fmt.Printf("Decision: MISMATCH (OPA=%s, eval=%s)\n", opaResult.Decision, decision.Status)
	}

	if len(result.findingMismatch) == 0 {
		fmt.Println("Findings: MATCH")
	} else {
		fmt.Printf("Findings: %d MISMATCH(ES)\n", len(result.findingMismatch))
		for _, m := range result.findingMismatch {
			fmt.Printf("  - %s\n", m)
		}
	}

	if !result.decisionMatch || len(result.findingMismatch) > 0 {
		fmt.Println("\nCOMPARISON FAILED")
		os.Exit(2)
	}
	fmt.Println("\nCOMPARISON PASSED")
}

func compareResults(opaResult *opa.EvaluationResult, evalDecision string, evalResults []eval.Result) comparisonResult {
	result := comparisonResult{}

	// Normalize decision strings for comparison
	normalizeDecision := func(d string) string {
		switch d {
		case "PASS", "pass", "Pass":
			return "PASS"
		case "REVIEW", "review", "Review":
			return "REVIEW"
		case "FAIL", "fail", "Fail":
			return "FAIL"
		default:
			return d
		}
	}

	opaDecision := normalizeDecision(opaResult.Decision)
	evDecision := normalizeDecision(evalDecision)
	result.decisionMatch = opaDecision == evDecision

	// Compare findings by rule identity and severity
	type opaFinding struct {
		rule     string
		severity string
	}
	type evalFinding struct {
		rule     string
		severity string
		status   string
	}

	opaFindings := make(map[opaFinding]bool)
	for _, f := range opaResult.Findings {
		opaFindings[opaFinding{rule: f.Rule, severity: f.Severity}] = true
	}

	evalFindings := make(map[evalFinding]bool)
	for _, r := range evalResults {
		evalFindings[evalFinding{rule: r.CheckID, severity: string(r.Severity), status: string(r.Status)}] = true
	}

	// Check for findings in OPA but not in eval
	var mismatches []string
	for of := range opaFindings {
		found := false
		for ef := range evalFindings {
			if ef.rule == of.rule {
				found = true
				if ef.severity != of.severity {
					mismatches = append(mismatches, fmt.Sprintf("rule %s: severity mismatch (OPA=%s, eval=%s)", of.rule, of.severity, ef.severity))
				}
				break
			}
		}
		if !found {
			mismatches = append(mismatches, fmt.Sprintf("rule %s: present in OPA but missing from eval", of.rule))
		}
	}

	// Check for findings in eval but not in OPA
	for ef := range evalFindings {
		found := false
		for of := range opaFindings {
			if of.rule == ef.rule {
				found = true
				break
			}
		}
		if !found {
			mismatches = append(mismatches, fmt.Sprintf("rule %s: present in eval but missing from OPA", ef.rule))
		}
	}

	sort.Strings(mismatches)
	result.findingMismatch = mismatches
	return result
}
