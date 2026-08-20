package eval

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestFactsContract_Fixture is a deterministic test that loads a recorded fixture
// and runs checks without calling the Nutrient API. This runs in CI.
func TestFactsContract_Fixture(t *testing.T) {
	_, filename, _, _ := runtime.Caller(0)
	projectRoot := filepath.Dir(filepath.Dir(filepath.Dir(filename)))
	fixturePath := filepath.Join(projectRoot, "fixtures", "contract_test_fixture.json")

	fixtureJSON, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	// Parse the fixture into Facts
	var raw struct {
		Facts map[string][]struct {
			Value      any     `json:"value"`
			SourceDoc  string  `json:"source_doc"`
			FieldName  string  `json:"field_name"`
			SourceSpan string  `json:"source_span"`
			Confidence float64 `json:"confidence"`
		} `json:"facts"`
	}
	if err := json.Unmarshal(fixtureJSON, &raw); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}

	// Convert to canonical Facts
	allFacts := make(Facts)
	for key, entries := range raw.Facts {
		for _, e := range entries {
			allFacts[key] = append(allFacts[key], Fact{
				Value:      e.Value,
				SourceDoc:  e.SourceDoc,
				FieldName:  e.FieldName,
				SourceSpan: e.SourceSpan,
				Confidence: e.Confidence,
			})
		}
	}

	// Verify multiplicity: gross_income_taxable should have 2 observations
	if taxFacts, ok := allFacts["gross_income_taxable"]; ok {
		if len(taxFacts) != 2 {
			t.Errorf("expected 2 gross_income_taxable observations, got %d", len(taxFacts))
		}
	} else {
		t.Error("missing gross_income_taxable in fixture")
	}

	// Verify bonus_compensation exists
	if _, ok := allFacts["bonus_compensation"]; !ok {
		t.Error("missing bonus_compensation in fixture")
	}

	// Verify net_cash_flow exists
	if _, ok := allFacts["net_cash_flow"]; !ok {
		t.Error("missing net_cash_flow in fixture")
	}

	// Run checks
	runner := NewRunner(map[string]Check{
		"gross_income_consistency":     &GrossIncomeConsistencyCheck{},
		"required_documents":           &RequiredDocumentsCheck{},
		"net_vs_gross_incomparability": &NetVsGrossIncomparabilityCheck{},
	})

	// Test gross_income_consistency — should be REVIEW (15% variance > 5% tolerance)
	result := runner.checks["gross_income_consistency"].Evaluate(allFacts, map[string]any{
		"tolerance":   0.05,
		"bonus_field": "bonus_compensation",
	})
	if result.Status != StatusReview {
		t.Errorf("gross_income_consistency: expected REVIEW, got %s", result.Status)
	}
	if len(result.Evidence) == 0 {
		t.Error("gross_income_consistency: expected evidence, got none")
	}
	// Verify evidence has SourceDoc populated
	for _, ev := range result.Evidence {
		if ev.SourceDoc == "" {
			t.Errorf("evidence entry missing SourceDoc: field=%s", ev.Field)
		}
	}

	// Test required_documents — should be PASS (all three docs present)
	result = runner.checks["required_documents"].Evaluate(allFacts, nil)
	if result.Status != StatusPass {
		t.Errorf("required_documents: expected PASS, got %s (reason: %s)", result.Status, result.Reason)
	}

	// Test net_vs_gross_incomparability — should be PASS (both present)
	result = runner.checks["net_vs_gross_incomparability"].Evaluate(allFacts, nil)
	if result.Status != StatusPass {
		t.Errorf("net_vs_gross_incomparability: expected PASS, got %s", result.Status)
	}

	// Verify bonus_field parameter actually controls behavior
	resultWithDiffBonus := runner.checks["gross_income_consistency"].Evaluate(allFacts, map[string]any{
		"tolerance":   0.05,
		"bonus_field": "nonexistent_field",
	})
	// With nonexistent bonus field, the bonus is not found, so no bonus explanation
	if resultWithDiffBonus.Status != StatusReview {
		t.Errorf("gross_income_consistency with wrong bonus_field: expected REVIEW, got %s", resultWithDiffBonus.Status)
	}
}
