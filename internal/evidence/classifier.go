package evidence

import (
	"strings"

	"github.com/doctrust/doctrust/internal/nutrient"
)

// ClassifyDocument determines the document type from the filename.
func ClassifyDocument(filename string) nutrient.DocumentType {
	lower := strings.ToLower(filename)
	switch {
	case strings.Contains(lower, "paystub") || strings.Contains(lower, "earnings"):
		return nutrient.DocTypePaystub
	case strings.Contains(lower, "w2") || strings.Contains(lower, "w-2"):
		return nutrient.DocTypeW2
	case strings.Contains(lower, "1040") || strings.Contains(lower, "tax"):
		return nutrient.DocType1040
	case strings.Contains(lower, "bank") || strings.Contains(lower, "statement"):
		return nutrient.DocTypeBankStmt
	default:
		return nutrient.DocTypeUnknown
	}
}
