// Package provider defines the provider-neutral evidence seam (plan10 §6).
//
// DocTrust asks for facts ("shipment.gross_weight from bill_of_lading");
// an EvidenceProvider implementation translates that generic need into
// provider-specific document operations. The first implementation wraps the
// Nutrient DWS extraction API via internal/nutrient; Foxit and Doctavian can
// satisfy the same contract without touching the Ruleset or evaluator.
//
// Boundary note (AGENTS R13): internal/service and cmd/doctrust-mcp MUST NOT
// import this package. Only the ingest path (cmd/ingest, cmd/evidence-mcp)
// sits on the provider side of the trust boundary.
package provider

import "encoding/json"

// RawExtraction is one provider-reported observation before normalization.
// Raw is retained for provenance/debugging only and never decision-bearing.
type RawExtraction struct {
	FieldName  string
	Value      string
	Page       int
	BBox       []float64 // provider-native page space, pre-rescale
	Confidence float64
	Raw        json.RawMessage
}

// ExtractionSchema is the generic per-document field request handed to a
// provider. Field descriptions are semantic, not provider-specific.
type ExtractionSchema struct {
	DocumentType string
	Fields       map[string]string // field name -> human description
}

// EvidenceProvider extracts structured fields from an artifact file.
type EvidenceProvider interface {
	ExtractFields(docPath string, schema ExtractionSchema) ([]RawExtraction, error)
	Name() string
}
