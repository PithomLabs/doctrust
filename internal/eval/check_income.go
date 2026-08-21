package eval

import (
	"fmt"
	"math"
	"strings"

	"github.com/doctrust/doctrust/internal/types"
)

// getFloat64 safely extracts float64 from Facts (first value in slice)
func getFloat64(facts Facts, key string) (float64, bool) {
	values, ok := facts[key]
	if !ok || len(values) == 0 {
		return 0, false
	}
	v := values[0].Value
	switch val := v.(type) {
	case float64:
		return val, true
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	default:
		return 0, false
	}
}

// getFloat64BySourceDoc extracts float64 from Facts for a specific source document.
// Returns the value from the fact whose SourceDoc contains the given identifier.
func getFloat64BySourceDoc(facts Facts, key string, sourceDoc string) (float64, bool) {
	values, ok := facts[key]
	if !ok || len(values) == 0 {
		return 0, false
	}
	// First, try exact match
	for _, fact := range values {
		if fact.SourceDoc == sourceDoc {
			if v, ok := toFloat64(fact.Value); ok {
				return v, true
			}
		}
	}
	// Then try substring match (case-insensitive)
	lower := strings.ToLower(sourceDoc)
	for _, fact := range values {
		if strings.Contains(strings.ToLower(fact.SourceDoc), lower) {
			if v, ok := toFloat64(fact.Value); ok {
				return v, true
			}
		}
	}
	// Fallback to first value
	return getFloat64(facts, key)
}

func toFloat64(v any) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	default:
		return 0, false
	}
}

// getFirstFact returns the first Fact for a given semantic type
func getFirstFact(facts Facts, key string) (Fact, bool) {
	values, ok := facts[key]
	if !ok || len(values) == 0 {
		return Fact{}, false
	}
	return values[0], true
}

// getAllFacts returns all Fact values for a given semantic type
func getAllFacts(facts Facts, key string) []Fact {
	return facts[key]
}

// normalizeDocType normalizes a source document name to a canonical type.
// Uses exact canonical matches only — no substring fallback.
func normalizeDocType(sourceDoc string) string {
	lower := strings.ToLower(strings.TrimSpace(sourceDoc))
	// Strip file extension
	if idx := strings.LastIndex(lower, "."); idx > 0 {
		lower = lower[:idx]
	}
	switch lower {
	case "paystub", "pay_stub", "paystub_extractor":
		return "paystub"
	case "w2", "w-2", "w_2":
		return "w2"
	case "form_1040", "1040", "irs_1040", "1040_form":
		return "form_1040"
	case "bank_statement", "bankstatement", "bank_stmt":
		return "bank_statement"
	}
	return sourceDoc
}

// GrossIncomeConsistencyCheck implements paystub variance check
type GrossIncomeConsistencyCheck struct{}

func (c *GrossIncomeConsistencyCheck) ID() string {
	return "gross_income_consistency"
}

func (c *GrossIncomeConsistencyCheck) Version() string {
	return "1.0"
}

func (c *GrossIncomeConsistencyCheck) Evaluate(facts Facts, params map[string]any) Result {
	// Parameters
	tolerance := 0.05
	if p, ok := params["tolerance"].(float64); ok && p > 0 {
		tolerance = p
	} else if _, ok := params["tolerance"]; ok {
		return Result{
			CheckID:  "gross_income_consistency",
			Status:   StatusFail,
			Severity: SeverityBlocking,
			Reason:   "invalid tolerance parameter",
		}
	}

	// bonus_field parameter must control which semantic type is used for bonus lookup
	bonusField := "bonus_compensation"
	if bf, ok := params["bonus_field"].(string); ok && bf != "" {
		bonusField = bf
	}

	// Get paystub gross (projected income)
	paystubGross, ok1 := getFloat64(facts, "gross_income_projected")

	// Get W-2 gross by document identity (not slice order)
	w2Gross, ok2 := getFloat64BySourceDoc(facts, "gross_income_taxable", "w2")
	if !ok2 {
		// Fallback: try any gross_income_taxable fact
		w2Gross, ok2 = getFloat64(facts, "gross_income_taxable")
	}

	if !ok1 || !ok2 {
		return Result{
			CheckID:  "gross_income_consistency",
			Status:   StatusReview,
			Severity: SeverityWarning,
			Reason:   "Missing required income fields",
			Evidence: []types.EvidenceRef{},
		}
	}

	// Use W-2 as primary comparison (corroborated by 1040)
	primaryGross := w2Gross

	variance := 0.0
	if primaryGross != 0 {
		variance = math.Abs(paystubGross-primaryGross) / primaryGross
	}

	// Build evidence with SourceSpan and Confidence from Fact.
	evidenceList := []types.EvidenceRef{}
	for _, fact := range facts["gross_income_projected"] {
		evidenceList = append(evidenceList, types.EvidenceRef{
			Field:      "gross_income_projected",
			SourceDoc:  fact.SourceDoc,
			SourceSpan: fact.SourceSpan,
			Confidence: fact.Confidence,
		})
	}
	for _, fact := range facts["gross_income_taxable"] {
		evidenceList = append(evidenceList, types.EvidenceRef{
			Field:      "gross_income_taxable",
			SourceDoc:  fact.SourceDoc,
			SourceSpan: fact.SourceSpan,
			Confidence: fact.Confidence,
		})
	}

	// Use the bonus_field parameter to look up bonus compensation
	if bonusVal, ok := getFloat64(facts, bonusField); ok && bonusVal > 0 {
		if variance > tolerance {
			for _, fact := range facts[bonusField] {
				evidenceList = append(evidenceList, types.EvidenceRef{
					Field:      bonusField,
					SourceDoc:  fact.SourceDoc,
					SourceSpan: fact.SourceSpan,
					Confidence: fact.Confidence,
				})
			}
			return Result{
				CheckID:  "gross_income_consistency",
				Status:   StatusReview,
				Severity: SeverityWarning,
				Reason:   "Variance exceeds tolerance; documented bonus may explain",
				Evidence: evidenceList,
				Metrics: map[string]any{
					"paystub_gross":       paystubGross,
					"w2_gross":           w2Gross,
					"variance_pct":       variance * 100,
					"tolerance_pct":      tolerance * 100,
					"bonus":              bonusVal,
					"explained_by_bonus": math.Abs(paystubGross-primaryGross-bonusVal) < primaryGross*0.01,
				},
			}
		}
	}

	if variance > tolerance {
		return Result{
			CheckID:  "gross_income_consistency",
			Status:   StatusReview,
			Severity: SeverityWarning,
			Reason:   "Paystub projected gross exceeds corroborated taxable income by " + formatPct(variance) + " (tolerance: " + formatPct(tolerance) + ")",
			Evidence: evidenceList,
			Metrics: map[string]any{
				"paystub_gross":  paystubGross,
				"w2_gross":       w2Gross,
				"variance_pct":   variance * 100,
				"tolerance_pct":  tolerance * 100,
			},
		}
	}

	return Result{
		CheckID:  "gross_income_consistency",
		Status:   StatusPass,
		Severity: SeverityInfo,
		Reason:   "Paystub gross within tolerance of W-2/1040",
		Evidence: evidenceList,
		Metrics: map[string]any{
			"paystub_gross": paystubGross,
			"w2_gross":      w2Gross,
			"variance_pct":  variance * 100,
			"tolerance_pct": tolerance * 100,
		},
	}
}

func formatPct(v float64) string {
	return fmt.Sprintf("%.1f%%", v*100)
}

// RequiredDocumentsCheck
type RequiredDocumentsCheck struct{}

func (c *RequiredDocumentsCheck) ID() string {
	return "required_documents"
}

func (c *RequiredDocumentsCheck) Version() string {
	return "1.0"
}

func (c *RequiredDocumentsCheck) Evaluate(facts Facts, params map[string]any) Result {
	// Required source document types (not semantic types).
	requiredDocs := []string{"paystub", "w2", "form_1040"}

	// Collect unique source document names from all facts, normalized to canonical types
	normalizedSeen := make(map[string]bool)
	for _, factList := range facts {
		for _, fact := range factList {
			if fact.SourceDoc != "" {
				normalized := normalizeDocType(fact.SourceDoc)
				normalizedSeen[normalized] = true
			}
		}
	}

	var missing []string
	for _, req := range requiredDocs {
		if !normalizedSeen[req] {
			missing = append(missing, req)
		}
	}

	if len(missing) > 0 {
		return Result{
			CheckID:  "required_documents",
			Status:   StatusFail,
			Severity: SeverityBlocking,
			Reason:   "Missing required documents: " + strings.Join(missing, ", "),
			Evidence: []types.EvidenceRef{},
			Metrics: map[string]any{
				"missing": missing,
			},
		}
	}

	return Result{
		CheckID:  "required_documents",
		Status:   StatusPass,
		Severity: SeverityInfo,
		Reason:   "All required documents present",
		Evidence: []types.EvidenceRef{},
	}
}

// NetVsGrossIncomparabilityCheck
type NetVsGrossIncomparabilityCheck struct{}

func (c *NetVsGrossIncomparabilityCheck) ID() string {
	return "net_vs_gross_incomparability"
}

func (c *NetVsGrossIncomparabilityCheck) Version() string {
	return "1.0"
}

func (c *NetVsGrossIncomparabilityCheck) Evaluate(facts Facts, params map[string]any) Result {
	_, hasNet := getFloat64(facts, "net_cash_flow")
	_, hasGross := getFloat64(facts, "gross_income_taxable")

	evList := []types.EvidenceRef{}
	for _, fact := range facts["net_cash_flow"] {
		evList = append(evList, types.EvidenceRef{
			Field:      "net_cash_flow",
			SourceDoc:  fact.SourceDoc,
			SourceSpan: fact.SourceSpan,
			Confidence: fact.Confidence,
		})
	}
	for _, fact := range facts["gross_income_taxable"] {
		evList = append(evList, types.EvidenceRef{
			Field:      "gross_income_taxable",
			SourceDoc:  fact.SourceDoc,
			SourceSpan: fact.SourceSpan,
			Confidence: fact.Confidence,
		})
	}

	if !hasNet {
		return Result{
			CheckID:  "net_vs_gross_incomparability",
			Status:   StatusReview,
			Severity: SeverityWarning,
			Reason:   "Bank statement missing; cannot verify net vs gross incomparability",
			Evidence: evList,
		}
	}

	if !hasGross {
		return Result{
			CheckID:  "net_vs_gross_incomparability",
			Status:   StatusReview,
			Severity: SeverityWarning,
			Reason:   "Gross income documents missing; cannot verify incomparability",
			Evidence: evList,
		}
	}

	return Result{
		CheckID:  "net_vs_gross_incomparability",
		Status:   StatusPass,
		Severity: SeverityInfo,
		Reason:   "Net cash flow correctly treated as incomparable to gross taxable income",
		Evidence: evList,
	}
}
