package facts

import (
	"fmt"

	"github.com/doctrust/doctrust/internal/evidence"
	"github.com/doctrust/doctrust/internal/extraction"
	"github.com/doctrust/doctrust/internal/nutrient"
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

// NormalizedOutput is the result of normalization - Facts + Evidence for the eval engine
type NormalizedOutput struct {
	Facts     Facts
	Evidence  []EvidenceWithSource
	Documents []Document
}

// EvidenceWithSource pairs an EvidenceRef with its source document info
type EvidenceWithSource struct {
	evidence.EvidenceRef
	DocumentID string
	Filename   string
	Page       int
}

// Document represents an ingested document (for Facts output)
type Document struct {
	ID       string `json:"id"`
	Filename string `json:"filename"`
	Hash     string `json:"hash"`
	Type     string `json:"type"` // string for JSON compatibility
}

// Normalize takes a Nutrient extraction result and produces Facts + Evidence for the eval engine
func (n *Normalizer) Normalize(docID, filename string, docHash string, docType evidence.DocumentType, result *nutrient.ExtractFieldsResponse) (NormalizedOutput, error) {
	config, ok := n.configs[docType]
	if !ok {
		return NormalizedOutput{}, fmt.Errorf("no extraction config for document type %s", docType)
	}

	var evidenceWithSource []EvidenceWithSource
	f := make(Facts)

	// Convert each extracted field to a Fact
	for field, norm := range config.FieldMapping {
		value, ok := result.Output.Data[field]
		if !ok {
			continue
		}

		// Get citation metadata
		citation, ok := result.Output.Metadata[field]
		if !ok {
			continue
		}

		// Parse the value
		parsedValue := extraction.ParseCurrencyValue(value)

		// Create Fact with full observation identity
		fact := Fact{
			Value:      parsedValue,
			SourceDoc:  filename,
			FieldName:  field,
			Confidence: citation.Confidence,
		}

		// Add bbox info to SourceSpan if available
		if citation.Bbox != nil {
			fact.SourceSpan = fmt.Sprintf("page=%d;bbox=[%.1f,%.1f,%.1f,%.1f]",
				citation.PageNumber, citation.Bbox.X, citation.Bbox.Y, citation.Bbox.Width, citation.Bbox.Height)
		} else if len(citation.SourceBboxes) > 0 {
			// Use first source bbox if available
			sb := citation.SourceBboxes[0]
			if sb.Bbox != nil {
				fact.SourceSpan = fmt.Sprintf("page=%d;bbox=[%.1f,%.1f,%.1f,%.1f]",
					sb.PageNumber, sb.Bbox.X, sb.Bbox.Y, sb.Bbox.Width, sb.Bbox.Height)
			}
		}

		// Append to slice (preserves multiple observations per semantic type)
		f[norm.SemanticType] = append(f[norm.SemanticType], fact)

		// Create evidence with source info
		if citation.Bbox != nil {
			evidenceWithSource = append(evidenceWithSource, EvidenceWithSource{
				EvidenceRef: evidence.EvidenceRef{
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
					evidenceWithSource = append(evidenceWithSource, EvidenceWithSource{
						EvidenceRef: evidence.EvidenceRef{
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

	return NormalizedOutput{
		Facts:    f,
		Evidence: evidenceWithSource,
		Documents: []Document{
			{ID: docID, Filename: filename, Hash: "", Type: string(docType)},
		},
	}, nil
}
