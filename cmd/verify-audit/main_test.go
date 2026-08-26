package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/PithomLabs/doctrust/internal/review"
)

func TestVerifyAuditArtifact_TamperFinalDisposition(t *testing.T) {
	dir := t.TempDir()
	snapshotPath := filepath.Join(dir, "evidence_snapshot.json")
	decisionPath := snapshotPath + ".decision.json"
	auditPath := snapshotPath + ".audit.json"
	reviewsPath := snapshotPath + ".doctrust_reviews.json"

	// Create minimal snapshot
	snapshot := map[string]interface{}{
		"case_id": "test123",
		"documents": []map[string]interface{}{
			{"hash": "abc123", "document_type": "test"},
		},
	}
	writeJSON(t, snapshotPath, snapshot)

	// Create decision sidecar
	sidecar := DecisionSidecar{
		LoadcaseID:     "test123",
		SnapshotSHA256: "test1234567890abcd",
		Status:         "REVIEW",
		Findings: []SidecarFinding{
			{Index: 0, CheckID: "test", Status: "REVIEW", Severity: "BLOCKING", Reason: "test"},
		},
		Ruleset: review.RuleBinding{ID: "test", Version: "1", Hash: "hash123"},
	}
	writeJSON(t, decisionPath, sidecar)

	// Create audit artifact with PASS disposition (inconsistent with REVIEW)
	audit := map[string]interface{}{
		"version":          "1.0",
		"ruleset_id":       "test",
		"ruleset_version":  "1",
		"ruleset_hash":     "hash123",
		"final_disposition": "PASS",
		"decisions":        []interface{}{},
		"documents":        []interface{}{},
		"manifest": map[string]interface{}{
			"doc_count":      0,
			"decision_count": 0,
			"review_count":   0,
			"artifact_hash":  "placeholder",
		},
	}
	// Compute correct hash for the audit
	audit["manifest"].(map[string]interface{})["artifact_hash"] = computeTestAuditHash(audit)
	writeJSON(t, auditPath, audit)

	// verify-audit should fail on inconsistent disposition
	err := verifyAuditArtifact(auditPath, &sidecar, snapshotPath, reviewsPath)
	if err == nil {
		t.Fatal("expected error for inconsistent final_disposition, got nil")
	}
	if err.Error() != "audit final_disposition=PASS inconsistent with REVIEW decision" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestVerifyAuditArtifact_TamperArtifactHash(t *testing.T) {
	dir := t.TempDir()
	snapshotPath := filepath.Join(dir, "evidence_snapshot.json")
	decisionPath := snapshotPath + ".decision.json"
	auditPath := snapshotPath + ".audit.json"
	reviewsPath := snapshotPath + ".doctrust_reviews.json"

	snapshot := map[string]interface{}{
		"case_id": "test123",
		"documents": []map[string]interface{}{
			{"hash": "abc123", "document_type": "test"},
		},
	}
	writeJSON(t, snapshotPath, snapshot)

	sidecar := DecisionSidecar{
		LoadcaseID:     "test123",
		SnapshotSHA256: "test1234567890abcd",
		Status:         "PASS",
		Findings: []SidecarFinding{
			{Index: 0, CheckID: "test", Status: "PASS", Severity: "INFO", Reason: "ok"},
		},
		Ruleset: review.RuleBinding{ID: "test", Version: "1", Hash: "hash123"},
	}
	writeJSON(t, decisionPath, sidecar)

	audit := map[string]interface{}{
		"version":          "1.0",
		"ruleset_id":       "test",
		"ruleset_version":  "1",
		"ruleset_hash":     "hash123",
		"final_disposition": "PASS",
		"decisions":        []interface{}{},
		"documents":        []interface{}{},
		"manifest": map[string]interface{}{
			"doc_count":      0,
			"decision_count": 0,
			"review_count":   0,
			"artifact_hash":  "wrong_hash_value",
		},
	}
	writeJSON(t, auditPath, audit)

	err := verifyAuditArtifact(auditPath, &sidecar, snapshotPath, reviewsPath)
	if err == nil {
		t.Fatal("expected error for tampered artifact_hash, got nil")
	}
}

func TestVerifyAuditArtifact_TamperRulesetHash(t *testing.T) {
	dir := t.TempDir()
	snapshotPath := filepath.Join(dir, "evidence_snapshot.json")
	decisionPath := snapshotPath + ".decision.json"
	auditPath := snapshotPath + ".audit.json"
	reviewsPath := snapshotPath + ".doctrust_reviews.json"

	snapshot := map[string]interface{}{
		"case_id": "test123",
		"documents": []map[string]interface{}{
			{"hash": "abc123", "document_type": "test"},
		},
	}
	writeJSON(t, snapshotPath, snapshot)

	sidecar := DecisionSidecar{
		LoadcaseID:     "test123",
		SnapshotSHA256: "test1234567890abcd",
		Status:         "PASS",
		Findings: []SidecarFinding{
			{Index: 0, CheckID: "test", Status: "PASS", Severity: "INFO", Reason: "ok"},
		},
		Ruleset: review.RuleBinding{ID: "test", Version: "1", Hash: "hash123"},
	}
	writeJSON(t, decisionPath, sidecar)

	audit := map[string]interface{}{
		"version":          "1.0",
		"ruleset_id":       "test",
		"ruleset_version":  "1",
		"ruleset_hash":     "different_hash",
		"final_disposition": "PASS",
		"decisions":        []interface{}{},
		"documents":        []interface{}{},
		"manifest": map[string]interface{}{
			"doc_count":      0,
			"decision_count": 0,
			"review_count":   0,
			"artifact_hash":  "placeholder",
		},
	}
	audit["manifest"].(map[string]interface{})["artifact_hash"] = computeTestAuditHash(audit)
	writeJSON(t, auditPath, audit)

	err := verifyAuditArtifact(auditPath, &sidecar, snapshotPath, reviewsPath)
	if err == nil {
		t.Fatal("expected error for tampered ruleset_hash, got nil")
	}
}

func TestVerifyAuditArtifact_TamperReviewAction(t *testing.T) {
	dir := t.TempDir()
	snapshotPath := filepath.Join(dir, "evidence_snapshot.json")
	decisionPath := snapshotPath + ".decision.json"
	auditPath := snapshotPath + ".audit.json"
	reviewsPath := snapshotPath + ".doctrust_reviews.json"

	snapshot := map[string]interface{}{
		"case_id": "test123",
		"documents": []map[string]interface{}{
			{"hash": "abc123", "document_type": "test"},
		},
	}
	writeJSON(t, snapshotPath, snapshot)

	sidecar := DecisionSidecar{
		LoadcaseID:     "test123",
		SnapshotSHA256: "test1234567890abcd",
		Status:         "REVIEW",
		Findings: []SidecarFinding{
			{Index: 0, CheckID: "test", Status: "REVIEW", Severity: "BLOCKING", Reason: "test"},
		},
		Ruleset: review.RuleBinding{ID: "test", Version: "1", Hash: "hash123"},
	}
	writeJSON(t, decisionPath, sidecar)

	// Create a valid signing key
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	// Write public key to reviewers ring
	ringDir := filepath.Join(dir, "reviewers")
	os.MkdirAll(ringDir, 0o755)
	os.WriteFile(filepath.Join(ringDir, "owner.pub"), []byte(base64.StdEncoding.EncodeToString(pub)), 0o644)

	// Create signed review record
	rec := review.SignedReview{
		CaseID:           "test123",
		SnapshotSHA256:   "test1234567890abcd",
		FindingIndex:     0,
		Action:           "confirm",
		Note:             "test note",
		ReviewerIdentity: "owner",
		Channel:          "human-tty",
		KeyID:            "owner",
		Alg:              "ed25519",
		Ruleset:          review.RuleBinding{ID: "test", Version: "1", Hash: "hash123"},
		ResolvedAt:       time.Now().UTC(),
	}
	if err := review.SignRecord(priv, &rec, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	sidecarRecords := &review.ReviewsSidecar{
		CaseID:  "test123",
		Records: []review.SignedReview{rec},
	}
	reviewData, _ := json.Marshal(sidecarRecords)
	os.WriteFile(reviewsPath, reviewData, 0o644)

	// Create audit with mismatched action
	audit := map[string]interface{}{
		"version":          "1.0",
		"ruleset_id":       "test",
		"ruleset_version":  "1",
		"ruleset_hash":     "hash123",
		"final_disposition": "REVIEW",
		"decisions":        []interface{}{},
		"documents":        []interface{}{},
		"human_reviews": []map[string]interface{}{
			{
				"finding_index":     0,
				"action":            "reject",
				"note":              "test note",
				"resolved_at":       rec.ResolvedAt.Format(time.RFC3339Nano),
				"reviewer_identity": "owner",
				"channel":           "human-tty",
			},
		},
		"manifest": map[string]interface{}{
			"doc_count":      0,
			"decision_count": 0,
			"review_count":   0,
			"artifact_hash":  "placeholder",
		},
	}
	audit["manifest"].(map[string]interface{})["artifact_hash"] = computeTestAuditHash(audit)
	writeJSON(t, auditPath, audit)

	err = verifyAuditArtifact(auditPath, &sidecar, snapshotPath, reviewsPath)
	if err == nil {
		t.Fatal("expected error for tampered review action, got nil")
	}
}

func TestVerifyAuditArtifact_Pass(t *testing.T) {
	dir := t.TempDir()
	snapshotPath := filepath.Join(dir, "evidence_snapshot.json")
	decisionPath := snapshotPath + ".decision.json"
	auditPath := snapshotPath + ".audit.json"
	reviewsPath := snapshotPath + ".doctrust_reviews.json"

	snapshot := map[string]interface{}{
		"case_id": "test123",
		"documents": []map[string]interface{}{
			{"hash": "abc123", "document_type": "test"},
		},
	}
	writeJSON(t, snapshotPath, snapshot)

	sidecar := DecisionSidecar{
		LoadcaseID:     "test123",
		SnapshotSHA256: "test1234567890abcd",
		Status:         "PASS",
		Findings: []SidecarFinding{
			{Index: 0, CheckID: "test", Status: "PASS", Severity: "INFO", Reason: "ok"},
		},
		Ruleset: review.RuleBinding{ID: "test", Version: "1", Hash: "hash123"},
	}
	writeJSON(t, decisionPath, sidecar)

	// Create the audit artifact and compute its hash using the production function
	artifact := auditArtifact{
		Version:          "1.0",
		RulesetID:        "test",
		RulesetVersion:   "1",
		RulesetHash:      "hash123",
		FinalDisposition: "PASS",
	}
	artifact.Manifest.ArtifactHash = recomputeAuditHash(&artifact)
	data, _ := json.MarshalIndent(artifact, "", "  ")
	os.WriteFile(auditPath, data, 0o644)

	err := verifyAuditArtifact(auditPath, &sidecar, snapshotPath, reviewsPath)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
}

func writeJSON(t *testing.T, path string, v interface{}) {
	t.Helper()
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(path, data, 0o644)
}

func computeTestAuditHash(audit map[string]interface{}) string {
	// Build the hashable struct to match the real recomputeAuditHash logic
	h := hashableAudit{
		Version:          audit["version"].(string),
		RulesetID:        audit["ruleset_id"].(string),
		RulesetVersion:   audit["ruleset_version"].(string),
		RulesetHash:      audit["ruleset_hash"].(string),
		FinalDisposition: audit["final_disposition"].(string),
	}
	m := audit["manifest"].(map[string]interface{})
	switch v := m["doc_count"].(type) {
	case int:
		h.Manifest.DocCount = v
	case float64:
		h.Manifest.DocCount = int(v)
	}
	switch v := m["decision_count"].(type) {
	case int:
		h.Manifest.DecisionCount = v
	case float64:
		h.Manifest.DecisionCount = int(v)
	}
	switch v := m["review_count"].(type) {
	case int:
		h.Manifest.ReviewCount = v
	case float64:
		h.Manifest.ReviewCount = int(v)
	}
	data, _ := json.Marshal(h)
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}
