package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/PithomLabs/doctrust/internal/service"
)

func setupTestService(t *testing.T) *service.DocTrustService {
	t.Helper()
	cwd, _ := os.Getwd()
	rulesetsDir := filepath.Join(cwd, "..", "..", "rulesets")
	svc, err := service.NewDocTrustService("income_verification", rulesetsDir)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}
	return svc
}

func testSnapshotRoot(t *testing.T) string {
	t.Helper()
	cwd, _ := os.Getwd()
	return filepath.Join(cwd, "..", "..", "demo")
}

type testClient struct {
	client *mcp.ClientSession
	cancel context.CancelFunc
}

func setupTestClient(t *testing.T, svc *service.DocTrustService, snapshotRoot string) *testClient {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "0.0.1"}, nil)
	registerTools(server, svc, snapshotRoot)

	ct, st := mcp.NewInMemoryTransports()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	ss, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { ss.Close() })

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { cs.Close() })

	return &testClient{client: cs, cancel: cancel}
}

func callTool(t *testing.T, tc *testClient, name string, args any) *mcp.CallToolResult {
	t.Helper()
	b, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}
	var raw json.RawMessage = b
	res, err := tc.client.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      name,
		Arguments: raw,
	})
	if err != nil {
		t.Fatalf("CallTool(%s): %v", name, err)
	}
	return res
}

func parseResult(t *testing.T, res *mcp.CallToolResult) map[string]any {
	t.Helper()
	if res == nil {
		t.Fatal("nil result")
	}
	if res.IsError {
		tc, _ := res.Content[0].(*mcp.TextContent)
		t.Fatalf("unexpected error: %s", tc.Text)
	}
	if len(res.Content) == 0 {
		t.Fatal("empty content")
	}
	tc, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("unexpected content type: %T", res.Content[0])
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(tc.Text), &out); err != nil {
		t.Fatalf("parse result JSON: %v\nraw: %s", err, tc.Text)
	}
	return out
}

func parseError(t *testing.T, res *mcp.CallToolResult) toolError {
	t.Helper()
	if res == nil {
		t.Fatal("nil result, expected error")
	}
	if !res.IsError {
		tc, _ := res.Content[0].(*mcp.TextContent)
		t.Fatalf("expected error result, got success: %s", tc.Text)
	}
	tc, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("unexpected content type: %T", res.Content[0])
	}
	var e toolError
	if err := json.Unmarshal([]byte(tc.Text), &e); err != nil {
		t.Fatalf("parse error JSON: %v", err)
	}
	return e
}

func TestMCP_EvaluateCase(t *testing.T) {
	svc := setupTestService(t)
	root := testSnapshotRoot(t)
	tc := setupTestClient(t, svc, root)

	res := callTool(t, tc, "evaluate_case", map[string]any{
		"snapshot_path": "income_verification/evidence_snapshot.json",
	})
	out := parseResult(t, res)

	caseID, ok := out["case_id"].(string)
	if !ok || caseID == "" {
		t.Error("expected non-empty case_id")
	}
	if out["status"] == "" {
		t.Error("expected non-empty status")
	}
	if out["ruleset_id"] != "income_verification" {
		t.Errorf("expected ruleset_id=income_verification, got %v", out["ruleset_id"])
	}
	t.Logf("case_id=%v status=%v findings=%v", caseID, out["status"], out["finding_count"])
}

func TestMCP_EvaluateCase_AbsolutePathRejected(t *testing.T) {
	svc := setupTestService(t)
	root := testSnapshotRoot(t)
	tc := setupTestClient(t, svc, root)

	res := callTool(t, tc, "evaluate_case", map[string]any{
		"snapshot_path": "/etc/passwd",
	})
	e := parseError(t, res)
	if e.Code != "INVALID_SNAPSHOT_PATH" {
		t.Errorf("expected INVALID_SNAPSHOT_PATH, got %s", e.Code)
	}
}

func TestMCP_EvaluateCase_TraversalRejected(t *testing.T) {
	svc := setupTestService(t)
	root := testSnapshotRoot(t)
	tc := setupTestClient(t, svc, root)

	res := callTool(t, tc, "evaluate_case", map[string]any{
		"snapshot_path": "../../../etc/passwd",
	})
	e := parseError(t, res)
	if e.Code != "INVALID_SNAPSHOT_PATH" {
		t.Errorf("expected INVALID_SNAPSHOT_PATH, got %s", e.Code)
	}
}

func TestMCP_EvaluateCase_NonexistentRejected(t *testing.T) {
	svc := setupTestService(t)
	root := testSnapshotRoot(t)
	tc := setupTestClient(t, svc, root)

	res := callTool(t, tc, "evaluate_case", map[string]any{
		"snapshot_path": "nonexistent.json",
	})
	e := parseError(t, res)
	if e.Code != "INVALID_SNAPSHOT_PATH" {
		t.Errorf("expected INVALID_SNAPSHOT_PATH, got %s", e.Code)
	}
}

func TestMCP_EvaluateCase_MissingArgument(t *testing.T) {
	svc := setupTestService(t)
	root := testSnapshotRoot(t)
	tc := setupTestClient(t, svc, root)

	res := callTool(t, tc, "evaluate_case", map[string]any{})
	e := parseError(t, res)
	if e.Code != "INVALID_ARGUMENT" {
		t.Errorf("expected INVALID_ARGUMENT, got %s", e.Code)
	}
}

func TestMCP_GetFindings(t *testing.T) {
	svc := setupTestService(t)
	root := testSnapshotRoot(t)
	tc := setupTestClient(t, svc, root)

	evalRes := callTool(t, tc, "evaluate_case", map[string]any{
		"snapshot_path": "income_verification/evidence_snapshot.json",
	})
	evalOut := parseResult(t, evalRes)
	caseID := evalOut["case_id"].(string)

	res := callTool(t, tc, "get_findings", map[string]any{
		"case_id": caseID,
	})
	out := parseResult(t, res)
	findings, ok := out["findings"].([]any)
	if !ok {
		t.Fatalf("expected findings array, got %T", out["findings"])
	}
	if len(findings) == 0 {
		t.Error("expected at least one finding")
	}
	for i, f := range findings {
		fm := f.(map[string]any)
		t.Logf("finding[%d]: check_id=%v status=%v", i, fm["check_id"], fm["status"])
	}
}

func TestMCP_GetEvidence(t *testing.T) {
	svc := setupTestService(t)
	root := testSnapshotRoot(t)
	tc := setupTestClient(t, svc, root)

	evalRes := callTool(t, tc, "evaluate_case", map[string]any{
		"snapshot_path": "income_verification/evidence_snapshot.json",
	})
	evalOut := parseResult(t, evalRes)
	caseID := evalOut["case_id"].(string)

	res := callTool(t, tc, "get_evidence", map[string]any{
		"case_id":       caseID,
		"finding_index": 0,
	})
	out := parseResult(t, res)
	evidence, ok := out["evidence"].([]any)
	if !ok {
		t.Fatalf("expected evidence array, got %T", out["evidence"])
	}
	if len(evidence) == 0 {
		t.Error("expected at least one evidence ref")
	}
	for i, e := range evidence {
		em := e.(map[string]any)
		t.Logf("evidence[%d]: field=%v source_doc=%v", i, em["field"], em["source_doc"])
	}
}

func TestMCP_GetEvidence_InvalidIndex(t *testing.T) {
	svc := setupTestService(t)
	root := testSnapshotRoot(t)
	tc := setupTestClient(t, svc, root)

	evalRes := callTool(t, tc, "evaluate_case", map[string]any{
		"snapshot_path": "income_verification/evidence_snapshot.json",
	})
	evalOut := parseResult(t, evalRes)
	caseID := evalOut["case_id"].(string)

	res := callTool(t, tc, "get_evidence", map[string]any{
		"case_id":       caseID,
		"finding_index": 99,
	})
	e := parseError(t, res)
	if e.Code != "INVALID_FINDING_INDEX" {
		t.Errorf("expected INVALID_FINDING_INDEX, got %s", e.Code)
	}
}

func TestMCP_GetRuleset(t *testing.T) {
	svc := setupTestService(t)
	root := testSnapshotRoot(t)
	tc := setupTestClient(t, svc, root)

	res := callTool(t, tc, "get_ruleset", map[string]any{})
	out := parseResult(t, res)

	if out["id"] != "income_verification" {
		t.Errorf("expected id=income_verification, got %v", out["id"])
	}
	if out["version"] == "" {
		t.Error("expected non-empty version")
	}
	checks, ok := out["checks"].([]any)
	if !ok || len(checks) == 0 {
		t.Error("expected non-empty checks")
	}
	t.Logf("ruleset: id=%v version=%v checks=%d", out["id"], out["version"], len(checks))
}

// TestMCP_RequestHumanReview_Removed (Phase 6 / plans12 P6-1): the
// human-authority capability must NOT exist on the agent-facing surface.
// A direct call fails at the protocol level (unknown tool), and tools/list
// does not advertise it.
func TestMCP_RequestHumanReview_Removed(t *testing.T) {
	svc := setupTestService(t)
	root := testSnapshotRoot(t)
	tc := setupTestClient(t, svc, root)

	evalRes := callTool(t, tc, "evaluate_case", map[string]any{
		"snapshot_path": "income_verification/evidence_snapshot.json",
	})
	parseResult(t, evalRes)

	// Direct call must fail at protocol level OR as an error result — never
	// succeed.
	var raw json.RawMessage
	rawArgs := map[string]any{
		"case_id":       svc.GetCaseID(),
		"finding_index": 0,
		"action":        "confirm",
	}
	if b, err := json.Marshal(rawArgs); err != nil {
		t.Fatal(err)
	} else {
		raw = b
	}
	res, callErr := tc.client.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "request_human_review",
		Arguments: raw,
	})
	denied := false
	if callErr != nil {
		denied = strings.Contains(strings.ToLower(callErr.Error()), "unknown tool")
	} else if res != nil && res.IsError {
		tc2, _ := res.Content[0].(*mcp.TextContent)
		denied = strings.Contains(strings.ToLower(tc2.Text), "unknown tool")
	}
	if !denied {
		t.Fatalf("request_human_review must be structurally denied; callErr=%v res=%+v",
			callErr, res)
	}

	lt, err := tc.client.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("tools/list failed: %v", err)
	}
	for _, tool := range lt.Tools {
		if tool.Name == "request_human_review" {
			t.Fatal("agent surface must not advertise request_human_review")
		}
	}
}

func TestMCP_NoCaseLoaded(t *testing.T) {
	svc := setupTestService(t)
	root := testSnapshotRoot(t)
	tc := setupTestClient(t, svc, root)

	tests := []struct {
		name string
		args any
	}{
		{"get_findings", map[string]any{"case_id": "bogus"}},
		{"get_evidence", map[string]any{"case_id": "bogus", "finding_index": 0}},
		{"get_audit_artifact", map[string]any{"case_id": "bogus"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := callTool(t, tc, tt.name, tt.args)
			e := parseError(t, res)
			if e.Code != "NO_CASE_LOADED" {
				t.Errorf("expected NO_CASE_LOADED, got %s", e.Code)
			}
		})
	}
}

func TestMCP_CaseIDMismatch(t *testing.T) {
	svc := setupTestService(t)
	root := testSnapshotRoot(t)
	tc := setupTestClient(t, svc, root)

	evalRes := callTool(t, tc, "evaluate_case", map[string]any{
		"snapshot_path": "income_verification/evidence_snapshot.json",
	})
	evalOut := parseResult(t, evalRes)
	caseID := evalOut["case_id"].(string)

	res := callTool(t, tc, "get_findings", map[string]any{
		"case_id": caseID + "-wrong",
	})
	e := parseError(t, res)
	if e.Code != "CASE_NOT_FOUND" {
		t.Errorf("expected CASE_NOT_FOUND, got %s", e.Code)
	}
}

func TestMCP_PanicRecovery(t *testing.T) {
	fn := withPanicRecovery("test", func() (*mcp.CallToolResult, error) {
		panic("test panic")
	})

	res, err := fn()
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if res == nil || !res.IsError {
		t.Fatal("expected error result")
	}
	e := parseError(t, res)
	if e.Code != "INTERNAL_ERROR" {
		t.Errorf("expected INTERNAL_ERROR, got %s", e.Code)
	}
}

func TestMCP_PanicRecovery_ProcessAlive(t *testing.T) {
	fn1 := withPanicRecovery("test", func() (*mcp.CallToolResult, error) {
		panic("boom")
	})
	fn1()

	svc := setupTestService(t)
	root := testSnapshotRoot(t)
	tc := setupTestClient(t, svc, root)

	res := callTool(t, tc, "get_ruleset", map[string]any{})
	if res.IsError {
		t.Errorf("expected success after panic recovery, got error")
	}
}

func TestMCP_Errors_Structured(t *testing.T) {
	svc := setupTestService(t)
	root := testSnapshotRoot(t)
	tc := setupTestClient(t, svc, root)

	tests := []struct {
		tool string
		args any
		code string
	}{
		{"evaluate_case", map[string]any{}, "INVALID_ARGUMENT"},
		{"evaluate_case", map[string]any{"snapshot_path": "/nope"}, "INVALID_SNAPSHOT_PATH"},
		{"get_findings", map[string]any{"case_id": "nope"}, "NO_CASE_LOADED"},
		{"get_evidence", map[string]any{"case_id": "nope", "finding_index": 0}, "NO_CASE_LOADED"},
	}

	for _, tt := range tests {
		t.Run(tt.code+"/"+tt.tool, func(t *testing.T) {
			res := callTool(t, tc, tt.tool, tt.args)
			e := parseError(t, res)
			if e.Code != tt.code {
				t.Errorf("expected %s, got %s", tt.code, e.Code)
			}
			if e.Message == "" {
				t.Error("error should have non-empty message")
			}
		})
	}
}

func TestMCP_Errors_IsError(t *testing.T) {
	svc := setupTestService(t)
	root := testSnapshotRoot(t)
	tc := setupTestClient(t, svc, root)

	res := callTool(t, tc, "get_findings", map[string]any{"case_id": "nope"})
	if !res.IsError {
		t.Error("expected IsError=true for tool errors")
	}
}

func TestMCP_Trust_SnapshotMutationAfterEvaluation(t *testing.T) {
	svc := setupTestService(t)
	root := testSnapshotRoot(t)
	tc := setupTestClient(t, svc, root)

	snapshotPath := filepath.Join(root, "income_verification", "evidence_snapshot.json")
	originalBytes, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	backupBytes := make([]byte, len(originalBytes))
	copy(backupBytes, originalBytes)
	defer os.WriteFile(snapshotPath, backupBytes, 0644)

	evalRes := callTool(t, tc, "evaluate_case", map[string]any{
		"snapshot_path": "income_verification/evidence_snapshot.json",
	})
	evalOut := parseResult(t, evalRes)
	caseID := evalOut["case_id"].(string)

	findingsRes := callTool(t, tc, "get_findings", map[string]any{
		"case_id": caseID,
	})
	originalFindings := parseResult(t, findingsRes)

	var snapshotData map[string]any
	if err := json.Unmarshal(originalBytes, &snapshotData); err != nil {
		t.Fatalf("unmarshal snapshot for mutation: %v", err)
	}
	snapshotData["_mutated_after_eval"] = true
	modifiedBytes, err := json.Marshal(snapshotData)
	if err != nil {
		t.Fatalf("marshal mutated snapshot: %v", err)
	}
	if err := os.WriteFile(snapshotPath, modifiedBytes, 0644); err != nil {
		t.Fatalf("mutate snapshot: %v", err)
	}

	postMutateRes := callTool(t, tc, "get_findings", map[string]any{
		"case_id": caseID,
	})
	postMutateFindings := parseResult(t, postMutateRes)

	origJSON, _ := json.Marshal(originalFindings)
	mutJSON, _ := json.Marshal(postMutateFindings)
	if string(origJSON) != string(mutJSON) {
		t.Errorf("findings changed after snapshot mutation:\noriginal: %s\nmutated:  %s", origJSON, mutJSON)
	}

	evidenceRes := callTool(t, tc, "get_evidence", map[string]any{
		"case_id":       caseID,
		"finding_index": 0,
	})
	postMutateEvidence := parseResult(t, evidenceRes)
	_ = postMutateEvidence
	t.Log("snapshot mutation test: pinned results unchanged after backing file overwrite")
}

func TestMCP_Trust_StaleCaseIDRejected(t *testing.T) {
	svc := setupTestService(t)
	root := testSnapshotRoot(t)
	tc := setupTestClient(t, svc, root)

	originalPath := filepath.Join(root, "income_verification", "evidence_snapshot.json")
	originalBytes, err := os.ReadFile(originalPath)
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}

	evalARes := callTool(t, tc, "evaluate_case", map[string]any{
		"snapshot_path": "income_verification/evidence_snapshot.json",
	})
	evalAOut := parseResult(t, evalARes)
	caseIDA := evalAOut["case_id"].(string)
	t.Logf("case_id A: %s", caseIDA)

	snapshotBDir := filepath.Join(root, "stale_test")
	if err := os.MkdirAll(snapshotBDir, 0755); err != nil {
		t.Fatalf("mkdir stale_test: %v", err)
	}
	snapshotB := filepath.Join(snapshotBDir, "evidence_snapshot_B.json")

	var snapshotData map[string]any
	if err := json.Unmarshal(originalBytes, &snapshotData); err != nil {
		t.Fatalf("unmarshal original snapshot: %v", err)
	}
	snapshotData["_stale_test_marker"] = true
	mutatedBytes, err := json.Marshal(snapshotData)
	if err != nil {
		t.Fatalf("marshal mutated snapshot: %v", err)
	}
	if err := os.WriteFile(snapshotB, mutatedBytes, 0644); err != nil {
		t.Fatalf("write snapshot B: %v", err)
	}
	defer os.RemoveAll(snapshotBDir)

	evalBRes := callTool(t, tc, "evaluate_case", map[string]any{
		"snapshot_path": snapshotB,
	})
	evalBOut := parseResult(t, evalBRes)
	caseIDB := evalBOut["case_id"].(string)
	t.Logf("case_id B: %s", caseIDB)

	if caseIDA == caseIDB {
		t.Fatalf("case_id A and B are identical: %s (expected different content hashes)", caseIDA)
	}

	staleTests := []struct {
		name string
		args map[string]any
	}{
		{"get_findings", map[string]any{"case_id": caseIDA}},
		{"get_evidence", map[string]any{"case_id": caseIDA, "finding_index": 0}},
		{"get_audit_artifact", map[string]any{"case_id": caseIDA}},
	}

	for _, tt := range staleTests {
		t.Run(tt.name, func(t *testing.T) {
			res := callTool(t, tc, tt.name, tt.args)
			e := parseError(t, res)
			if e.Code != "CASE_NOT_FOUND" {
				t.Errorf("expected CASE_NOT_FOUND for stale case_id on %s, got %s", tt.name, e.Code)
			}
		})
	}

	newEvalRes := callTool(t, tc, "evaluate_case", map[string]any{
		"snapshot_path": "income_verification/evidence_snapshot.json",
	})
	newEvalOut := parseResult(t, newEvalRes)
	newCaseID := newEvalOut["case_id"].(string)
	if newCaseID != caseIDA {
		t.Errorf("re-evaluating original snapshot produced different case_id: expected %s, got %s", caseIDA, newCaseID)
	}
}

func TestMCP_Trust_GetFindingsMatchesDecision(t *testing.T) {
	svc := setupTestService(t)
	root := testSnapshotRoot(t)
	tc := setupTestClient(t, svc, root)

	evalRes := callTool(t, tc, "evaluate_case", map[string]any{
		"snapshot_path": "income_verification/evidence_snapshot.json",
	})
	evalOut := parseResult(t, evalRes)
	caseID := evalOut["case_id"].(string)

	findingsRes := callTool(t, tc, "get_findings", map[string]any{
		"case_id": caseID,
	})
	handlerOut := parseResult(t, findingsRes)

	decision := svc.GetDecision()
	if decision == nil {
		t.Fatal("GetDecision returned nil after evaluation")
	}

	type findingSummary struct {
		Index    int            `json:"index"`
		CheckID  string         `json:"check_id"`
		Status   string         `json:"status"`
		Severity string         `json:"severity"`
		Reason   string         `json:"reason"`
		Metrics  map[string]any `json:"metrics,omitempty"`
	}

	expected := make([]findingSummary, len(decision.Results))
	for i, r := range decision.Results {
		expected[i] = findingSummary{
			Index:    i,
			CheckID:  r.CheckID,
			Status:   string(r.Status),
			Severity: string(r.Severity),
			Reason:   r.Reason,
			Metrics:  r.Metrics,
		}
	}

	expectedJSON, _ := json.Marshal(map[string]any{"findings": expected})
	actualJSON, _ := json.Marshal(handlerOut)

	var expectedNorm, actualNorm any
	json.Unmarshal(expectedJSON, &expectedNorm)
	json.Unmarshal(actualJSON, &actualNorm)
	normalizedExpected, _ := json.Marshal(expectedNorm)
	normalizedActual, _ := json.Marshal(actualNorm)

	if string(normalizedExpected) != string(normalizedActual) {
		t.Errorf("get_findings output does not match Decision projection:\nexpected: %s\nactual:   %s", normalizedExpected, normalizedActual)
	}
}

func TestMCP_Trust_ArtifactHashIntegrity(t *testing.T) {
	svc := setupTestService(t)
	root := testSnapshotRoot(t)
	tc := setupTestClient(t, svc, root)

	evalRes := callTool(t, tc, "evaluate_case", map[string]any{
		"snapshot_path": "income_verification/evidence_snapshot.json",
	})
	evalOut := parseResult(t, evalRes)
	caseID := evalOut["case_id"].(string)

	artifactRes := callTool(t, tc, "get_audit_artifact", map[string]any{
		"case_id": caseID,
	})
	artifactOut := parseResult(t, artifactRes)

	outerHash, ok := artifactOut["artifact_hash"].(string)
	if !ok || outerHash == "" {
		t.Fatal("expected non-empty artifact_hash")
	}

	artifact, ok := artifactOut["artifact"].(map[string]any)
	if !ok {
		t.Fatal("expected artifact object in response")
	}

	manifest, ok := artifact["manifest"].(map[string]any)
	if !ok {
		t.Fatal("expected manifest object in artifact")
	}

	manifestHash, ok := manifest["artifact_hash"].(string)
	if !ok || manifestHash == "" {
		t.Fatal("expected non-empty manifest.artifact_hash")
	}

	if outerHash != manifestHash {
		t.Errorf("artifact_hash mismatch: outer=%s manifest=%s", outerHash, manifestHash)
	}
}
