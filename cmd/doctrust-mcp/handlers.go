package main

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/doctrust/doctrust/internal/review"
	"github.com/doctrust/doctrust/internal/service"
)

type toolError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func errResult(code, msg string) *mcp.CallToolResult {
	b, _ := json.Marshal(toolError{Code: code, Message: msg})
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(b)}},
		IsError: true,
	}
}

func okResult(out any) *mcp.CallToolResult {
	b, _ := json.Marshal(out)
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(b)}},
	}
}

func validateCaseID(svc *service.DocTrustService, caseID string) *mcp.CallToolResult {
	if svc.GetCaseID() == "" {
		return errResult("NO_CASE_LOADED", "no case has been evaluated yet")
	}
	if svc.GetCaseID() != caseID {
		return errResult("CASE_NOT_FOUND", "case_id does not match the current case")
	}
	return nil
}

func validateEvaluated(svc *service.DocTrustService) *mcp.CallToolResult {
	if svc.GetCaseID() == "" {
		return errResult("NO_CASE_LOADED", "no case has been evaluated yet")
	}
	if svc.GetDecision() == nil {
		return errResult("NO_CASE_LOADED", "case loaded but not yet evaluated")
	}
	return nil
}

type findingSummary struct {
	Index    int            `json:"index"`
	CheckID  string         `json:"check_id"`
	Status   string         `json:"status"`
	Severity string         `json:"severity"`
	Reason   string         `json:"reason"`
	Metrics  map[string]any `json:"metrics,omitempty"`
}

type evidenceRefOut struct {
	Field      string  `json:"field"`
	SourceDoc  string  `json:"source_doc"`
	SourceSpan string  `json:"source_span"`
	Confidence float64 `json:"confidence"`
}

type checkInfo struct {
	ID      string `json:"id"`
	Version string `json:"version"`
}

// handlerSerial enforces strictly ordered tool execution: the Go MCP SDK may
// dispatch calls concurrently, but DocTrust's pinned-case lifecycle assumes
// sequential handling (evaluate must complete before findings/evidence/audit).
var handlerSerial sync.Mutex

func withPanicRecovery(name string, h func() (*mcp.CallToolResult, error)) func() (*mcp.CallToolResult, error) {
	return func() (res *mcp.CallToolResult, err error) {
		handlerSerial.Lock()
		defer handlerSerial.Unlock()
		defer func() {
			if r := recover(); r != nil {
				slog.Error("tool panic recovered", "tool", name, "panic", r)
				res = errResult("INTERNAL_ERROR", "internal error in tool")
				err = nil
			}
		}()
		return h()
	}
}

// mergeAuthorizedReviews loads the human-channel reviews sidecar associated
// with the pinned snapshot (if any), verifies every Ed25519 signature against
// the provisioned reviewers ring (<snapshot-root>/reviewers/*.pub), and feeds
// valid records into the pinned case. Fail-closed: any verification error is
// returned and blocks disposition/finalize/audit until resolved.
func mergeAuthorizedReviews(svc *service.DocTrustService, snapshotRoot string) *mcp.CallToolResult {
	snapPath := svc.GetSnapshotPath()
	if snapPath == "" || svc.GetCaseID() == "" {
		return nil // nothing loaded yet
	}
	reviewsPath := snapPath + ".doctrust_reviews.json"
	if _, err := os.Stat(reviewsPath); err != nil {
		return nil // no human action recorded yet — normal pre-review state
	}
	ringDir := filepath.Join(snapshotRoot, "reviewers")
	entries, err := os.ReadDir(ringDir)
	if err != nil && !os.IsNotExist(err) {
		return errResult("INTERNAL_ERROR", "reviews ring unreadable: "+err.Error())
	}
	ring := map[string]ed25519.PublicKey{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".pub") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(ringDir, e.Name()))
		if err != nil {
			continue
		}
		pub, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(data)))
		if err != nil || len(pub) != ed25519.PublicKeySize {
			continue
		}
		ring[strings.TrimSuffix(e.Name(), ".pub")] = ed25519.PublicKey(pub)
	}
	sc, err := review.LoadReviewsSidecar(reviewsPath)
	if err != nil {
		return errResult("REVIEWS_INVALID", err.Error())
	}
	if sc.CaseID != "" && sc.CaseID != svc.GetCaseID() && sc.CaseID != svc.GetGraphCaseID() {
		return errResult("REVIEWS_INVALID", "reviews sidecar bound to a different case")
	}
	if err := svc.LoadAuthorizedReviews(sc.Records, ring); err != nil {
		return errResult("REVIEWS_REJECTED", err.Error())
	}
	return nil
}

func registerTools(server *mcp.Server, svc *service.DocTrustService, snapshotRoot string) {
	server.AddTool(&mcp.Tool{
		Name:        "evaluate_case",
		Description: "Load an evidence snapshot, evaluate compliance, and return the decision.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"snapshot_path": map[string]any{
					"type":        "string",
					"description": "Filesystem path to the evidence snapshot JSON",
				},
			},
			"required": []string{"snapshot_path"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return withPanicRecovery("evaluate_case", func() (*mcp.CallToolResult, error) {
			var args struct {
				SnapshotPath string `json:"snapshot_path"`
			}
			if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
				return errResult("INVALID_ARGUMENT", fmt.Sprintf("invalid arguments: %v", err)), nil
			}
			if args.SnapshotPath == "" {
				return errResult("INVALID_ARGUMENT", "snapshot_path is required"), nil
			}
			resolved, err := validateSnapshotPath(args.SnapshotPath, snapshotRoot)
			if err != nil {
				return errResult("INVALID_SNAPSHOT_PATH", err.Error()), nil
			}
			if err := svc.LoadCase(ctx, resolved); err != nil {
				return errResult("SNAPSHOT_INVALID", fmt.Sprintf("failed to load snapshot: %v", err)), nil
			}
			decision, err := svc.Evaluate(ctx)
			if err != nil {
				return errResult("INTERNAL_ERROR", fmt.Sprintf("evaluation failed: %v", err)), nil
			}
			caseID := svc.GetCaseID()
			// Phase 6 (P6-7): persist the decision context so the human-only
			// review channel can consume it. Additive sidecar; evaluation
			// semantics unchanged.
			if sc, err := svc.BuildDecisionSidecar(); err == nil {
				data, jerr := json.MarshalIndent(sc, "", "  ")
				if jerr == nil {
					_ = os.WriteFile(resolved+".decision.json", data, 0o644)
				}
			}
			var severity string
			for _, r := range decision.Results {
				if string(r.Severity) == "BLOCKING" {
					severity = "BLOCKING"
					break
				}
				if string(r.Severity) == "WARNING" && severity != "BLOCKING" {
					severity = "WARNING"
				}
			}
			if severity == "" {
				severity = "INFO"
			}
			return okResult(map[string]any{
				"case_id":         caseID,
				"status":          string(decision.Status),
				"severity":        severity,
				"ruleset_id":      decision.RulesetID,
				"ruleset_version": decision.RulesetVersion,
				"finding_count":   len(decision.Results),
			}), nil
		})()
	})

	server.AddTool(&mcp.Tool{
		Name:        "get_findings",
		Description: "Return all findings from the last evaluation.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"case_id": map[string]any{
					"type":        "string",
					"description": "The case ID from evaluate_case",
				},
			},
			"required": []string{"case_id"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return withPanicRecovery("get_findings", func() (*mcp.CallToolResult, error) {
			var args struct {
				CaseID string `json:"case_id"`
			}
			if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
				return errResult("INVALID_ARGUMENT", fmt.Sprintf("invalid arguments: %v", err)), nil
			}
			if r := validateCaseID(svc, args.CaseID); r != nil {
				return r, nil
			}
			if r := validateEvaluated(svc); r != nil {
				return r, nil
			}
			decision := svc.GetDecision()
			findings := make([]findingSummary, len(decision.Results))
			for i, r := range decision.Results {
				findings[i] = findingSummary{
					Index:    i,
					CheckID:  r.CheckID,
					Status:   string(r.Status),
					Severity: string(r.Severity),
					Reason:   r.Reason,
					Metrics:  r.Metrics,
				}
			}
			return okResult(map[string]any{"findings": findings}), nil
		})()
	})

	server.AddTool(&mcp.Tool{
		Name:        "get_evidence",
		Description: "Return evidence references supporting a specific finding.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"case_id": map[string]any{
					"type":        "string",
					"description": "The case ID from evaluate_case",
				},
				"finding_index": map[string]any{
					"type":        "integer",
					"description": "Zero-based index of the finding",
				},
			},
			"required": []string{"case_id", "finding_index"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return withPanicRecovery("get_evidence", func() (*mcp.CallToolResult, error) {
			var args struct {
				CaseID       string `json:"case_id"`
				FindingIndex int    `json:"finding_index"`
			}
			if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
				return errResult("INVALID_ARGUMENT", fmt.Sprintf("invalid arguments: %v", err)), nil
			}
			if r := validateCaseID(svc, args.CaseID); r != nil {
				return r, nil
			}
			if r := validateEvaluated(svc); r != nil {
				return r, nil
			}
			refs, err := svc.GetEvidence(args.FindingIndex)
			if err != nil {
				return errResult("INVALID_FINDING_INDEX", err.Error()), nil
			}
			out := make([]evidenceRefOut, len(refs))
			for i, ref := range refs {
				out[i] = evidenceRefOut{
					Field:      ref.Field,
					SourceDoc:  ref.SourceDoc,
					SourceSpan: ref.SourceSpan,
					Confidence: ref.Confidence,
				}
			}
			return okResult(map[string]any{"evidence": out}), nil
		})()
	})

	server.AddTool(&mcp.Tool{
		Name:        "get_ruleset",
		Description: "Return the currently active compliance ruleset metadata.",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return withPanicRecovery("get_ruleset", func() (*mcp.CallToolResult, error) {
			info := svc.GetRuleset()
			checks := make([]checkInfo, len(info.Checks))
			for i, c := range info.Checks {
				checks[i] = checkInfo{ID: c.ID, Version: c.Version}
			}
			return okResult(map[string]any{
				"id":      info.ID,
				"version": info.Version,
				"checks":  checks,
			}), nil
		})()
	})

	// NOTE (Phase 6 / plans12): request_human_review was REMOVED from the
	// agent-facing surface. Human authority is exercised exclusively through
	// the human-only TTY channel (cmd/doctrust-review), whose Ed25519-signed
	// records are merged fail-closed by get_audit_artifact. See AGENTS.md R29.
	server.AddTool(&mcp.Tool{
		Name:        "get_audit_artifact",
		Description: "Generate the tamper-evident audit artifact for the evaluated case.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"case_id": map[string]any{
					"type":        "string",
					"description": "The case ID from evaluate_case",
				},
			},
			"required": []string{"case_id"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return withPanicRecovery("get_audit_artifact", func() (*mcp.CallToolResult, error) {
			if r := mergeAuthorizedReviews(svc, snapshotRoot); r != nil {
				return r, nil
			}
			var args struct {
				CaseID string `json:"case_id"`
			}
			if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
				return errResult("INVALID_ARGUMENT", fmt.Sprintf("invalid arguments: %v", err)), nil
			}
			if r := validateCaseID(svc, args.CaseID); r != nil {
				return r, nil
			}
			if r := validateEvaluated(svc); r != nil {
				return r, nil
			}
			artifact, err := svc.BuildArtifact()
			if err != nil {
				return errResult("INTERNAL_ERROR", fmt.Sprintf("artifact build failed: %v", err)), nil
			}
			return okResult(map[string]any{
				"artifact":          artifact,
				"artifact_hash":     artifact.Hash(),
				"final_disposition": artifact.FinalDisposition,
			}), nil
		})()
	})
}
