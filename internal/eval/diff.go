package eval

import (
	"math"

	"github.com/doctrust/doctrust/internal/evidence"
)

type ScenarioDiff struct {
	ScenarioName     string
	Before           Result
	After            Result
	Changed          bool
	StatusChanged    bool
	SeverityChanged  bool
	ReasonChanged    bool
	EvidenceChanged  bool
}

func DiffResults(after, before Result) *ScenarioDiff {
	if after.Status == before.Status &&
		after.Severity == before.Severity &&
		after.Reason == before.Reason &&
		evidenceEqual(after.Evidence, before.Evidence) {
		return nil
	}
	return &ScenarioDiff{
		Changed:         true,
		StatusChanged:   after.Status != before.Status,
		SeverityChanged: after.Severity != before.Severity,
		ReasonChanged:   after.Reason != before.Reason,
		EvidenceChanged: !evidenceEqual(after.Evidence, before.Evidence),
	}
}

type ScenarioDiffSummary struct {
	ScenarioName     string `json:"name"`
	Changed          bool   `json:"changed"`
	StatusChanged    bool   `json:"status_changed"`
	SeverityChanged  bool   `json:"severity_changed"`
	ReasonChanged    bool   `json:"reason_changed"`
	EvidenceChanged  bool   `json:"evidence_changed"`
	Before           Result `json:"before"`
	After            Result `json:"after"`
}

func (d *ScenarioDiff) ToSummary() ScenarioDiffSummary {
	if d == nil {
		return ScenarioDiffSummary{}
	}
	return ScenarioDiffSummary{
		ScenarioName:     d.ScenarioName,
		Changed:          d.Changed,
		StatusChanged:    d.StatusChanged,
		SeverityChanged:  d.SeverityChanged,
		ReasonChanged:    d.ReasonChanged,
		EvidenceChanged:  d.EvidenceChanged,
		Before:           d.Before,
		After:            d.After,
	}
}

// evidenceKey is the identity of an evidence entry: Field + SourceDoc.
type evidenceKey struct {
	Field     string
	SourceDoc string
}

// evidenceEqual compares two evidence lists using set semantics.
// Duplicate entries in actual (one per observation) are collapsed to unique fields.
// Identity is by Field. When expected specifies SourceDoc, actual must have at least
// one matching entry. When expected specifies SourceSpan/Confidence, actual must have
// at least one entry matching those provenance fields.
func evidenceEqual(a, b []evidence.EvidenceRef) bool {
	// Build unique field set from actual
	type actualEntry struct {
		SourceDoc  string
		SourceSpan string
		Confidence float64
	}
	actualByField := make(map[string][]actualEntry)
	for _, e := range a {
		actualByField[e.Field] = append(actualByField[e.Field], actualEntry{
			SourceDoc:  e.SourceDoc,
			SourceSpan: e.SourceSpan,
			Confidence: e.Confidence,
		})
	}

	// Count unique fields in expected
	expectedFields := make(map[string]bool)
	for _, e := range b {
		expectedFields[e.Field] = true
	}

	// Must have the same set of unique fields
	if len(actualByField) != len(expectedFields) {
		return false
	}
	for field := range expectedFields {
		if _, ok := actualByField[field]; !ok {
			return false
		}
	}

	// For each expected entry, check that at least one actual entry satisfies constraints
	for _, e := range b {
		entries := actualByField[e.Field]
		matched := false
		for _, entry := range entries {
			if e.SourceDoc != "" && e.SourceDoc != entry.SourceDoc {
				continue
			}
			if e.SourceSpan != "" && e.SourceSpan != entry.SourceSpan {
				continue
			}
			if e.Confidence != 0 && math.Abs(e.Confidence-entry.Confidence) > 0.01 {
				continue
			}
			matched = true
			break
		}
		if !matched {
			return false
		}
	}
	return true
}