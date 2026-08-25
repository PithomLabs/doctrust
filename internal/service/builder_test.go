package service

import (
	"strings"
	"testing"

	"github.com/PithomLabs/doctrust/internal/evidence"
	"github.com/PithomLabs/doctrust/internal/types"
)

func TestBuildFactsFromSnapshot_SourceDocCanonical(t *testing.T) {
	snapshot := &evidence.EvidenceGraph{
		Documents: []evidence.Document{
			{ID: "doc1", Filename: "1_Paystub_2025.pdf", Type: types.DocTypePaystub},
			{ID: "doc2", Filename: "2_W2_Form_2025.pdf", Type: types.DocTypeW2},
			{ID: "doc3", Filename: "3_Form1040_TaxReturn_2025.pdf", Type: types.DocType1040},
		},
		Claims: []evidence.Claim{
			{
				ID: "c1", SemanticType: "gross_income_projected",
				Sources: []evidence.Source{
					{DocumentID: "doc1", Filename: "1_Paystub_2025.pdf", Page: 1, BBox: []float64{508, 274, 50, 8}, Confidence: 0.95, FieldName: "annualized_gross_ytd"},
				},
			},
			{
				ID: "c2", SemanticType: "gross_income_taxable",
				Sources: []evidence.Source{
					{DocumentID: "doc2", Filename: "2_W2_Form_2025.pdf", Page: 1, BBox: []float64{224, 120, 44, 8}, Confidence: 0.95, FieldName: "wages_tips_other_compensation"},
				},
			},
		},
	}

	f, err := BuildFactsFromSnapshot(snapshot)
	if err != nil {
		t.Fatalf("BuildFactsFromSnapshot: %v", err)
	}

	// SourceDoc must be canonical type, NOT filename
	projected := f["gross_income_projected"]
	if len(projected) == 0 {
		t.Fatal("expected gross_income_projected facts")
	}
	if projected[0].SourceDoc != "paystub" {
		t.Errorf("SourceDoc = %q, want %q (not %q)", projected[0].SourceDoc, "paystub", "1_Paystub_2025.pdf")
	}

	taxable := f["gross_income_taxable"]
	if len(taxable) == 0 {
		t.Fatal("expected gross_income_taxable facts")
	}
	if taxable[0].SourceDoc != "w2" {
		t.Errorf("SourceDoc = %q, want %q (not %q)", taxable[0].SourceDoc, "w2", "2_W2_Form_2025.pdf")
	}
}

func TestBuildFactsFromSnapshot_BBoxPassThrough(t *testing.T) {
	snapshot := &evidence.EvidenceGraph{
		Documents: []evidence.Document{
			{ID: "doc1", Filename: "paystub.pdf", Type: types.DocTypePaystub},
		},
		Claims: []evidence.Claim{
			{
				ID: "c1", SemanticType: "gross_income_projected",
				Sources: []evidence.Source{
					{DocumentID: "doc1", Filename: "paystub.pdf", Page: 1, BBox: []float64{508.0, 274.0, 50.0, 8.0}, Confidence: 0.95, FieldName: "annualized_gross_ytd"},
				},
			},
		},
	}

	f, err := BuildFactsFromSnapshot(snapshot)
	if err != nil {
		t.Fatalf("BuildFactsFromSnapshot: %v", err)
	}

	facts := f["gross_income_projected"]
	if len(facts) == 0 {
		t.Fatal("expected facts")
	}

	span := facts[0].SourceSpan
	// Exact pass-through: no y-flip, no scaling, no rounding
	if !strings.Contains(span, "page=1") {
		t.Errorf("SourceSpan missing page=1: %q", span)
	}
	if !strings.Contains(span, "bbox=[508.0,274.0,50.0,8.0]") {
		t.Errorf("SourceSpan missing exact bbox: %q", span)
	}
}

func TestBuildFactsFromSnapshot_UnknownFilename(t *testing.T) {
	snapshot := &evidence.EvidenceGraph{
		Documents: []evidence.Document{
			{ID: "doc1", Filename: "unknown_file.pdf", Type: types.DocTypeUnknown},
		},
		Claims: []evidence.Claim{
			{
				ID: "c1", SemanticType: "some_type",
				Sources: []evidence.Source{
					{DocumentID: "doc1", Filename: "unknown_file.pdf", Page: 1, Confidence: 0.8, FieldName: "field"},
				},
			},
		},
	}

	f, err := BuildFactsFromSnapshot(snapshot)
	if err != nil {
		t.Fatalf("BuildFactsFromSnapshot: %v", err)
	}

	facts := f["some_type"]
	if len(facts) == 0 {
		t.Fatal("expected facts")
	}
	if facts[0].SourceDoc != "unknown" {
		t.Errorf("SourceDoc = %q, want %q", facts[0].SourceDoc, "unknown")
	}
}

func TestBuildFactsFromSnapshot_NilSnapshot(t *testing.T) {
	_, err := BuildFactsFromSnapshot(nil)
	if err == nil {
		t.Error("expected error for nil snapshot")
	}
}

func TestBuildFactsFromSnapshot_MultipleSourcesPerClaim(t *testing.T) {
	snapshot := &evidence.EvidenceGraph{
		Documents: []evidence.Document{
			{ID: "doc1", Filename: "w2.pdf", Type: types.DocTypeW2},
			{ID: "doc2", Filename: "1040.pdf", Type: types.DocType1040},
		},
		Claims: []evidence.Claim{
			{
				ID: "c1", SemanticType: "gross_income_taxable",
				Sources: []evidence.Source{
					{DocumentID: "doc1", Filename: "w2.pdf", Page: 1, BBox: []float64{224, 120, 44, 8}, Confidence: 0.95, FieldName: "wages"},
					{DocumentID: "doc2", Filename: "1040.pdf", Page: 1, BBox: []float64{100, 200, 40, 8}, Confidence: 0.90, FieldName: "line1z"},
				},
			},
		},
	}

	f, err := BuildFactsFromSnapshot(snapshot)
	if err != nil {
		t.Fatalf("BuildFactsFromSnapshot: %v", err)
	}

	facts := f["gross_income_taxable"]
	if len(facts) != 2 {
		t.Fatalf("expected 2 facts, got %d", len(facts))
	}

	if facts[0].SourceDoc != "w2" {
		t.Errorf("fact[0].SourceDoc = %q, want %q", facts[0].SourceDoc, "w2")
	}
	if facts[1].SourceDoc != "form_1040" {
		t.Errorf("fact[1].SourceDoc = %q, want %q", facts[1].SourceDoc, "form_1040")
	}
}
