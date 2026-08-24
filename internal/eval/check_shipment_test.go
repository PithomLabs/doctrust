package eval

import (
	"strings"
	"testing"

	"github.com/doctrust/doctrust/internal/types"
)

func shipmentFacts(values map[string]float64) Facts {
	f := Facts{}
	for doc, v := range values {
		f["shipment.gross_weight"] = append(f["shipment.gross_weight"], Fact{
			Value:      v,
			SourceDoc:  doc,
			FieldName:  "gross_weight",
			SourceSpan: "page=1;bbox=[100.0,200.0,80.0,10.0]",
			Confidence: 0.95,
		})
	}
	return f
}

func TestGrossWeightReconciliation_AllEqual(t *testing.T) {
	c := &GrossWeightReconciliationCheck{}
	res := c.Evaluate(shipmentFacts(map[string]float64{
		"commercial_invoice":    4650,
		"packing_list":          4650,
		"certificate_of_origin": 4650,
		"bill_of_lading":        4650,
	}), map[string]any{})
	if res.Status != StatusPass {
		t.Fatalf("expected PASS, got %s (%s)", res.Status, res.Reason)
	}
	if len(res.Evidence) != 4 {
		t.Fatalf("expected 4 evidence refs, got %d", len(res.Evidence))
	}
}

func TestGrossWeightReconciliation_MismatchIsReviewWithOutlier(t *testing.T) {
	c := &GrossWeightReconciliationCheck{}
	res := c.Evaluate(shipmentFacts(map[string]float64{
		"commercial_invoice":    4650,
		"packing_list":          4650,
		"certificate_of_origin": 4650,
		"bill_of_lading":        5150,
	}), map[string]any{})
	if res.Status != StatusReview {
		t.Fatalf("expected REVIEW, got %s", res.Status)
	}
	if res.Severity != SeverityBlocking {
		t.Fatalf("expected BLOCKING severity, got %s", res.Severity)
	}
	if !strings.Contains(res.Reason, "bill_of_lading") || !strings.Contains(res.Reason, "5150") {
		t.Fatalf("reason must name outlier and value, got %q", res.Reason)
	}
	if _, ok := res.Metrics["observations"]; !ok {
		t.Fatal("metrics must carry concrete observations")
	}
}

func TestGrossWeightReconciliation_PartialEvidenceIsReviewNotFail(t *testing.T) {
	c := &GrossWeightReconciliationCheck{}
	res := c.Evaluate(shipmentFacts(map[string]float64{
		"commercial_invoice": 4650,
		"bill_of_lading":     5150,
	}), map[string]any{})
	if res.Status != StatusReview {
		t.Fatalf("partial evidence must REVIEW, got %s", res.Status)
	}
	if !strings.Contains(res.Reason, "missing sources") {
		t.Fatalf("reason must identify missing sources, got %q", res.Reason)
	}
	mm, ok := res.Metrics["missing_sources"].([]string)
	if !ok || len(mm) != 2 {
		t.Fatalf("expected 2 missing sources, got %v", res.Metrics["missing_sources"])
	}
}

func TestRequiredShipmentDocuments_MissingIsReview(t *testing.T) {
	c := &RequiredShipmentDocumentsCheck{}
	f := Facts{}
	f["shipment.gross_weight"] = append(f["shipment.gross_weight"],
		Fact{Value: 4650.0, SourceDoc: "commercial_invoice"})
	res := c.Evaluate(f, map[string]any{})
	if res.Status != StatusReview {
		t.Fatalf("missing docs must REVIEW, got %s (%s)", res.Status, res.Reason)
	}
	if !strings.Contains(res.Reason, "1 of 4") {
		t.Fatalf("reason must count present/required, got %q", res.Reason)
	}
}

func TestRequiredShipmentDocuments_ParamsOverride(t *testing.T) {
	c := &RequiredShipmentDocumentsCheck{}
	f := Facts{}
	f["shipment.gross_weight"] = append(f["shipment.gross_weight"],
		Fact{Value: 4650.0, SourceDoc: "commercial_invoice"},
		Fact{Value: 5150.0, SourceDoc: "bill_of_lading"})
	res := c.Evaluate(f, map[string]any{
		"required": []any{"commercial_invoice", "bill_of_lading"},
	})
	if res.Status != StatusPass {
		t.Fatalf("params-driven required set should PASS, got %s (%s)", res.Status, res.Reason)
	}
}

func TestNormalizeDocType_TradeDocsPassThrough(t *testing.T) {
	for _, d := range []types.DocumentType{
		types.DocTypeCommercialInvoice, types.DocTypePackingList,
		types.DocTypeBillOfLading, types.DocTypeCertificateOfOrigin,
	} {
		if got := normalizeDocType(string(d)); got != string(d) {
			t.Fatalf("canonical trade doc %q mangled to %q", d, got)
		}
	}
}
