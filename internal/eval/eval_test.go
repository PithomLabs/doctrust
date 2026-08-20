package eval

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/doctrust/doctrust/internal/evidence"
)

func TestAggregator(t *testing.T) {
	agg := &DecisionAggregator{}

	// Test all PASS
	results := []Result{
		{Status: StatusPass, Severity: SeverityInfo},
		{Status: StatusPass, Severity: SeverityInfo},
	}
	status, blocked := agg.Aggregate(results)
	if status != "PASS" || len(blocked) != 0 {
		t.Errorf("all PASS: expected PASS, got %s, blocked=%v", status, blocked)
	}

	// Test one REVIEW
	results = []Result{
		{Status: StatusPass, Severity: SeverityInfo},
		{Status: StatusReview, Severity: SeverityWarning},
	}
	status, blocked = agg.Aggregate(results)
	if status != "REVIEW" {
		t.Errorf("one REVIEW: expected REVIEW, got %s", status)
	}

	// Test one BLOCKING FAIL
	results = []Result{
		{Status: StatusPass, Severity: SeverityInfo},
		{Status: StatusFail, Severity: SeverityBlocking},
	}
	status, blocked = agg.Aggregate(results)
	if status != "FAIL" || len(blocked) != 1 {
		t.Errorf("one BLOCKING FAIL: expected FAIL with 1 blocked, got %s, blocked=%v", status, blocked)
	}

	// Test non-blocking FAIL + REVIEW
	results = []Result{
		{Status: StatusFail, Severity: SeverityWarning},
		{Status: StatusReview, Severity: SeverityWarning},
	}
	status, blocked = agg.Aggregate(results)
	if status != "REVIEW" {
		t.Errorf("WARNING FAIL + REVIEW: expected REVIEW, got %s", status)
	}

	// Test empty
	results = []Result{}
	status, blocked = agg.Aggregate(results)
	if status != "PASS" {
		t.Errorf("empty: expected PASS, got %s", status)
	}
}

func TestDiffResults(t *testing.T) {
	before := Result{Status: StatusPass, Severity: SeverityInfo, Reason: "ok"}
	after := Result{Status: StatusPass, Severity: SeverityInfo, Reason: "ok"}
	diff := DiffResults(after, before)
	if diff != nil {
		t.Errorf("identical results should have no diff")
	}

	after = Result{Status: StatusReview, Severity: SeverityWarning, Reason: "changed"}
	diff = DiffResults(after, before)
	if diff == nil {
		t.Errorf("different status should have diff")
	}
	if !diff.StatusChanged {
		t.Errorf("diff should show status changed")
	}

	after = Result{Status: StatusPass, Severity: SeverityBlocking, Reason: "ok"}
	diff = DiffResults(after, before)
	if diff == nil || !diff.SeverityChanged {
		t.Errorf("different severity should have diff with SeverityChanged=true")
	}

	after = Result{Status: StatusPass, Severity: SeverityInfo, Reason: "different reason"}
	diff = DiffResults(after, before)
	if diff == nil || !diff.ReasonChanged {
		t.Errorf("different reason should have diff with ReasonChanged=true")
	}

	// Evidence change
	after = Result{Status: StatusPass, Severity: SeverityInfo, Reason: "ok",
		Evidence: []evidence.EvidenceRef{{Field: "new_field"}}}
	diff = DiffResults(after, before)
	if diff == nil || !diff.EvidenceChanged {
		t.Errorf("different evidence should have diff with EvidenceChanged=true")
	}
}

func TestRunner(t *testing.T) {
	check := &GrossIncomeConsistencyCheck{}
	runner := NewRunner(map[string]Check{"test": check})

	// Test successful registration
	if err := runner.Register(&RequiredDocumentsCheck{}); err != nil {
		t.Errorf("register new check: %v", err)
	}

	// Test duplicate registration
	if err := runner.Register(&RequiredDocumentsCheck{}); err == nil {
		t.Errorf("duplicate registration should fail")
	}

	// Test missing check
	_, err := runner.GetCheck("nonexistent")
	if err == nil {
		t.Errorf("GetCheck should fail for nonexistent check")
	}

	// Test RunScenario
	scenarioInput := ScenarioInput{
		Facts: []ScenarioFact{
			{SemanticType: "gross_income_projected", Value: 138000.0, SourceDoc: "paystub", FieldName: "annualized_gross_ytd", SourceSpan: "page=1", Confidence: 0.95},
			{SemanticType: "gross_income_taxable", Value: 120000.0, SourceDoc: "w2", FieldName: "wages_tips_other_compensation", SourceSpan: "page=1", Confidence: 0.95},
			{SemanticType: "bonus_compensation", Value: 18000.0, SourceDoc: "paystub", FieldName: "bonus_ytd", SourceSpan: "page=1", Confidence: 0.95},
		},
	}
	runner2 := NewRunner(map[string]Check{
		"gross_income_consistency": &GrossIncomeConsistencyCheck{},
	})
	result := runner2.RunScenario(nil, Scenario{
		Name:   "test",
		Input:  scenarioInput,
		Params: map[string]any{"tolerance": 0.05},
		Expected: Result{
			CheckID:  "gross_income_consistency",
			Status:   StatusReview,
			Severity: SeverityWarning,
			Reason:   "Variance exceeds tolerance; documented bonus may explain",
			Evidence: []evidence.EvidenceRef{
				{Field: "gross_income_projected", SourceSpan: "page=1", Confidence: 0.95},
				{Field: "gross_income_taxable", SourceSpan: "page=1", Confidence: 0.95},
				{Field: "bonus_compensation", SourceSpan: "page=1", Confidence: 0.95},
			},
		},
	})
	if !result.Passed {
		t.Errorf("RunScenario should pass for matching result, got diff: %+v", result.Diff)
	}
}

func TestVersioning(t *testing.T) {
	rs := Ruleset{
		ID:      "test",
		Version: "1",
		Checks: []CheckRef{
			{ID: "a", Version: "1.0", Params: map[string]any{"x": 1}},
			{ID: "b", Version: "2.0", Params: map[string]any{"y": 2}},
		},
	}
	hash1, err := rs.ComputeHash()
	if err != nil {
		t.Fatalf("ComputeHash error: %v", err)
	}

	// Same content should produce same hash
	hash2, err := rs.ComputeHash()
	if err != nil {
		t.Fatalf("ComputeHash error: %v", err)
	}
	if hash1 != hash2 {
		t.Errorf("hash not deterministic: %s != %s", hash1, hash2)
	}

	// Different order should produce same hash (sorted)
	rs2 := Ruleset{
		ID:      "test",
		Version: "1",
		Checks: []CheckRef{
			{ID: "b", Version: "2.0", Params: map[string]any{"y": 2}},
			{ID: "a", Version: "1.0", Params: map[string]any{"x": 1}},
		},
	}
	hash3, err := rs2.ComputeHash()
	if err != nil {
		t.Fatalf("ComputeHash error: %v", err)
	}
	if hash1 != hash3 {
		t.Errorf("hash should be order-independent")
	}

	// Manifest
	manifest, err := rs.Manifest()
	if err != nil {
		t.Fatalf("Manifest error: %v", err)
	}
	if manifest.Hash != hash1 {
		t.Errorf("manifest hash mismatch")
	}
}

func TestVersionNumber(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"v1", 1},
		{"v2", 2},
		{"v10", 10},
		{"v100", 100},
		{"working", 0},
	}
	for _, tt := range tests {
		got := versionNumber(tt.input)
		if got != tt.expected {
			t.Errorf("versionNumber(%q) = %d, want %d", tt.input, got, tt.expected)
		}
	}

	// Verify natural sorting: v10 > v2 lexicographically but v2 < v10 naturally
	versions := []string{"v10", "v2", "v1", "v20"}
	for i := range versions {
		for j := i + 1; j < len(versions); j++ {
			if versionNumber(versions[i]) > versionNumber(versions[j]) {
				versions[i], versions[j] = versions[j], versions[i]
			}
		}
	}
	expected := []string{"v1", "v2", "v10", "v20"}
	for i, v := range versions {
		if v != expected[i] {
			t.Errorf("natural sort: got %v, want %v", versions, expected)
		}
	}
}

func TestRunAllScenarios(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	projectRoot := filepath.Dir(filepath.Dir(filepath.Dir(thisFile)))
	scenariosDir := filepath.Join(projectRoot, "scenarios", "income_verification")

	// Load all scenarios from YAML
	scenarios, err := LoadAllScenariosFromDir(scenariosDir)
	if err != nil {
		t.Fatalf("load scenarios: %v", err)
	}

	if len(scenarios) == 0 {
		t.Fatal("no scenarios found")
	}

	t.Logf("loaded %d scenarios", len(scenarios))

	// Build check registry
	checks := map[string]Check{
		"gross_income_consistency":     &GrossIncomeConsistencyCheck{},
		"required_documents":           &RequiredDocumentsCheck{},
		"net_vs_gross_incomparability": &NetVsGrossIncomparabilityCheck{},
	}
	runner := NewRunner(checks)

	// Run all scenarios
	results := runner.RunAllScenarios(context.Background(), scenarios)

	passed := 0
	failed := 0
	for _, r := range results {
		if r.Passed {
			passed++
			t.Logf("PASS: %s", r.ScenarioName)
		} else {
			failed++
			if r.Diff != nil {
				t.Errorf("FAIL: %s — EvidenceChanged=%v, StatusChanged=%v, ReasonChanged=%v", r.ScenarioName, r.Diff.EvidenceChanged, r.Diff.StatusChanged, r.Diff.ReasonChanged)
			} else {
				// GetCheck failed
				t.Errorf("FAIL: %s — check not found or no actual result", r.ScenarioName)
			}
			// Debug: print actual vs expected
			actual := r.Actual
			expected := r.Expected
			t.Logf("  actual:   status=%s severity=%s reason=%q evidence=%d", actual.Status, actual.Severity, actual.Reason, len(actual.Evidence))
			for _, ev := range actual.Evidence {
				t.Logf("    ev: field=%q", ev.Field)
			}
			t.Logf("  expected: status=%s severity=%s reason=%q evidence=%d", expected.Status, expected.Severity, expected.Reason, len(expected.Evidence))
			for _, ev := range expected.Evidence {
				t.Logf("    ev: field=%q", ev.Field)
			}
		}
	}

	t.Logf("results: %d passed, %d failed out of %d total", passed, failed, len(scenarios))
}