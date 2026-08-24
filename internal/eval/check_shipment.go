package eval

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/doctrust/doctrust/internal/types"
)

// GrossWeightReconciliationCheck verifies that the declared shipment gross
// weight agrees across all required trade documents. Missing sources yield
// REVIEW (insufficient evidence), never a silent PASS; a genuine mismatch
// yields REVIEW with concrete per-source observations so a human authority
// can resolve it.
type GrossWeightReconciliationCheck struct{}

func (c *GrossWeightReconciliationCheck) ID() string {
	return "gross_weight_reconciliation"
}

func (c *GrossWeightReconciliationCheck) Version() string {
	return "1.0"
}

func (c *GrossWeightReconciliationCheck) Evaluate(facts Facts, params map[string]any) Result {
	semanticType := "shipment.gross_weight"
	if s, ok := params["semantic_type"].(string); ok && s != "" {
		semanticType = s
	}
	unit := "KG"
	if s, ok := params["unit"].(string); ok && s != "" {
		unit = s
	}
	tolerance := 0.005 // exact at 2-decimal document precision
	if t, ok := params["tolerance"].(float64); ok && t >= 0 {
		tolerance = t
	}
	requiredSources := []string{
		string(types.DocTypeCommercialInvoice),
		string(types.DocTypePackingList),
		string(types.DocTypeCertificateOfOrigin),
		string(types.DocTypeBillOfLading),
	}
	if raw, ok := params["sources"].([]any); ok && len(raw) > 0 {
		requiredSources = requiredSources[:0]
		for _, r := range raw {
			if s, ok := r.(string); ok {
				requiredSources = append(requiredSources, s)
			}
		}
	}

	all := getAllFacts(facts, semanticType)
	bySource := map[string]Fact{}
	for _, f := range all {
		canonical := normalizeDocType(f.SourceDoc)
		if _, dup := bySource[canonical]; dup {
			ev := evidenceRefs(semanticType, all)
			return Result{
				CheckID:  c.ID(),
				Status:   StatusReview,
				Severity: SeverityBlocking,
				Reason: fmt.Sprintf("conflicting %s observations within source %q",
					semanticType, canonical),
				Evidence: ev,
				Metrics:  map[string]any{"condition": "all_equal", "unit": unit},
			}
		}
		bySource[canonical] = f
	}

	var missing []string
	for _, src := range requiredSources {
		if _, ok := bySource[src]; !ok {
			missing = append(missing, src)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return Result{
			CheckID:  c.ID(),
			Status:   StatusReview,
			Severity: SeverityBlocking,
			Reason: fmt.Sprintf("insufficient evidence for %s: missing sources %s",
				semanticType, strings.Join(missing, ", ")),
			Evidence: evidenceRefs(semanticType, all),
			Metrics: map[string]any{
				"condition":        "all_equal",
				"unit":             unit,
				"missing_sources":  missing,
				"present_sources":  len(bySource),
				"required_sources": len(requiredSources),
			},
		}
	}

	values := make(map[string]float64, len(requiredSources))
	var first float64
	outliers := []string{}
	for i, src := range requiredSources {
		f := bySource[src]
		v, ok := toFloat64(f.Value)
		if !ok {
			return Result{
				CheckID:  c.ID(),
				Status:   StatusReview,
				Severity: SeverityBlocking,
				Reason: fmt.Sprintf("non-numeric %s observation from %q (%v)",
					semanticType, src, f.Value),
				Evidence: evidenceRefs(semanticType, all),
			}
		}
		values[src] = v
		if i == 0 {
			first = v
		} else if math.Abs(v-first) > tolerance {
			outliers = append(outliers, src)
		}
	}

	ev := evidenceRefs(semanticType, all)
	if len(outliers) > 0 {
		obs := map[string]any{}
		for src, v := range values {
			obs[src] = v
		}
		sort.Strings(outliers)
		detailParts := make([]string, 0, len(values))
		for _, src := range requiredSources {
			detailParts = append(detailParts, fmt.Sprintf("%s=%.2f %s", src, values[src], unit))
		}
		return Result{
			CheckID:  c.ID(),
			Status:   StatusReview,
			Severity: SeverityBlocking,
			Reason: fmt.Sprintf("gross weight reconciliation failed: %s differ (%s); outliers: %s",
				strings.Join(requiredSources, ", "),
				strings.Join(detailParts, ", "),
				strings.Join(outliers, ", ")),
			Evidence: ev,
			Metrics: map[string]any{
				"condition":    "all_equal",
				"unit":         unit,
				"observations": obs,
				"outliers":     outliers,
			},
		}
	}

	return Result{
		CheckID:  c.ID(),
		Status:   StatusPass,
		Severity: SeverityInfo,
		Reason: fmt.Sprintf("gross weight reconciles across %d documents (%.2f %s)",
			len(requiredSources), first, unit),
		Evidence: ev,
		Metrics: map[string]any{
			"condition":    "all_equal",
			"unit":         unit,
			"value":        first,
			"source_count": len(requiredSources),
		},
	}
}

func evidenceRefs(semanticType string, factsList []Fact) []types.EvidenceRef {
	refs := make([]types.EvidenceRef, 0, len(factsList))
	for _, f := range factsList {
		refs = append(refs, types.EvidenceRef{
			Field:      semanticType,
			SourceDoc:  f.SourceDoc,
			SourceSpan: f.SourceSpan,
			Confidence: f.Confidence,
		})
	}
	return refs
}

// RequiredShipmentDocumentsCheck verifies that every required trade-document
// type contributed at least one fact. Missing documents mean the evidence set
// is insufficient for ANY approval decision, so the check returns REVIEW
// (never FAIL and never PASS) — the progressive-evidence workflow depends on
// this semantics.
type RequiredShipmentDocumentsCheck struct{}

func (c *RequiredShipmentDocumentsCheck) ID() string {
	return "required_shipment_documents"
}

func (c *RequiredShipmentDocumentsCheck) Version() string {
	return "1.0"
}

func (c *RequiredShipmentDocumentsCheck) Evaluate(facts Facts, params map[string]any) Result {
	required := []string{
		string(types.DocTypeCommercialInvoice),
		string(types.DocTypePackingList),
		string(types.DocTypeCertificateOfOrigin),
		string(types.DocTypeBillOfLading),
	}
	if raw, ok := params["required"].([]any); ok && len(raw) > 0 {
		required = required[:0]
		for _, r := range raw {
			if s, ok := r.(string); ok {
				required = append(required, s)
			}
		}
	}

	present := map[string]bool{}
	for _, factList := range facts {
		for _, fact := range factList {
			if fact.SourceDoc != "" {
				present[normalizeDocType(fact.SourceDoc)] = true
			}
		}
	}

	var missing []string
	for _, req := range required {
		if !present[req] {
			missing = append(missing, req)
		}
	}
	sort.Strings(missing)

	if len(missing) > 0 {
		return Result{
			CheckID:  c.ID(),
			Status:   StatusReview,
			Severity: SeverityBlocking,
			Reason: fmt.Sprintf("insufficient evidence: %d of %d required shipment documents present; missing: %s",
				len(required)-len(missing), len(required), strings.Join(missing, ", ")),
			Evidence: []types.EvidenceRef{},
			Metrics: map[string]any{
				"missing":        missing,
				"present_count":  len(required) - len(missing),
				"required_count": len(required),
			},
		}
	}

	return Result{
		CheckID:  c.ID(),
		Status:   StatusPass,
		Severity: SeverityInfo,
		Reason:   fmt.Sprintf("all %d required shipment documents present", len(required)),
		Evidence: []types.EvidenceRef{},
		Metrics:  map[string]any{"required_count": len(required)},
	}
}
