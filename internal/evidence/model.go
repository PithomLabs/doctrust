package evidence

import (
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/PithomLabs/doctrust/internal/types"
)

// Re-export shared leaf types.
type DocumentType = types.DocumentType
type EvidenceRef = types.EvidenceRef

const (
	DocTypePaystub  = types.DocTypePaystub
	DocTypeW2       = types.DocTypeW2
	DocType1040     = types.DocType1040
	DocTypeBankStmt = types.DocTypeBankStmt
	DocTypeUnknown  = types.DocTypeUnknown

	DocTypeCommercialInvoice   = types.DocTypeCommercialInvoice
	DocTypePackingList         = types.DocTypePackingList
	DocTypeBillOfLading        = types.DocTypeBillOfLading
	DocTypeCertificateOfOrigin = types.DocTypeCertificateOfOrigin
)

// Document represents an ingested document.
type Document struct {
	ID       string       `json:"id"`
	Filename string       `json:"filename"`
	Hash     string       `json:"hash"`
	Type     DocumentType `json:"type"`
}

// Source represents provenance for every extracted value.
type Source struct {
	DocumentID string    `json:"document_id"`
	Filename   string    `json:"filename"`
	Page       int       `json:"page"`
	BBox       []float64 `json:"bbox,omitempty"`
	Confidence float64   `json:"confidence"`
	FieldName  string    `json:"field_name"`
}

// ClaimStatus describes the relationship status of a claim.
type ClaimStatus string

const (
	ClaimCorroborated ClaimStatus = "corroborated"
	ClaimConflicting  ClaimStatus = "conflicting"
	ClaimUncertain    ClaimStatus = "uncertain"
	ClaimSingular     ClaimStatus = "singular"
)

// Claim represents a semantically classified extracted value.
type Claim struct {
	ID           string      `json:"id"`
	Field        string      `json:"field"`
	SemanticType string      `json:"semantic_type"`
	Value        any         `json:"value"`
	ValueType    string      `json:"value_type"`
	Sources      []Source    `json:"sources"`
	Status       ClaimStatus `json:"status"`
}

// RelationshipType describes the type of relationship between claims.
type RelationshipType string

const (
	RelCorroboration RelationshipType = "corroborates"
	RelContradiction RelationshipType = "conflicts"
	RelVariance      RelationshipType = "variance"
	RelIncomparable  RelationshipType = "incomparable"
	RelDerivedFrom   RelationshipType = "derived_from"
)

// Relationship represents a cross-document relationship between claims.
type Relationship struct {
	ID        string           `json:"id"`
	Type      RelationshipType `json:"type"`
	ClaimA    string           `json:"claim_a"`
	ClaimB    string           `json:"claim_b"`
	Delta     any              `json:"delta,omitempty"`
	Semantics string           `json:"semantics,omitempty"`
}

// EvidenceGraph is the complete evidence snapshot for a case.
type EvidenceGraph struct {
	CaseID        string         `json:"case_id"`
	Documents     []Document     `json:"documents"`
	Claims        []Claim        `json:"claims"`
	Relationships []Relationship `json:"relationships"`
	CreatedAt     time.Time      `json:"created_at"`
}

// ComputeHash computes a SHA-256 hash of the document content.
func ComputeHash(data []byte) string {
	h := sha256.Sum256(data)
	return fmt.Sprintf("%x", h)
}
