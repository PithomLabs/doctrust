package ingest

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/PithomLabs/doctrust/internal/evidence"
	"github.com/PithomLabs/doctrust/internal/extraction"
	"github.com/PithomLabs/doctrust/internal/nutrient"
)

// EvidenceNormalizer converts Nutrient extraction output into claims and relationships.
type EvidenceNormalizer struct {
	configs map[nutrient.DocumentType]extraction.ExtractionConfig
}

// NewEvidenceNormalizer creates a normalizer with income verification configs.
func NewEvidenceNormalizer() *EvidenceNormalizer {
	return &EvidenceNormalizer{
		configs: extraction.IncomeVerificationConfigs(),
	}
}

// Normalize takes a Nutrient extraction result and produces claims.
func (n *EvidenceNormalizer) Normalize(doc evidence.Document, result *nutrient.ExtractionResult, pdfW, pdfH float64) []evidence.Claim {
	config, ok := n.configs[result.DocumentType]
	if !ok {
		return nil
	}

	var claims []evidence.Claim
	for field, norm := range config.FieldMapping {
		value, ok := result.Fields[field]
		if !ok {
			continue
		}

		claim := evidence.Claim{
			ID:           fmt.Sprintf("%s.%s", result.DocumentType, field),
			Field:        field,
			SemanticType: norm.SemanticType,
			Value:        extraction.ParseCurrencyValue(value),
			ValueType:    norm.ValueType,
			Sources:      extractSources(doc, result, field, pdfW, pdfH),
			Status:       evidence.ClaimSingular,
		}
		claims = append(claims, claim)
	}
	return claims
}

// BuildRelationships analyzes claims and produces cross-document relationships.
func (n *EvidenceNormalizer) BuildRelationships(claims []evidence.Claim) []evidence.Relationship {
	var rels []evidence.Relationship

	byType := make(map[string][]evidence.Claim)
	for _, c := range claims {
		byType[c.SemanticType] = append(byType[c.SemanticType], c)
	}

	for stype, group := range byType {
		if len(group) < 2 {
			continue
		}
		for i := 0; i < len(group); i++ {
			for j := i + 1; j < len(group); j++ {
				rels = append(rels, evidence.Relationship{
					ID:     fmt.Sprintf("rel_%s_%s_%s", stype, group[i].ID, group[j].ID),
					Type:   evidence.RelCorroboration,
					ClaimA: group[i].ID,
					ClaimB: group[j].ID,
				})
			}
		}
	}

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

			rels = append(rels, evidence.Relationship{
				ID:     fmt.Sprintf("rel_%s_%s", projected.ID, taxable.ID),
				Type:   evidence.RelVariance,
				ClaimA: projected.ID,
				ClaimB: taxable.ID,
				Delta:  diff,
			})

			bonus := findClaimBySemanticType(claims, "bonus_compensation")
			if bonus != nil {
				rels = append(rels, evidence.Relationship{
					ID:        fmt.Sprintf("rel_derived_%s_%s", projected.ID, bonus.ID),
					Type:      evidence.RelDerivedFrom,
					ClaimA:    projected.ID,
					ClaimB:    bonus.ID,
					Semantics: "gross_ytd includes bonus_compensation",
				})
			}
		}
	}

	netClaims := findClaimsBySemanticType(claims, "net_cash_flow")
	taxableClaims := findClaimsBySemanticType(claims, "gross_income_taxable")
	for _, net := range netClaims {
		for _, tax := range taxableClaims {
			rels = append(rels, evidence.Relationship{
				ID:        fmt.Sprintf("rel_incomp_%s_%s", net.ID, tax.ID),
				Type:      evidence.RelIncomparable,
				ClaimA:    net.ID,
				ClaimB:    tax.ID,
				Semantics: "net_cash_flow vs gross_taxable_income",
			})
		}
	}

	return rels
}

func extractSources(doc evidence.Document, result *nutrient.ExtractionResult, field string, pdfW, pdfH float64) []evidence.Source {
	citation, ok := result.Metadata[field]
	if !ok {
		return nil
	}

	nutW, nutH := 1700.0, 2200.0
	if len(result.Pages) > 0 {
		nutW = result.Pages[0].Width
		nutH = result.Pages[0].Height
	}

	scaleX := pdfW / nutW
	scaleY := pdfH / nutH

	var sources []evidence.Source

	if citation.Bbox != nil {
		sources = append(sources, evidence.Source{
			DocumentID: doc.ID,
			Filename:   doc.Filename,
			Page:       citation.PageNumber,
			BBox:       toViewerCoords(citation.Bbox, scaleX, scaleY),
			Confidence: citation.Confidence,
			FieldName:  field,
		})
	} else {
		for _, sb := range citation.SourceBboxes {
			if sb.Bbox != nil {
				sources = append(sources, evidence.Source{
					DocumentID: doc.ID,
					Filename:   doc.Filename,
					Page:       sb.PageNumber,
					BBox:       toViewerCoords(sb.Bbox, scaleX, scaleY),
					Confidence: citation.Confidence,
					FieldName:  field,
				})
			}
		}
	}

	return sources
}

func toViewerCoords(bbox *nutrient.BBox, scaleX, scaleY float64) []float64 {
	if bbox == nil {
		return nil
	}
	return []float64{bbox.X * scaleX, bbox.Y * scaleY, bbox.Width * scaleX, bbox.Height * scaleY}
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

func findClaimByID(claims []evidence.Claim, id string) *evidence.Claim {
	for i := range claims {
		if claims[i].ID == id {
			return &claims[i]
		}
	}
	return nil
}

func findClaimBySemanticType(claims []evidence.Claim, stype string) *evidence.Claim {
	for i := range claims {
		if claims[i].SemanticType == stype {
			return &claims[i]
		}
	}
	return nil
}

func findClaimsBySemanticType(claims []evidence.Claim, stype string) []evidence.Claim {
	var result []evidence.Claim
	for _, c := range claims {
		if c.SemanticType == stype {
			result = append(result, c)
		}
	}
	return result
}
