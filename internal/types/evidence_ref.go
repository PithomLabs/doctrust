package types

// EvidenceRef is the pointer from a check result back to source evidence.
// Used by eval results and service layer.
type EvidenceRef struct {
	Field      string  `json:"field" yaml:"field"`
	SourceDoc  string  `json:"source_doc" yaml:"source_doc"`
	SourceSpan string  `json:"source_span" yaml:"source_span"`
	Confidence float64 `json:"confidence" yaml:"confidence"`
}
