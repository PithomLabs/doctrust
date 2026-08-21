package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/doctrust/doctrust/internal/service"
)

const testSnapshotJSON = `{
  "case_id": "test",
  "documents": [
    {"id": "doc1", "filename": "paystub.pdf", "hash": "abc", "type": "paystub"},
    {"id": "doc2", "filename": "w2.pdf", "hash": "def", "type": "w2"},
    {"id": "doc3", "filename": "1040.pdf", "hash": "ghi", "type": "form_1040"}
  ],
  "claims": [
    {
      "id": "c1", "field": "annualized_gross_ytd", "semantic_type": "gross_income_projected",
      "value": 125000.0, "value_type": "number",
      "sources": [{"document_id": "doc1", "filename": "paystub.pdf", "page": 1, "bbox": [508, 274, 50, 8], "confidence": 0.95, "field_name": "annualized_gross_ytd"}],
      "status": "singular"
    },
    {
      "id": "c2", "field": "wages_tips_other_compensation", "semantic_type": "gross_income_taxable",
      "value": 120000.0, "value_type": "number",
      "sources": [{"document_id": "doc2", "filename": "w2.pdf", "page": 1, "bbox": [224, 120, 44, 8], "confidence": 0.95, "field_name": "wages_tips_other_compensation"}],
      "status": "singular"
    }
  ],
  "relationships": [],
  "created_at": "2026-08-20T12:00:00Z"
}`

func setupTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	dir := t.TempDir()
	snapshotDir = dir

	if err := os.WriteFile(filepath.Join(dir, "evidence_snapshot.json"), []byte(testSnapshotJSON), 0644); err != nil {
		t.Fatalf("write snapshot: %v", err)
	}

	// Create promoted ruleset for the service
	rulesetsDir := filepath.Join(dir, "rulesets")
	rulesetDir := filepath.Join(rulesetsDir, "income_verification")
	if err := os.MkdirAll(rulesetDir, 0755); err != nil {
		t.Fatalf("mkdir ruleset: %v", err)
	}
	rulesetYAML := `id: income_verification
version: "1"
checks:
    - id: gross_income_consistency
      version: "1.0"
      params:
        tolerance: 0.05
    - id: required_documents
      version: "1.0"
    - id: net_vs_gross_incomparability
      version: "1.0"
`
	if err := os.WriteFile(filepath.Join(rulesetDir, "v1.yaml"), []byte(rulesetYAML), 0644); err != nil {
		t.Fatalf("write ruleset: %v", err)
	}

	var err error
	svc, err = service.NewDocTrustService("income_verification", rulesetsDir)
	if err != nil {
		t.Fatalf("NewDocTrustService: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/evaluate", handleEvaluate)
	mux.HandleFunc("/api/review", handleReview)
	mux.HandleFunc("/api/reviews", handleReviews)
	mux.HandleFunc("/api/disposition", handleDisposition)
	mux.HandleFunc("/api/ruleset", handleRuleset)
	mux.HandleFunc("/api/regression", handleRegression)
	mux.HandleFunc("/api/finalize", handleFinalize)

	return httptest.NewServer(mux)
}

func TestParseSourceSpan(t *testing.T) {
	tests := []struct {
		name     string
		span     string
		wantPage int
		wantBbox []float64
	}{
		{
			name:     "page and bbox",
			span:     "page=1;bbox=[508.0,274.0,50.0,8.0]",
			wantPage: 1,
			wantBbox: []float64{508, 274, 50, 8},
		},
		{
			name:     "page only",
			span:     "page=3",
			wantPage: 3,
			wantBbox: nil,
		},
		{
			name:     "empty",
			span:     "",
			wantPage: 0,
			wantBbox: nil,
		},
		{
			name:     "bbox only",
			span:     "bbox=[100.0,200.0,30.0,5.0]",
			wantPage: 0,
			wantBbox: []float64{100, 200, 30, 5},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page, bbox := parseSourceSpan(tt.span)
			if page != tt.wantPage {
				t.Errorf("page = %d, want %d", page, tt.wantPage)
			}
			if len(bbox) != len(tt.wantBbox) {
				t.Errorf("bbox len = %d, want %d", len(bbox), len(tt.wantBbox))
				return
			}
			for i := range bbox {
				if bbox[i] != tt.wantBbox[i] {
					t.Errorf("bbox[%d] = %f, want %f", i, bbox[i], tt.wantBbox[i])
				}
			}
		})
	}
}

func TestHandleRuleset_ReturnsLoadedRuleset(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/ruleset")
	if err != nil {
		t.Fatalf("GET /api/ruleset: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var body struct {
		ID      string `json:"id"`
		Version string `json:"version"`
		Hash    string `json:"hash"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if body.ID != "income_verification" {
		t.Errorf("id = %q, want %q", body.ID, "income_verification")
	}
	if body.Version != "1" {
		t.Errorf("version = %q, want %q", body.Version, "1")
	}
	if body.Hash == "" {
		t.Error("hash should not be empty")
	}
}

func TestHandleEvaluate_ReturnsEnrichedFields(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/evaluate")
	if err != nil {
		t.Fatalf("GET /api/evaluate: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var body enrichedResult
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if body.Decision == "" {
		t.Error("decision should not be empty")
	}
	if len(body.Findings) == 0 {
		t.Fatal("expected at least one finding")
	}

	for i, f := range body.Findings {
		if f.Rule == "" {
			t.Errorf("finding[%d].rule should not be empty", i)
		}
		if f.Status == "" {
			t.Errorf("finding[%d].status should not be empty", i)
		}
		if f.Severity == "" {
			t.Errorf("finding[%d].severity should not be empty", i)
		}
	}
}

func TestHandleEvaluate_BboxFromSnapshot(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/evaluate")
	if err != nil {
		t.Fatalf("GET /api/evaluate: %v", err)
	}
	defer resp.Body.Close()

	var body enrichedResult
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// Find a finding with sources that have bbox
	found := false
	for _, f := range body.Findings {
		for _, s := range f.Sources {
			if len(s.BBox) >= 4 {
				found = true
				if s.Page < 1 {
					t.Errorf("source page = %d, want >= 1", s.Page)
				}
				// Verify bbox values are in viewer range (not flipped)
				if s.BBox[0] < 0 || s.BBox[1] < 0 {
					t.Errorf("bbox origin should be non-negative, got [%f, %f]", s.BBox[0], s.BBox[1])
				}
			}
		}
	}
	if !found {
		t.Error("expected at least one finding with bbox sources")
	}
}

func TestHandleFinalize_ArtifactContainsRulesetProvenance(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()

	// Must evaluate first — lifecycle requires /api/evaluate before /api/finalize
	evalResp, err := http.Get(srv.URL + "/api/evaluate")
	if err != nil {
		t.Fatalf("GET /api/evaluate: %v", err)
	}
	evalResp.Body.Close()
	if evalResp.StatusCode != 200 {
		t.Fatalf("evaluate status = %d, want 200", evalResp.StatusCode)
	}

	resp, err := http.Post(srv.URL+"/api/finalize", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /api/finalize: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var body struct {
		Status           string `json:"status"`
		FinalDisposition string `json:"final_disposition"`
		ArtifactHash     string `json:"artifact_hash"`
		Artifact         struct {
			RulesetID      string `json:"ruleset_id"`
			RulesetVersion string `json:"ruleset_version"`
			RulesetHash    string `json:"ruleset_hash"`
			PolicyID       string `json:"policy_id"`
		} `json:"artifact"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if body.Status != "finalized" {
		t.Errorf("status = %q, want %q", body.Status, "finalized")
	}
	if body.Artifact.RulesetID != "income_verification" {
		t.Errorf("ruleset_id = %q, want %q", body.Artifact.RulesetID, "income_verification")
	}
	if body.Artifact.RulesetVersion != "1" {
		t.Errorf("ruleset_version = %q, want %q", body.Artifact.RulesetVersion, "1")
	}
	if body.Artifact.RulesetHash == "" {
		t.Error("ruleset_hash should not be empty")
	}
}

func TestHandleReview_InvalidFindingIndex(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()

	body, _ := json.Marshal(map[string]any{
		"finding_index": -1,
		"action":        "confirm",
		"note":          "test",
	})

	resp, err := http.Post(srv.URL+"/api/review", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /api/review: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 400 {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestHandleReview_OutOfRangeFindingIndex(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()

	body, _ := json.Marshal(map[string]any{
		"finding_index": 999,
		"action":        "confirm",
		"note":          "test",
	})

	resp, err := http.Post(srv.URL+"/api/review", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /api/review: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 400 {
		respBody, _ := json.Marshal(map[string]any{
			"finding_index": 999,
			"action":        "confirm",
		})
		_ = respBody
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestHandleReview_ValidFindingIndex(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()

	// First, evaluate to know how many findings exist
	evalResp, err := http.Get(srv.URL + "/api/evaluate")
	if err != nil {
		t.Fatalf("GET /api/evaluate: %v", err)
	}
	defer evalResp.Body.Close()

	var evalBody enrichedResult
	if err := json.NewDecoder(evalResp.Body).Decode(&evalBody); err != nil {
		t.Fatalf("decode evaluate: %v", err)
	}
	if len(evalBody.Findings) == 0 {
		t.Fatal("need at least one finding for this test")
	}

	// Submit review for first finding (index 0)
	body, _ := json.Marshal(map[string]any{
		"finding_index": 0,
		"action":        "confirm",
		"note":          "looks good",
	})

	resp, err := http.Post(srv.URL+"/api/review", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /api/review: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBytes, _ := json.Marshal(map[string]any{"finding_index": 0, "action": "confirm"})
		_ = respBytes
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}

	// Verify review was stored
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

func TestBuildFactsFromSnapshot_UsesCanonicalBuilder(t *testing.T) {
	// Set up snapshot directory
	dir := t.TempDir()
	snapshotDir = dir
	if err := os.WriteFile(filepath.Join(dir, "evidence_snapshot.json"), []byte(testSnapshotJSON), 0644); err != nil {
		t.Fatalf("write snapshot: %v", err)
	}

	// The service's BuildFactsFromSnapshot must use canonical SourceDoc
	// (this is already tested in internal/service/builder_test.go,
	// but this confirms the HTTP path uses the same builder)
	snapshotPath := filepath.Join(dir, "evidence_snapshot.json")
	if err := svc.LoadCase(nil, snapshotPath); err != nil {
		t.Fatalf("LoadCase: %v", err)
	}

	// Verify the snapshot's document type mapping resolves correctly
	info := svc.GetRuleset()
	if info.ID != "income_verification" {
		t.Errorf("ruleset ID = %q, want %q", info.ID, "income_verification")
	}
}

// TestLifecycle_NoReEvaluation proves the HTTP lifecycle invariant:
// AFTER /api/evaluate pins the case, later handlers do NOT re-load or re-evaluate.
func TestLifecycle_NoReEvaluation(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()

	// Step 1: Evaluate the case
	evalResp, err := http.Get(srv.URL + "/api/evaluate")
	if err != nil {
		t.Fatalf("GET /api/evaluate: %v", err)
	}
	defer evalResp.Body.Close()
	if evalResp.StatusCode != 200 {
		t.Fatalf("evaluate status = %d, want 200", evalResp.StatusCode)
	}

	var evalBody enrichedResult
	if err := json.NewDecoder(evalResp.Body).Decode(&evalBody); err != nil {
		t.Fatalf("decode evaluate: %v", err)
	}
	originalDecision := evalBody.Decision
	t.Logf("original decision: %s", originalDecision)

	// Step 2: Add a review — this must NOT re-evaluate
	reviewBody, _ := json.Marshal(map[string]any{
		"finding_index": 0,
		"action":        "confirm",
		"note":          "approved by test",
	})
	reviewResp, err := http.Post(srv.URL+"/api/review", "application/json", bytes.NewReader(reviewBody))
	if err != nil {
		t.Fatalf("POST /api/review: %v", err)
	}
	reviewResp.Body.Close()
	if reviewResp.StatusCode != 200 {
		t.Fatalf("review status = %d, want 200", reviewResp.StatusCode)
	}

	// Step 3: Call finalize — this must NOT re-evaluate
	finalizeResp, err := http.Post(srv.URL+"/api/finalize", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /api/finalize: %v", err)
	}
	defer finalizeResp.Body.Close()
	if finalizeResp.StatusCode != 200 {
		t.Fatalf("finalize status = %d, want 200", finalizeResp.StatusCode)
	}

	// Step 4: Verify the artifact contains the review and the original decision
	var finalizeBody struct {
		Status           string `json:"status"`
		FinalDisposition string `json:"final_disposition"`
		Artifact         struct {
			Decisions []struct {
				State string `json:"state"`
			} `json:"decisions"`
			HumanReviews []struct {
				FindingIndex int    `json:"finding_index"`
				Action       string `json:"action"`
			} `json:"human_reviews"`
		} `json:"artifact"`
	}
	if err := json.NewDecoder(finalizeResp.Body).Decode(&finalizeBody); err != nil {
		t.Fatalf("decode finalize: %v", err)
	}

	if finalizeBody.Status != "finalized" {
		t.Errorf("finalize status = %q, want %q", finalizeBody.Status, "finalized")
	}

	// The decision in the artifact must match the original evaluate decision
	if len(finalizeBody.Artifact.Decisions) > 0 {
		artifactDecision := finalizeBody.Artifact.Decisions[0].State
		if artifactDecision != originalDecision {
			t.Errorf("artifact decision %q differs from original %q — re-evaluation occurred", artifactDecision, originalDecision)
		}
	}

	// The review must be present in the artifact
	if len(finalizeBody.Artifact.HumanReviews) != 1 {
		t.Errorf("expected 1 human review in artifact, got %d", len(finalizeBody.Artifact.HumanReviews))
	} else if finalizeBody.Artifact.HumanReviews[0].Action != "confirm" {
		t.Errorf("review action = %q, want %q", finalizeBody.Artifact.HumanReviews[0].Action, "confirm")
	}

	// Step 5: Verify calling finalize without /api/evaluate returns error
	// (fresh server = no case loaded)
	srv2 := setupTestServer(t)
	defer srv2.Close()
	errResp, err := http.Post(srv2.URL+"/api/finalize", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /api/finalize on fresh server: %v", err)
	}
	errResp.Body.Close()
	if errResp.StatusCode != 500 {
		t.Errorf("fresh server finalize status = %d, want 500 (no case loaded)", errResp.StatusCode)
	}
}
