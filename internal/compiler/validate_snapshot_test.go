package compiler

// These tests exercise ValidateSnapshot against a REAL module-graph worktree.
// They are slower than unit tests because each validation compiles the
// candidate inside a full copy of the repository module graph.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/doctrust/doctrust/internal/eval"
)

const validateSnapCheckID = "validate_snap_check"

// validCandidateSource returns a conforming candidate importing the canonical
// eval package path exactly as production candidates must.
func validCandidateSource() string {
	return `package candidate

import (
	"strings"

	"github.com/doctrust/doctrust/internal/eval"
)

type ValidateSnapCheck struct{}

func (c *ValidateSnapCheck) ID() string      { return "` + validateSnapCheckID + `" }
func (c *ValidateSnapCheck) Version() string { return "1.0" }

func (c *ValidateSnapCheck) Evaluate(facts eval.Facts, params map[string]any) eval.Result {
	_ = strings.ToLower("probe")
	return eval.Result{Status: eval.StatusPass}
}
`
}

// setupValidateSnapshotCandidate builds an approved candidate bound to a valid
// approval record, plus an isolated rulesets directory whose promoted ruleset
// does not contain the candidate's check ID.
func setupValidateSnapshotCandidate(t *testing.T, goSource string) (*CandidateSnapshot, string) {
	t.Helper()

	candidateDir := t.TempDir()
	rulesetsDir := t.TempDir()

	domainDir := filepath.Join(rulesetsDir, "income_verification")
	if err := os.MkdirAll(domainDir, 0755); err != nil {
		t.Fatalf("mkdir domain: %v", err)
	}
	promoted := `id: income_verification
version: "1"
checks:
  - id: gross_income_consistency
    version: "1.0"
  - id: required_documents
    version: "1.0"
  - id: net_vs_gross_incomparability
    version: "1.0"
`
	if err := os.WriteFile(filepath.Join(domainDir, "v1.yaml"), []byte(promoted), 0644); err != nil {
		t.Fatalf("write v1.yaml: %v", err)
	}

	writeErr := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(candidateDir, name), []byte(content), 0644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	writeErr("metadata.yaml", "id: "+validateSnapCheckID+"\nversion: \"1.0\"\ndescription: validate-snapshot test check\n")
	writeErr("scenarios.yaml", "scenarios: []\n")
	writeErr("adversarial.yaml", `scenarios:
  - name: edge_case
    origin: human_adversarial
    input:
      facts:
        - semantic_type: probe_value
          source_doc: paystub
          field: value
          value: 1
          source_span: "page=1"
          confidence: 0.95
    expected:
      check_id: `+validateSnapCheckID+`
      status: PASS
`)
	if goSource == "" {
		goSource = validCandidateSource()
	}
	writeErr("check.go", goSource)
	writeErr("state", string(StateApproved))

	if err := WriteApproval(candidateDir, validateSnapCheckID, "1.0", "test_user"); err != nil {
		t.Fatalf("WriteApproval: %v", err)
	}

	snapshot, err := SnapshotCandidate(candidateDir)
	if err != nil {
		t.Fatalf("SnapshotCandidate: %v", err)
	}
	return snapshot, rulesetsDir
}

func TestValidateSnapshot_ValidCandidate(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping module-graph compilation test in short mode")
	}

	snapshot, rulesetsDir := setupValidateSnapshotCandidate(t, "")

	result, err := ValidateSnapshot(snapshot, eval.DefaultRegistry(), rulesetsDir)
	if err != nil {
		if result != nil {
			t.Fatalf("ValidateSnapshot: %v\nerrors: %v", err, result.Errors)
		}
		t.Fatalf("ValidateSnapshot: %v", err)
	}
	if result.Failed != 0 {
		t.Errorf("expected zero failures, got %d: %v", result.Failed, result.Errors)
	}
	if result.Passed < 2 {
		t.Errorf("expected build+vet passes, got %d", result.Passed)
	}
}

// The dependency-graph assertion lives INSIDE ValidateSnapshot: the canonical
// import path must appear in `go list -deps ./candidate/` output, proving the
// real import path resolves rather than some altered arrangement compiling.
// This test proves the gate reports success only when resolution held.
func TestValidateSnapshot_ImportPathResolvesInWorktree(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping module-graph compilation test in short mode")
	}

	snapshot, rulesetsDir := setupValidateSnapshotCandidate(t, "")

	result, err := ValidateSnapshot(snapshot, eval.DefaultRegistry(), rulesetsDir)
	if err != nil {
		for _, e := range result.Errors {
			t.Logf("validation error: %s", e)
		}
		t.Fatalf("ValidateSnapshot should succeed when canonical import path resolves: %v", err)
	}
	for _, e := range result.Errors {
		if strings.Contains(e, "import-path assertion failed") {
			t.Fatalf("import-path assertion fired on conforming candidate: %s", e)
		}
	}
}

func TestValidateSnapshot_ForbiddenImport(t *testing.T) {
	badSource := `package candidate

import (
	"os"

	"github.com/doctrust/doctrust/internal/eval"
)

type ValidateSnapCheck struct{}

func (c *ValidateSnapCheck) ID() string      { return "` + validateSnapCheckID + `" }
func (c *ValidateSnapCheck) Version() string { return "1.0" }

func (c *ValidateSnapCheck) Evaluate(facts eval.Facts, params map[string]any) eval.Result {
	_ = os.Getenv("PATH")
	return eval.Result{Status: eval.StatusPass}
}
`
	snapshot, rulesetsDir := setupValidateSnapshotCandidate(t, badSource)

	result, err := ValidateSnapshot(snapshot, eval.DefaultRegistry(), rulesetsDir)
	if err == nil {
		t.Fatal("ValidateSnapshot should reject forbidden import")
	}
	if !strings.Contains(err.Error(), "forbidden imports") || !strings.Contains(err.Error(), "os") {
		t.Errorf("expected forbidden-imports error naming os, got: %v", err)
	}
	_ = result
}

func TestValidateSnapshot_DuplicateCheckID(t *testing.T) {
	snapshot, rulesetsDir := setupValidateSnapshotCandidate(t, "")

	// Register the same check ID in the promoted ruleset before validation.
	v1Path := filepath.Join(rulesetsDir, "income_verification", "v1.yaml")
	dup := `id: income_verification
version: "1"
checks:
  - id: gross_income_consistency
    version: "1.0"
  - id: ` + validateSnapCheckID + `
    version: "1.0"
`
	if err := os.WriteFile(v1Path, []byte(dup), 0644); err != nil {
		t.Fatalf("write duplicate v1.yaml: %v", err)
	}

	_, err := ValidateSnapshot(snapshot, eval.DefaultRegistry(), rulesetsDir)
	if err == nil {
		t.Fatal("ValidateSnapshot should reject check ID already present in a ruleset")
	}
	if !strings.Contains(err.Error(), "already exists in ruleset") {
		t.Errorf("expected already-exists error, got: %v", err)
	}
}

func TestValidateSnapshot_FromRepoRoot(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping module-graph compilation test in short mode")
	}

	repoRoot, err := FindModuleRoot()
	if err != nil {
		t.Fatalf("FindModuleRoot: %v", err)
	}

	snapshot, rulesetsDir := setupValidateSnapshotCandidate(t, "")

	// The production command runs from repo root; prove Gate 4 succeeds there.
	t.Chdir(repoRoot)

	result, verr := ValidateSnapshot(snapshot, eval.DefaultRegistry(), rulesetsDir)
	if verr != nil {
		if result != nil {
			t.Fatalf("ValidateSnapshot from repo root: %v\nerrors: %v", verr, result.Errors)
		}
		t.Fatalf("ValidateSnapshot from repo root: %v", verr)
	}
}

// adv_review3 P1-C: a failing `go list -deps` must FAIL Gate 4 — the
// import-graph tripwire can never be silently waived.
func TestValidateSnapshot_GoListFailureFailsGate(t *testing.T) {
	snapshot, rulesetsDir := setupValidateSnapshotCandidate(t, "")

	orig := goListDeps
	goListDeps = func(dir string) ([]byte, error) {
		return nil, fmt.Errorf("injected go list failure")
	}
	defer func() { goListDeps = orig }()

	result, err := ValidateSnapshot(snapshot, eval.DefaultRegistry(), rulesetsDir)
	if err == nil {
		t.Fatal("Gate 4 must fail when the import-path assertion cannot run")
	}
	found := false
	if result != nil {
		for _, e := range result.Errors {
			if strings.Contains(e, "go list -deps error") {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("expected recorded go-list assertion failure, got errors: %v", result.Errors)
	}
}
