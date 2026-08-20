package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/doctrust/doctrust/internal/compiler"
	"github.com/doctrust/doctrust/internal/eval"
)

	// Verify that compiler exports used by promote-check are available.
var (
	_ = compiler.VerifyApprovalAgainstSnapshot
	_ = compiler.HasAdversarial
	_ = compiler.HasAdversarialInSnapshot
	_ = compiler.LoadApproval
)

func main() {
	candidateDir := flag.String("candidate", "", "candidate directory (required)")
	domain := flag.String("domain", "income_verification", "ruleset domain")
	evalDir := flag.String("eval-dir", "internal/eval", "path to internal/eval directory")
	rulesetsDir := flag.String("rulesets-dir", "rulesets", "path to rulesets directory")
	flag.Parse()

	if *candidateDir == "" {
		fmt.Fprintln(os.Stderr, "Error: --candidate is required")
		flag.Usage()
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
	// This is the ONLY read of candidate files after approval.
	// Everything downstream uses these in-memory bytes.
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

	// Gate 4: Candidate validation (go build, go vet on snapshot bytes)
	registry := eval.DefaultRegistry()

	fmt.Println("Validating candidate...")
	valResult, err := compiler.ValidateSnapshot(snapshot, registry)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Validation failed: %v\n", err)
		for _, e := range valResult.Errors {
			fmt.Fprintf(os.Stderr, "  - %s\n", e)
		}
		os.Exit(1)
	}
	fmt.Printf("  Candidate validation: %d passed\n", valResult.Passed)

	// Stage promotion (uses snapshot bytes, not candidate dir)
	fmt.Println("Staging promotion...")
	stagingDir, err := compiler.StagePromotion(snapshot, *evalDir, *domain, *rulesetsDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Stage failed: %v\n", err)
		os.Exit(1)
	}
	defer compiler.RollbackPromotion(stagingDir)

	// Build transformed artifact
	fmt.Println("Building transformed artifact...")
	if err := compiler.ValidateTransformedArtifact(); err != nil {
		fmt.Fprintf(os.Stderr, "Transformed build failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("  Transformed artifact: PASS")

	// Commit promotion (atomic: check + registry + Ruleset)
	fmt.Println("Committing promotion...")
	if err := compiler.CommitPromotion(stagingDir, *evalDir, *domain, *rulesetsDir, snapshot); err != nil {
		fmt.Fprintf(os.Stderr, "Commit failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println()
	fmt.Println("Promotion complete!")
	fmt.Println()
	fmt.Printf("  Check written to: %s/check_*.go\n", *evalDir)
	fmt.Printf("  Registry updated: %s/checks.go\n", *evalDir)
	fmt.Printf("  Ruleset updated: %s/%s/working.yaml\n", *rulesetsDir, *domain)
	fmt.Printf("  Candidate archived: candidates/archive/\n")
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Printf("  1. Run: bin/regression --domain %s\n", *domain)
	fmt.Printf("  2. Run: bin/promote --domain %s\n", *domain)
	fmt.Printf("  3. Run: bin/server --domain %s\n", *domain)
}
