package service

import (
	"fmt"

	"github.com/doctrust/doctrust/internal/evidence"
	"github.com/doctrust/doctrust/internal/facts"
	"github.com/doctrust/doctrust/internal/types"
)

// BuildFactsFromSnapshot converts an EvidenceGraph into canonical Facts.
//
// CONTRACT:
//   - Input EvidenceGraph contains BBoxes that are ALREADY viewer-scaled
//     (scaled by toViewerCoords during ingestion in ingest/evidence_normalizer.go).
//     This function passes them through with NO additional transform.
//   - Fact.SourceDoc is the canonical document TYPE ("paystub", "w2", "form_1040",
//     "bank_statement"), NOT the filename. Resolved via snapshot.Documents[].Type map.
//   - Fact.FieldName is the extracted field name from Sources[].FieldName.
//   - Fact.SourceSpan is "page=N;bbox=[x,y,w,h]" — pass-through from snapshot.
//
// Returns error only for nil/invalid snapshot structure.
// Unmatched source filenames map to DocTypeUnknown (no error).
// Preserves source order within each semantic type.
func BuildFactsFromSnapshot(snapshot *evidence.EvidenceGraph) (facts.Facts, error) {
	if snapshot == nil {
		return nil, fmt.Errorf("nil snapshot")
	}

	docTypeByFilename := make(map[string]types.DocumentType, len(snapshot.Documents))
	for _, doc := range snapshot.Documents {
		docTypeByFilename[doc.Filename] = doc.Type
	}

	f := make(facts.Facts)
	for _, c := range snapshot.Claims {
		for _, src := range c.Sources {
			canonicalType, ok := docTypeByFilename[src.Filename]
			if !ok {
				canonicalType = types.DocTypeUnknown
			}

			sourceSpan := ""
			if len(src.BBox) >= 4 {
				sourceSpan = fmt.Sprintf("page=%d;bbox=[%.1f,%.1f,%.1f,%.1f]",
					src.Page, src.BBox[0], src.BBox[1], src.BBox[2], src.BBox[3])
			} else if src.Page > 0 {
				sourceSpan = fmt.Sprintf("page=%d", src.Page)
			}

			f[c.SemanticType] = append(f[c.SemanticType], facts.Fact{
				Value:      c.Value,
				SourceDoc:  string(canonicalType),
				FieldName:  src.FieldName,
				SourceSpan: sourceSpan,
				Confidence: src.Confidence,
			})
		}
	}
	return f, nil
}
