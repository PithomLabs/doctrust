package integration

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/PithomLabs/doctrust/internal/eval"
	"github.com/PithomLabs/doctrust/internal/evidence"
	"github.com/PithomLabs/doctrust/internal/facts"
	"github.com/PithomLabs/doctrust/internal/ingest"
	"github.com/PithomLabs/doctrust/internal/nutrient"
)

// TestFactsContract_NutrientToChecks verifies the Nutrient → Facts → Checks contract.
// This test requires NUTRIENT_DWS_EXTRACTION_API_KEY to be set.
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

	normalizer := ingest.NewNormalizer()

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

		docType := documentTypeFromFilename(file)
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
	runner := eval.NewRunner(map[string]eval.Check{
		"gross_income_consistency":     &eval.GrossIncomeConsistencyCheck{},
		"required_documents":           &eval.RequiredDocumentsCheck{},
		"net_vs_gross_incomparability": &eval.NetVsGrossIncomparabilityCheck{},
	})

	// Direct check evaluation
	check, _ := runner.GetCheck("gross_income_consistency")
	checkResult := check.Evaluate(allFacts, map[string]any{
		"tolerance": 0.05,
	})

	fmt.Printf("gross_income_consistency: status=%s, evidence=%d, reason=%s\n", checkResult.Status, len(checkResult.Evidence), checkResult.Reason)
	for _, ev := range checkResult.Evidence {
		fmt.Printf("  Evidence: field=%s, sourceSpan=%s\n", ev.Field, ev.SourceSpan)
	}

	if checkResult.CheckID != "gross_income_consistency" {
		t.Errorf("check ID mismatch: %s", checkResult.CheckID)
	}

	// Should produce REVIEW due to variance
	if checkResult.Status != eval.StatusReview && checkResult.Status != eval.StatusPass {
		t.Errorf("unexpected status for gross_income_consistency: %s", checkResult.Status)
	}

	if len(checkResult.Evidence) == 0 {
		t.Errorf("gross_income_consistency check produced no evidence")
	}

	// Test required_documents
	check, _ = runner.GetCheck("required_documents")
	checkResult = check.Evaluate(allFacts, nil)
	fmt.Printf("required_documents: status=%s, reason=%s\n", checkResult.Status, checkResult.Reason)
	if checkResult.Status != eval.StatusPass {
		t.Errorf("required_documents expected PASS, got %s", checkResult.Status)
	}

	// Test net_vs_gross_incomparability
	check, _ = runner.GetCheck("net_vs_gross_incomparability")
	checkResult = check.Evaluate(allFacts, nil)
	fmt.Printf("net_vs_gross: status=%s, reason=%s\n", checkResult.Status, checkResult.Reason)
	if checkResult.Status != eval.StatusPass && checkResult.Status != eval.StatusReview {
		t.Errorf("unexpected status for net_vs_gross: %s", checkResult.Status)
	}

	// Verify evidence in results
	if len(checkResult.Evidence) == 0 && checkResult.Status != eval.StatusPass {
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

func documentTypeFromFilename(name string) evidence.DocumentType {
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
