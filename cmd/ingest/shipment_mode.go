package main

// Shipment-domain ingest mode (plan10 D1): builds normalized evidence
// snapshots for trade documents via the Nutrient provider, with optional
// extension of an existing snapshot (progressive evidence). The legacy
// income-verification positional-arg mode in main() is untouched.

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/PithomLabs/doctrust/internal/ingest"
	"github.com/PithomLabs/doctrust/internal/nutrient"
	"github.com/PithomLabs/doctrust/internal/types"
)

func isShipmentInvocation(args []string) bool {
	for _, a := range args {
		if a == "-domain" || a == "--domain" {
			return true
		}
	}
	return false
}

func loadExtractionKey() (string, error) {
	if k := os.Getenv("NUTRIENT_DWS_EXTRACTION_API_KEY"); k != "" {
		return k, nil
	}
	candidates := []string{
		filepath.Join("doctrust", ".env"),
		".env",
	}
	for _, p := range candidates {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "extraction_apikey=") {
				return strings.TrimPrefix(line, "extraction_apikey="), nil
			}
		}
	}
	return "", fmt.Errorf("NUTRIENT_DWS_EXTRACTION_API_KEY not set")
}

func parseDocPairs(raw string) (map[types.DocumentType]string, error) {
	out := map[types.DocumentType]string{}
	for _, pair := range strings.Split(raw, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("malformed --docs pair %q (want type=path)", pair)
		}
		t := types.DocumentType(strings.TrimSpace(parts[0]))
		switch t {
		case types.DocTypeCommercialInvoice, types.DocTypePackingList,
			types.DocTypeBillOfLading, types.DocTypeCertificateOfOrigin:
		default:
			return nil, fmt.Errorf("unknown document type %q", t)
		}
		out[t] = strings.TrimSpace(parts[1])
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no documents given")
	}
	return out, nil
}

func runShipmentMode(args []string) error {
	fs := flag.NewFlagSet("ingest-shipment", flag.ExitOnError)
	domain := fs.String("domain", "shipment_release", "compliance domain (must be shipment_release)")
	docsFlag := fs.String("docs", "", "comma-separated type=path pairs (commercial_invoice|packing_list|bill_of_lading|certificate_of_origin)")
	extendFrom := fs.String("extend-from", "", "existing snapshot to extend")
	outDir := fs.String("out", "", "output directory (default: directory of first document)")
	mode := fs.String("mode", "understand", "provider extraction mode")
	reportPath := fs.String("report", "", "optional path for the extraction report JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *domain != "shipment_release" {
		return fmt.Errorf("unsupported domain %q (only shipment_release)", *domain)
	}

	docs, err := parseDocPairs(*docsFlag)
	if err != nil {
		return err
	}
	key, err := loadExtractionKey()
	if err != nil {
		return err
	}

	client := nutrient.NewClient(key, "")
	graph, report, err := ingest.BuildShipmentSnapshot(client, ingest.SnapshotOptions{
		Docs:       docs,
		ExtendFrom: *extendFrom,
		Mode:       *mode,
	})
	if err != nil {
		if report != nil && *reportPath != "" {
			writeShipmentReport(report, *reportPath)
		}
		return err
	}

	dir := *outDir
	if dir == "" {
		for _, p := range docs {
			dir = filepath.Dir(p)
			break
		}
	}
	name := "evidence_snapshot.json"
	if *extendFrom != "" {
		name = "evidence_snapshot_extended.json"
	}
	path, err := ingest.WriteSnapshot(graph, dir, name)
	if err != nil {
		return err
	}
	if *reportPath != "" {
		writeShipmentReport(report, *reportPath)
	}

	fmt.Printf("%d documents\n%d claims\n%d relationships\ncase_id: %s\nsnapshot: %s\n",
		len(graph.Documents), len(graph.Claims), len(graph.Relationships),
		graph.CaseID, path)
	return nil
}

func writeShipmentReport(report *ingest.ExtractionReport, path string) {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0o644)
}
