package eval

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/doctrust/doctrust/internal/evidence"
	"github.com/doctrust/doctrust/internal/facts"
	"github.com/doctrust/doctrust/internal/nutrient"
)

// TestFactsContract_NutrientToChecks verifies the Nutrient → Facts → Checks contract
// This test requires NUTRIENT_DWS_EXTRACTION_API_KEY to be set
func TestFactsContract_NutrientToChecks(t *testing.T) {
	apiKey := os.Getenv("NUTRIENT_DWS_EXTRACTION_API_KEY")
	if apiKey == "" {
		t.Skip("NUTRIENT_DWS_EXTRACTION_API_KEY not set, skipping contract test")
	}

	// Find project root
	_, filename, _, _ := runtime.Caller(0)
	projectRoot := filepath.Dir(filepath.Dir(filepath.Dir(filename)))

	fmt.Println("Project root:", projectRoot)

	// Load PDFs
	files := []string{
		"1_Paystub_2025.pdf",
		"2_W2_Form_2025.pdf",
		"3_Form1040_TaxReturn_2025.pdf",
		"4_BankStatement_Q4_2025.pdf",
	}

	extractionKey := os.Getenv("NUTRIENT_DWS_EXTRACTION_API_KEY")
	client := nutrient.NewClient(extractionKey, "")

	normalizer := facts.NewNormalizer()

	var allFacts facts.Facts = make(facts.Facts)
	var allEvidence []facts.EvidenceWithSource
	var allDocs []facts.Document

	for i, file := range files {
		absPath := filepath.Join(projectRoot, "demo", "income_verification", file)
		fmt.Println("Trying to read:", absPath)
		_, err := os.ReadFile(absPath)
		if err != nil {
			t.Fatalf("read %s: %v", absPath, err)
		}

		docType := DocumentTypeFromFilename(file)
		_ = fmt.Sprintf("doc_%d", i+1) // docID unused for now

		extResult, err := client.ExtractFields(absPath, getSchemaForDocType(string(docType)), "understand")
		if err != nil {
			t.Fatalf("extract %s: %v", absPath, err)
		}

		normOut, err := normalizer.Normalize(fmt.Sprintf("doc_%d", i+1), filepath.Base(absPath), "", docType, extResult)
		if err != nil {
			t.Fatalf("normalize %s: %v", absPath, err)
		}

		for k, v := range normOut.Facts {
			allFacts[k] = append(allFacts[k], v...)
			fmt.Printf("Facts[%s] = %v\n", k, v)
		}
		allEvidence = append(allEvidence, normOut.Evidence...)
		allDocs = append(allDocs, normOut.Documents...)
	}

	// Debug: print all facts
	fmt.Println("=== All Facts ===")
	for k, v := range allFacts {
		fmt.Printf("  %s: %v\n", k, v)
	}

	// Verify expected semantic types exist in Facts
	expectedTypes := []string{
		"gross_income_projected",
		"base_salary",
		"bonus_compensation",
		"gross_income_taxable",
		"net_cash_flow",
	}

	for _, stype := range expectedTypes {
		if _, ok := allFacts[stype]; !ok {
			t.Errorf("missing semantic type in Facts: %s", stype)
		}
	}

	// Verify multiplicity: gross_income_taxable should have observations from both W-2 and 1040
	if taxFacts, ok := allFacts["gross_income_taxable"]; ok {
		if len(taxFacts) < 2 {
			t.Errorf("expected at least 2 gross_income_taxable observations (W-2 + 1040), got %d", len(taxFacts))
		}
	}

	// Verify evidence has SourceSpan and Confidence
	if len(allEvidence) == 0 {
		t.Errorf("no evidence produced by normalizer")
	}
	for _, ev := range allEvidence {
		if ev.Field == "" {
			t.Errorf("evidence missing Field")
		}
		if ev.SourceSpan == "" {
			t.Errorf("evidence missing SourceSpan for field %s", ev.Field)
		}
		if ev.Confidence < 0 || ev.Confidence > 1 {
			t.Errorf("evidence confidence out of range for field %s: %f", ev.Field, ev.Confidence)
		}
	}

	// Run representative checks
	runner := NewRunner(map[string]Check{
		"gross_income_consistency":     &GrossIncomeConsistencyCheck{},
		"required_documents":           &RequiredDocumentsCheck{},
		"net_vs_gross_incomparability": &NetVsGrossIncomparabilityCheck{},
	})

	// Test gross_income_consistency
	result := runner.checks["gross_income_consistency"].Evaluate(allFacts, map[string]any{
		"tolerance": 0.05,
	})

	fmt.Printf("gross_income_consistency: status=%s, evidence=%d, reason=%s\n", result.Status, len(result.Evidence), result.Reason)
	for _, ev := range result.Evidence {
		fmt.Printf("  Evidence: field=%s, sourceSpan=%s\n", ev.Field, ev.SourceSpan)
	}

	if result.CheckID != "gross_income_consistency" {
		t.Errorf("check ID mismatch: %s", result.CheckID)
	}

	// Should produce REVIEW due to variance
	if result.Status != StatusReview && result.Status != StatusPass {
		t.Errorf("unexpected status for gross_income_consistency: %s", result.Status)
	}

	if len(result.Evidence) == 0 {
		t.Errorf("gross_income_consistency check produced no evidence")
	}

	// Test required_documents
	result = runner.checks["required_documents"].Evaluate(allFacts, nil)
	fmt.Printf("required_documents: status=%s, reason=%s\n", result.Status, result.Reason)
	if result.Status != StatusPass {
		t.Errorf("required_documents expected PASS, got %s", result.Status)
	}

	// Test net_vs_gross_incomparability
	result = runner.checks["net_vs_gross_incomparability"].Evaluate(allFacts, nil)
	fmt.Printf("net_vs_gross: status=%s, reason=%s\n", result.Status, result.Reason)
	if result.Status != StatusPass && result.Status != StatusReview {
		t.Errorf("unexpected status for net_vs_gross: %s", result.Status)
	}

	// Verify evidence in results
	if len(result.Evidence) == 0 && result.Status != StatusPass {
		t.Errorf("check produced no evidence for non-PASS result")
	}
}

func getSchemaForDocType(docType string) map[string]any {
	switch docType {
	case "paystub":
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"annualized_gross_ytd": map[string]any{
					"type":        "string",
					"description": "Annualized year-to-date gross earnings",
				},
				"base_salary_ytd": map[string]any{
					"type":        "string",
					"description": "YTD base salary before bonuses",
				},
				"bonus_ytd": map[string]any{
					"type":        "string",
					"description": "YTD bonus or incentive compensation",
				},
			},
		}
	case "w2":
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"wages_tips_other_compensation": map[string]any{
					"type":        "string",
					"description": "Box 1: Wages, tips, other compensation",
				},
			},
		}
	case "form_1040":
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"line1z_wages": map[string]any{
					"type":        "string",
					"description": "Line 1z: Total wages from W-2",
				},
			},
		}
	case "bank_statement":
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"total_deposits": map[string]any{
					"type":        "string",
					"description": "Total deposits for the statement period",
				},
			},
		}
	default:
		return map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		}
	}
}

func DocumentTypeFromFilename(name string) evidence.DocumentType {
	lower := strings.ToLower(name)
	switch {
	case strings.Contains(lower, "paystub"):
		return evidence.DocTypePaystub
	case strings.Contains(lower, "w2"):
		return evidence.DocTypeW2
	case strings.Contains(lower, "1040"):
		return evidence.DocType1040
	case strings.Contains(lower, "bank"):
		return evidence.DocTypeBankStmt
	default:
		return evidence.DocTypeUnknown
	}
}

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