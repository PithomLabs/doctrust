package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/PithomLabs/doctrust/internal/review"
)

type DecisionSidecar struct {
	LoadcaseID     string             `json:"loadcase_id"`
	GraphCaseID    string             `json:"graph_case_id,omitempty"`
	SnapshotSHA256 string             `json:"snapshot_sha256"`
	Status         string             `json:"status"`
	Findings       []SidecarFinding   `json:"findings"`
	Ruleset        review.RuleBinding `json:"ruleset"`
}

type SidecarFinding struct {
	Index    int                    `json:"index"`
	CheckID  string                 `json:"check_id"`
	Status   string                 `json:"status"`
	Severity string                 `json:"severity"`
	Reason   string                 `json:"reason"`
	Metrics  map[string]interface{} `json:"metrics,omitempty"`
	Evidence []interface{}          `json:"evidence,omitempty"`
}

func main() {
	var reviewersRing string
	flag.StringVar(&reviewersRing, "reviewers-ring", "", "path to reviewers ring directory (default: auto-detect from snapshot path)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: verify-audit [flags] SNAPSHOT_PATH\n")
		fmt.Fprintf(os.Stderr, "  SNAPSHOT_PATH: path to evidence_snapshot.json (will verify sibling .decision.json)\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	args := flag.Args()
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "ERROR: SNAPSHOT_PATH is required")
		flag.Usage()
		os.Exit(2)
	}

	snapshotPath := args[0]

	// Verify snapshot exists
	_, err := os.ReadFile(snapshotPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: failed to read snapshot %s: %v\n", snapshotPath, err)
		os.Exit(1)
	}

	// Find decision sidecar
	decisionPath := snapshotPath + ".decision.json"
	decisionData, err := os.ReadFile(decisionPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: decision sidecar not found at %s: %v\n", decisionPath, err)
		os.Exit(1)
	}

	// Parse decision sidecar
	var sidecar DecisionSidecar
	if err := json.Unmarshal(decisionData, &sidecar); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: failed to parse decision sidecar: %v\n", err)
		os.Exit(1)
	}

	// Verify required fields
	if err := validateStructure(&sidecar); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: structure validation failed: %v\n", err)
		os.Exit(1)
	}

	// Verify snapshot SHA-256 binding
	if err := verifySnapshotHash(snapshotPath, sidecar.SnapshotSHA256); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: snapshot hash verification failed: %v\n", err)
		os.Exit(1)
	}

	// Verify Ruleset hash binding
	if err := verifyRulesetHash(sidecar.Ruleset); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: ruleset hash verification failed: %v\n", err)
		os.Exit(1)
	}

	// Verify loadcase/case identity
	if err := verifyIdentityBinding(&sidecar); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: identity binding verification failed: %v\n", err)
		os.Exit(1)
	}

	// Verify decision integrity (PASS/REVIEW)
	if err := verifyDecisionIntegrity(&sidecar); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: decision integrity verification failed: %v\n", err)
		os.Exit(1)
	}

	// Verify human reviews if present
	reviewsPath := snapshotPath + ".doctrust_reviews.json"
	if _, err := os.Stat(reviewsPath); err == nil {
		if err := verifyHumanReviews(reviewsPath, sidecar, reviewersRing); err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: human review verification failed: %v\n", err)
			os.Exit(1)
		}
	}

	// Success
	fmt.Printf("VERIFIED: %s\n", decisionPath)
	fmt.Printf("  snapshot_sha256: %s\n", sidecar.SnapshotSHA256[:16]+"...")
	fmt.Printf("  ruleset: %s v%s hash=%s...\n", sidecar.Ruleset.ID, sidecar.Ruleset.Version, sidecar.Ruleset.Hash[:16])
	fmt.Printf("  status: %s\n", sidecar.Status)
	fmt.Printf("  findings: %d\n", len(sidecar.Findings))
	for _, f := range sidecar.Findings {
		fmt.Printf("    [%d] %s: %s/%s\n", f.Index, f.CheckID, f.Status, f.Severity)
	}
}

func validateStructure(s *DecisionSidecar) error {
	if s.LoadcaseID == "" {
		return fmt.Errorf("missing loadcase_id")
	}
	if s.SnapshotSHA256 == "" {
		return fmt.Errorf("missing snapshot_sha256")
	}
	if s.Ruleset.ID == "" {
		return fmt.Errorf("missing ruleset.id")
	}
	if s.Ruleset.Version == "" {
		return fmt.Errorf("missing ruleset.version")
	}
	if s.Ruleset.Hash == "" {
		return fmt.Errorf("missing ruleset.hash")
	}
	if s.Status == "" {
		return fmt.Errorf("missing status")
	}
	if len(s.Findings) == 0 {
		return fmt.Errorf("no findings")
	}
	return nil
}

func verifySnapshotHash(snapshotPath, expectedHash string) error {
	data, err := os.ReadFile(snapshotPath)
	if err != nil {
		return err
	}
	hash := sha256.Sum256(data)
	actual := hex.EncodeToString(hash[:])
	if actual != expectedHash {
		return fmt.Errorf("snapshot hash mismatch: expected %s, got %s", expectedHash, actual)
	}
	return nil
}

func verifyRulesetHash(rb review.RuleBinding) error {
	// Read the manifest file which contains the canonical ruleset hash
	manifestPath := filepath.Join("rulesets", rb.ID, fmt.Sprintf("v%s.manifest.json", rb.Version))
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("failed to read ruleset manifest %s: %w", manifestPath, err)
	}

	var manifest struct {
		ID      string `json:"id"`
		Version string `json:"version"`
		Hash    string `json:"hash"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return fmt.Errorf("failed to parse ruleset manifest: %w", err)
	}

	if manifest.Hash != rb.Hash {
		return fmt.Errorf("ruleset hash mismatch: expected %s, got %s", rb.Hash, manifest.Hash)
	}
	return nil
}

func verifyIdentityBinding(s *DecisionSidecar) error {
	// loadcase_id should be first 16 hex chars of snapshot_sha256
	expectedLoadcase := s.SnapshotSHA256[:16]
	if s.LoadcaseID != expectedLoadcase {
		return fmt.Errorf("loadcase_id mismatch: expected %s, got %s", expectedLoadcase, s.LoadcaseID)
	}
	// graph_case_id is optional but if present should be valid
	return nil
}

func verifyDecisionIntegrity(s *DecisionSidecar) error {
	hasReview := false
	hasFail := false
	for _, f := range s.Findings {
		switch f.Status {
		case "REVIEW":
			hasReview = true
		case "FAIL":
			hasFail = true
		case "PASS":
			// OK
		default:
			return fmt.Errorf("unknown finding status: %s", f.Status)
		}
	}

	switch s.Status {
	case "PASS":
		if hasReview || hasFail {
			return fmt.Errorf("status PASS but has REVIEW/FAIL findings")
		}
	case "REVIEW":
		if !hasReview {
			return fmt.Errorf("status REVIEW but no REVIEW findings")
		}
	case "FAIL":
		if !hasFail && !hasReview {
			return fmt.Errorf("status FAIL but no FAIL/REVIEW findings")
		}
	default:
		return fmt.Errorf("unknown decision status: %s", s.Status)
	}
	return nil
}

func verifyHumanReviews(reviewsPath string, sidecar DecisionSidecar, reviewersRing string) error {
	// Load reviews sidecar
	sc, err := review.LoadReviewsSidecar(reviewsPath)
	if err != nil {
		return fmt.Errorf("failed to load reviews sidecar: %w", err)
	}

	// Use provided reviewers ring or auto-detect
	ringDir := reviewersRing
	if ringDir == "" {
		// Try to auto-detect: look for reviewers/ in snapshot's parent, grandparent, etc.
		// This handles both /tmp/demo-XXX/ and demo/shipment_release/runs/.../available/
		dir := filepath.Dir(reviewsPath)
		for {
			candidate := filepath.Join(dir, "reviewers")
			if _, err := os.Stat(candidate); err == nil {
				ringDir = candidate
				break
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}

	if ringDir == "" {
		return fmt.Errorf("reviewers ring directory not found (use --reviewers-ring to specify)")
	}

	entries, err := os.ReadDir(ringDir)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("reviewers ring directory not found: %s", ringDir)
		}
		return fmt.Errorf("failed to read reviewers ring: %w", err)
	}

	ring := map[string]ed25519.PublicKey{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".pub") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(ringDir, e.Name()))
		if err != nil {
			continue
		}
		pub, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(data)))
		if err != nil || len(pub) != ed25519.PublicKeySize {
			continue
		}
		ring[strings.TrimSuffix(e.Name(), ".pub")] = ed25519.PublicKey(pub)
	}

	if len(ring) == 0 {
		return fmt.Errorf("no public keys found in reviewers ring")
	}

	// Verify each record
	expectedBinding := review.RuleBinding{
		ID:      sidecar.Ruleset.ID,
		Version: sidecar.Ruleset.Version,
		Hash:    sidecar.Ruleset.Hash,
	}

	snapshotSHA := sidecar.SnapshotSHA256

	for _, rec := range sc.Records {
		// Verify case binding
		if rec.CaseID != sidecar.LoadcaseID && rec.GraphCaseID != sidecar.GraphCaseID {
			return fmt.Errorf("review record bound to different case (%s/%s vs %s/%s)",
				rec.CaseID, rec.GraphCaseID, sidecar.LoadcaseID, sidecar.GraphCaseID)
		}
		if rec.SnapshotSHA256 != "" && rec.SnapshotSHA256 != snapshotSHA {
			return fmt.Errorf("review record snapshot hash mismatch")
		}
		if rec.Ruleset != expectedBinding {
			return fmt.Errorf("review record ruleset binding mismatch (%s v%s %s)",
				rec.Ruleset.ID, rec.Ruleset.Version, rec.Ruleset.Hash)
		}
		if rec.ReviewerIdentity != rec.KeyID {
			return fmt.Errorf("reviewer identity %q does not match signing key %q",
				rec.ReviewerIdentity, rec.KeyID)
		}
		if rec.FindingIndex < 0 || rec.FindingIndex >= len(sidecar.Findings) {
			return fmt.Errorf("review record finding_index %d out of range", rec.FindingIndex)
		}
		pub, ok := ring[rec.KeyID]
		if !ok {
			return fmt.Errorf("unknown review key_id %q", rec.KeyID)
		}
		if err := review.VerifyRecord(pub, rec); err != nil {
			return fmt.Errorf("signature verification failed: %w", err)
		}
	}

	fmt.Printf("  human reviews: %d verified\n", len(sc.Records))
	for _, rec := range sc.Records {
		fmt.Printf("    finding[%d] %s by %s (key=%s)\n", rec.FindingIndex, rec.Action, rec.ReviewerIdentity, rec.KeyID)
	}
	return nil
}

// copy of io functions we need
func init() {
	// ensure io is used
	_ = io.EOF
}
