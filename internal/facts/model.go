package facts

import "github.com/doctrust/doctrust/internal/types"

// Re-export shared leaf types.
type DocumentType = types.DocumentType
type EvidenceRef = types.EvidenceRef

const (
	DocTypePaystub  = types.DocTypePaystub
	DocTypeW2       = types.DocTypeW2
	DocType1040     = types.DocType1040
	DocTypeBankStmt = types.DocTypeBankStmt
	DocTypeUnknown  = types.DocTypeUnknown
)

// Facts maps semantic type → all observations of that type.
type Facts map[string][]Fact

// Fact represents a canonical observation with full provenance.
type Fact struct {
	Value      any
	SourceDoc  string
	FieldName  string
	SourceSpan string
	Confidence float64
}
