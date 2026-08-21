package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/doctrust/doctrust/internal/eval"
	"github.com/doctrust/doctrust/internal/review"
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

	store = review.NewReviewStore()

	loadedRuleset = eval.Ruleset{
		ID:      "income_verification",
		Version: "1",
		Checks: []eval.CheckRef{
			{ID: "gross_income_consistency", Version: "1.0", Params: map[string]any{"tolerance": 0.05}},
			{ID: "required_documents", Version: "1.0"},
			{ID: "net_vs_gross_incomparability", Version: "1.0"},
		},
	}

	evalRunner = eval.NewRunner(eval.DefaultRegistry().All())
	policyHash = "test_hash_abc123"

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
	reviews := store.GetAll()
	if len(reviews) != 1 {
		t.Fatalf("expected 1 review, got %d", len(reviews))
	}
	if reviews[0].FindingIndex != 0 {
		t.Errorf("review finding_index = %d, want 0", reviews[0].FindingIndex)
	}
}

func TestBuildFactsFromSnapshot(t *testing.T) {
	// Set up snapshot directory directly (no HTTP server needed)
	dir := t.TempDir()
	snapshotDir = dir
	if err := os.WriteFile(filepath.Join(dir, "evidence_snapshot.json"), []byte(testSnapshotJSON), 0644); err != nil {
		t.Fatalf("write snapshot: %v", err)
	}

	snapshot, err := loadSnapshot()
	if err != nil {
		t.Fatalf("loadSnapshot: %v", err)
	}

	facts := buildFactsFromSnapshot(snapshot)

	// Should have gross_income_projected and gross_income_taxable
	projected := facts["gross_income_projected"]
	if len(projected) == 0 {
		t.Fatal("expected gross_income_projected facts")
	}

	taxable := facts["gross_income_taxable"]
	if len(taxable) == 0 {
		t.Fatal("expected gross_income_taxable facts")
	}

	// Check that bbox is in the SourceSpan
	span := projected[0].SourceSpan
	if !strings.Contains(span, "page=1") {
		t.Errorf("SourceSpan should contain page=1, got %q", span)
	}
	if !strings.Contains(span, "bbox=[508.0,274.0,50.0,8.0]") {
		t.Errorf("SourceSpan should contain bbox, got %q", span)
	}
}
