package ingest

import (
	"strings"

	"github.com/doctrust/doctrust/internal/types"
)

// ClassifyDocument determines the document type from the filename.
func ClassifyDocument(filename string) types.DocumentType {
	lower := strings.ToLower(filename)
	switch {
	case strings.Contains(lower, "paystub") || strings.Contains(lower, "earnings"):
		return types.DocTypePaystub
	case strings.Contains(lower, "w2") || strings.Contains(lower, "w-2"):
		return types.DocTypeW2
	case strings.Contains(lower, "1040") || strings.Contains(lower, "tax"):
		return types.DocType1040
	case strings.Contains(lower, "bank") || strings.Contains(lower, "statement"):
		return types.DocTypeBankStmt
	default:
		return types.DocTypeUnknown
	}
}
