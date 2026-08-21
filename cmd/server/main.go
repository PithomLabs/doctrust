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
	"github.com/doctrust/doctrust/internal/nutrient"
	"github.com/doctrust/doctrust/internal/review"
	"github.com/doctrust/doctrust/internal/service"
)

var (
	snapshotDir    string
	nutrientKey    string
	processorKey   string
	nutrientClient *nutrient.Client
	svc            *service.DocTrustService
)

func main() {
	domain := flag.String("domain", "", "compiled domain (required)")
	port := flag.String("port", "8080", "listen port")
	snapDir := flag.String("dir", "demo/income_verification", "snapshot directory")
	flag.Parse()

	snapshotDir = *snapDir

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

	// Initialize service layer (loads promoted ruleset, creates runner)
	var err error
	svc, err = service.NewDocTrustService(*domain, "rulesets")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	info := svc.GetRuleset()
	log.Printf("Loaded promoted ruleset: %s v%s", info.ID, info.Version)

	mux := http.NewServeMux()
	mux.HandleFunc("/", handleIndex)
	mux.HandleFunc("/api/snapshot", handleSnapshot)
	mux.HandleFunc("/api/evaluate", handleEvaluate)
	mux.HandleFunc("/api/audit", handleAudit)
	mux.HandleFunc("/api/review", handleReview)
	mux.HandleFunc("/api/reviews", handleReviews)
	mux.HandleFunc("/api/disposition", handleDisposition)
	mux.HandleFunc("/api/ruleset", handleRuleset)
	mux.HandleFunc("/api/regression", handleRegression)
	mux.HandleFunc("/api/finalize", handleFinalize)
	mux.HandleFunc("/api/sign", handleSign)
	mux.HandleFunc("/api/documents/", handleDocument)
	mux.HandleFunc("/static/", handleStatic)

	fmt.Printf("DocTrust server listening on :%s\n", *port)
	fmt.Printf("Ruleset: %s v%s\n", info.ID, info.Version)
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

// Local HTTP DTOs — these are presentation models, not engine types.
// All explanation/presentation enrichment lives here, not in internal/eval.

type enrichedSourceRef struct {
	Filename   string  `json:"filename"`
	FieldName  string  `json:"field_name"`
	SourceSpan string  `json:"source_span,omitempty"`
	Page       int     `json:"page,omitempty"`
	BBox       []float64 `json:"bbox,omitempty"`
	Confidence float64 `json:"confidence,omitempty"`
}

type enrichedFinding struct {
	Rule       string              `json:"rule"`
	Status     string              `json:"status"`
	Severity   string              `json:"severity"`
	Reason     string              `json:"reason"`
	Confidence float64             `json:"confidence"`
	Sources    []enrichedSourceRef `json:"sources,omitempty"`
	Metrics    map[string]any      `json:"metrics,omitempty"`
}

type enrichedResult struct {
	Decision string            `json:"decision"`
	Findings []enrichedFinding `json:"findings"`
}

// checkDescriptions provides human-readable descriptions for each check.
// This is presentation-layer metadata — not part of the eval engine.
var checkDescriptions = map[string]string{
	"gross_income_consistency":     "Compares paystub projected gross income against W-2 taxable wages. Detects variances and checks if documented bonus compensation explains the gap.",
	"required_documents":           "Verifies all required document types (paystub, W-2, Form 1040) are present in the evidence snapshot.",
	"net_vs_gross_incomparability": "Confirms that net cash flow (bank deposits) is correctly treated as incomparable to gross taxable income.",
}

// parseSourceSpan extracts page and bbox from a source_span string like "page=1;bbox=[508,274,50,8]".
func parseSourceSpan(span string) (page int, bbox []float64) {
	if span == "" {
		return 0, nil
	}
	// Parse page
	for _, part := range splitSpan(span) {
		if len(part) > 5 && part[:5] == "page=" {
			fmt.Sscanf(part[5:], "%d", &page)
		}
		if len(part) > 5 && part[:5] == "bbox=" {
			bboxStr := part[5:]
			bboxStr = strings.Trim(bboxStr, "[]")
			var b []float64
			for _, s := range strings.Split(bboxStr, ",") {
				s = strings.TrimSpace(s)
				var v float64
				if _, err := fmt.Sscanf(s, "%f", &v); err == nil {
					b = append(b, v)
				}
			}
			bbox = b
		}
	}
	return page, bbox
}

func splitSpan(span string) []string {
	var parts []string
	for _, part := range strings.Split(span, ";") {
		part = strings.TrimSpace(part)
		if part != "" {
			parts = append(parts, part)
		}
	}
	return parts
}

// maxConfidence returns the maximum confidence from a list of evidence refs.
func maxConfidence(refs []evidence.EvidenceRef) float64 {
	max := 0.0
	for _, r := range refs {
		if r.Confidence > max {
			max = r.Confidence
		}
	}
	return max
}

// ensureEvaluated verifies that a case has been loaded and evaluated.
// It must NOT load or evaluate — only handleEvaluate may do that.
func ensureEvaluated() error {
	if svc.GetDecision() == nil {
		return fmt.Errorf("no case loaded — call /api/evaluate first")
	}
	return nil
}

func handleEvaluate(w http.ResponseWriter, r *http.Request) {
	snapshotPath := filepath.Join(snapshotDir, "evidence_snapshot.json")
	if err := svc.LoadCase(r.Context(), snapshotPath); err != nil {
		http.Error(w, fmt.Sprintf("load case: %v", err), 500)
		return
	}

	decision, err := svc.Evaluate(r.Context())
	if err != nil {
		http.Error(w, fmt.Sprintf("evaluation error: %v", err), 500)
		return
	}

	// Build HTTP-specific enriched response from engine Decision
	enriched := enrichedResult{Decision: string(decision.Status)}
	for _, res := range decision.Results {
		var sources []enrichedSourceRef
		var totalConfidence float64
		var confCount int
		for _, ev := range res.Evidence {
			page, bbox := parseSourceSpan(ev.SourceSpan)
			sources = append(sources, enrichedSourceRef{
				Filename:   ev.SourceDoc,
				FieldName:  ev.Field,
				SourceSpan: ev.SourceSpan,
				Page:       page,
				BBox:       bbox,
				Confidence: ev.Confidence,
			})
			if ev.Confidence > 0 {
				totalConfidence += ev.Confidence
				confCount++
			}
		}
		avgConfidence := 0.0
		if confCount > 0 {
			avgConfidence = totalConfidence / float64(confCount)
		}

		ef := enrichedFinding{
			Rule:       res.CheckID,
			Status:     string(res.Status),
			Severity:   string(res.Severity),
			Reason:     res.Reason,
			Confidence: avgConfidence,
			Sources:    sources,
			Metrics:    res.Metrics,
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

	// Operate on already-evaluated case — NO reload, NO re-evaluate
	_, err := svc.RequestHumanReview(req.FindingIndex, req.Action, req.Note)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "ok"}); err != nil {
		log.Printf("json encode error: %v", err)
		http.Error(w, "internal error", 500)
		return
	}
}

func handleReviews(w http.ResponseWriter, r *http.Request) {
	reviews, err := svc.GetReviews()
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(reviews); err != nil {
		log.Printf("json encode error: %v", err)
		http.Error(w, "internal error", 500)
		return
	}
}

func handleRuleset(w http.ResponseWriter, r *http.Request) {
	info := svc.GetRuleset()

	// Compute ruleset hash
	ruleset := &eval.Ruleset{ID: info.ID, Version: info.Version, Checks: info.Checks}
	hash, err := ruleset.ComputeHash()
	if err != nil {
		http.Error(w, fmt.Sprintf("compute hash: %v", err), 500)
		return
	}

	// Build check metadata with descriptions
	type checkInfo struct {
		ID          string `json:"id"`
		Version     string `json:"version"`
		Description string `json:"description"`
	}
	var checks []checkInfo
	for _, ref := range info.Checks {
		desc := checkDescriptions[ref.ID]
		checks = append(checks, checkInfo{
			ID:          ref.ID,
			Version:     ref.Version,
			Description: desc,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{
		"id":             info.ID,
		"version":        info.Version,
		"hash":           hash,
		"checks":         checks,
		"checks_count":   len(info.Checks),
	}); err != nil {
		log.Printf("json encode error: %v", err)
		http.Error(w, "internal error", 500)
		return
	}
}

func handleRegression(w http.ResponseWriter, r *http.Request) {
	info := svc.GetRuleset()

	// Load all scenarios for this domain
	scenariosDir := filepath.Join("scenarios", info.ID)
	scenarios, err := eval.LoadAllScenariosFromDir(scenariosDir)
	if err != nil {
		http.Error(w, fmt.Sprintf("load scenarios: %v", err), 500)
		return
	}

	// Run all scenarios against the loaded ruleset
	runner := eval.NewRunner(eval.DefaultRegistry().All())
	results := runner.RunAllScenarios(r.Context(), scenarios)

	// Build response
	type scenarioResult struct {
		Name   string `json:"name"`
		Origin string `json:"origin"`
		Passed bool   `json:"passed"`
	}
	var scenarioResults []scenarioResult
	passed := 0

	for _, sr := range results {
		scenarioResults = append(scenarioResults, scenarioResult{
			Name:   sr.ScenarioName,
			Origin: "", // will be filled from scenario metadata
			Passed: sr.Passed,
		})
		if sr.Passed {
			passed++
		}
	}

	// Re-count by origin from the scenarios themselves
	originCounts := map[string]int{}
	for i, s := range scenarios {
		originCounts[s.Origin]++
		if i < len(scenarioResults) {
			scenarioResults[i].Origin = s.Origin
		}
	}

	// Compute ruleset hash for version binding
	ruleset := &eval.Ruleset{ID: info.ID, Version: info.Version, Checks: info.Checks}
	hash, _ := ruleset.ComputeHash()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{
		"ruleset_id":      info.ID,
		"ruleset_version": info.Version,
		"ruleset_hash":    hash,
		"total":           len(results),
		"passed":          passed,
		"by_origin":       originCounts,
		"scenarios":       scenarioResults,
	}); err != nil {
		log.Printf("json encode error: %v", err)
		http.Error(w, "internal error", 500)
		return
	}
}

func handleDisposition(w http.ResponseWriter, r *http.Request) {
	// Ensure case is loaded and evaluated
	if err := ensureEvaluated(); err != nil {
		http.Error(w, fmt.Sprintf("evaluate: %v", err), 500)
		return
	}

	// Operate on already-evaluated case
	artifact, err := svc.BuildArtifact()
	if err != nil {
		http.Error(w, fmt.Sprintf("build artifact: %v", err), 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"eval_decision":     string(svc.GetDecision().Status),
		"final_disposition": artifact.FinalDisposition,
		"reviews_count":     len(artifact.HumanReviews),
		"findings_count":    len(artifact.Decisions),
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

	// Ensure case is loaded and evaluated
	if err := ensureEvaluated(); err != nil {
		http.Error(w, fmt.Sprintf("evaluate: %v", err), 500)
		return
	}

	// Build artifact from already-evaluated case
	artifact, err := svc.BuildArtifact()
	if err != nil {
		http.Error(w, fmt.Sprintf("build artifact: %v", err), 500)
		return
	}

	// HTTP-specific: generate PDF report
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
		"final_disposition": artifact.FinalDisposition,
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
	// Ensure case is loaded and evaluated
	if err := ensureEvaluated(); err != nil {
		http.Error(w, fmt.Sprintf("evaluate: %v", err), 500)
		return
	}

	// Build artifact from already-evaluated case
	artifact, err := svc.BuildArtifact()
	if err != nil {
		http.Error(w, fmt.Sprintf("build artifact: %v", err), 500)
		return
	}

	artifactJSON, _ := artifact.ToJSON()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"artifact":          json.RawMessage(artifactJSON),
		"artifact_hash":     artifact.Manifest.ArtifactHash,
		"final_disposition": artifact.FinalDisposition,
		"reviews_count":     len(artifact.HumanReviews),
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
