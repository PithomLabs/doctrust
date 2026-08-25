package extraction

import (
	"strconv"
	"strings"

	"github.com/PithomLabs/doctrust/internal/types"
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
func IncomeVerificationConfigs() map[types.DocumentType]ExtractionConfig {
	return map[types.DocumentType]ExtractionConfig{
		types.DocTypePaystub: {
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
		types.DocTypeW2: {
			DocumentType: "w2",
			FieldMapping: map[string]FieldNormalization{
				"wages_tips_other_compensation": {
					SemanticType: "gross_income_taxable",
					ValueType:    "currency",
				},
			},
		},
		types.DocType1040: {
			DocumentType: "form_1040",
			FieldMapping: map[string]FieldNormalization{
				"line1z_wages": {
					SemanticType: "gross_income_taxable",
					ValueType:    "currency",
				},
			},
		},
		types.DocTypeBankStmt: {
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

// ShipmentReleaseConfigs returns the extraction configs for the
// shipment_release domain. All four trade documents expose their gross weight
// under the same semantic type so the reconciliation check sees one
// comparable fact family. Identity fields are captured per document for
// provenance and cross-document corroboration reporting.
func ShipmentReleaseConfigs() map[types.DocumentType]ExtractionConfig {
	gw := func() FieldNormalization {
		return FieldNormalization{SemanticType: "shipment.gross_weight", ValueType: "currency"}
	}
	return map[types.DocumentType]ExtractionConfig{
		types.DocTypeCommercialInvoice: {
			DocumentType: "commercial_invoice",
			FieldMapping: map[string]FieldNormalization{
				"total_gross_weight": gw(),
				"invoice_number":     {SemanticType: "shipment.invoice_reference", ValueType: "string"},
				"shipment_id":        {SemanticType: "shipment.reference", ValueType: "string"},
				"container_number":   {SemanticType: "shipment.container", ValueType: "string"},
				"seal_number":        {SemanticType: "shipment.seal", ValueType: "string"},
			},
		},
		types.DocTypePackingList: {
			DocumentType: "packing_list",
			FieldMapping: map[string]FieldNormalization{
				"total_gross_weight": gw(),
				"packing_list_number": {SemanticType: "shipment.packing_list_reference",
					ValueType: "string"},
				"container_number": {SemanticType: "shipment.container", ValueType: "string"},
				"seal_number":      {SemanticType: "shipment.seal", ValueType: "string"},
			},
		},
		types.DocTypeBillOfLading: {
			DocumentType: "bill_of_lading",
			FieldMapping: map[string]FieldNormalization{
				"gross_weight": gw(),
				"bill_of_lading_number": {SemanticType: "shipment.bill_of_lading_reference",
					ValueType: "string"},
				"container_number": {SemanticType: "shipment.container", ValueType: "string"},
				"seal_number":      {SemanticType: "shipment.seal", ValueType: "string"},
			},
		},
		types.DocTypeCertificateOfOrigin: {
			DocumentType: "certificate_of_origin",
			FieldMapping: map[string]FieldNormalization{
				"total_gross_weight": gw(),
				"certificate_number": {SemanticType: "shipment.certificate_reference",
					ValueType: "string"},
				"container_number": {SemanticType: "shipment.container", ValueType: "string"},
				"seal_number":      {SemanticType: "shipment.seal", ValueType: "string"},
			},
		},
	}
}

// ParseCurrencyValue parses a currency or mass string (e.g., "$120,000",
// "4,650.00 KG") into a float64. Trailing unit tokens are formatting, not
// semantics, and are stripped before the numeric parse.
func ParseCurrencyValue(v any) any {
	s, ok := v.(string)
	if !ok {
		return v
	}
	s = strings.ReplaceAll(s, "$", "")
	s = strings.ReplaceAll(s, ",", "")
	s = strings.TrimSpace(s)
	upper := strings.ToUpper(s)
	for _, unit := range []string{"KG", "CBM", "PCS"} {
		if strings.HasSuffix(upper, " "+unit) {
			s = strings.TrimSpace(s[:len(s)-len(unit)-1])
			break
		}
		if upper == unit {
			return v
		}
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f
	}
	return v
}
