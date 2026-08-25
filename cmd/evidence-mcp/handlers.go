package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/PithomLabs/doctrust/internal/ingest"
	"github.com/PithomLabs/doctrust/internal/nutrient"
	"github.com/PithomLabs/doctrust/internal/types"
)

func okResult(out any) *mcp.CallToolResult {
	b, _ := json.Marshal(out)
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(b)}},
	}
}

func errResult(code, msg string) *mcp.CallToolResult {
	b, _ := json.Marshal(map[string]string{"code": code, "message": msg})
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(b)}},
		IsError: true,
	}
}

func withPanicRecovery(name string, h func() (*mcp.CallToolResult, error)) func() (*mcp.CallToolResult, error) {
	return func() (res *mcp.CallToolResult, err error) {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("tool panic recovered", "tool", name, "panic", r)
				res = errResult("INTERNAL_ERROR", fmt.Sprintf("internal error in %s", name))
			}
		}()
		return h()
	}
}

// parseDocs converts "commercial_invoice=/a.pdf,bill_of_lading=/b.pdf" into
// the canonical map. Unknown types are rejected — the agent cannot smuggle in
// arbitrary document classifications.
func parseDocs(raw string) (map[string]string, error) {
	out := map[string]string{}
	for _, pair := range strings.Split(raw, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return nil, fmt.Errorf("malformed pair %q (want type=path)", pair)
		}
		t := strings.TrimSpace(parts[0])
		if !knownShipmentType(t) {
			return nil, fmt.Errorf("unknown document type %q", t)
		}
		out[t] = strings.TrimSpace(parts[1])
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no documents given")
	}
	return out, nil
}

func knownShipmentType(t string) bool {
	switch t {
	case "commercial_invoice", "packing_list", "bill_of_lading",
		"certificate_of_origin":
		return true
	}
	return false
}

func extractionKey() (string, error) {
	if k := lookupCred("NUTRIENT_DWS_EXTRACTION_API_KEY"); k != "" {
		return k, nil
	}
	if k := lookupCred("extraction_apikey"); k != "" {
		return k, nil
	}
	return "", fmt.Errorf("NUTRIENT_DWS_EXTRACTION_API_KEY not set (env or --env-file)")
}

type snapshotOutput struct {
	SnapshotPath    string                  `json:"snapshot_path"`
	CaseID          string                  `json:"case_id"`
	PreviousCaseID  string                  `json:"previous_case_id,omitempty"`
	ProvenancePath  string                  `json:"provenance_path,omitempty"`
	DocumentCount   int                     `json:"document_count"`
	ClaimCount      int                     `json:"claim_count"`
	ExtractionState []ingest.DocumentResult `json:"extraction_documents"`
}

func registerTools(server *mcp.Server, root string) {
	server.AddTool(&mcp.Tool{
		Name:        "build_evidence_snapshot",
		Description: "Extract normalized compliance evidence from trade-document PDFs via the authorized provider and write an evidence snapshot. Accepts a SUBSET of required documents; missing documents surface at evaluation.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"documents": map[string]any{
					"type":        "string",
					"description": "Comma-separated type=path pairs; types: commercial_invoice|packing_list|bill_of_lading|certificate_of_origin",
				},
				"out_dir": map[string]any{
					"type":        "string",
					"description": "Directory for the snapshot file (must be inside the allowed root)",
				},
				"mode": map[string]any{
					"type":        "string",
					"description": "Provider extraction mode (default: understand)",
				},
			},
			"required": []string{"documents"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return withPanicRecovery("build_evidence_snapshot", func() (*mcp.CallToolResult, error) {
			var args struct {
				Documents string `json:"documents"`
				OutDir    string `json:"out_dir"`
				Mode      string `json:"mode"`
			}
			if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
				return errResult("INVALID_ARGUMENT", fmt.Sprintf("invalid arguments: %v", err)), nil
			}
			pairs, err := parseDocs(args.Documents)
			if err != nil {
				return errResult("INVALID_ARGUMENT", err.Error()), nil
			}
			typed := mapDocs(pairs)
			for _, p := range typed {
				if _, err := validateDocPath(p, root); err != nil {
					return errResult("INVALID_DOCUMENT_PATH", err.Error()), nil
				}
			}
			outDir := args.OutDir
			if outDir == "" {
				outDir = filepathDirOfFirst(typed)
			}
			if _, err := validateDirPath(outDir, root); err != nil {
				return errResult("INVALID_SNAPSHOT_PATH", err.Error()), nil
			}
			key, err := extractionKey()
			if err != nil {
				return errResult("PROVIDER_UNCONFIGURED", err.Error()), nil
			}
			client := nutrient.NewClient(key, "")
			graph, report, err := ingest.BuildShipmentSnapshot(client, ingest.SnapshotOptions{
				Docs: typed,
				Mode: args.Mode,
			})
			if err != nil {
				rep, _ := json.Marshal(report)
				slog.Error("extraction failed", "error", err, "report", string(rep))
				return errResult("EXTRACTION_FAILED", err.Error()), nil
			}
			path, err := ingest.WriteSnapshot(graph, outDir, "evidence_snapshot.json")
			if err != nil {
				return errResult("INTERNAL_ERROR", err.Error()), nil
			}
			writeReport(report, outDir, "")
			return okResult(snapshotOutput{
				SnapshotPath:    path,
				CaseID:          graph.CaseID,
				DocumentCount:   len(graph.Documents),
				ClaimCount:      len(graph.Claims),
				ExtractionState: report.Documents,
			}), nil
		})()
	})

	server.AddTool(&mcp.Tool{
		Name:        "extend_evidence_snapshot",
		Description: "Add further documents to an existing evidence snapshot. Produces a NEW snapshot file with a fresh content-derived case_id; the previous case remains referenced for provenance.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"snapshot_path": map[string]any{
					"type":        "string",
					"description": "Existing evidence snapshot to extend",
				},
				"add_documents": map[string]any{
					"type":        "string",
					"description": "Comma-separated type=path pairs to add",
				},
				"out_dir": map[string]any{
					"type":        "string",
					"description": "Directory for the extended snapshot (default: same directory as base)",
				},
			},
			"required": []string{"snapshot_path", "add_documents"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return withPanicRecovery("extend_evidence_snapshot", func() (*mcp.CallToolResult, error) {
			var args struct {
				SnapshotPath string `json:"snapshot_path"`
				AddDocuments string `json:"add_documents"`
				OutDir       string `json:"out_dir"`
			}
			if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
				return errResult("INVALID_ARGUMENT", fmt.Sprintf("invalid arguments: %v", err)), nil
			}
			base, err := validateSnapshotPath(args.SnapshotPath, root)
			if err != nil {
				return errResult("INVALID_SNAPSHOT_PATH", err.Error()), nil
			}
			pairs, err := parseDocs(args.AddDocuments)
			if err != nil {
				return errResult("INVALID_ARGUMENT", err.Error()), nil
			}
			typed := mapDocs(pairs)
			for _, p := range typed {
				if _, err := validateDocPath(p, root); err != nil {
					return errResult("INVALID_DOCUMENT_PATH", err.Error()), nil
				}
			}
			key, err := extractionKey()
			if err != nil {
				return errResult("PROVIDER_UNCONFIGURED", err.Error()), nil
			}
			client := nutrient.NewClient(key, "")
			graph, report, err := ingest.BuildShipmentSnapshot(client, ingest.SnapshotOptions{
				Docs:       typed,
				ExtendFrom: base,
			})
			if err != nil {
				return errResult("EXTRACTION_FAILED", err.Error()), nil
			}
			prevID := ""
			if prev, lerr := ingest.LoadSnapshot(base); lerr == nil {
				prevID = prev.CaseID
			}
			outDir := args.OutDir
			if outDir == "" {
				outDir = filepathDir(base)
			}
			path, err := ingest.WriteSnapshot(graph, outDir, "evidence_snapshot_extended.json")
			if err != nil {
				return errResult("INTERNAL_ERROR", err.Error()), nil
			}
			provPath, perr := ingest.WriteProvenance(path, base, typed)
			if perr != nil {
				slog.Warn("provenance sidecar write failed", "error", perr)
			}
			writeReport(report, outDir, "extended_")
			out := snapshotOutput{
				SnapshotPath:    path,
				CaseID:          graph.CaseID,
				PreviousCaseID:  prevID,
				DocumentCount:   len(graph.Documents),
				ClaimCount:      len(graph.Claims),
				ExtractionState: report.Documents,
			}
			if provPath != "" {
				out.ProvenancePath = provPath
			}
			return okResult(out), nil
		})()
	})
}

func mapDocs(pairs map[string]string) map[types.DocumentType]string {
	out := make(map[types.DocumentType]string, len(pairs))
	for t, p := range pairs {
		out[types.DocumentType(t)] = p
	}
	return out
}

func filepathDirOfFirst(m map[types.DocumentType]string) string {
	for _, p := range m {
		return filepathDir(p)
	}
	return "."
}

func writeReport(report *ingest.ExtractionReport, outDir, prefix string) {
	if report == nil {
		return
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(filepathJoin(outDir, prefix+"extraction_report.json"), data, 0o644)
}
