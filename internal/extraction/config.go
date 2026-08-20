package extraction

import (
	"strconv"
	"strings"

	"github.com/doctrust/doctrust/internal/nutrient"
)

// ExtractionConfig defines how to normalize a specific document type.
type ExtractionConfig struct {
	DocumentType string
	FieldMapping map[string]FieldNormalization
}

// FieldNormalization defines how a single extracted field becomes a fact/claim.
type FieldNormalization struct {
	SemanticType string  // e.g. "gross_income_taxable"
	ValueType    string  // "currency", "string", "number"
	CompareWith  string  // claim ID to compare against for corroboration
	Threshold    float64 // max allowed percentage variance before triggering review
}

// IncomeVerificationConfigs returns the extraction configs for income verification.
func IncomeVerificationConfigs() map[nutrient.DocumentType]ExtractionConfig {
	return map[nutrient.DocumentType]ExtractionConfig{
		nutrient.DocTypePaystub: {
			DocumentType: "paystub",
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
			DocumentType: "w2",
			FieldMapping: map[string]FieldNormalization{
				"wages_tips_other_compensation": {
					SemanticType: "gross_income_taxable",
					ValueType:    "currency",
				},
			},
		},
		nutrient.DocType1040: {
			DocumentType: "form_1040",
			FieldMapping: map[string]FieldNormalization{
				"line1z_wages": {
					SemanticType: "gross_income_taxable",
					ValueType:    "currency",
				},
			},
		},
		nutrient.DocTypeBankStmt: {
			DocumentType: "bank_statement",
			FieldMapping: map[string]FieldNormalization{
				"total_deposits": {
					SemanticType: "net_cash_flow",
					ValueType:    "currency",
				},
			},
		},
	}
}

// ParseCurrencyValue parses a currency string (e.g., "$120,000") into a float64.
func ParseCurrencyValue(v any) any {
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
