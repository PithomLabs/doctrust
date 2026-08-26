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

	// Verify audit artifact if present
	auditPath := snapshotPath + ".audit.json"
	if _, err := os.Stat(auditPath); err == nil {
		if err := verifyAuditArtifact(auditPath, &sidecar, snapshotPath, reviewsPath); err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: audit artifact verification failed: %v\n", err)
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

// auditArtifact is the on-disk audit artifact structure.
type auditArtifact struct {
	Version          string              `json:"version"`
	PolicyID         string              `json:"policy_id"`
	PolicyHash       string              `json:"policy_hash"`
	RulesetID        string              `json:"ruleset_id"`
	RulesetVersion   string              `json:"ruleset_version"`
	RulesetHash      string              `json:"ruleset_hash"`
	Decisions        []auditDecision     `json:"decisions"`
	Documents        []auditDocument     `json:"documents"`
	HumanReviews     []auditHumanReview  `json:"human_reviews,omitempty"`
	FinalDisposition string              `json:"final_disposition,omitempty"`
	CreatedAt        string              `json:"created_at,omitempty"`
	CompletedAt      *string             `json:"completed_at,omitempty"`
	Manifest         auditManifest       `json:"manifest"`
}

type auditDecision struct {
	CaseID    string         `json:"case_id"`
	State     string         `json:"state"`
	Findings  []auditFinding `json:"findings"`
	DecidedAt string         `json:"decided_at"`
}

type auditFinding struct {
	Rule     string  `json:"rule"`
	Severity string  `json:"severity"`
	ClaimA   string  `json:"claim_a"`
	ClaimB   string  `json:"claim_b"`
	ValueA   float64 `json:"value_a"`
	ValueB   float64 `json:"value_b"`
}

type auditDocument struct {
	FileName    string  `json:"file_name"`
	DocType     string  `json:"doc_type"`
	Hash        string  `json:"hash"`
	ExtractedAt string  `json:"extracted_at"`
	Confidence  float64 `json:"confidence"`
}

type auditHumanReview struct {
	FindingIndex     int    `json:"finding_index"`
	Action           string `json:"action"`
	Note             string `json:"note"`
	ResolvedAt       string `json:"resolved_at"`
	ReviewerIdentity string `json:"reviewer_identity,omitempty"`
	Channel          string `json:"channel,omitempty"`
}

type auditManifest struct {
	DocCount      int    `json:"doc_count"`
	DecisionCount int    `json:"decision_count"`
	ReviewCount   int    `json:"review_count"`
	ArtifactHash  string `json:"artifact_hash"`
}

// hashableAudit is the canonical representation for artifact hash computation.
// Manifest.ArtifactHash is excluded to break the self-referential cycle.
type hashableAudit struct {
	Version          string              `json:"version"`
	PolicyID         string              `json:"policy_id"`
	PolicyHash       string              `json:"policy_hash"`
	RulesetID        string              `json:"ruleset_id"`
	RulesetVersion   string              `json:"ruleset_version"`
	RulesetHash      string              `json:"ruleset_hash"`
	Decisions        []auditDecision     `json:"decisions"`
	Documents        []auditDocument     `json:"documents"`
	HumanReviews     []auditHumanReview  `json:"human_reviews,omitempty"`
	FinalDisposition string              `json:"final_disposition,omitempty"`
	CreatedAt        string              `json:"created_at,omitempty"`
	CompletedAt      *string             `json:"completed_at,omitempty"`
	Manifest         struct {
		DocCount      int `json:"doc_count"`
		DecisionCount int `json:"decision_count"`
		ReviewCount   int `json:"review_count"`
	} `json:"manifest"`
}

func verifyAuditArtifact(auditPath string, sidecar *DecisionSidecar, snapshotPath, reviewsPath string) error {
	data, err := os.ReadFile(auditPath)
	if err != nil {
		return fmt.Errorf("read audit artifact: %w", err)
	}

	var artifact auditArtifact
	if err := json.Unmarshal(data, &artifact); err != nil {
		return fmt.Errorf("parse audit artifact: %w", err)
	}

	// 1. Verify ruleset binding matches decision sidecar
	if artifact.RulesetHash != sidecar.Ruleset.Hash {
		return fmt.Errorf("audit ruleset_hash mismatch: audit=%s sidecar=%s", artifact.RulesetHash, sidecar.Ruleset.Hash)
	}
	if artifact.RulesetID != sidecar.Ruleset.ID {
		return fmt.Errorf("audit ruleset_id mismatch: audit=%s sidecar=%s", artifact.RulesetID, sidecar.Ruleset.ID)
	}

	// 2. Verify document hashes match snapshot
	snapshotData, err := os.ReadFile(snapshotPath)
	if err != nil {
		return fmt.Errorf("read snapshot for document verification: %w", err)
	}
	var snapshot struct {
		Documents []struct {
			Hash string `json:"hash"`
		} `json:"documents"`
	}
	if err := json.Unmarshal(snapshotData, &snapshot); err != nil {
		return fmt.Errorf("parse snapshot for document verification: %w", err)
	}
	snapshotHashes := make(map[string]bool)
	for _, d := range snapshot.Documents {
		snapshotHashes[d.Hash] = true
	}
	for _, doc := range artifact.Documents {
		if doc.Hash != "" && !snapshotHashes[doc.Hash] {
			return fmt.Errorf("audit document hash %s not found in snapshot", doc.Hash)
		}
	}

	// 3. Verify final disposition consistency with decision status
	if len(sidecar.Findings) > 0 {
		hasReview := false
		for _, f := range sidecar.Findings {
			if f.Status == "REVIEW" {
				hasReview = true
			}
		}
		switch sidecar.Status {
		case "PASS":
			if artifact.FinalDisposition != "" && artifact.FinalDisposition != "PASS" {
				return fmt.Errorf("audit final_disposition=%s inconsistent with decision status=PASS", artifact.FinalDisposition)
			}
		case "REVIEW":
			if hasReview && artifact.FinalDisposition != "FAIL" && artifact.FinalDisposition != "REVIEW" {
				return fmt.Errorf("audit final_disposition=%s inconsistent with REVIEW decision", artifact.FinalDisposition)
			}
		case "FAIL":
			if artifact.FinalDisposition != "FAIL" {
				return fmt.Errorf("audit final_disposition=%s inconsistent with FAIL decision", artifact.FinalDisposition)
			}
		}
	}

	// 4. Verify human_reviews[] linkage to review sidecar
	if _, err := os.Stat(reviewsPath); err == nil {
		sc, err := review.LoadReviewsSidecar(reviewsPath)
		if err != nil {
			return fmt.Errorf("load reviews sidecar for audit verification: %w", err)
		}
		// Build lookup of sidecar records by finding_index
		sidecarRecords := make(map[int]review.SignedReview)
		for _, rec := range sc.Records {
			sidecarRecords[rec.FindingIndex] = rec
		}
		for i, hr := range artifact.HumanReviews {
			rec, ok := sidecarRecords[hr.FindingIndex]
			if !ok {
				return fmt.Errorf("audit human_reviews[%d] finding_index %d not found in review sidecar", i, hr.FindingIndex)
			}
			if hr.Action != string(rec.Action) {
				return fmt.Errorf("audit human_reviews[%d] action mismatch: audit=%s sidecar=%s", i, hr.Action, rec.Action)
			}
			if hr.ReviewerIdentity != "" && hr.ReviewerIdentity != rec.ReviewerIdentity {
				return fmt.Errorf("audit human_reviews[%d] reviewer_identity mismatch: audit=%s sidecar=%s", i, hr.ReviewerIdentity, rec.ReviewerIdentity)
			}
		}
	}

	// 5. Verify artifact_hash (recompute from canonical content)
	recomputed := recomputeAuditHash(&artifact)
	if recomputed != artifact.Manifest.ArtifactHash {
		return fmt.Errorf("audit artifact_hash mismatch: expected %s, got %s", artifact.Manifest.ArtifactHash, recomputed)
	}

	fmt.Printf("  audit artifact: VERIFIED (hash=%s...)\n", artifact.Manifest.ArtifactHash[:16])
	return nil
}

func recomputeAuditHash(a *auditArtifact) string {
	h := hashableAudit{
		Version:          a.Version,
		PolicyID:         a.PolicyID,
		PolicyHash:       a.PolicyHash,
		RulesetID:        a.RulesetID,
		RulesetVersion:   a.RulesetVersion,
		RulesetHash:      a.RulesetHash,
		Decisions:        a.Decisions,
		Documents:        a.Documents,
		HumanReviews:     a.HumanReviews,
		FinalDisposition: a.FinalDisposition,
		CreatedAt:        a.CreatedAt,
		CompletedAt:      a.CompletedAt,
	}
	h.Manifest.DocCount = a.Manifest.DocCount
	h.Manifest.DecisionCount = a.Manifest.DecisionCount
	h.Manifest.ReviewCount = a.Manifest.ReviewCount
	data, _ := json.Marshal(h)
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// copy of io functions we need
func init() {
	// ensure io is used
	_ = io.EOF
}
