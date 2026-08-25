package ingest

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/PithomLabs/doctrust/internal/evidence"
	"github.com/PithomLabs/doctrust/internal/extraction"
	"github.com/PithomLabs/doctrust/internal/nutrient"
	"github.com/PithomLabs/doctrust/internal/types"
)

// ShipmentSchemas returns the Nutrient extraction schema per trade-document
// type. Field names mirror internal/extraction.ShipmentReleaseConfigs so the
// normalizer maps them onto shipment.gross_weight.
func ShipmentSchemas() map[types.DocumentType]map[string]any {
	gw := func(desc string) map[string]any {
		return map[string]any{"type": "string", "description": desc}
	}
	str := func(desc string) map[string]any {
		return map[string]any{"type": "string", "description": desc}
	}
	return map[types.DocumentType]map[string]any{
		types.DocTypeCommercialInvoice: {
			"type": "object",
			"properties": map[string]any{
				"total_gross_weight": gw("Total gross weight in kilograms from the invoice summary block (e.g. 4,650.00 KG)"),
				"invoice_number":     str("Invoice number, e.g. INV-2026-1047"),
				"shipment_id":        str("Shipment reference identifier, e.g. PH-EXP-1047"),
				"container_number":   str("Shipping container number, e.g. TCKU-918234-0"),
				"seal_number":        str("Container seal number digits"),
			},
		},
		types.DocTypePackingList: {
			"type": "object",
			"properties": map[string]any{
				"total_gross_weight":  gw("Gross weight in kilograms from the crate-schedule TOTALS row (e.g. 4,650.00 KG)"),
				"packing_list_number": str("Packing list number, e.g. PKL-2026-1047"),
				"container_number":    str("Shipping container number, e.g. TCKU-918234-0"),
				"seal_number":         str("Container seal number digits"),
			},
		},
		types.DocTypeBillOfLading: {
			"type": "object",
			"properties": map[string]any{
				"gross_weight":          gw("Gross weight in kilograms of the shipped goods including packaging, stated in the cargo block (e.g. 5,150.00 KG)"),
				"bill_of_lading_number": str("Bill of lading number, e.g. MSL-620984110"),
				"container_number":      str("Shipping container number, e.g. TCKU-918234-0"),
				"seal_number":           str("Container seal number digits"),
			},
		},
		types.DocTypeCertificateOfOrigin: {
			"type": "object",
			"properties": map[string]any{
				"total_gross_weight": gw("TOTAL GROSS WEIGHT in kilograms from the goods-table footer row (e.g. 4,650.00 KG)"),
				"certificate_number": str("Certificate of origin number, e.g. CO-PH-2026-08912"),
				"container_number":   str("Shipping container number, e.g. TCKU-918234-0"),
				"seal_number":        str("Container seal number digits"),
			},
		},
	}
}

// ClassifyShipmentDocument classifies a trade document by filename.
func ClassifyShipmentDocument(filename string) types.DocumentType {
	lower := strings.ToLower(filename)
	switch {
	case strings.Contains(lower, "commercial-invoice"):
		return types.DocTypeCommercialInvoice
	case strings.Contains(lower, "packing-list"):
		return types.DocTypePackingList
	case strings.Contains(lower, "bill-of-lading"):
		return types.DocTypeBillOfLading
	case strings.Contains(lower, "certificate-of-origin"):
		return types.DocTypeCertificateOfOrigin
	default:
		return types.DocTypeUnknown
	}
}

// DocumentResult captures the per-document outcome for the extraction report.
type DocumentResult struct {
	Type      string   `json:"type"`
	Filename  string   `json:"filename"`
	Status    string   `json:"status"` // extracted | failed | skipped_duplicate
	Error     string   `json:"error,omitempty"`
	Fields    []string `json:"fields,omitempty"`
	Citations int      `json:"citations,omitempty"`
	PageCount int      `json:"page_count,omitempty"`
}

// ExtractionReport is returned alongside every snapshot build.
type ExtractionReport struct {
	Domain    string           `json:"domain"`
	Documents []DocumentResult `json:"documents"`
	StartedAt time.Time        `json:"started_at"`
}

// SnapshotOptions configures BuildShipmentSnapshot.
type SnapshotOptions struct {
	// Docs maps canonical trade-document types to PDF paths. A subset is
	// allowed; missing required documents surface as REVIEW at evaluation.
	Docs map[types.DocumentType]string
	// ExtendFrom optionally loads a base snapshot; new documents are merged
	// on top of it and a fresh content-derived case_id is computed.
	ExtendFrom string
	// Mode passed to the Nutrient extraction API ("understand").
	Mode string
}

func shipmentNormalizer() *EvidenceNormalizer {
	return &EvidenceNormalizer{configs: extraction.ShipmentReleaseConfigs()}
}

// BuildShipmentSnapshot extracts evidence from the given trade documents via
// the Nutrient client and produces a deterministic evidence snapshot. The
// snapshot CaseID is derived from canonical snapshot content (documents,
// claims, relationships) — identical inputs yield identical case IDs.
func BuildShipmentSnapshot(client *nutrient.Client, opts SnapshotOptions) (*evidence.EvidenceGraph, *ExtractionReport, error) {
	if len(opts.Docs) == 0 {
		return nil, nil, fmt.Errorf("no documents provided")
	}
	if opts.Mode == "" {
		opts.Mode = "understand"
	}

	report := &ExtractionReport{Domain: "shipment_release", StartedAt: time.Now().UTC()}
	graph := &evidence.EvidenceGraph{}
	seenHashes := map[string]bool{}

	if opts.ExtendFrom != "" {
		base, err := LoadSnapshot(opts.ExtendFrom)
		if err != nil {
			return nil, report, fmt.Errorf("load base snapshot: %w", err)
		}
		for _, d := range base.Documents {
			graph.Documents = append(graph.Documents, d)
			seenHashes[d.Hash] = true
		}
		graph.Claims = append(graph.Claims, base.Claims...)
	}

	normalizer := shipmentNormalizer()
	schemas := ShipmentSchemas()

	// Iterate document types deterministically.
	keys := make([]types.DocumentType, 0, len(opts.Docs))
	for t := range opts.Docs {
		keys = append(keys, t)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })

	for _, docType := range keys {
		path := opts.Docs[docType]
		report.Documents = append(report.Documents, DocumentResult{
			Type:     string(docType),
			Filename: filepath.Base(path),
		})
		dr := &report.Documents[len(report.Documents)-1]

		fileBytes, err := os.ReadFile(path)
		if err != nil {
			dr.Status = "failed"
			dr.Error = fmt.Sprintf("read: %v", err)
			return nil, report, fmt.Errorf("read %s: %w", path, err)
		}
		docHash := evidence.ComputeHash(fileBytes)
		if seenHashes[docHash] {
			dr.Status = "skipped_duplicate"
			continue
		}

		schema, ok := schemas[docType]
		if !ok {
			dr.Status = "failed"
			dr.Error = "no extraction schema for document type"
			return nil, report, fmt.Errorf("no extraction schema for %s", docType)
		}

		result, err := client.ExtractFields(path, schema, opts.Mode)
		if err != nil {
			dr.Status = "failed"
			dr.Error = err.Error()
			return nil, report, fmt.Errorf("extract %s (%s): %w", filepath.Base(path), docType, err)
		}

		pdfW, pdfH := 612.0, 792.0 // Letter defaults
		if mw, mh, perr := ParseMediaBox(fileBytes); perr == nil && mw > 0 && mh > 0 {
			pdfW, pdfH = mw, mh
		}

		extResult := &nutrient.ExtractionResult{
			FileName:     filepath.Base(path),
			DocumentType: docType,
			Fields:       result.Output.Data,
			Metadata:     result.Output.Metadata,
			Pages:        result.Output.Pages,
		}

		doc := evidence.Document{
			ID:       fmt.Sprintf("doc_%s", strings.ToLower(string(docType))),
			Filename: filepath.Base(path),
			Hash:     docHash,
			Type:     docType,
		}
		graph.Documents = append(graph.Documents, doc)
		seenHashes[docHash] = true

		claims := normalizer.Normalize(doc, extResult, pdfW, pdfH)
		graph.Claims = append(graph.Claims, claims...)

		dr.Status = "extracted"
		dr.PageCount = len(result.Output.Pages)
		for field := range result.Output.Data {
			dr.Fields = append(dr.Fields, field)
		}
		sort.Strings(dr.Fields)
		for _, c := range result.Output.Metadata {
			if len(c.SourceBboxes) > 0 || c.Bbox != nil {
				dr.Citations++
			}
		}
	}

	graph.Relationships = normalizer.BuildRelationships(graph.Claims)

	// Deterministic claim ordering + IDs stable across runs.
	sort.Slice(graph.Claims, func(i, j int) bool { return graph.Claims[i].ID < graph.Claims[j].ID })
	graph.CaseID = deriveShipmentCaseID(graph)
	graph.CreatedAt = time.Now().UTC()

	return graph, report, nil
}

// WriteSnapshot persists the graph next to outPath and returns the path.
func WriteSnapshot(graph *evidence.EvidenceGraph, outDir, baseName string) (string, error) {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir out dir: %w", err)
	}
	path := filepath.Join(outDir, baseName)
	data, err := json.MarshalIndent(graph, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal snapshot: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", fmt.Errorf("write snapshot: %w", err)
	}
	return path, nil
}

// LoadSnapshot reads an existing evidence snapshot from disk.
func LoadSnapshot(path string) (*evidence.EvidenceGraph, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var g evidence.EvidenceGraph
	if err := json.Unmarshal(data, &g); err != nil {
		return nil, fmt.Errorf("parse snapshot %s: %w", path, err)
	}
	if len(g.Documents) == 0 {
		return nil, fmt.Errorf("snapshot %s has no documents", path)
	}
	return &g, nil
}

// deriveShipmentCaseID hashes a canonical projection of the graph that
// excludes CreatedAt (wall clock) so identical evidence yields identical IDs.
func deriveShipmentCaseID(g *evidence.EvidenceGraph) string {
	type canonicalDoc struct {
		ID       string `json:"id"`
		Filename string `json:"filename"`
		Hash     string `json:"hash"`
		Type     string `json:"type"`
	}
	type canonicalSource struct {
		DocumentID string    `json:"document_id"`
		Filename   string    `json:"filename"`
		Page       int       `json:"page"`
		BBox       []float64 `json:"bbox"`
		Confidence float64   `json:"confidence"`
		FieldName  string    `json:"field_name"`
	}
	type canonicalClaim struct {
		ID           string            `json:"id"`
		Field        string            `json:"field"`
		SemanticType string            `json:"semantic_type"`
		Value        any               `json:"value"`
		ValueType    string            `json:"value_type"`
		Sources      []canonicalSource `json:"sources"`
		Status       string            `json:"status"`
	}
	type canonicalRel struct {
		ID     string `json:"id"`
		Type   string `json:"type"`
		ClaimA string `json:"claim_a"`
		ClaimB string `json:"claim_b"`
		Delta  any    `json:"delta,omitempty"`
	}

	docs := make([]canonicalDoc, 0, len(g.Documents))
	for _, d := range g.Documents {
		docs = append(docs, canonicalDoc{d.ID, d.Filename, d.Hash, string(d.Type)})
	}
	claims := make([]canonicalClaim, 0, len(g.Claims))
	for _, c := range g.Claims {
		cc := canonicalClaim{c.ID, c.Field, c.SemanticType, c.Value, c.ValueType, nil, string(c.Status)}
		for _, s := range c.Sources {
			cc.Sources = append(cc.Sources, canonicalSource{s.DocumentID, s.Filename, s.Page, s.BBox, s.Confidence, s.FieldName})
		}
		claims = append(claims, cc)
	}
	rels := make([]canonicalRel, 0, len(g.Relationships))
	for _, r := range g.Relationships {
		rels = append(rels, canonicalRel{r.ID, string(r.Type), r.ClaimA, r.ClaimB, r.Delta})
	}
	sort.Slice(docs, func(i, j int) bool { return docs[i].ID < docs[j].ID })
	sort.Slice(claims, func(i, j int) bool { return claims[i].ID < claims[j].ID })
	sort.Slice(rels, func(i, j int) bool { return rels[i].ID < rels[j].ID })

	blob, err := json.Marshal(struct {
		Documents     []canonicalDoc   `json:"documents"`
		Claims        []canonicalClaim `json:"claims"`
		Relationships []canonicalRel   `json:"relationships"`
	}{docs, claims, rels})
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(blob)
	return fmt.Sprintf("shipment_%x", sum[:8])
}

// ProvenanceRecord documents the chain between an extended snapshot and the
// base it was built from (plan11 P5-5). Written as a sidecar JSON next to the
// extended snapshot; the frozen EvidenceGraph schema is unchanged.
type ProvenanceRecord struct {
	PreviousCaseID       string            `json:"previous_case_id"`
	PreviousSnapshotSha  string            `json:"previous_snapshot_sha256"`
	PreviousSnapshotPath string            `json:"previous_snapshot_path"`
	AddedDocuments       map[string]string `json:"added_documents"`
	ExtendedAt           time.Time         `json:"extended_at"`
}

// WriteProvenance writes <snapshot>.provenance.json describing the chain from
// previousSnapshotPath to snapshotPath.
func WriteProvenance(snapshotPath, previousSnapshotPath string, added map[types.DocumentType]string) (string, error) {
	prevBytes, err := os.ReadFile(previousSnapshotPath)
	if err != nil {
		return "", fmt.Errorf("read previous snapshot: %w", err)
	}
	sum := sha256.Sum256(prevBytes)
	addedStr := map[string]string{}
	for t, p := range added {
		addedStr[string(t)] = p
	}
	rec := ProvenanceRecord{
		PreviousSnapshotSha:  fmt.Sprintf("%x", sum),
		PreviousSnapshotPath: previousSnapshotPath,
		AddedDocuments:       addedStr,
		ExtendedAt:           time.Now().UTC(),
	}
	if prev, err := LoadSnapshot(previousSnapshotPath); err == nil {
		rec.PreviousCaseID = prev.CaseID
	}
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return "", err
	}
	path := snapshotPath + ".provenance.json"
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	return path, nil
}
