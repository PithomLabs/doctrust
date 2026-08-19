package main

import (
	"context"
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
	"github.com/doctrust/doctrust/internal/evidence"
	"github.com/doctrust/doctrust/internal/opa"
)

var (
	snapshotDir   string
	policyRego    string
	policyHash    string
	nutrientKey   string
	processorKey  string
)

func main() {
	dir := flag.String("dir", "demo/income_verification", "snapshot directory")
	policy := flag.String("policy", "policies/income_verification/policy.rego", "policy.rego path")
	port := flag.String("port", "8080", "listen port")
	flag.Parse()

	snapshotDir = *dir
	policyRego = *policy

	nutrientKey = os.Getenv("NUTRIENT_EXTRACTION_KEY")
	processorKey = os.Getenv("NUTRIENT_PROCESSOR_KEY")

	data, _ := os.ReadFile(policyRego)
	policyHash = fmt.Sprintf("%x", sha256Hash(data))

	mux := http.NewServeMux()
	mux.HandleFunc("/", handleIndex)
	mux.HandleFunc("/api/snapshot", handleSnapshot)
	mux.HandleFunc("/api/evaluate", handleEvaluate)
	mux.HandleFunc("/api/audit", handleAudit)
	mux.HandleFunc("/api/documents/", handleDocument)
	mux.HandleFunc("/static/", handleStatic)

	fmt.Printf("DocTrust server listening on :%s\n", *port)
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

func handleEvaluate(w http.ResponseWriter, r *http.Request) {
	snapshotPath := filepath.Join(snapshotDir, "evidence_snapshot.json")
	snapshotJSON, err := os.ReadFile(snapshotPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("snapshot not found: %v", err), 404)
		return
	}

	policyData, err := os.ReadFile(policyRego)
	if err != nil {
		http.Error(w, fmt.Sprintf("policy not found: %v", err), 500)
		return
	}

	result, err := opa.Evaluate(context.Background(), snapshotJSON, string(policyData))
	if err != nil {
		http.Error(w, fmt.Sprintf("evaluation error: %v", err), 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func handleAudit(w http.ResponseWriter, r *http.Request) {
	snapshotPath := filepath.Join(snapshotDir, "evidence_snapshot.json")
	snapshotJSON, err := os.ReadFile(snapshotPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("snapshot not found: %v", err), 404)
		return
	}

	policyData, err := os.ReadFile(policyRego)
	if err != nil {
		http.Error(w, fmt.Sprintf("policy not found: %v", err), 500)
		return
	}

	var snapshot evidence.EvidenceGraph
	json.Unmarshal(snapshotJSON, &snapshot)

	result, err := opa.Evaluate(context.Background(), snapshotJSON, string(policyData))
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

	resultJSON, _ := json.Marshal(result)
	var opaResult struct {
		Decision string `json:"decision"`
		Findings []struct {
			Rule     string  `json:"rule"`
			Severity string  `json:"severity"`
			ClaimA   string  `json:"claim_a"`
			ClaimB   string  `json:"claim_b"`
			ValueA   float64 `json:"value_a"`
			ValueB   float64 `json:"value_b"`
			Variance float64 `json:"variance"`
		} `json:"findings"`
	}
	json.Unmarshal(resultJSON, &opaResult)

	artifact.AddDecision(audit.Decision{
		CaseID: "income_verification",
		State:  opaResult.Decision,
		Findings: convertFindings(opaResult.Findings),
	})

	artifact.Finalize()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(artifact)
}

func convertFindings(in []struct {
	Rule     string  `json:"rule"`
	Severity string  `json:"severity"`
	ClaimA   string  `json:"claim_a"`
	ClaimB   string  `json:"claim_b"`
	ValueA   float64 `json:"value_a"`
	ValueB   float64 `json:"value_b"`
	Variance float64 `json:"variance"`
}) []audit.Finding {
	out := make([]audit.Finding, len(in))
	for i, f := range in {
		out[i] = audit.Finding{
			Rule:     f.Rule,
			Severity: f.Severity,
			ClaimA:   f.ClaimA,
			ClaimB:   f.ClaimB,
			ValueA:   f.ValueA,
			ValueB:   f.ValueB,
			Variance: f.Variance,
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

	// Sanitize path
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

func sha256Hash(data []byte) [32]byte {
	return sha256.Sum256(data)
}
