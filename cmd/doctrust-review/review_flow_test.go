package main

import (
	"strings"
	"testing"

	"github.com/doctrust/doctrust/internal/eval"
	"github.com/doctrust/doctrust/internal/service"
	"github.com/doctrust/doctrust/internal/types"
)

func TestCompareFindings_Match(t *testing.T) {
	sidecar := []service.SidecarFinding{
		{Index: 0, CheckID: "required_shipment_documents", Status: "REVIEW", Severity: "BLOCKING",
			Reason: "insufficient evidence", Evidence: []types.EvidenceRef{
				{Field: "gross_weight", SourceDoc: "bill_of_lading", SourceSpan: "page=1", Confidence: 0.95},
			}},
	}
	evalResults := []eval.Result{
		{CheckID: "required_shipment_documents", Status: "REVIEW", Severity: "BLOCKING",
			Reason: "insufficient evidence", Evidence: []types.EvidenceRef{
				{Field: "gross_weight", SourceDoc: "bill_of_lading", SourceSpan: "page=1", Confidence: 0.95},
			}},
	}
	if err := compareFindings(sidecar, evalResults); err != nil {
		t.Fatalf("identical findings should match: %v", err)
	}
}

func TestCompareFindings_CountMismatch(t *testing.T) {
	sidecar := []service.SidecarFinding{{Index: 0, CheckID: "a", Status: "PASS", Severity: "INFO", Reason: "ok"}}
	evalResults := []eval.Result{{CheckID: "a", Status: "PASS", Severity: "INFO", Reason: "ok"},
		{CheckID: "b", Status: "REVIEW", Severity: "WARNING", Reason: "extra"}}
	if err := compareFindings(sidecar, evalResults); err == nil {
		t.Fatal("count mismatch must be detected")
	} else if !strings.Contains(err.Error(), "count mismatch") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCompareFindings_CheckIDMismatch(t *testing.T) {
	sidecar := []service.SidecarFinding{{Index: 0, CheckID: "a", Status: "PASS", Severity: "INFO", Reason: "ok"}}
	evalResults := []eval.Result{{CheckID: "b", Status: "PASS", Severity: "INFO", Reason: "ok"}}
	if err := compareFindings(sidecar, evalResults); err == nil {
		t.Fatal("check_id mismatch must be detected")
	} else if !strings.Contains(err.Error(), "check_id mismatch") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCompareFindings_StatusMismatch(t *testing.T) {
	sidecar := []service.SidecarFinding{{Index: 0, CheckID: "a", Status: "PASS", Severity: "INFO", Reason: "ok"}}
	evalResults := []eval.Result{{CheckID: "a", Status: "REVIEW", Severity: "INFO", Reason: "ok"}}
	if err := compareFindings(sidecar, evalResults); err == nil {
		t.Fatal("status mismatch must be detected")
	} else if !strings.Contains(err.Error(), "status mismatch") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCompareFindings_SeverityMismatch(t *testing.T) {
	sidecar := []service.SidecarFinding{{Index: 0, CheckID: "a", Status: "PASS", Severity: "INFO", Reason: "ok"}}
	evalResults := []eval.Result{{CheckID: "a", Status: "PASS", Severity: "BLOCKING", Reason: "ok"}}
	if err := compareFindings(sidecar, evalResults); err == nil {
		t.Fatal("severity mismatch must be detected")
	} else if !strings.Contains(err.Error(), "severity mismatch") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCompareFindings_ReasonMismatch(t *testing.T) {
	sidecar := []service.SidecarFinding{{Index: 0, CheckID: "a", Status: "PASS", Severity: "INFO", Reason: "all good"}}
	evalResults := []eval.Result{{CheckID: "a", Status: "PASS", Severity: "INFO", Reason: "something wrong"}}
	if err := compareFindings(sidecar, evalResults); err == nil {
		t.Fatal("reason mismatch must be detected")
	} else if !strings.Contains(err.Error(), "reason mismatch") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCompareFindings_EvidenceCountMismatch(t *testing.T) {
	sidecar := []service.SidecarFinding{{Index: 0, CheckID: "a", Status: "PASS", Severity: "INFO", Reason: "ok",
		Evidence: []types.EvidenceRef{{Field: "x"}}}}
	evalResults := []eval.Result{{CheckID: "a", Status: "PASS", Severity: "INFO", Reason: "ok",
		Evidence: []types.EvidenceRef{{Field: "x"}, {Field: "y"}}}}
	if err := compareFindings(sidecar, evalResults); err == nil {
		t.Fatal("evidence count mismatch must be detected")
	} else if !strings.Contains(err.Error(), "evidence count mismatch") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCompareFindings_EvidenceFieldMismatch(t *testing.T) {
	sidecar := []service.SidecarFinding{{Index: 0, CheckID: "a", Status: "PASS", Severity: "INFO", Reason: "ok",
		Evidence: []types.EvidenceRef{{Field: "weight", SourceDoc: "b_l", SourceSpan: "page=1", Confidence: 0.9}}}}
	evalResults := []eval.Result{{CheckID: "a", Status: "PASS", Severity: "INFO", Reason: "ok",
		Evidence: []types.EvidenceRef{{Field: "weight", SourceDoc: "b_l", SourceSpan: "page=2", Confidence: 0.9}}}}
	if err := compareFindings(sidecar, evalResults); err == nil {
		t.Fatal("evidence field mismatch must be detected")
	} else if !strings.Contains(err.Error(), "evidence") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCompareFindings_Empty(t *testing.T) {
	if err := compareFindings(nil, nil); err != nil {
		t.Fatalf("nil/nil should match: %v", err)
	}
	if err := compareFindings([]service.SidecarFinding{}, []eval.Result{}); err != nil {
		t.Fatalf("empty/empty should match: %v", err)
	}
}
