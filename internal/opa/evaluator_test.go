package opa

import (
	"context"
	"os"
	"testing"
)

const testPolicyPath = "../../policies/income_verification/policy.rego"

func loadTestPolicy(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(testPolicyPath)
	if err != nil {
		t.Fatalf("read policy: %v", err)
	}
	return string(data)
}

func TestEvaluate_PassWithinThreshold(t *testing.T) {
	policy := loadTestPolicy(t)
	snapshot := []byte(`{
		"documents": [
			{"id": "d1", "type": "paystub", "filename": "paystub.pdf"},
			{"id": "d2", "type": "w2", "filename": "w2.pdf"},
			{"id": "d3", "type": "form_1040", "filename": "1040.pdf"}
		],
		"claims": [
			{"id": "c1", "semantic_type": "gross_income_projected", "value": 105000, "sources": [{"document_id": "d1", "confidence": 0.95}]},
			{"id": "c2", "semantic_type": "gross_income_taxable", "value": 100000, "sources": [{"document_id": "d2", "confidence": 0.95}]},
			{"id": "c3", "semantic_type": "gross_income_taxable", "value": 100000, "sources": [{"document_id": "d3", "confidence": 0.95}]}
		]
	}`)

	result, err := Evaluate(context.Background(), snapshot, policy)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if result.Decision != "PASS" {
		t.Errorf("expected PASS, got %s", result.Decision)
	}
	if len(result.Findings) != 0 {
		t.Errorf("expected 0 findings, got %d", len(result.Findings))
	}
}

func TestEvaluate_ConflictWithVariance(t *testing.T) {
	policy := loadTestPolicy(t)
	snapshot := []byte(`{
		"documents": [
			{"id": "d1", "type": "paystub", "filename": "paystub.pdf"},
			{"id": "d2", "type": "w2", "filename": "w2.pdf"},
			{"id": "d3", "type": "form_1040", "filename": "1040.pdf"}
		],
		"claims": [
			{"id": "c1", "semantic_type": "gross_income_projected", "value": 115000, "sources": [{"document_id": "d1", "confidence": 0.95}]},
			{"id": "c2", "semantic_type": "gross_income_taxable", "value": 100000, "sources": [{"document_id": "d2", "confidence": 0.95}]},
			{"id": "c3", "semantic_type": "gross_income_taxable", "value": 100000, "sources": [{"document_id": "d3", "confidence": 0.95}]}
		]
	}`)

	result, err := Evaluate(context.Background(), snapshot, policy)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if result.Decision != "REVIEW" {
		t.Errorf("expected REVIEW, got %s", result.Decision)
	}
	if len(result.Findings) != 1 {
		t.Errorf("expected 1 finding, got %d", len(result.Findings))
	}
}

func TestEvaluate_MissingEvidence(t *testing.T) {
	policy := loadTestPolicy(t)
	snapshot := []byte(`{
		"documents": [
			{"id": "d1", "type": "paystub", "filename": "paystub.pdf"},
			{"id": "d3", "type": "form_1040", "filename": "1040.pdf"}
		],
		"claims": [
			{"id": "c1", "semantic_type": "gross_income_projected", "value": 105000, "sources": [{"document_id": "d1", "confidence": 0.95}]},
			{"id": "c3", "semantic_type": "gross_income_taxable", "value": 100000, "sources": [{"document_id": "d3", "confidence": 0.95}]}
		]
	}`)

	result, err := Evaluate(context.Background(), snapshot, policy)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if result.Decision != "MISSING_EVIDENCE" {
		t.Errorf("expected MISSING_EVIDENCE, got %s", result.Decision)
	}
}

func TestEvaluate_ZeroDenominator(t *testing.T) {
	policy := loadTestPolicy(t)
	snapshot := []byte(`{
		"documents": [
			{"id": "d1", "type": "paystub", "filename": "paystub.pdf"},
			{"id": "d2", "type": "w2", "filename": "w2.pdf"},
			{"id": "d3", "type": "form_1040", "filename": "1040.pdf"}
		],
		"claims": [
			{"id": "c1", "semantic_type": "gross_income_projected", "value": 50000, "sources": [{"document_id": "d1", "confidence": 0.95}]},
			{"id": "c2", "semantic_type": "gross_income_taxable", "value": 0, "sources": [{"document_id": "d2", "confidence": 0.95}]},
			{"id": "c3", "semantic_type": "gross_income_taxable", "value": 0, "sources": [{"document_id": "d3", "confidence": 0.95}]}
		]
	}`)

	result, err := Evaluate(context.Background(), snapshot, policy)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if result.Decision != "REVIEW" {
		t.Errorf("expected REVIEW, got %s", result.Decision)
	}
	if len(result.Findings) == 0 {
		t.Error("expected at least one finding for zero-denominator")
	}
}

func TestEvaluate_MalformedInput(t *testing.T) {
	policy := loadTestPolicy(t)
	_, err := Evaluate(context.Background(), []byte("not json"), policy)
	if err == nil {
		t.Error("expected error for malformed input")
	}
}
