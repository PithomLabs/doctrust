package facts

import (
	"github.com/PithomLabs/doctrust/internal/types"
)

// NormalizedOutput is the result of normalization - Facts + Evidence for the eval engine
type NormalizedOutput struct {
	Facts     Facts
	Evidence  []EvidenceWithSource
	Documents []Document
}

// EvidenceWithSource pairs an EvidenceRef with its source document info
type EvidenceWithSource struct {
	types.EvidenceRef
	DocumentID string
	Filename   string
	Page       int
}

// Document represents an ingested document (for Facts output)
type Document struct {
	ID       string `json:"id"`
	Filename string `json:"filename"`
	Hash     string `json:"hash"`
	Type     string `json:"type"`
}
