package ingest

import (
	"fmt"

	"github.com/PithomLabs/doctrust/internal/extraction"
	"github.com/PithomLabs/doctrust/internal/facts"
	"github.com/PithomLabs/doctrust/internal/nutrient"
	"github.com/PithomLabs/doctrust/internal/types"
)

// Normalizer converts Nutrient extraction output into Facts.
type Normalizer struct {
	configs map[nutrient.DocumentType]extraction.ExtractionConfig
}

// NewNormalizer creates a normalizer with income verification configs.
func NewNormalizer() *Normalizer {
	return &Normalizer{
		configs: extraction.IncomeVerificationConfigs(),
	}
}

// Normalize takes a Nutrient extraction result and produces Facts + Evidence for the eval engine
func (n *Normalizer) Normalize(docID, filename string, docHash string, docType types.DocumentType, result *nutrient.ExtractFieldsResponse) (facts.NormalizedOutput, error) {
	config, ok := n.configs[docType]
	if !ok {
		return facts.NormalizedOutput{}, fmt.Errorf("no extraction config for document type %s", docType)
	}

	var evidenceWithSource []facts.EvidenceWithSource
	f := make(facts.Facts)

	for field, norm := range config.FieldMapping {
		value, ok := result.Output.Data[field]
		if !ok {
			continue
		}
		citation, ok := result.Output.Metadata[field]
		if !ok {
			continue
		}
		parsedValue := extraction.ParseCurrencyValue(value)

		fact := facts.Fact{
			Value:      parsedValue,
			SourceDoc:  filename,
			FieldName:  field,
			Confidence: citation.Confidence,
		}

		if citation.Bbox != nil {
			fact.SourceSpan = fmt.Sprintf("page=%d;bbox=[%.1f,%.1f,%.1f,%.1f]",
				citation.PageNumber, citation.Bbox.X, citation.Bbox.Y, citation.Bbox.Width, citation.Bbox.Height)
		} else if len(citation.SourceBboxes) > 0 {
			sb := citation.SourceBboxes[0]
			if sb.Bbox != nil {
				fact.SourceSpan = fmt.Sprintf("page=%d;bbox=[%.1f,%.1f,%.1f,%.1f]",
					sb.PageNumber, sb.Bbox.X, sb.Bbox.Y, sb.Bbox.Width, sb.Bbox.Height)
			}
		}

		f[norm.SemanticType] = append(f[norm.SemanticType], fact)

		if citation.Bbox != nil {
			evidenceWithSource = append(evidenceWithSource, facts.EvidenceWithSource{
				EvidenceRef: types.EvidenceRef{
					Field:      norm.SemanticType,
					SourceSpan: fmt.Sprintf("page=%d;bbox=[%.1f,%.1f,%.1f,%.1f]", citation.PageNumber, citation.Bbox.X, citation.Bbox.Y, citation.Bbox.Width, citation.Bbox.Height),
					Confidence: citation.Confidence,
				},
				DocumentID: docID,
				Filename:   filename,
				Page:       citation.PageNumber,
			})
		} else if len(citation.SourceBboxes) > 0 {
			for _, sb := range citation.SourceBboxes {
				if sb.Bbox != nil {
					evidenceWithSource = append(evidenceWithSource, facts.EvidenceWithSource{
						EvidenceRef: types.EvidenceRef{
							Field:      norm.SemanticType,
							SourceSpan: fmt.Sprintf("page=%d;bbox=[%.1f,%.1f,%.1f,%.1f]", sb.PageNumber, sb.Bbox.X, sb.Bbox.Y, sb.Bbox.Width, sb.Bbox.Height),
							Confidence: citation.Confidence,
						},
						DocumentID: docID,
						Filename:   filename,
						Page:       sb.PageNumber,
					})
				}
			}
		}
	}

	return facts.NormalizedOutput{
		Facts:    f,
		Evidence: evidenceWithSource,
		Documents: []facts.Document{
			{ID: docID, Filename: filename, Hash: "", Type: string(docType)},
		},
	}, nil
}
