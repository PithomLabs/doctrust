package evidence

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/doctrust/doctrust/internal/nutrient"
)

// ExtractionConfig defines how to normalize a specific document type.
type ExtractionConfig struct {
	DocumentType DocumentType
	// FieldMapping maps Nutrient field names to semantic claim IDs.
	FieldMapping map[string]FieldNormalization
}

// FieldNormalization defines how a single extracted field becomes a claim.
type FieldNormalization struct {
	SemanticType string // e.g. "gross_income_taxable"
	ValueType    string // "currency", "string", "number"
	// CompareWith is the claim ID this field should be compared against for corroboration.
	CompareWith string
	// Threshold is the max allowed percentage variance before triggering review.
	Threshold float64
}

// Normalizer converts Nutrient extraction output into claims and relationships.
type Normalizer struct {
	configs map[nutrient.DocumentType]ExtractionConfig
}

// NewNormalizer creates a normalizer with income verification configs.
func NewNormalizer() *Normalizer {
	return &Normalizer{
		configs: incomeVerificationConfigs(),
	}
}

func incomeVerificationConfigs() map[nutrient.DocumentType]ExtractionConfig {
	return map[nutrient.DocumentType]ExtractionConfig{
		nutrient.DocTypePaystub: {
			DocumentType: DocTypePaystub,
			FieldMapping: map[string]FieldNormalization{
				"annualized_gross_ytd": {
					SemanticType: "gross_income_projected",
					ValueType:    "currency",
					CompareWith:  "w2.wages_tips_other_compensation",
					Threshold:    5.0,
				},
				"base_salary_ytd": {
					SemanticType: "base_salary",
					ValueType:    "currency",
				},
				"bonus_ytd": {
					SemanticType: "bonus_compensation",
					ValueType:    "currency",
				},
			},
		},
		nutrient.DocTypeW2: {
			DocumentType: DocTypeW2,
			FieldMapping: map[string]FieldNormalization{
				"wages_tips_other_compensation": {
					SemanticType: "gross_income_taxable",
					ValueType:    "currency",
				},
			},
		},
		nutrient.DocType1040: {
			DocumentType: DocType1040,
			FieldMapping: map[string]FieldNormalization{
				"line1z_wages": {
					SemanticType: "gross_income_taxable",
					ValueType:    "currency",
				},
			},
		},
		nutrient.DocTypeBankStmt: {
			DocumentType: DocTypeBankStmt,
			FieldMapping: map[string]FieldNormalization{
				"total_deposits": {
					SemanticType: "net_cash_flow",
					ValueType:    "currency",
				},
			},
		},
	}
}

// Normalize takes a Nutrient extraction result and produces claims.
func (n *Normalizer) Normalize(doc Document, result *nutrient.ExtractionResult) []Claim {
	config, ok := n.configs[result.DocumentType]
	if !ok {
		return nil
	}

	var claims []Claim
	for field, norm := range config.FieldMapping {
		value, ok := result.Fields[field]
		if !ok {
			continue
		}

		claim := Claim{
			ID:           fmt.Sprintf("%s.%s", result.DocumentType, field),
			Field:        field,
			SemanticType: norm.SemanticType,
			Value:        parseCurrencyValue(value),
			ValueType:    norm.ValueType,
			Sources:      extractSources(doc, result, field),
			Status:       ClaimSingular,
		}
		claims = append(claims, claim)
	}
	return claims
}

// BuildRelationships analyzes claims and produces cross-document relationships.
func (n *Normalizer) BuildRelationships(claims []Claim) []Relationship {
	var rels []Relationship

	// Group claims by semantic type
	byType := make(map[string][]Claim)
	for _, c := range claims {
		byType[c.SemanticType] = append(byType[c.SemanticType], c)
	}

	// Corroboration: multiple claims with same semantic type
	for stype, group := range byType {
		if len(group) < 2 {
			continue
		}
		for i := 0; i < len(group); i++ {
			for j := i + 1; j < len(group); j++ {
				rels = append(rels, Relationship{
					ID:     fmt.Sprintf("rel_%s_%s_%s", stype, group[i].ID, group[j].ID),
					Type:   RelCorroboration,
					ClaimA: group[i].ID,
					ClaimB: group[j].ID,
				})
			}
		}
	}

	// Variance and incomparable: compare claims across semantic types
	if paystubConfig, ok := n.configs[nutrient.DocTypePaystub]; ok {
		for _, norm := range paystubConfig.FieldMapping {
			if norm.CompareWith == "" {
				continue
			}
			projected := findClaimBySemanticType(claims, norm.SemanticType)
			taxable := findClaimByID(claims, norm.CompareWith)
			if projected == nil || taxable == nil {
				continue
			}

			// Check if both are currency
			if projected.ValueType != "currency" || taxable.ValueType != "currency" {
				continue
			}

			valA := toFloat64(projected.Value)
			valB := toFloat64(taxable.Value)
			if valA == 0 || valB == 0 {
				continue
			}

			diff := valA - valB
			if diff < 0 {
				diff = -diff
			}

			rels = append(rels, Relationship{
				ID:     fmt.Sprintf("rel_%s_%s", projected.ID, taxable.ID),
				Type:   RelVariance,
				ClaimA: projected.ID,
				ClaimB: taxable.ID,
				Delta:  diff,
			})

			// Derived from: gross_ytd includes bonus
			bonus := findClaimBySemanticType(claims, "bonus_compensation")
			if bonus != nil {
				rels = append(rels, Relationship{
					ID:        fmt.Sprintf("rel_derived_%s_%s", projected.ID, bonus.ID),
					Type:      RelDerivedFrom,
					ClaimA:    projected.ID,
					ClaimB:    bonus.ID,
					Semantics: "gross_ytd includes bonus_compensation",
				})
			}
		}
	}

	// Incomparable: net_cash_flow vs gross_income_taxable
	netClaims := findClaimsBySemanticType(claims, "net_cash_flow")
	taxableClaims := findClaimsBySemanticType(claims, "gross_income_taxable")
	for _, net := range netClaims {
		for _, tax := range taxableClaims {
			rels = append(rels, Relationship{
				ID:        fmt.Sprintf("rel_incomp_%s_%s", net.ID, tax.ID),
				Type:      RelIncomparable,
				ClaimA:    net.ID,
				ClaimB:    tax.ID,
				Semantics: "net_cash_flow vs gross_taxable_income",
			})
		}
	}

	return rels
}

func extractSources(doc Document, result *nutrient.ExtractionResult, field string) []Source {
	citation, ok := result.Metadata[field]
	if !ok {
		return nil
	}

	var sources []Source
	for _, sb := range citation.SourceBboxes {
		s := Source{
			DocumentID: doc.ID,
			Filename:   doc.Filename,
			Page:       sb.PageNumber,
			Confidence: citation.Confidence,
			FieldName:  field,
		}
		if sb.Bbox != nil {
			s.BBox = []float64{sb.Bbox.X, sb.Bbox.Y, sb.Bbox.Width, sb.Bbox.Height}
		}
		sources = append(sources, s)
	}

	if len(sources) == 0 && citation.Bbox != nil {
		sources = append(sources, Source{
			DocumentID: doc.ID,
			Filename:   doc.Filename,
			Page:       citation.PageNumber,
			BBox:       []float64{citation.Bbox.X, citation.Bbox.Y, citation.Bbox.Width, citation.Bbox.Height},
			Confidence: citation.Confidence,
			FieldName:  field,
		})
	}

	return sources
}

func parseCurrencyValue(v any) any {
	s, ok := v.(string)
	if !ok {
		return v
	}
	s = strings.ReplaceAll(s, "$", "")
	s = strings.ReplaceAll(s, ",", "")
	s = strings.TrimSpace(s)
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f
	}
	return v
}

func toFloat64(v any) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case string:
		val = strings.ReplaceAll(val, "$", "")
		val = strings.ReplaceAll(val, ",", "")
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			return f
		}
	}
	return 0
}

func findClaimByID(claims []Claim, id string) *Claim {
	for i := range claims {
		if claims[i].ID == id {
			return &claims[i]
		}
	}
	return nil
}

func findClaimBySemanticType(claims []Claim, stype string) *Claim {
	for i := range claims {
		if claims[i].SemanticType == stype {
			return &claims[i]
		}
	}
	return nil
}

func findClaimsBySemanticType(claims []Claim, stype string) []Claim {
	var result []Claim
	for _, c := range claims {
		if c.SemanticType == stype {
			result = append(result, c)
		}
	}
	return result
}
