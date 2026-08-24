package types

// DocumentType is the canonical document type identifier.
// Used by facts, evidence, nutrient, and extraction layers.
type DocumentType string

const (
	DocTypePaystub  DocumentType = "paystub"
	DocTypeW2       DocumentType = "w2"
	DocType1040     DocumentType = "form_1040"
	DocTypeBankStmt DocumentType = "bank_statement"
	DocTypeUnknown  DocumentType = "unknown"

	// Trade-document domain (shipment_release).
	DocTypeCommercialInvoice   DocumentType = "commercial_invoice"
	DocTypePackingList         DocumentType = "packing_list"
	DocTypeBillOfLading        DocumentType = "bill_of_lading"
	DocTypeCertificateOfOrigin DocumentType = "certificate_of_origin"
)
