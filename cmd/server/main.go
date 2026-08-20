package main

import (
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/doctrust/doctrust/internal/audit"
	"github.com/doctrust/doctrust/internal/eval"
	"github.com/doctrust/doctrust/internal/evidence"
	"github.com/doctrust/doctrust/internal/facts"
	"github.com/doctrust/doctrust/internal/nutrient"
	"github.com/doctrust/doctrust/internal/review"
)

var (
	snapshotDir    string
	policyHash     string
	nutrientKey    string
	processorKey   string
	store          *review.ReviewStore
	nutrientClient *nutrient.Client
	evalRunner     *eval.Runner
	loadedRuleset  eval.Ruleset
)

func main() {
	domain := flag.String("domain", "", "compiled domain (required)")
	port := flag.String("port", "8080", "listen port")
	snapDir := flag.String("dir", "demo/income_verification", "snapshot directory")
	flag.Parse()

	snapshotDir = *snapDir
	store = review.NewReviewStore()

	nutrientKey = os.Getenv("NUTRIENT_EXTRACTION_KEY")
	processorKey = os.Getenv("NUTRIENT_PROCESSOR_KEY")
	if nutrientKey != "" && processorKey != "" {
		nutrientClient = nutrient.NewClient(nutrientKey, processorKey)
	}

	if *domain == "" {
		fmt.Fprintln(os.Stderr, "Error: --domain is required")
		flag.Usage()
		os.Exit(1)
	}

	// Load ruleset via registry (promoted version) — no fallback
	registry := eval.NewRegistry("rulesets")
	rs, err := registry.LoadPromoted(*domain)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: no promoted ruleset found for domain %s: %v\n", *domain, err)
		os.Exit(1)
	}
	log.Printf("Loaded promoted ruleset: %s v%s", rs.ID, rs.Version)
	loadedRuleset = rs

	// Build check registry
	evalRunner = eval.NewRunner(eval.DefaultRegistry().All())

	// Load snapshot for policy hash
	snapshotPath := filepath.Join(snapshotDir, "evidence_snapshot.json")
	snapshotData, err := os.ReadFile(snapshotPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading snapshot: %v\n", err)
		os.Exit(1)
	}
	policyHash = fmt.Sprintf("%x", sha256.Sum256(snapshotData))

	mux := http.NewServeMux()
	mux.HandleFunc("/", handleIndex)
	mux.HandleFunc("/api/snapshot", handleSnapshot)
	mux.HandleFunc("/api/evaluate", handleEvaluate)
	mux.HandleFunc("/api/audit", handleAudit)
	mux.HandleFunc("/api/review", handleReview)
	mux.HandleFunc("/api/reviews", handleReviews)
	mux.HandleFunc("/api/disposition", handleDisposition)
	mux.HandleFunc("/api/finalize", handleFinalize)
	mux.HandleFunc("/api/sign", handleSign)
	mux.HandleFunc("/api/documents/", handleDocument)
	mux.HandleFunc("/static/", handleStatic)

	fmt.Printf("DocTrust server listening on :%s\n", *port)
	fmt.Printf("Ruleset: %s v%s\n", loadedRuleset.ID, loadedRuleset.Version)
	fmt.Printf("Snapshot: %s\n", snapshotDir)
	log.Fatal(http.ListenAndServe(":"+*port, mux))
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	tmpl, err := os.ReadFile("web/templates/index.html")
	if err != nil {
		http.Error(w, "template not found", 500)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	w.Write(tmpl)
}

func handleSnapshot(w http.ResponseWriter, r *http.Request) {
	snapshotPath := filepath.Join(snapshotDir, "evidence_snapshot.json")
	data, err := os.ReadFile(snapshotPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("snapshot not found: %v", err), 404)
		return
	}

	var snapshot evidence.EvidenceGraph
	if err := json.Unmarshal(data, &snapshot); err != nil {
		http.Error(w, fmt.Sprintf("invalid snapshot: %v", err), 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

// buildFactsFromSnapshot converts an EvidenceGraph into canonical Facts.
// Preserves all source observations for each claim (not just the first).
func buildFactsFromSnapshot(snapshot *evidence.EvidenceGraph) facts.Facts {
	f := make(facts.Facts)
	for _, c := range snapshot.Claims {
		for _, src := range c.Sources {
			sourceSpan := ""
			if len(src.BBox) >= 4 {
				sourceSpan = fmt.Sprintf("page=%d;bbox=[%.1f,%.1f,%.1f,%.1f]", src.Page, src.BBox[0], src.BBox[1], src.BBox[2], src.BBox[3])
			} else if src.Page > 0 {
				sourceSpan = fmt.Sprintf("page=%d", src.Page)
			}
			f[c.SemanticType] = append(f[c.SemanticType], facts.Fact{
				Value:      c.Value,
				SourceDoc:  src.Filename,
				FieldName:  src.FieldName,
				SourceSpan: sourceSpan,
				Confidence: src.Confidence,
			})
		}
	}
	return f
}

// loadSnapshot reads and unmarshals the evidence snapshot.
func loadSnapshot() (*evidence.EvidenceGraph, error) {
	snapshotPath := filepath.Join(snapshotDir, "evidence_snapshot.json")
	snapshotJSON, err := os.ReadFile(snapshotPath)
	if err != nil {
		return nil, fmt.Errorf("snapshot not found: %w", err)
	}
	var snapshot evidence.EvidenceGraph
	if err := json.Unmarshal(snapshotJSON, &snapshot); err != nil {
		return nil, fmt.Errorf("invalid snapshot: %w", err)
	}
	return &snapshot, nil
}

// Local HTTP DTOs — these are presentation models, not engine types.
type enrichedSourceRef struct {
	Filename  string `json:"filename"`
	FieldName string `json:"field_name"`
}

type enrichedFinding struct {
	Rule     string              `json:"rule"`
	Severity string              `json:"severity"`
	ClaimA   string              `json:"claim_a"`
	ClaimB   string              `json:"claim_b"`
	ValueA   interface{}         `json:"value_a"`
	ValueB   interface{}         `json:"value_b"`
	Sources  []enrichedSourceRef `json:"sources,omitempty"`
}

type enrichedResult struct {
	Decision string            `json:"decision"`
	Findings []enrichedFinding `json:"findings"`
}

func handleEvaluate(w http.ResponseWriter, r *http.Request) {
	snapshot, err := loadSnapshot()
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}

	f := buildFactsFromSnapshot(snapshot)

	decision, err := evalRunner.Evaluate(r.Context(), loadedRuleset, f)
	if err != nil {
		http.Error(w, fmt.Sprintf("evaluation error: %v", err), 500)
		return
	}

	// Build HTTP-specific enriched response from engine Decision
	enriched := enrichedResult{Decision: string(decision.Status)}
	for _, res := range decision.Results {
		var sources []enrichedSourceRef
		for _, ev := range res.Evidence {
			sources = append(sources, enrichedSourceRef{
				Filename:  ev.SourceDoc,
				FieldName: ev.Field,
			})
		}
		ef := enrichedFinding{
			Rule:     res.CheckID,
			Severity: string(res.Severity),
			ClaimA:   res.CheckID,
			ClaimB:   "",
			ValueA:   nil,
			ValueB:   nil,
			Sources:  sources,
		}
		enriched.Findings = append(enriched.Findings, ef)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(enriched); err != nil {
		log.Printf("json encode error: %v", err)
		http.Error(w, "internal error", 500)
		return
	}
}

func handleReview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}

	var req struct {
		FindingIndex int               `json:"finding_index"`
		Action       review.FindingAction `json:"action"`
		Note         string            `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid request: %v", err), 400)
		return
	}

	if req.Action != review.ActionConfirm && req.Action != review.ActionReject && req.Action != review.ActionOverride {
		http.Error(w, "action must be confirm, reject, or override", 400)
		return
	}

	store.AddReview(&review.HumanReview{
		FindingIndex: req.FindingIndex,
		Action:       req.Action,
		Note:         req.Note,
	})

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "ok"}); err != nil {
		log.Printf("json encode error: %v", err)
		http.Error(w, "internal error", 500)
		return
	}
}

func handleReviews(w http.ResponseWriter, r *http.Request) {
	reviews := store.GetAll()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(reviews); err != nil {
		log.Printf("json encode error: %v", err)
		http.Error(w, "internal error", 500)
		return
	}
}

func handleDisposition(w http.ResponseWriter, r *http.Request) {
	snapshot, err := loadSnapshot()
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}

	f := buildFactsFromSnapshot(snapshot)

	decision, err := evalRunner.Evaluate(r.Context(), loadedRuleset, f)
	if err != nil {
		http.Error(w, fmt.Sprintf("evaluation error: %v", err), 500)
		return
	}

	var findings []review.Finding
	for _, res := range decision.Results {
		findings = append(findings, review.Finding{
			Rule:     res.CheckID,
			Severity: string(res.Severity),
			ClaimA:   res.CheckID,
			ClaimB:   "",
		})
	}

	reviewsMap := make(map[int]*review.HumanReview)
	for _, r := range store.GetAll() {
		reviewsMap[r.FindingIndex] = r
	}

	final := review.ComputeFinalDisposition(string(decision.Status), findings, reviewsMap)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"eval_decision":     string(decision.Status),
		"final_disposition": final,
		"reviews_count":     store.Count(),
		"findings_count":    len(findings),
	}); err != nil {
		log.Printf("json encode error: %v", err)
		http.Error(w, "internal error", 500)
		return
	}
}

func handleFinalize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}

	snapshot, err := loadSnapshot()
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}

	f := buildFactsFromSnapshot(snapshot)

	decision, err := evalRunner.Evaluate(r.Context(), loadedRuleset, f)
	if err != nil {
		http.Error(w, fmt.Sprintf("evaluation error: %v", err), 500)
		return
	}

	artifact := audit.NewArtifact("income_verification", policyHash)

	for _, doc := range snapshot.Documents {
		artifact.AddDocument(audit.DocumentRecord{
			FileName: doc.Filename,
			DocType:  string(doc.Type),
			Hash:     doc.Hash,
		})
	}

	var findings []review.Finding
	for _, res := range decision.Results {
		findings = append(findings, review.Finding{
			Rule:     res.CheckID,
			Severity: string(res.Severity),
			ClaimA:   res.CheckID,
			ClaimB:   "",
		})
	}

	artifact.AddDecision(audit.Decision{
		CaseID:   "income_verification",
		State:    string(decision.Status),
		Findings: convertFindings(decision.Results),
	})

	reviewsMap := make(map[int]*review.HumanReview)
	for _, r := range store.GetAll() {
		reviewsMap[r.FindingIndex] = r
		artifact.AddHumanReview(audit.HumanReviewRecord{
			FindingIndex: r.FindingIndex,
			Action:       string(r.Action),
			Note:         r.Note,
			ResolvedAt:   r.ResolvedAt,
		})
	}

	final := review.ComputeFinalDisposition(string(decision.Status), findings, reviewsMap)
	artifact.SetFinalDisposition(final)
	artifact.Finalize()

	pdfBytes, err := audit.GenerateAuditReport(artifact)
	if err != nil {
		http.Error(w, fmt.Sprintf("generate report: %v", err), 500)
		return
	}

	reportPath := filepath.Join(snapshotDir, "audit_report.pdf")
	if err := os.WriteFile(reportPath, pdfBytes, 0644); err != nil {
		http.Error(w, fmt.Sprintf("write report: %v", err), 500)
		return
	}

	artifactJSON, _ := artifact.ToJSON()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"status":            "finalized",
		"final_disposition": final,
		"artifact_hash":     artifact.Manifest.ArtifactHash,
		"report_path":       reportPath,
		"report_size":       len(pdfBytes),
		"artifact":          json.RawMessage(artifactJSON),
	}); err != nil {
		log.Printf("json encode error: %v", err)
		http.Error(w, "internal error", 500)
		return
	}
}

func handleAudit(w http.ResponseWriter, r *http.Request) {
	snapshot, err := loadSnapshot()
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}

	f := buildFactsFromSnapshot(snapshot)

	decision, err := evalRunner.Evaluate(r.Context(), loadedRuleset, f)
	if err != nil {
		http.Error(w, fmt.Sprintf("evaluation error: %v", err), 500)
		return
	}

	artifact := audit.NewArtifact("income_verification", policyHash)

	for _, doc := range snapshot.Documents {
		artifact.AddDocument(audit.DocumentRecord{
			FileName: doc.Filename,
			DocType:  string(doc.Type),
			Hash:     doc.Hash,
		})
	}

	var findings []review.Finding
	for _, res := range decision.Results {
		findings = append(findings, review.Finding{
			Rule:     res.CheckID,
			Severity: string(res.Severity),
			ClaimA:   res.CheckID,
			ClaimB:   "",
		})
	}

	artifact.AddDecision(audit.Decision{
		CaseID:   "income_verification",
		State:    string(decision.Status),
		Findings: convertFindings(decision.Results),
	})

	reviewsMap := make(map[int]*review.HumanReview)
	for _, r := range store.GetAll() {
		reviewsMap[r.FindingIndex] = r
		artifact.AddHumanReview(audit.HumanReviewRecord{
			FindingIndex: r.FindingIndex,
			Action:       string(r.Action),
			Note:         r.Note,
			ResolvedAt:   r.ResolvedAt,
		})
	}

	final := review.ComputeFinalDisposition(string(decision.Status), findings, reviewsMap)
	artifact.SetFinalDisposition(final)
	artifact.Finalize()

	artifactJSON, _ := artifact.ToJSON()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"artifact":          json.RawMessage(artifactJSON),
		"artifact_hash":     artifact.Manifest.ArtifactHash,
		"final_disposition": final,
		"reviews_count":     store.Count(),
	}); err != nil {
		log.Printf("json encode error: %v", err)
		http.Error(w, "internal error", 500)
		return
	}
}

func handleSign(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}

	if nutrientClient == nil {
		http.Error(w, "signing not configured (NUTRIENT_PROCESSOR_KEY missing)", 500)
		return
	}

	reportPath := filepath.Join(snapshotDir, "audit_report.pdf")
	if _, err := os.Stat(reportPath); err != nil {
		http.Error(w, "audit report not found, run /api/finalize first", 404)
		return
	}

	signatureConfig := map[string]any{
		"signatureType": "cades",
		"reason":        "DocTrust audit artifact approval",
		"location":      "DocTrust Hackathon",
		"contactInfo":   "doctrust@demo.local",
	}

	signedBytes, err := nutrientClient.SignPDF(reportPath, signatureConfig)
	if err != nil {
		http.Error(w, fmt.Sprintf("signing error: %v", err), 500)
		return
	}

	signedPath := filepath.Join(snapshotDir, "audit_report_signed.pdf")
	if err := os.WriteFile(signedPath, signedBytes, 0644); err != nil {
		http.Error(w, fmt.Sprintf("write signed file: %v", err), 500)
		return
	}

	signedHash := fmt.Sprintf("%x", sha256.Sum256(signedBytes))

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"status":      "signed",
		"signed_path": signedPath,
		"signed_hash": signedHash,
		"size":        len(signedBytes),
	}); err != nil {
		log.Printf("json encode error: %v", err)
		http.Error(w, "internal error", 500)
		return
	}
}

func convertFindings(in []eval.Result) []audit.Finding {
	out := make([]audit.Finding, len(in))
	for i, f := range in {
		out[i] = audit.Finding{
			Rule:     f.CheckID,
			Severity: string(f.Severity),
			ClaimA:   f.CheckID,
			ClaimB:   "",
		}
	}
	return out
}

func handleDocument(w http.ResponseWriter, r *http.Request) {
	fileName := strings.TrimPrefix(r.URL.Path, "/api/documents/")
	if fileName == "" {
		http.Error(w, "missing filename", 400)
		return
	}

	fileName = filepath.Base(fileName)
	docPath := filepath.Join(snapshotDir, fileName)

	data, err := os.ReadFile(docPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("document not found: %v", err), 404)
		return
	}

	contentType := "application/pdf"
	if strings.HasSuffix(fileName, ".json") {
		contentType = "application/json"
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%s", fileName))
	w.Write(data)
}

func handleStatic(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/static/")
	filePath := filepath.Join("web/static", path)

	data, err := os.ReadFile(filePath)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	contentType := "application/octet-stream"
	if strings.HasSuffix(path, ".js") {
		contentType = "application/javascript"
	} else if strings.HasSuffix(path, ".css") {
		contentType = "text/css"
	}

	w.Header().Set("Content-Type", contentType)
	w.Write(data)
}
