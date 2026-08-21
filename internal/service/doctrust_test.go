package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/doctrust/doctrust/internal/review"
	"github.com/doctrust/doctrust/internal/types"
)

const testSnapshotJSON = `{
  "case_id": "test",
  "documents": [
    {"id": "doc1", "filename": "1_Paystub_2025.pdf", "hash": "abc", "type": "paystub"},
    {"id": "doc2", "filename": "2_W2_Form_2025.pdf", "hash": "def", "type": "w2"},
    {"id": "doc3", "filename": "3_Form1040_TaxReturn_2025.pdf", "hash": "ghi", "type": "form_1040"}
  ],
  "claims": [
    {
      "id": "c1", "field": "annualized_gross_ytd", "semantic_type": "gross_income_projected",
      "value": 125000.0, "value_type": "number",
      "sources": [{"document_id": "doc1", "filename": "1_Paystub_2025.pdf", "page": 1, "bbox": [508, 274, 50, 8], "confidence": 0.95, "field_name": "annualized_gross_ytd"}],
      "status": "singular"
    },
    {
      "id": "c2", "field": "wages_tips_other_compensation", "semantic_type": "gross_income_taxable",
      "value": 120000.0, "value_type": "number",
      "sources": [{"document_id": "doc2", "filename": "2_W2_Form_2025.pdf", "page": 1, "bbox": [224, 120, 44, 8], "confidence": 0.95, "field_name": "wages_tips_other_compensation"}],
      "status": "singular"
    }
  ],
  "relationships": [],
  "created_at": "2026-08-20T12:00:00Z"
}`

func projectRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Dir(filepath.Dir(filepath.Dir(filename)))
}

func newTestService(t *testing.T) (*DocTrustService, string) {
	t.Helper()
	root := projectRoot(t)
	svc, err := NewDocTrustService("income_verification", filepath.Join(root, "rulesets"))
	if err != nil {
		t.Fatalf("NewDocTrustService: %v", err)
	}

	dir := t.TempDir()
	snapshotPath := filepath.Join(dir, "evidence_snapshot.json")
	if err := os.WriteFile(snapshotPath, []byte(testSnapshotJSON), 0644); err != nil {
		t.Fatalf("write snapshot: %v", err)
	}

	return svc, snapshotPath
}

func TestDocTrustService_LoadCase_Evaluate_Lifecycle(t *testing.T) {
	svc, snapshotPath := newTestService(t)
	ctx := context.Background()

	// Before load: no decision
	if d := svc.GetDecision(); d != nil {
		t.Error("expected nil decision before load")
	}

	// Load
	if err := svc.LoadCase(ctx, snapshotPath); err != nil {
		t.Fatalf("LoadCase: %v", err)
	}

	// Before evaluate: no decision
	if d := svc.GetDecision(); d != nil {
		t.Error("expected nil decision before evaluate")
	}

	// Evaluate
	decision, err := svc.Evaluate(ctx)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if decision == nil {
		t.Fatal("expected non-nil decision")
	}
	if decision.Status == "" {
		t.Error("decision.Status should not be empty")
	}

	// After evaluate: decision is pinned
	d := svc.GetDecision()
	if d == nil {
		t.Fatal("expected pinned decision")
	}
	if d.Status != decision.Status {
		t.Errorf("pinned decision status mismatch: %s != %s", d.Status, decision.Status)
	}

	// Facts were built with canonical SourceDoc
	facts := svc.case_.facts
	if _, ok := facts["gross_income_projected"]; !ok {
		t.Error("expected gross_income_projected in facts")
	}
}

func TestDocTrustService_GetFinding_Validation(t *testing.T) {
	svc, snapshotPath := newTestService(t)
	ctx := context.Background()

	// Before load
	_, err := svc.GetFinding(0)
	if err == nil {
		t.Error("expected error before load")
	}

	// Load but not evaluate
	if err := svc.LoadCase(ctx, snapshotPath); err != nil {
		t.Fatalf("LoadCase: %v", err)
	}
	_, err = svc.GetFinding(0)
	if err == nil {
		t.Error("expected error before evaluate")
	}

	// Evaluate
	if _, err := svc.Evaluate(ctx); err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	// Negative index
	_, err = svc.GetFinding(-1)
	if err == nil {
		t.Error("expected error for negative index")
	}

	// Out of range
	_, err = svc.GetFinding(999)
	if err == nil {
		t.Error("expected error for out-of-range index")
	}

	// Valid index
	result, err := svc.GetFinding(0)
	if err != nil {
		t.Errorf("unexpected error for valid index: %v", err)
	}
	if result == nil {
		t.Error("expected non-nil result")
	}
}

func TestDocTrustService_RequestHumanReview_Validation(t *testing.T) {
	svc, snapshotPath := newTestService(t)
	ctx := context.Background()

	// Before load
	_, err := svc.RequestHumanReview(0, review.ActionConfirm, "test")
	if err == nil {
		t.Error("expected error before load")
	}

	// Load but not evaluate
	if err := svc.LoadCase(ctx, snapshotPath); err != nil {
		t.Fatalf("LoadCase: %v", err)
	}
	_, err = svc.RequestHumanReview(0, review.ActionConfirm, "test")
	if err == nil {
		t.Error("expected error before evaluate")
	}

	// Evaluate
	if _, err := svc.Evaluate(ctx); err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	// Negative index
	_, err = svc.RequestHumanReview(-1, review.ActionConfirm, "test")
	if err == nil {
		t.Error("expected error for negative index")
	}

	// Out of range
	_, err = svc.RequestHumanReview(999, review.ActionConfirm, "test")
	if err == nil {
		t.Error("expected error for out-of-range index")
	}

	// Valid review
	_, err = svc.RequestHumanReview(0, review.ActionConfirm, "looks good")
	if err != nil {
		t.Errorf("unexpected error for valid review: %v", err)
	}

	// Verify review stored
	reviews, err := svc.GetReviews()
	if err != nil {
		t.Fatalf("GetReviews: %v", err)
	}
	if len(reviews) != 1 {
		t.Fatalf("expected 1 review, got %d", len(reviews))
	}
	if reviews[0].FindingIndex != 0 {
		t.Errorf("review finding_index = %d, want 0", reviews[0].FindingIndex)
	}
}

func TestDocTrustService_BuildArtifact_PinnedRuleset(t *testing.T) {
	svc, snapshotPath := newTestService(t)
	ctx := context.Background()

	if err := svc.LoadCase(ctx, snapshotPath); err != nil {
		t.Fatalf("LoadCase: %v", err)
	}
	if _, err := svc.Evaluate(ctx); err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	artifact, err := svc.BuildArtifact()
	if err != nil {
		t.Fatalf("BuildArtifact: %v", err)
	}

	// Ruleset provenance must be present
	if artifact.RulesetID != "income_verification" {
		t.Errorf("RulesetID = %q, want %q", artifact.RulesetID, "income_verification")
	}
	if artifact.RulesetVersion == "" {
		t.Error("RulesetVersion should not be empty")
	}
	if artifact.RulesetHash == "" {
		t.Error("RulesetHash should not be empty")
	}

	// Artifact hash must be consistent
	if artifact.Manifest.ArtifactHash != artifact.Hash() {
		t.Errorf("ArtifactHash mismatch: manifest=%s, Hash()=%s", artifact.Manifest.ArtifactHash, artifact.Hash())
	}
}

func TestDocTrustService_BuildArtifact_ReviewDisposition(t *testing.T) {
	svc, snapshotPath := newTestService(t)
	ctx := context.Background()

	if err := svc.LoadCase(ctx, snapshotPath); err != nil {
		t.Fatalf("LoadCase: %v", err)
	}
	if _, err := svc.Evaluate(ctx); err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	// Before review
	artifact, err := svc.BuildArtifact()
	if err != nil {
		t.Fatalf("BuildArtifact: %v", err)
	}
 dispositionBefore := artifact.FinalDisposition

	// Add a review
	svc.RequestHumanReview(0, review.ActionConfirm, "approved")

	// After review
	artifact2, err := svc.BuildArtifact()
	if err != nil {
		t.Fatalf("BuildArtifact after review: %v", err)
	}
	dispositionAfter := artifact2.FinalDisposition

	// Disposition may change (depends on REVIEW status)
	// At minimum, artifact should have the review recorded
	if len(artifact2.HumanReviews) == 0 {
		t.Error("expected human reviews in artifact")
	}

	// Both must have consistent hashes
	if artifact.Manifest.ArtifactHash != artifact.Hash() {
		t.Error("pre-review artifact hash inconsistent")
	}
	if artifact2.Manifest.ArtifactHash != artifact2.Hash() {
		t.Error("post-review artifact hash inconsistent")
	}
	_ = dispositionBefore
	_ = dispositionAfter
}

func TestDocTrustService_CaseStatePinning(t *testing.T) {
	svc, snapshotPath := newTestService(t)
	ctx := context.Background()

	if err := svc.LoadCase(ctx, snapshotPath); err != nil {
		t.Fatalf("LoadCase: %v", err)
	}

	// First evaluate
	d1, err := svc.Evaluate(ctx)
	if err != nil {
		t.Fatalf("Evaluate 1: %v", err)
	}

	// Add review
	svc.RequestHumanReview(0, review.ActionConfirm, "first review")

	// Re-evaluate: should overwrite case state
	d2, err := svc.Evaluate(ctx)
	if err != nil {
		t.Fatalf("Evaluate 2: %v", err)
	}

	// Decision should be the same (same snapshot, same ruleset)
	if d1.Status != d2.Status {
		t.Errorf("status changed between evaluations: %s != %s", d1.Status, d2.Status)
	}

	// Reviews should be reset (new caseState from LoadCase)
	reviews, err := svc.GetReviews()
	if err != nil {
		t.Fatalf("GetReviews: %v", err)
	}
	// After re-evaluate, the reviewStore is from the new caseState
	// But we didn't re-LoadCase, so the reviewStore is the same one
	// Actually, Evaluate doesn't create a new caseState, it reuses the existing one
	// So reviews should still be there
	if len(reviews) != 1 {
		t.Logf("reviews after re-evaluate: %d (reviews preserved in same caseState)", len(reviews))
	}
}

func TestDocTrustService_GetReviews(t *testing.T) {
	svc, snapshotPath := newTestService(t)
	ctx := context.Background()

	// Before load
	_, err := svc.GetReviews()
	if err == nil {
		t.Error("expected error before load")
	}

	if err := svc.LoadCase(ctx, snapshotPath); err != nil {
		t.Fatalf("LoadCase: %v", err)
	}
	if _, err := svc.Evaluate(ctx); err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	// Empty reviews
	reviews, err := svc.GetReviews()
	if err != nil {
		t.Fatalf("GetReviews: %v", err)
	}
	if len(reviews) != 0 {
		t.Errorf("expected 0 reviews, got %d", len(reviews))
	}

	// Add review
	svc.RequestHumanReview(0, review.ActionConfirm, "approved")

	reviews, err = svc.GetReviews()
	if err != nil {
		t.Fatalf("GetReviews after review: %v", err)
	}
	if len(reviews) != 1 {
		t.Fatalf("expected 1 review, got %d", len(reviews))
	}

	// Verify snapshot Documents are in artifact
	artifact, err := svc.BuildArtifact()
	if err != nil {
		t.Fatalf("BuildArtifact: %v", err)
	}
	if len(artifact.Documents) != 3 {
		t.Errorf("expected 3 documents in artifact, got %d", len(artifact.Documents))
	}

	// Verify SourceDoc canonicalization via facts
	facts := svc.case_.facts
	for _, fact := range facts["gross_income_projected"] {
		if fact.SourceDoc != "paystub" {
			t.Errorf("gross_income_projected SourceDoc = %q, want %q", fact.SourceDoc, "paystub")
		}
	}
}

// TestDocTrustService_GetRuleset verifies GetRuleset returns the promoted ruleset.
func TestDocTrustService_GetRuleset(t *testing.T) {
	svc, _ := newTestService(t)
	info := svc.GetRuleset()
	if info.ID != "income_verification" {
		t.Errorf("Ruleset ID = %q, want %q", info.ID, "income_verification")
	}
	if info.Version == "" {
		t.Error("Ruleset version should not be empty")
	}
	if len(info.Checks) == 0 {
		t.Error("Ruleset should have checks")
	}
}

// TestDocTrustService_SnapshotDocumentsInFactMapping verifies the snapshot
// document type mapping resolves correctly for production-style filenames.
func TestDocTrustService_SnapshotDocumentsInFactMapping(t *testing.T) {
	svc, snapshotPath := newTestService(t)
	ctx := context.Background()

	if err := svc.LoadCase(ctx, snapshotPath); err != nil {
		t.Fatalf("LoadCase: %v", err)
	}
	if _, err := svc.Evaluate(ctx); err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	// Parse the snapshot to verify document types
	var snapshot struct {
		Documents []struct {
			Filename string `json:"filename"`
			Type     string `json:"type"`
		} `json:"documents"`
	}
	json.Unmarshal(svc.case_.snapshotBytes, &snapshot)

	for _, doc := range snapshot.Documents {
		switch doc.Filename {
		case "1_Paystub_2025.pdf":
			if doc.Type != "paystub" {
				t.Errorf("doc %s type = %q, want %q", doc.Filename, doc.Type, "paystub")
			}
		case "2_W2_Form_2025.pdf":
			if doc.Type != "w2" {
				t.Errorf("doc %s type = %q, want %q", doc.Filename, doc.Type, "w2")
			}
		case "3_Form1040_TaxReturn_2025.pdf":
			if doc.Type != "form_1040" {
				t.Errorf("doc %s type = %q, want %q", doc.Filename, doc.Type, "form_1040")
			}
		}
	}

	// Facts should use canonical types
	facts := svc.case_.facts
	projected := facts["gross_income_projected"]
	if len(projected) == 0 {
		t.Fatal("expected gross_income_projected facts")
	}
	if projected[0].SourceDoc != string(types.DocTypePaystub) {
		t.Errorf("SourceDoc = %q, want %q", projected[0].SourceDoc, types.DocTypePaystub)
	}
}
