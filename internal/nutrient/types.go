package nutrient

import (
	"time"

	"github.com/PithomLabs/doctrust/internal/types"
)

// ExtractFieldsRequest is the instructions JSON sent with extract_fields.
type ExtractFieldsRequest struct {
	Schema      map[string]any  `json:"schema"`
	ParseConfig ParseConfig     `json:"parseConfig"`
	Options     *ExtractOptions `json:"options,omitempty"`
}

type ParseConfig struct {
	Mode string `json:"mode"` // "structure", "understand", "agentic"
}

type ExtractOptions struct {
	IncludeCitations *bool `json:"includeCitations,omitempty"`
	Strict           *bool `json:"strict,omitempty"`
	Multimodal       *bool `json:"multimodal,omitempty"`
}

// ExtractFieldsResponse is the top-level response from POST /extraction/extract.
type ExtractFieldsResponse struct {
	Output  ExtractFieldsOutput `json:"output"`
	Metrics *ExtractionMetrics  `json:"metrics,omitempty"`
	Usage   *CreditUsage        `json:"usage,omitempty"`
}

type ExtractFieldsOutput struct {
	Data     map[string]any           `json:"data"`
	Metadata map[string]FieldCitation `json:"metadata,omitempty"`
	Pages    []PageInfo               `json:"pages,omitempty"`
}

// FieldCitation is the per-field citation from extract_fields metadata.
type FieldCitation struct {
	Bbox                 *BBox                 `json:"bbox,omitempty"`
	Confidence           float64               `json:"confidence,omitempty"`
	ConfidenceComponents *ConfidenceComponents `json:"confidenceComponents,omitempty"`
	Match                string                `json:"match,omitempty"` // "id_match", "not_found", etc.
	PageIndex            int                   `json:"pageIndex,omitempty"`
	PageNumber           int                   `json:"pageNumber,omitempty"`
	SourceBboxes         []SourceBBox          `json:"source_bboxes,omitempty"`
}

type ConfidenceComponents struct {
	GroundingScore float64 `json:"groundingScore,omitempty"`
	Source         string  `json:"source,omitempty"`
}

type SourceBBox struct {
	Bbox       *BBox  `json:"bbox,omitempty"`
	BlockID    string `json:"block_id,omitempty"`
	PageIndex  int    `json:"pageIndex,omitempty"`
	PageNumber int    `json:"pageNumber,omitempty"`
}

type BBox struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// ParseDocumentRequest is the instructions JSON sent with parse_document.
type ParseDocumentRequest struct {
	Mode   string      `json:"mode"` // "text", "structure", "understand", "agentic"
	Output ParseOutput `json:"output"`
}

type ParseOutput struct {
	Format string `json:"format,omitempty"` // "spatial", "markdown"
}

// ParseDocumentResponse is the top-level response from POST /extraction/parse.
type ParseDocumentResponse struct {
	Output  ParseOutputResponse `json:"output"`
	Metrics *ExtractionMetrics  `json:"metrics,omitempty"`
	Usage   *CreditUsage        `json:"usage,omitempty"`
}

type ParseOutputResponse struct {
	Elements []SpatialElement `json:"elements,omitempty"`
	Markdown string           `json:"markdown,omitempty"`
}

// SpatialElement represents a spatial element from parse_document.
type SpatialElement struct {
	Type       string    `json:"type,omitempty"`
	Role       string    `json:"role,omitempty"`
	Confidence float64   `json:"confidence,omitempty"`
	Bounds     *BBox     `json:"bounds,omitempty"`
	Page       *PageInfo `json:"page,omitempty"`
	Text       string    `json:"text,omitempty"`
}

type PageInfo struct {
	PageIndex  int     `json:"pageIndex,omitempty"`
	PageNumber int     `json:"pageNumber,omitempty"`
	Width      float64 `json:"width,omitempty"`
	Height     float64 `json:"height,omitempty"`
}

type ExtractionMetrics struct {
	PagesProcessed int `json:"pagesProcessed,omitempty"`
}

type CreditUsage struct {
	DataExtractionCredits *DataExtractionCredits `json:"data_extraction_credits,omitempty"`
}

type DataExtractionCredits struct {
	Cost             float64 `json:"cost,omitempty"`
	RemainingCredits float64 `json:"remainingCredits,omitempty"`
}

// DocumentType represents the classified type of a document.
type DocumentType = types.DocumentType

const (
	DocTypePaystub  = types.DocTypePaystub
	DocTypeW2       = types.DocTypeW2
	DocType1040     = types.DocType1040
	DocTypeBankStmt = types.DocTypeBankStmt
	DocTypeUnknown  = types.DocTypeUnknown

	DocTypeCommercialInvoice   = types.DocTypeCommercialInvoice
	DocTypePackingList         = types.DocTypePackingList
	DocTypeBillOfLading        = types.DocTypeBillOfLading
	DocTypeCertificateOfOrigin = types.DocTypeCertificateOfOrigin
)

// ExtractionResult holds the full extraction output for a single document.
type ExtractionResult struct {
	FileName     string                   `json:"file_name"`
	DocumentType DocumentType             `json:"document_type"`
	Fields       map[string]any           `json:"fields"`
	Metadata     map[string]FieldCitation `json:"metadata"`
	Pages        []PageInfo               `json:"pages,omitempty"`
	ExtractedAt  time.Time                `json:"extracted_at"`
}
