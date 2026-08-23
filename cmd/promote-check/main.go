package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/doctrust/doctrust/internal/compiler"
	"github.com/doctrust/doctrust/internal/eval"
)

func main() {
	candidateDir := flag.String("candidate", "", "candidate directory (required)")
	domain := flag.String("domain", "income_verification", "ruleset domain")
	evalDir := flag.String("eval-dir", "internal/eval", "path to internal/eval directory")
	rulesetsDir := flag.String("rulesets-dir", "rulesets", "path to rulesets directory")
	scenariosDir := flag.String("scenarios-dir", "scenarios", "path to scenarios root directory")
	flag.Parse()

	if *candidateDir == "" {
		fmt.Fprintln(os.Stderr, "Error: --candidate is required")
		flag.Usage()
		os.Exit(1)
	}

	// Resolve repo root for staged compilation
	repoRoot, err := compiler.FindModuleRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot find module root: %v\n", err)
		os.Exit(1)
	}

	// === INDEPENDENT TRUST GATES (state file alone is not the authority) ===

	// Gate 1: State must be APPROVED
	state, err := compiler.GetState(*candidateDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if state != compiler.StateApproved {
		fmt.Fprintf(os.Stderr, "Error: candidate must be APPROVED (current: %s)\n", state)
		os.Exit(1)
	}

	// Gate 2: Adversarial scenario must exist (check filesystem)
	hasAdv, advName := compiler.HasAdversarial(*candidateDir)
	if !hasAdv {
		fmt.Fprintf(os.Stderr, "Error: human-authored adversarial scenario required (found: %s)\n", advName)
		os.Exit(1)
	}

	// === SINGLE-READ SNAPSHOT ===
	fmt.Println("Snapshotting candidate (single read)...")
	snapshot, err := compiler.SnapshotCandidate(*candidateDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Snapshot failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("  Snapshot: check_id=%s version=%s\n", snapshot.CheckID, snapshot.Version)

	// Gate 3: Approval must be identity-bound AND content-bound to snapshot
	fmt.Println("Verifying approval against snapshot...")
	if err := compiler.VerifyApprovalAgainstSnapshot(*candidateDir, snapshot); err != nil {
		fmt.Fprintf(os.Stderr, "Error: approval invalid: %v\n", err)
		os.Exit(1)
	}

	// Gate 4: Candidate validation (go build, go vet, import allowlist)
	registry := eval.DefaultRegistry()

	fmt.Println("Validating candidate...")
	valResult, err := compiler.ValidateSnapshot(snapshot, registry, *rulesetsDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Validation failed: %v\n", err)
		// Early rejections (approval invalid, missing adversarial, forbidden
		// import, duplicate Check-ID) return a nil result — print detail
		// lines only when they exist. A retry must exit cleanly, not panic.
		if valResult != nil {
			for _, e := range valResult.Errors {
				fmt.Fprintf(os.Stderr, "  - %s\n", e)
			}
		}
		os.Exit(1)
	}
	fmt.Printf("  Candidate validation: %d passed\n", valResult.Passed)

	// Gate 5: Execute candidate scenarios deterministically
	fmt.Println("Executing candidate scenarios...")
	execResult, err := compiler.ExecuteCandidateScenarios(snapshot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Scenario execution failed: %v\n", err)
		if execResult != nil {
			for _, r := range execResult.Results {
				status := "PASS"
				if !r.Match {
					status = "FAIL"
				}
				fmt.Fprintf(os.Stderr, "  %s: expected=%s/%s actual=%s/%s [%s]\n",
					r.Name, r.Expected.Status, r.Expected.Severity,
					r.Actual.Status, r.Actual.Severity, status)
			}
		}
		os.Exit(1)
	}
	fmt.Printf("  Scenarios: %d/%d passed\n", execResult.Passed, execResult.Total)
	if execResult.Total < 1 {
		fmt.Fprintf(os.Stderr, "Error: zero scenarios executed — cannot promote untested check\n")
		os.Exit(1)
	}
	if execResult.Failed > 0 {
		fmt.Fprintf(os.Stderr, "Error: %d scenario(s) failed\n", execResult.Failed)
		for _, r := range execResult.Results {
			if !r.Match {
				fmt.Fprintf(os.Stderr, "  FAIL %s: expected=%s/%s actual=%s/%s\n",
					r.Name, r.Expected.Status, r.Expected.Severity,
					r.Actual.Status, r.Actual.Severity)
			}
		}
		os.Exit(1)
	}

	// Stage promotion (uses snapshot bytes, not candidate dir)
	fmt.Println("Staging promotion...")
	stagingDir, err := compiler.StagePromotion(snapshot, *evalDir, *domain, *rulesetsDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Stage failed: %v\n", err)
		os.Exit(1)
	}
	defer compiler.RollbackPromotion(stagingDir)

	// Gate 7: Build staged artifact (compile full module tree with staged files)
	fmt.Println("Building staged artifact...")
	if err := compiler.ValidateStagedArtifact(stagingDir, *evalDir, repoRoot); err != nil {
		fmt.Fprintf(os.Stderr, "Staged build failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("  Staged artifact: PASS")

	// Gate 8: Run staged regression (regression against staged ruleset + corpus)
	fmt.Println("Running staged regression...")
	if err := compiler.RunStagedRegression(stagingDir, *evalDir, *domain, *scenariosDir); err != nil {
		fmt.Fprintf(os.Stderr, "Staged regression failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("  Staged regression: PASS")

	// Gate 9: Commit promotion (atomic: check + registry + Ruleset + scenarios)
	fmt.Println("Committing promotion...")
	if err := compiler.CommitPromotion(stagingDir, *evalDir, *domain, *rulesetsDir, *scenariosDir, snapshot); err != nil {
		fmt.Fprintf(os.Stderr, "Commit failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println()
	fmt.Println("Promotion complete!")
	fmt.Println()
	fmt.Printf("  Check written to: %s/check_*.go\n", *evalDir)
	fmt.Printf("  Registry updated: %s/checks.go\n", *evalDir)
	fmt.Printf("  Ruleset updated: %s/%s/working.yaml\n", *rulesetsDir, *domain)
	fmt.Printf("  Scenarios merged: %s/%s/check_%s.yaml\n", *scenariosDir, *domain, snapshot.CheckID)
	fmt.Printf("  Candidate archived: candidates/archive/\n")
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Printf("  1. bin/promote --domain %s\n", *domain)
	fmt.Printf("  2. make build   (REQUIRED: new Go Checks compile into DefaultRegistry)\n")
	fmt.Printf("  3. bin/verify-ruleset --domain %s --expect-version <N>\n", *domain)
	fmt.Printf("  4. Restart server/MCP to load new Ruleset\n")
	fmt.Printf("  5. Hermes evaluate_case via MCP\n")
}
