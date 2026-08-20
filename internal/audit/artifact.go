package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

type Artifact struct {
	Version          string                 `json:"version"`
	PolicyID         string                 `json:"policy_id"`
	PolicyHash       string                 `json:"policy_hash"`
	Decisions        []Decision             `json:"decisions"`
	Documents        []DocumentRecord       `json:"documents"`
	HumanReviews     []HumanReviewRecord    `json:"human_reviews,omitempty"`
	FinalDisposition string                 `json:"final_disposition,omitempty"`
	CreatedAt        time.Time              `json:"created_at"`
	CompletedAt      *time.Time             `json:"completed_at,omitempty"`
	Signatures       []Signature            `json:"signatures,omitempty"`
	Manifest         Manifest               `json:"manifest"`
}

type Decision struct {
	CaseID    string   `json:"case_id"`
	State     string   `json:"state"`
	Findings  []Finding `json:"findings"`
	DecidedAt time.Time `json:"decided_at"`
}

type Finding struct {
	Rule     string  `json:"rule"`
	Severity string  `json:"severity"`
	ClaimA   string  `json:"claim_a"`
	ClaimB   string  `json:"claim_b"`
	ValueA   float64 `json:"value_a"`
	ValueB   float64 `json:"value_b"`
	Variance float64 `json:"variance,omitempty"`
}

type DocumentRecord struct {
	FileName    string         `json:"file_name"`
	DocType     string         `json:"doc_type"`
	Hash        string         `json:"hash"`
	ExtractedAt time.Time      `json:"extracted_at"`
	Confidence  float64        `json:"confidence"`
	Citations   []Citation     `json:"citations,omitempty"`
}

type Citation struct {
	Field      string  `json:"field"`
	Page       int     `json:"page"`
	BBox       *BBox   `json:"bbox,omitempty"`
	Confidence float64 `json:"confidence"`
}

type BBox struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

type Signature struct {
	Algorithm string    `json:"algorithm"`
	SignedAt  time.Time `json:"signed_at"`
	CertID    string    `json:"cert_id"`
	Hash      string    `json:"hash"`
}

type Manifest struct {
	DocCount      int    `json:"doc_count"`
	DecisionCount int    `json:"decision_count"`
	ReviewCount   int    `json:"review_count"`
	ArtifactHash  string `json:"artifact_hash"`
}

type HumanReviewRecord struct {
	FindingIndex int       `json:"finding_index"`
	Action       string    `json:"action"`
	Note         string    `json:"note"`
	ResolvedAt   time.Time `json:"resolved_at"`
}

func NewArtifact(policyID, policyHash string) *Artifact {
	return &Artifact{
		Version:   "1.0",
		PolicyID:  policyID,
		PolicyHash: policyHash,
		CreatedAt: time.Now().UTC(),
	}
}

func (a *Artifact) AddDecision(d Decision) {
	a.Decisions = append(a.Decisions, d)
}

func (a *Artifact) AddDocument(d DocumentRecord) {
	a.Documents = append(a.Documents, d)
}

func (a *Artifact) AddHumanReview(r HumanReviewRecord) {
	a.HumanReviews = append(a.HumanReviews, r)
}

func (a *Artifact) SetFinalDisposition(disposition string) {
	a.FinalDisposition = disposition
}

func (a *Artifact) Finalize() {
	now := time.Now().UTC()
	a.CompletedAt = &now

	reviewCount := 0
	for _, d := range a.Decisions {
		if d.State == "REVIEW" {
			reviewCount++
		}
	}

	data, _ := json.Marshal(a)
	hash := sha256.Sum256(data)

	a.Manifest = Manifest{
		DocCount:      len(a.Documents),
		DecisionCount: len(a.Decisions),
		ReviewCount:   reviewCount,
		ArtifactHash:  hex.EncodeToString(hash[:]),
	}
}

func (a *Artifact) ToJSON() ([]byte, error) {
	return json.MarshalIndent(a, "", "  ")
}

func (a *Artifact) Hash() string {
	data, _ := json.Marshal(a)
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

func (a *Artifact) Summary() string {
	states := make(map[string]int)
	for _, d := range a.Decisions {
		states[d.State]++
	}
	return fmt.Sprintf("docs=%d decisions=%d (pass=%d fail=%d review=%d missing=%d)",
		len(a.Documents), len(a.Decisions),
		states["PASS"], states["FAIL"], states["REVIEW"], states["MISSING_EVIDENCE"])
}
