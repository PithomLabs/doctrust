package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/doctrust/doctrust/internal/evidence"
	"github.com/doctrust/doctrust/internal/nutrient"
)

// Schema per document type for extract_fields.
var schemas = map[nutrient.DocumentType]map[string]any{
	nutrient.DocTypePaystub: {
		"type": "object",
		"properties": map[string]any{
			"annualized_gross_ytd": map[string]any{
				"type":        "string",
				"description": "Annualized year-to-date gross earnings",
			},
			"base_salary_ytd": map[string]any{
				"type":        "string",
				"description": "YTD base salary before bonuses",
			},
			"bonus_ytd": map[string]any{
				"type":        "string",
				"description": "YTD bonus or incentive compensation",
			},
		},
	},
	nutrient.DocTypeW2: {
		"type": "object",
		"properties": map[string]any{
			"wages_tips_other_compensation": map[string]any{
				"type":        "string",
				"description": "Box 1: Wages, tips, other compensation",
			},
		},
	},
	nutrient.DocType1040: {
		"type": "object",
		"properties": map[string]any{
			"line1z_wages": map[string]any{
				"type":        "string",
				"description": "Line 1z: Total wages from W-2",
			},
		},
	},
	nutrient.DocTypeBankStmt: {
		"type": "object",
		"properties": map[string]any{
			"total_deposits": map[string]any{
				"type":        "string",
				"description": "Total deposits for the statement period",
			},
		},
	},
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <directory>\n", os.Args[0])
		os.Exit(1)
	}
	dir := os.Args[1]

	// Load API key
	extractionKey := os.Getenv("NUTRIENT_DWS_EXTRACTION_API_KEY")
	if extractionKey == "" {
		data, err := os.ReadFile("/home/chaschel/Desktop/biz/nutrient/.env")
		if err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "extraction_apikey=") {
					extractionKey = strings.TrimPrefix(line, "extraction_apikey=")
				}
			}
		}
	}
	if extractionKey == "" {
		fmt.Fprintf(os.Stderr, "Error: NUTRIENT_DWS_EXTRACTION_API_KEY not set\n")
		os.Exit(1)
	}

	client := nutrient.NewClient(extractionKey, "")
	normalizer := evidence.NewNormalizer()

	// Find PDF files
	entries, err := os.ReadDir(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading directory: %v\n", err)
		os.Exit(1)
	}

	var documents []evidence.Document
	var allClaims []evidence.Claim
	totalFields := 0

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".pdf") {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		docType := evidence.ClassifyDocument(entry.Name())

		fileBytes, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading %s: %v\n", entry.Name(), err)
			continue
		}
		docHash := evidence.ComputeHash(fileBytes)

		schema, ok := schemas[docType]
		if !ok {
			fmt.Fprintf(os.Stderr, "Warning: no schema for %s (type=%s), treating as unknown\n", entry.Name(), docType)
			doc := evidence.Document{
				ID:       fmt.Sprintf("doc_%d", len(documents)+1),
				Filename: entry.Name(),
				Hash:     docHash,
				Type:     nutrient.DocTypeUnknown,
			}
			documents = append(documents, doc)
			uncertainClaim := evidence.Claim{
				ID:           fmt.Sprintf("claim_%d", len(allClaims)+1),
				Field:        "document_classification",
				SemanticType: "document_classification",
				Value:        "unknown",
				ValueType:    "string",
				Status:       evidence.ClaimUncertain,
				Sources: []evidence.Source{
					{DocumentID: doc.ID, Filename: entry.Name(), Page: 1, Confidence: 0, FieldName: "document_type"},
				},
			}
			allClaims = append(allClaims, uncertainClaim)
			continue
		}

		fmt.Fprintf(os.Stderr, "Extracting: %s (type=%s)\n", entry.Name(), docType)

		result, err := client.ExtractFields(path, schema, "understand")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error extracting %s: %v\n", entry.Name(), err)
			continue
		}

		doc := evidence.Document{
			ID:       fmt.Sprintf("doc_%d", len(documents)+1),
			Filename: entry.Name(),
			Hash:     docHash,
			Type:     docType,
		}
		documents = append(documents, doc)
		totalFields += len(result.Output.Data)

		extResult := &nutrient.ExtractionResult{
			FileName:     entry.Name(),
			DocumentType: docType,
			Fields:       result.Output.Data,
			Metadata:     result.Output.Metadata,
			Pages:        result.Output.Pages,
		}

		claims := normalizer.Normalize(doc, extResult)
		allClaims = append(allClaims, claims...)
	}

	// Build relationships
	relationships := normalizer.BuildRelationships(allClaims)

	// Build evidence graph
	graph := evidence.EvidenceGraph{
		CaseID:        "income_verification_001",
		Documents:     documents,
		Claims:        allClaims,
		Relationships: relationships,
		CreatedAt:     time.Now(),
	}

	// Print summary
	fmt.Printf("%d documents\n", len(documents))
	fmt.Printf("%d extracted fields\n", totalFields)
	fmt.Printf("%d semantic claims\n", len(allClaims))
	fmt.Printf("%d relationships\n", len(relationships))
	fmt.Printf("%d corroboration\n", countByType(relationships, evidence.RelCorroboration))
	fmt.Printf("%d variance\n", countByType(relationships, evidence.RelVariance))
	fmt.Printf("%d incomparable\n", countByType(relationships, evidence.RelIncomparable))
	fmt.Printf("%d derived_from\n", countByType(relationships, evidence.RelDerivedFrom))
	fmt.Printf("all provenance present: %v\n", allProvenancePresent(allClaims))

	// Write evidence snapshot
	outPath := filepath.Join(dir, "evidence_snapshot.json")
	data, _ := json.MarshalIndent(graph, "", "  ")
	if err := os.WriteFile(outPath, data, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing snapshot: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "\nEvidence snapshot written to %s\n", outPath)
}

func countByType(rels []evidence.Relationship, t evidence.RelationshipType) int {
	count := 0
	for _, r := range rels {
		if r.Type == t {
			count++
		}
	}
	return count
}

func allProvenancePresent(claims []evidence.Claim) bool {
	for _, c := range claims {
		if len(c.Sources) == 0 {
			return false
		}
		for _, s := range c.Sources {
			if s.Confidence == 0 {
				return false
			}
		}
	}
	return true
}
