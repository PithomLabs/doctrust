package compiler

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestExecuteCandidateScenarios_Pass(t *testing.T) {
	// Create a candidate with a check that always passes
	candidateDir := createTestCandidate(t, &testCandidateOpts{
		checkID:  "test_always_pass",
		version:  "1.0",
		goSource: alwaysPassCheckGo,
		scenarios: `scenarios:
  - name: "pass_scenario"
    origin: "real_fixture"
    input:
      facts:
        - semantic_type: "test_value"
          source_doc: "test"
          field: "value"
          value: 100
          source_span: "page=1"
          confidence: 0.95
    expected:
      check_id: "test_always_pass"
      status: "PASS"
      severity: "INFO"
      reason: "always passes"
`,
	})

	snapshot, err := SnapshotCandidate(candidateDir)
	if err != nil {
		t.Fatalf("SnapshotCandidate: %v", err)
	}

	result, err := ExecuteCandidateScenarios(snapshot)
	if err != nil {
		t.Fatalf("ExecuteCandidateScenarios: %v", err)
	}

	if result.Total != 1 {
		t.Errorf("expected 1 scenario, got %d", result.Total)
	}
	if result.Passed != 1 {
		t.Errorf("expected 1 passed, got %d", result.Passed)
	}
	if result.Failed != 0 {
		t.Errorf("expected 0 failed, got %d", result.Failed)
	}
	if len(result.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result.Results))
	}
	if !result.Results[0].Match {
		t.Errorf("expected match=true, got match=false: actual=%s/%s",
			result.Results[0].Actual.Status, result.Results[0].Actual.Severity)
	}
}

func TestExecuteCandidateScenarios_Mismatch(t *testing.T) {
	// Create a candidate whose check always returns PASS,
	// but the scenario expects REVIEW
	candidateDir := createTestCandidate(t, &testCandidateOpts{
		checkID:  "test_mismatch",
		version:  "1.0",
		goSource: alwaysPassCheckGo,
		scenarios: `scenarios:
  - name: "expect_review"
    origin: "real_fixture"
    input:
      facts:
        - semantic_type: "test_value"
          source_doc: "test"
          field: "value"
          value: 100
          source_span: "page=1"
          confidence: 0.95
    expected:
      check_id: "test_mismatch"
      status: "REVIEW"
      severity: "WARNING"
      reason: "expects review"
`,
	})

	snapshot, err := SnapshotCandidate(candidateDir)
	if err != nil {
		t.Fatalf("SnapshotCandidate: %v", err)
	}

	result, err := ExecuteCandidateScenarios(snapshot)
	// Should return results (not error) since scenarios ran
	if err != nil {
		t.Fatalf("ExecuteCandidateScenarios unexpected error: %v", err)
	}

	if result.Total != 1 {
		t.Errorf("expected 1 scenario, got %d", result.Total)
	}
	if result.Passed != 0 {
		t.Errorf("expected 0 passed, got %d", result.Passed)
	}
	if result.Failed != 1 {
		t.Errorf("expected 1 failed, got %d", result.Failed)
	}
	if result.Results[0].Match {
		t.Error("expected match=false for mismatched scenario")
	}
}

func TestExecuteCandidateScenarios_CompileError(t *testing.T) {
	candidateDir := createTestCandidate(t, &testCandidateOpts{
		checkID:  "test_bad_compile",
		version:  "1.0",
		goSource: `package candidate

import "github.com/doctrust/doctrust/internal/eval"

type BadCheck struct{}

func (c *BadCheck) ID() string      { return "test_bad_compile" }
func (c *BadCheck) Version() string { return "1.0" }

func (c *BadCheck) Evaluate(facts eval.Facts, params map[string]any) eval.Result {
	undefined_function()
	return eval.Result{Status: eval.StatusPass}
}
`,
		scenarios: `scenarios: []
`,
	})

	snapshot, err := SnapshotCandidate(candidateDir)
	if err != nil {
		t.Fatalf("SnapshotCandidate: %v", err)
	}

	_, err = ExecuteCandidateScenarios(snapshot)
	if err == nil {
		t.Fatal("expected error for compilation failure, got nil")
	}
}

func TestExecuteCandidateScenarios_EmptyScenarios(t *testing.T) {
	candidateDir := createTestCandidate(t, &testCandidateOpts{
		checkID:  "test_empty",
		version:  "1.0",
		goSource: alwaysPassCheckGo,
		scenarios: `scenarios: []
`,
	})

	snapshot, err := SnapshotCandidate(candidateDir)
	if err != nil {
		t.Fatalf("SnapshotCandidate: %v", err)
	}

	result, err := ExecuteCandidateScenarios(snapshot)
	if err != nil {
		t.Fatalf("ExecuteCandidateScenarios: %v", err)
	}

	if result.Total != 0 {
		t.Errorf("expected 0 scenarios, got %d", result.Total)
	}
	if result.Passed != 0 {
		t.Errorf("expected 0 passed, got %d", result.Passed)
	}
}

func TestExecuteCandidateScenarios_AdversarialIncluded(t *testing.T) {
	candidateDir := createTestCandidate(t, &testCandidateOpts{
		checkID:  "test_always_pass",
		version:  "1.0",
		goSource: alwaysPassCheckGo,
		scenarios: `scenarios:
  - name: "normal"
    origin: "real_fixture"
    input:
      facts:
        - semantic_type: "test_value"
          source_doc: "test"
          field: "value"
          value: 100
          source_span: "page=1"
          confidence: 0.95
    expected:
      check_id: "test_always_pass"
      status: "PASS"
      severity: "INFO"
      reason: "always passes"
`,
		adversarial: `scenarios:
  - name: "adversarial_edge"
    origin: "human_adversarial"
    input:
      facts:
        - semantic_type: "test_value"
          source_doc: "test"
          field: "value"
          value: 999
          source_span: "page=1"
          confidence: 0.95
    expected:
      check_id: "test_always_pass"
      status: "PASS"
      severity: "INFO"
      reason: "always passes"
`,
	})

	snapshot, err := SnapshotCandidate(candidateDir)
	if err != nil {
		t.Fatalf("SnapshotCandidate: %v", err)
	}

	result, err := ExecuteCandidateScenarios(snapshot)
	if err != nil {
		t.Fatalf("ExecuteCandidateScenarios: %v", err)
	}

	if result.Total != 2 {
		t.Errorf("expected 2 scenarios (normal + adversarial), got %d", result.Total)
	}
	if result.Passed != 2 {
		t.Errorf("expected 2 passed, got %d", result.Passed)
	}

	// Verify adversarial is included
	names := make(map[string]bool)
	for _, r := range result.Results {
		names[r.Name] = true
	}
	if !names["normal"] {
		t.Error("missing normal scenario")
	}
	if !names["adversarial_edge"] {
		t.Error("missing adversarial scenario")
	}
}

func TestExecuteCandidateScenarios_ComplexCheck(t *testing.T) {
	// Test a more realistic check that actually inspects facts
	candidateDir := createTestCandidate(t, &testCandidateOpts{
		checkID:  "test_threshold",
		version:  "1.0",
		goSource: thresholdCheckGo,
		scenarios: `scenarios:
  - name: "below_threshold"
    origin: "real_fixture"
    input:
      facts:
        - semantic_type: "test_value"
          source_doc: "test"
          field: "value"
          value: 3
          source_span: "page=1"
          confidence: 0.95
    params:
      threshold: 5.0
    expected:
      check_id: "test_threshold"
      status: "PASS"
      severity: "INFO"
      reason: "below threshold: 3.0 <= 5.0"
  - name: "above_threshold"
    origin: "real_fixture"
    input:
      facts:
        - semantic_type: "test_value"
          source_doc: "test"
          field: "value"
          value: 10
          source_span: "page=1"
          confidence: 0.95
    params:
      threshold: 5.0
    expected:
      check_id: "test_threshold"
      status: "REVIEW"
      severity: "WARNING"
      reason: "above threshold: 10.0 > 5.0"
`,
	})

	snapshot, err := SnapshotCandidate(candidateDir)
	if err != nil {
		t.Fatalf("SnapshotCandidate: %v", err)
	}

	result, err := ExecuteCandidateScenarios(snapshot)
	if err != nil {
		t.Fatalf("ExecuteCandidateScenarios: %v", err)
	}

	if result.Total != 2 {
		t.Errorf("expected 2 scenarios, got %d", result.Total)
	}
	if result.Passed != 2 {
		t.Errorf("expected 2 passed, got %d", result.Passed)
		for _, r := range result.Results {
			if !r.Match {
				t.Logf("MISMATCH: %s expected=%s/%s actual=%s/%s",
					r.Name, r.Expected.Status, r.Expected.Severity,
					r.Actual.Status, r.Actual.Severity)
			}
		}
	}
}

// --- Test helpers ---

type testCandidateOpts struct {
	checkID     string
	version     string
	goSource    string
	scenarios   string
	adversarial string
}

func createTestCandidate(t *testing.T, opts *testCandidateOpts) string {
	t.Helper()
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "check.go"), []byte(opts.goSource), 0644); err != nil {
		t.Fatalf("write check.go: %v", err)
	}

	meta := map[string]any{
		"id":          opts.checkID,
		"version":     opts.version,
		"description": "test check",
	}
	metaBytes, _ := json.Marshal(meta)
	if err := os.WriteFile(filepath.Join(dir, "metadata.yaml"), metaBytes, 0644); err != nil {
		t.Fatalf("write metadata.yaml: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "scenarios.yaml"), []byte(opts.scenarios), 0644); err != nil {
		t.Fatalf("write scenarios.yaml: %v", err)
	}

	if opts.adversarial != "" {
		if err := os.WriteFile(filepath.Join(dir, "adversarial.yaml"), []byte(opts.adversarial), 0644); err != nil {
			t.Fatalf("write adversarial.yaml: %v", err)
		}
	}

	if err := os.WriteFile(filepath.Join(dir, "state"), []byte("DRAFT"), 0644); err != nil {
		t.Fatalf("write state: %v", err)
	}

	return dir
}

const alwaysPassCheckGo = `package candidate

import "github.com/doctrust/doctrust/internal/eval"

type AlwaysPassCheck struct{}

func (c *AlwaysPassCheck) ID() string      { return "test_always_pass" }
func (c *AlwaysPassCheck) Version() string { return "1.0" }

func (c *AlwaysPassCheck) Evaluate(facts eval.Facts, params map[string]any) eval.Result {
	return eval.Result{
		CheckID:  "test_always_pass",
		Status:   eval.StatusPass,
		Severity: eval.SeverityInfo,
		Reason:   "always passes",
	}
}
`

// Override the check ID for different test cases
func init() {
	// The check ID is hardcoded in the source, but we use different candidates
	// with different IDs. For tests that need a specific ID, we adjust the source.
}

const thresholdCheckGo = `package candidate

import (
	"fmt"
	"math"

	"github.com/doctrust/doctrust/internal/eval"
)

type ThresholdCheck struct{}

func (c *ThresholdCheck) ID() string      { return "test_threshold" }
func (c *ThresholdCheck) Version() string { return "1.0" }

func (c *ThresholdCheck) Evaluate(facts eval.Facts, params map[string]any) eval.Result {
	threshold := 5.0
	if v, ok := params["threshold"].(float64); ok {
		threshold = v
	}

	values, ok := facts["test_value"]
	if !ok || len(values) == 0 {
		return eval.Result{
			CheckID:  "test_threshold",
			Status:   eval.StatusReview,
			Severity: eval.SeverityWarning,
			Reason:   "no test_value found",
		}
	}

	val := 0.0
	switch v := values[0].Value.(type) {
	case float64:
		val = v
	case int:
		val = float64(v)
	}

	if math.Abs(val-threshold) < 0.001 || val < threshold {
		return eval.Result{
			CheckID:  "test_threshold",
			Status:   eval.StatusPass,
			Severity: eval.SeverityInfo,
			Reason:   fmt.Sprintf("below threshold: %.1f <= %.1f", val, threshold),
		}
	}
	return eval.Result{
		CheckID:  "test_threshold",
		Status:   eval.StatusReview,
		Severity: eval.SeverityWarning,
		Reason:   fmt.Sprintf("above threshold: %.1f > %.1f", val, threshold),
	}
}
`

func TestExecuteCandidateScenarios_FromRepoRoot(t *testing.T) {
	// Save and restore CWD
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer os.Chdir(origDir)

	// Find the repo root by walking up to go.mod
	repoRoot, err := FindModuleRoot()
	if err != nil {
		t.Fatalf("FindModuleRoot: %v", err)
	}

	// Change CWD to repo root (the production invocation path)
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("chdir to repo root: %v", err)
	}

	candidateDir := createTestCandidate(t, &testCandidateOpts{
		checkID:  "test_from_root",
		version:  "1.0",
		goSource: alwaysPassCheckGo,
		scenarios: `scenarios:
  - name: "pass_from_root"
    origin: "real_fixture"
    input:
      facts:
        - semantic_type: "test_value"
          source_doc: "test"
          field: "value"
          value: 100
          source_span: "page=1"
          confidence: 0.95
    expected:
      check_id: "test_always_pass"
      status: "PASS"
      severity: "INFO"
      reason: "always passes"
`,
	})

	snapshot, err := SnapshotCandidate(candidateDir)
	if err != nil {
		t.Fatalf("SnapshotCandidate: %v", err)
	}

	result, err := ExecuteCandidateScenarios(snapshot)
	if err != nil {
		t.Fatalf("ExecuteCandidateScenarios from repo root: %v", err)
	}

	if result.Total != 1 {
		t.Errorf("expected 1 scenario, got %d", result.Total)
	}
	if result.Passed != 1 {
		t.Errorf("expected 1 passed, got %d", result.Passed)
	}
}
