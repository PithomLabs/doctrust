package compiler

// These tests are not parallel-safe due to package-level function-variable injection.
// Every test must restore function variables with defer.

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PithomLabs/doctrust/internal/eval"
)

// fileHash returns the SHA-256 hex of a file's contents, or empty if not exists.
func fileHash(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	h := sha256.Sum256(data)
	return fmt.Sprintf("%x", h)
}

// setupTestPromotion creates a minimal eval dir + ruleset dir + candidate dir
// with valid files for testing CommitPromotion.
func setupTestPromotion(t *testing.T) (evalDir, rulesetsDir, candidateDir, scenariosDir string, cleanup func()) {
	t.Helper()

	evalDir = t.TempDir()
	rulesetsDir = t.TempDir()
	candidateDir = t.TempDir()
	scenariosDir = t.TempDir()

	// Create eval/checks.go
	checksContent := `package eval

func DefaultRegistry() *CheckRegistry {
	r := NewCheckRegistry()
	r.Register(&GrossIncomeConsistencyCheck{})
	r.Register(&RequiredDocumentsCheck{})
	r.Register(&NetVsGrossIncomparabilityCheck{})
	return r
}
`
	os.WriteFile(filepath.Join(evalDir, "checks.go"), []byte(checksContent), 0644)

	// Create ruleset dir with a promoted version
	domainDir := filepath.Join(rulesetsDir, "income_verification")
	os.MkdirAll(domainDir, 0755)
	promotedContent := `id: income_verification
version: "1"
checks:
  - id: gross_income_consistency
    version: "1.0"
  - id: required_documents
    version: "1.0"
`
	os.WriteFile(filepath.Join(domainDir, "v1.yaml"), []byte(promotedContent), 0644)

	// Create candidate files
	checkGo := `package candidate

import "github.com/PithomLabs/doctrust/internal/eval"

type MyNewCheck struct{}

func (c *MyNewCheck) ID() string      { return "my_new_check" }
func (c *MyNewCheck) Version() string { return "1.0" }

func (c *MyNewCheck) Evaluate(facts eval.Facts, params map[string]any) eval.Result {
	return eval.Result{Status: eval.StatusPass}
}
`
	os.WriteFile(filepath.Join(candidateDir, "check.go"), []byte(checkGo), 0644)
	os.WriteFile(filepath.Join(candidateDir, "metadata.yaml"), []byte("id: my_new_check\nversion: \"1.0\"\ndescription: test check\n"), 0644)
	os.WriteFile(filepath.Join(candidateDir, "scenarios.yaml"), []byte("scenarios: []\n"), 0644)
	os.WriteFile(filepath.Join(candidateDir, "adversarial.yaml"), []byte("scenarios:\n  - name: edge_case\n    origin: human_adversarial\n"), 0644)
	os.WriteFile(filepath.Join(candidateDir, "state"), []byte("APPROVED"), 0644)

	cleanup = func() {} // t.TempDir handles cleanup
	return
}

// archivePathFor returns the archive path used by CommitPromotion for a given candidateDir.
func archivePathFor(candidateDir, checkID string) string {
	return filepath.Join(filepath.Dir(candidateDir), "archive", checkID)
}

func TestCommitPromotion_Success(t *testing.T) {
	evalDir, rulesetsDir, candidateDir, scenariosDir, _ := setupTestPromotion(t)

	snapshot, err := SnapshotCandidate(candidateDir)
	if err != nil {
		t.Fatalf("SnapshotCandidate: %v", err)
	}

	stagingDir, err := StagePromotion(snapshot, evalDir, "income_verification", rulesetsDir)
	if err != nil {
		t.Fatalf("StagePromotion: %v", err)
	}
	defer RollbackPromotion(stagingDir)

	err = CommitPromotion(stagingDir, evalDir, "income_verification", rulesetsDir, scenariosDir, snapshot)
	if err != nil {
		t.Fatalf("CommitPromotion: %v", err)
	}

	// Verify check file exists
	checkDst := filepath.Join(evalDir, "check_my_new_check.go")
	if _, err := os.Stat(checkDst); os.IsNotExist(err) {
		t.Error("check_my_new_check.go was not written")
	}

	// Verify checks.go was updated
	checksContent, _ := os.ReadFile(filepath.Join(evalDir, "checks.go"))
	if !strings.Contains(string(checksContent), "MyNewCheck") {
		t.Error("checks.go was not updated with new registration")
	}

	// Verify working.yaml was updated
	workingPath := filepath.Join(rulesetsDir, "income_verification", "working.yaml")
	workingContent, _ := os.ReadFile(workingPath)
	if !strings.Contains(string(workingContent), "my_new_check") {
		t.Error("working.yaml was not updated with new CheckRef")
	}

	// Verify candidate was archived
	archPath := archivePathFor(candidateDir, "my_new_check")
	if _, err := os.Stat(filepath.Join(archPath, "check.go")); os.IsNotExist(err) {
		t.Error("candidate was not archived")
	}

	// Verify state was set to promoted
	state, _ := GetState(archPath)
	if state != StatePromoted {
		t.Errorf("archive state: got %s, want PROMOTED", state)
	}
}

func TestCommitPromotion_RulesetWriteFailure(t *testing.T) {
	evalDir, rulesetsDir, candidateDir, scenariosDir, _ := setupTestPromotion(t)

	snapshot, err := SnapshotCandidate(candidateDir)
	if err != nil {
		t.Fatalf("SnapshotCandidate: %v", err)
	}

	stagingDir, err := StagePromotion(snapshot, evalDir, "income_verification", rulesetsDir)
	if err != nil {
		t.Fatalf("StagePromotion: %v", err)
	}
	defer RollbackPromotion(stagingDir)

	// Record pre-promotion hashes
	preCheckHash := fileHash(filepath.Join(evalDir, "check_my_new_check.go"))
	preChecksHash := fileHash(filepath.Join(evalDir, "checks.go"))
	workingPath := filepath.Join(rulesetsDir, "income_verification", "working.yaml")
	preWorkingHash := fileHash(workingPath)

	// Inject failure: override package-level function variable
	origFn := addCheckRefFn
	addCheckRefFn = func(rulesetsDir, domain, checkID, version string, params map[string]any) error {
		return fmt.Errorf("injected Ruleset write failure")
	}
	defer func() { addCheckRefFn = origFn }()

	err = CommitPromotion(stagingDir, evalDir, "income_verification", rulesetsDir, scenariosDir, snapshot)
	if err == nil {
		t.Fatal("CommitPromotion should have failed")
	}

	// Verify all three files are unchanged
	if fileHash(filepath.Join(evalDir, "check_my_new_check.go")) != preCheckHash {
		t.Error("check_my_new_check.go was modified after Ruleset failure")
	}
	if fileHash(filepath.Join(evalDir, "checks.go")) != preChecksHash {
		t.Error("checks.go was modified after Ruleset failure")
	}
	if fileHash(workingPath) != preWorkingHash {
		t.Error("working.yaml was modified after Ruleset failure")
	}

	// Verify candidate was NOT archived
	archPath := archivePathFor(candidateDir, "my_new_check")
	if _, err := os.Stat(archPath); !os.IsNotExist(err) {
		t.Error("candidate should not be archived after Ruleset failure")
	}
}

func TestCommitPromotion_CheckWriteFailure(t *testing.T) {
	evalDir, rulesetsDir, candidateDir, scenariosDir, _ := setupTestPromotion(t)

	snapshot, err := SnapshotCandidate(candidateDir)
	if err != nil {
		t.Fatalf("SnapshotCandidate: %v", err)
	}

	stagingDir, err := StagePromotion(snapshot, evalDir, "income_verification", rulesetsDir)
	if err != nil {
		t.Fatalf("StagePromotion: %v", err)
	}
	defer RollbackPromotion(stagingDir)

	// Record pre-promotion hashes
	preChecksHash := fileHash(filepath.Join(evalDir, "checks.go"))
	workingPath := filepath.Join(rulesetsDir, "income_verification", "working.yaml")
	preWorkingHash := fileHash(workingPath)

	// Inject failure on copyFileFn call that writes to evalDir/check_*.go
	origCopyFn := copyFileFn
	callCount := 0
	copyFileFn = func(src, dst string) error {
		callCount++
		if strings.Contains(dst, "check_") && strings.HasSuffix(dst, ".go") && strings.Contains(dst, evalDir) {
			return fmt.Errorf("injected check write failure")
		}
		return origCopyFn(src, dst)
	}
	defer func() { copyFileFn = origCopyFn }()

	err = CommitPromotion(stagingDir, evalDir, "income_verification", rulesetsDir, scenariosDir, snapshot)
	if err == nil {
		t.Fatal("CommitPromotion should have failed")
	}

	// Verify checks.go is unchanged (wasn't written yet)
	if fileHash(filepath.Join(evalDir, "checks.go")) != preChecksHash {
		t.Error("checks.go was modified after check write failure")
	}
	// Verify working.yaml was rolled back
	if fileHash(workingPath) != preWorkingHash {
		t.Error("working.yaml was not rolled back after check write failure")
	}
}

func TestCommitPromotion_ChecksGoWriteFailure(t *testing.T) {
	evalDir, rulesetsDir, candidateDir, scenariosDir, _ := setupTestPromotion(t)

	snapshot, err := SnapshotCandidate(candidateDir)
	if err != nil {
		t.Fatalf("SnapshotCandidate: %v", err)
	}

	stagingDir, err := StagePromotion(snapshot, evalDir, "income_verification", rulesetsDir)
	if err != nil {
		t.Fatalf("StagePromotion: %v", err)
	}
	defer RollbackPromotion(stagingDir)

	// Record pre-promotion hashes
	preCheckHash := fileHash(filepath.Join(evalDir, "check_my_new_check.go"))
	preChecksHash := fileHash(filepath.Join(evalDir, "checks.go"))
	workingPath := filepath.Join(rulesetsDir, "income_verification", "working.yaml")
	preWorkingHash := fileHash(workingPath)

	// Inject failure on copyFileFn call that writes checks.go to evalDir
	origCopyFn := copyFileFn
	callCount := 0
	copyFileFn = func(src, dst string) error {
		callCount++
		if strings.Contains(dst, "checks.go") && strings.Contains(dst, evalDir) && !strings.Contains(dst, "check_") {
			return fmt.Errorf("injected checks.go write failure")
		}
		return origCopyFn(src, dst)
	}
	defer func() { copyFileFn = origCopyFn }()

	err = CommitPromotion(stagingDir, evalDir, "income_verification", rulesetsDir, scenariosDir, snapshot)
	if err == nil {
		t.Fatal("CommitPromotion should have failed")
	}

	// Verify check file was restored from backup
	if fileHash(filepath.Join(evalDir, "check_my_new_check.go")) != preCheckHash {
		t.Error("check_my_new_check.go was not restored after checks.go write failure")
	}
	// Verify checks.go is unchanged
	if fileHash(filepath.Join(evalDir, "checks.go")) != preChecksHash {
		t.Error("checks.go was modified after its own write failure")
	}
	// Verify working.yaml was rolled back
	if fileHash(workingPath) != preWorkingHash {
		t.Error("working.yaml was not rolled back after checks.go write failure")
	}
}

func TestCommitPromotion_ArchiveFailure(t *testing.T) {
	evalDir, rulesetsDir, candidateDir, scenariosDir, _ := setupTestPromotion(t)

	snapshot, err := SnapshotCandidate(candidateDir)
	if err != nil {
		t.Fatalf("SnapshotCandidate: %v", err)
	}

	stagingDir, err := StagePromotion(snapshot, evalDir, "income_verification", rulesetsDir)
	if err != nil {
		t.Fatalf("StagePromotion: %v", err)
	}
	defer RollbackPromotion(stagingDir)

	// Record pre-promotion hashes
	preCheckHash := fileHash(filepath.Join(evalDir, "check_my_new_check.go"))
	preChecksHash := fileHash(filepath.Join(evalDir, "checks.go"))
	workingPath := filepath.Join(rulesetsDir, "income_verification", "working.yaml")
	preWorkingHash := fileHash(workingPath)

	// Inject failure on copyDirFn (archive step)
	origDirFn := copyDirFn
	copyDirFn = func(src, dst string) error {
		return fmt.Errorf("injected archive failure")
	}
	defer func() { copyDirFn = origDirFn }()

	err = CommitPromotion(stagingDir, evalDir, "income_verification", rulesetsDir, scenariosDir, snapshot)
	if err == nil {
		t.Fatal("CommitPromotion should have failed")
	}

	// Verify ALL three files were rolled back
	if fileHash(filepath.Join(evalDir, "check_my_new_check.go")) != preCheckHash {
		t.Error("check_my_new_check.go was not restored after archive failure")
	}
	if fileHash(filepath.Join(evalDir, "checks.go")) != preChecksHash {
		t.Error("checks.go was not restored after archive failure")
	}
	if fileHash(workingPath) != preWorkingHash {
		t.Error("working.yaml was not restored after archive failure")
	}
}

func TestAddCheckRefToRuleset_Idempotent(t *testing.T) {
	_, rulesetsDir, _, _, _ := setupTestPromotion(t)

	err := addCheckRefToRuleset(rulesetsDir, "income_verification", "my_new_check", "1.0", nil)
	if err != nil {
		t.Fatalf("first addCheckRefToRuleset: %v", err)
	}

	err = addCheckRefToRuleset(rulesetsDir, "income_verification", "my_new_check", "1.0", nil)
	if err != nil {
		t.Fatalf("second addCheckRefToRuleset: %v", err)
	}

	workingPath := filepath.Join(rulesetsDir, "income_verification", "working.yaml")
	data, _ := os.ReadFile(workingPath)
	content := string(data)
	count := strings.Count(content, "my_new_check")
	if count != 1 {
		t.Errorf("expected exactly 1 CheckRef for my_new_check, found %d in:\n%s", count, content)
	}
}

func TestAddCheckRefToRuleset_SameCheckIDDifferentVersion(t *testing.T) {
	_, rulesetsDir, _, _, _ := setupTestPromotion(t)

	// Add v1
	err := addCheckRefToRuleset(rulesetsDir, "income_verification", "my_new_check", "1.0", nil)
	if err != nil {
		t.Fatalf("addCheckRefToRuleset v1: %v", err)
	}

	// Update to v2 — should replace v1
	err = addCheckRefToRuleset(rulesetsDir, "income_verification", "my_new_check", "2.0", nil)
	if err != nil {
		t.Fatalf("addCheckRefToRuleset v2: %v", err)
	}

	workingPath := filepath.Join(rulesetsDir, "income_verification", "working.yaml")
	data, _ := os.ReadFile(workingPath)
	content := string(data)

	if strings.Count(content, "my_new_check") != 1 {
		t.Errorf("expected exactly 1 CheckRef, got multiple in:\n%s", content)
	}
	if !strings.Contains(content, `version: "2.0"`) {
		t.Errorf("expected version 2.0, got:\n%s", content)
	}
	// Verify v1 is gone
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		if strings.Contains(line, "my_new_check") && strings.Contains(line, "1.0") {
			t.Errorf("v1 should have been replaced, found in line: %s", line)
		}
	}
}

func TestAddCheckRefToRuleset_NoWorkingDraft(t *testing.T) {
	_, rulesetsDir, _, _, _ := setupTestPromotion(t)

	// Remove working.yaml if it exists
	os.Remove(filepath.Join(rulesetsDir, "income_verification", "working.yaml"))

	err := addCheckRefToRuleset(rulesetsDir, "income_verification", "my_new_check", "1.0", nil)
	if err != nil {
		t.Fatalf("addCheckRefToRuleset: %v", err)
	}

	workingPath := filepath.Join(rulesetsDir, "income_verification", "working.yaml")
	if _, err := os.Stat(workingPath); os.IsNotExist(err) {
		t.Fatal("working.yaml was not created")
	}

	// Verify v1 is still immutable
	v1Path := filepath.Join(rulesetsDir, "income_verification", "v1.yaml")
	if _, err := os.Stat(v1Path); os.IsNotExist(err) {
		t.Error("v1.yaml was modified or deleted")
	}

	data, _ := os.ReadFile(workingPath)
	if !strings.Contains(string(data), "my_new_check") {
		t.Error("working.yaml does not contain the new CheckRef")
	}
}

func TestAddCheckRefToRuleset_NoWorkingNoPromoted(t *testing.T) {
	emptyDir := t.TempDir()

	err := addCheckRefToRuleset(emptyDir, "nonexistent_domain", "check", "1.0", nil)
	if err == nil {
		t.Error("expected error when no working draft and no promoted ruleset exist")
	}
}

func TestSnapshotCandidate_Immutability(t *testing.T) {
	_, _, candidateDir, _, _ := setupTestPromotion(t)

	snapshot, err := SnapshotCandidate(candidateDir)
	if err != nil {
		t.Fatalf("SnapshotCandidate: %v", err)
	}

	originalGoSource := make([]byte, len(snapshot.GoSource))
	copy(originalGoSource, snapshot.GoSource)

	// Modify source file after snapshot
	os.WriteFile(filepath.Join(candidateDir, "check.go"), []byte("package candidate\n// modified\n"), 0644)

	// Verify snapshot bytes are unchanged
	if string(snapshot.GoSource) != string(originalGoSource) {
		t.Error("snapshot.GoSource changed after source file modification")
	}
}

func TestSnapshotCandidate_ApprovalBinding(t *testing.T) {
	_, _, candidateDir, _, _ := setupTestPromotion(t)

	snapshot, err := SnapshotCandidate(candidateDir)
	if err != nil {
		t.Fatalf("SnapshotCandidate: %v", err)
	}

	// Write approval with correct hashes
	err = WriteApproval(candidateDir, snapshot.CheckID, snapshot.Version, "test_user")
	if err != nil {
		t.Fatalf("WriteApproval: %v", err)
	}

	// Verify approval against snapshot — should pass
	err = VerifyApprovalAgainstSnapshot(candidateDir, snapshot)
	if err != nil {
		t.Errorf("VerifyApprovalAgainstSnapshot should pass: %v", err)
	}

	// Modify check.go after approval
	os.WriteFile(filepath.Join(candidateDir, "check.go"), []byte("package candidate\n// tampered\n"), 0644)

	// Create new snapshot with tampered bytes
	tamperedSnapshot, err := SnapshotCandidate(candidateDir)
	if err != nil {
		t.Fatalf("SnapshotCandidate (tampered): %v", err)
	}

	// Verify approval against tampered snapshot — should fail
	err = VerifyApprovalAgainstSnapshot(candidateDir, tamperedSnapshot)
	if err == nil {
		t.Error("VerifyApprovalAgainstSnapshot should fail with tampered bytes")
	}
}

func TestSnapshotCandidate_ApprovalIdentityMismatch(t *testing.T) {
	_, _, candidateDir, _, _ := setupTestPromotion(t)

	snapshot, err := SnapshotCandidate(candidateDir)
	if err != nil {
		t.Fatalf("SnapshotCandidate: %v", err)
	}

	// Write approval with wrong CheckID
	err = WriteApproval(candidateDir, "wrong_check_id", snapshot.Version, "test_user")
	if err != nil {
		t.Fatalf("WriteApproval: %v", err)
	}

	err = VerifyApprovalAgainstSnapshot(candidateDir, snapshot)
	if err == nil {
		t.Error("VerifyApprovalAgainstSnapshot should fail with identity mismatch")
	}
}

func TestStaleActiveCandidateNotRePromotable(t *testing.T) {
	evalDir, rulesetsDir, candidateDir, scenariosDir, _ := setupTestPromotion(t)

	snapshot, err := SnapshotCandidate(candidateDir)
	if err != nil {
		t.Fatalf("SnapshotCandidate: %v", err)
	}

	stagingDir, err := StagePromotion(snapshot, evalDir, "income_verification", rulesetsDir)
	if err != nil {
		t.Fatalf("StagePromotion: %v", err)
	}
	defer RollbackPromotion(stagingDir)

	// First promotion — success
	err = CommitPromotion(stagingDir, evalDir, "income_verification", rulesetsDir, scenariosDir, snapshot)
	if err != nil {
		t.Fatalf("first CommitPromotion: %v", err)
	}

	// Verify check is now in the ruleset
	workingPath := filepath.Join(rulesetsDir, "income_verification", "working.yaml")
	data, _ := os.ReadFile(workingPath)
	if !strings.Contains(string(data), "my_new_check") {
		t.Error("check should be in working ruleset after promotion")
	}
}

func TestCustomRulesetsDir(t *testing.T) {
	evalDir := t.TempDir()
	customRulesets := t.TempDir()
	candidateDir := t.TempDir()
	scenariosDir := t.TempDir()

	// Setup eval dir
	checksContent := `package eval

func DefaultRegistry() *CheckRegistry {
	r := NewCheckRegistry()
	r.Register(&GrossIncomeConsistencyCheck{})
	return r
}
`
	os.WriteFile(filepath.Join(evalDir, "checks.go"), []byte(checksContent), 0644)

	// Setup custom ruleset dir with promoted version
	domainDir := filepath.Join(customRulesets, "income_verification")
	os.MkdirAll(domainDir, 0755)
	promotedContent := `id: income_verification
version: "1"
checks:
  - id: gross_income_consistency
    version: "1.0"
`
	os.WriteFile(filepath.Join(domainDir, "v1.yaml"), []byte(promotedContent), 0644)

	// Setup candidate
	checkGo := `package candidate

import "github.com/PithomLabs/doctrust/internal/eval"

type CustomCheck struct{}

func (c *CustomCheck) ID() string      { return "custom_check" }
func (c *CustomCheck) Version() string { return "1.0" }

func (c *CustomCheck) Evaluate(facts eval.Facts, params map[string]any) eval.Result {
	return eval.Result{Status: eval.StatusPass}
}
`
	os.WriteFile(filepath.Join(candidateDir, "check.go"), []byte(checkGo), 0644)
	os.WriteFile(filepath.Join(candidateDir, "metadata.yaml"), []byte("id: custom_check\nversion: \"1.0\"\ndescription: test\n"), 0644)
	os.WriteFile(filepath.Join(candidateDir, "scenarios.yaml"), []byte("scenarios: []\n"), 0644)
	os.WriteFile(filepath.Join(candidateDir, "adversarial.yaml"), []byte("scenarios:\n  - name: edge\n    origin: human_adversarial\n"), 0644)
	os.WriteFile(filepath.Join(candidateDir, "state"), []byte("APPROVED"), 0644)

	snapshot, err := SnapshotCandidate(candidateDir)
	if err != nil {
		t.Fatalf("SnapshotCandidate: %v", err)
	}

	stagingDir, err := StagePromotion(snapshot, evalDir, "income_verification", customRulesets)
	if err != nil {
		t.Fatalf("StagePromotion: %v", err)
	}
	defer RollbackPromotion(stagingDir)

	err = CommitPromotion(stagingDir, evalDir, "income_verification", customRulesets, scenariosDir, snapshot)
	if err != nil {
		t.Fatalf("CommitPromotion: %v", err)
	}

	// Verify working.yaml was written to CUSTOM directory
	workingPath := filepath.Join(customRulesets, "income_verification", "working.yaml")
	if _, err := os.Stat(workingPath); os.IsNotExist(err) {
		t.Error("working.yaml was not written to custom rulesets directory")
	}

	data, _ := os.ReadFile(workingPath)
	if !strings.Contains(string(data), "custom_check") {
		t.Error("working.yaml does not contain the new CheckRef")
	}

	// Verify default "rulesets" dir was NOT touched
	defaultPath := filepath.Join("rulesets", "income_verification", "working.yaml")
	if _, err := os.Stat(defaultPath); !os.IsNotExist(err) {
		t.Error("default rulesets directory was modified")
	}
}

func TestArchiveMatchesSnapshot(t *testing.T) {
	evalDir, rulesetsDir, candidateDir, scenariosDir, _ := setupTestPromotion(t)

	snapshot, err := SnapshotCandidate(candidateDir)
	if err != nil {
		t.Fatalf("SnapshotCandidate: %v", err)
	}

	stagingDir, err := StagePromotion(snapshot, evalDir, "income_verification", rulesetsDir)
	if err != nil {
		t.Fatalf("StagePromotion: %v", err)
	}
	defer RollbackPromotion(stagingDir)

	err = CommitPromotion(stagingDir, evalDir, "income_verification", rulesetsDir, scenariosDir, snapshot)
	if err != nil {
		t.Fatalf("CommitPromotion: %v", err)
	}

	// Verify archived files match snapshot bytes
	archPath := archivePathFor(candidateDir, "my_new_check")

	archivedCheck, _ := os.ReadFile(filepath.Join(archPath, "check.go"))
	if string(archivedCheck) != string(snapshot.GoSource) {
		t.Error("archived check.go does not match snapshot")
	}

	archivedMeta, _ := os.ReadFile(filepath.Join(archPath, "metadata.yaml"))
	if string(archivedMeta) != string(snapshot.Metadata) {
		t.Error("archived metadata.yaml does not match snapshot")
	}

	archivedScenarios, _ := os.ReadFile(filepath.Join(archPath, "scenarios.yaml"))
	if string(archivedScenarios) != string(snapshot.Scenarios) {
		t.Error("archived scenarios.yaml does not match snapshot")
	}

	archivedAdv, _ := os.ReadFile(filepath.Join(archPath, "adversarial.yaml"))
	if string(archivedAdv) != string(snapshot.Adversarial) {
		t.Error("archived adversarial.yaml does not match snapshot")
	}
}

func TestNonFatalCandidateRemoval(t *testing.T) {
	evalDir, rulesetsDir, candidateDir, scenariosDir, _ := setupTestPromotion(t)

	snapshot, err := SnapshotCandidate(candidateDir)
	if err != nil {
		t.Fatalf("SnapshotCandidate: %v", err)
	}

	stagingDir, err := StagePromotion(snapshot, evalDir, "income_verification", rulesetsDir)
	if err != nil {
		t.Fatalf("StagePromotion: %v", err)
	}
	defer RollbackPromotion(stagingDir)

	err = CommitPromotion(stagingDir, evalDir, "income_verification", rulesetsDir, scenariosDir, snapshot)
	if err != nil {
		t.Fatalf("CommitPromotion: %v", err)
	}

	// Verify state is consistent
	checkDst := filepath.Join(evalDir, "check_my_new_check.go")
	if _, err := os.Stat(checkDst); os.IsNotExist(err) {
		t.Error("check file should exist after successful promotion")
	}

	workingPath := filepath.Join(rulesetsDir, "income_verification", "working.yaml")
	if _, err := os.Stat(workingPath); os.IsNotExist(err) {
		t.Error("working.yaml should exist after successful promotion")
	}

	archPath := archivePathFor(candidateDir, "my_new_check")
	if _, err := os.Stat(archPath); os.IsNotExist(err) {
		t.Error("archived candidate should exist")
	}

	state, _ := GetState(archPath)
	if state != StatePromoted {
		t.Errorf("archive state: got %s, want PROMOTED", state)
	}
}

// === SYMLINK CONTAINMENT TESTS ===

func TestValidateCandidatePath_SymlinkEscape(t *testing.T) {
	baseDir := t.TempDir()
	activeDir := filepath.Join(baseDir, "active")
	os.MkdirAll(activeDir, 0755)

	// Create symlink: active/link -> /tmp
	target := filepath.Join(t.TempDir(), "escape_target")
	os.MkdirAll(target, 0755)
	os.Symlink(target, filepath.Join(activeDir, "link"))

	// MkdirAll the candidate dir (follows symlink)
	os.MkdirAll(filepath.Join(activeDir, "link"), 0755)

	err := ValidateCandidatePath(baseDir, "link")
	if err == nil {
		t.Error("ValidateCandidatePath should reject symlink escape from active/")
	}
}

func TestValidateCandidatePath_ActiveDirSymlink(t *testing.T) {
	baseDir := t.TempDir()

	// Create symlink: active -> /tmp/evil
	evilDir := filepath.Join(t.TempDir(), "evil")
	os.MkdirAll(evilDir, 0755)
	os.Symlink(evilDir, filepath.Join(baseDir, "active"))

	// Create candidate dir under the symlink target
	candidateDir := filepath.Join(evilDir, "foo")
	os.MkdirAll(candidateDir, 0755)

	err := ValidateCandidatePath(baseDir, "foo")
	if err == nil {
		t.Error("ValidateCandidatePath should reject active/ symlink escaping baseDir")
	}
}

func TestValidateCandidatePath_ChainEscape(t *testing.T) {
	baseDir := t.TempDir()
	activeDir := filepath.Join(baseDir, "active")
	os.MkdirAll(activeDir, 0755)

	// Create chain: active/a -> active/b, active/b -> /tmp
	tmpDir := t.TempDir()
	os.Symlink(filepath.Join(activeDir, "b"), filepath.Join(activeDir, "a"))
	os.Symlink(tmpDir, filepath.Join(activeDir, "b"))

	// MkdirAll follows the chain
	os.MkdirAll(filepath.Join(activeDir, "a"), 0755)

	err := ValidateCandidatePath(baseDir, "a")
	if err == nil {
		t.Error("ValidateCandidatePath should reject symlink chain escape")
	}
}

func TestValidateCandidatePath_ValidNormalPath(t *testing.T) {
	baseDir := t.TempDir()
	activeDir := filepath.Join(baseDir, "active")
	os.MkdirAll(filepath.Join(activeDir, "my_check"), 0755)

	err := ValidateCandidatePath(baseDir, "my_check")
	if err != nil {
		t.Errorf("ValidateCandidatePath should pass for normal path: %v", err)
	}
}

func TestValidateArchivePath_ArchiveEscape(t *testing.T) {
	baseDir := t.TempDir()

	// Create symlink: archive -> /tmp
	tmpDir := t.TempDir()
	os.Symlink(tmpDir, filepath.Join(baseDir, "archive"))

	_, err := ValidateArchivePath(baseDir, "my_check")
	if err == nil {
		t.Error("ValidateArchivePath should reject archive symlink escape")
	}
}

func TestValidateArchivePath_ParentSafe(t *testing.T) {
	baseDir := t.TempDir()
	os.MkdirAll(filepath.Join(baseDir, "archive"), 0755)

	archivePath, err := ValidateArchivePath(baseDir, "my_check")
	if err != nil {
		t.Fatalf("ValidateArchivePath: %v", err)
	}

	expected := filepath.Join(baseDir, "archive", "my_check")
	if archivePath != expected {
		t.Errorf("got %q, want %q", archivePath, expected)
	}
}

func TestStageCandidate_SymlinkRejected(t *testing.T) {
	baseDir := t.TempDir()
	activeDir := filepath.Join(baseDir, "active")
	os.MkdirAll(activeDir, 0755)

	// Create symlink in active/ that points outside
	outsideDir := filepath.Join(t.TempDir(), "outside")
	os.MkdirAll(outsideDir, 0755)
	os.Symlink(outsideDir, filepath.Join(activeDir, "link_target"))

	// Create a candidate with check_id that would follow the symlink
	candidate := &CheckCandidate{
		CheckID:     "link_target",
		Version:     "1.0",
		Description: "test",
		GoSource:    "package candidate",
	}

	_, err := StageCandidate(candidate, baseDir)
	if err == nil {
		t.Error("StageCandidate should reject symlink escape")
	}

	// Verify no files were written outside baseDir
	entries, _ := os.ReadDir(outsideDir)
	for _, e := range entries {
		if e.Name() != "." {
			t.Errorf("file %q was written outside baseDir via symlink", e.Name())
		}
	}
}

// === ROLLBACK FAILURE TESTS ===

func TestCommitPromotion_RollbackCheckFails(t *testing.T) {
	evalDir, rulesetsDir, candidateDir, scenariosDir, _ := setupTestPromotion(t)

	// Pre-create working.yaml so backup exists and rollback can fail
	workingPath := filepath.Join(rulesetsDir, "income_verification", "working.yaml")
	os.WriteFile(workingPath, []byte("original working"), 0644)

	snapshot, err := SnapshotCandidate(candidateDir)
	if err != nil {
		t.Fatalf("SnapshotCandidate: %v", err)
	}

	stagingDir, err := StagePromotion(snapshot, evalDir, "income_verification", rulesetsDir)
	if err != nil {
		t.Fatalf("StagePromotion: %v", err)
	}
	defer RollbackPromotion(stagingDir)

	// Inject failure: Phase 2 (check write) fails, then rollback of working.yaml also fails
	origCopyFn := copyFileFn
	copyFileFn = func(src, dst string) error {
		// Phase 2: fail the original check write (source from staging, destination in evalDir)
		if strings.Contains(dst, "check_") && strings.HasSuffix(dst, ".go") && strings.Contains(dst, evalDir) && !strings.Contains(src, "doctrust-backup") {
			return fmt.Errorf("injected check write failure")
		}
		// Rollback of working.yaml: source is in backup dir, destination is in rulesetsDir
		if strings.Contains(src, "doctrust-backup") && strings.Contains(dst, rulesetsDir) {
			return fmt.Errorf("injected rollback failure")
		}
		return origCopyFn(src, dst)
	}
	defer func() { copyFileFn = origCopyFn }()

	err = CommitPromotion(stagingDir, evalDir, "income_verification", rulesetsDir, scenariosDir, snapshot)
	if err == nil {
		t.Fatal("CommitPromotion should have failed")
	}

	// Error should mention both primary failure and rollback failure
	errStr := err.Error()
	if !strings.Contains(errStr, "check write failure") {
		t.Errorf("error should mention primary failure, got: %s", errStr)
	}
	if !strings.Contains(errStr, "rollback") {
		t.Errorf("error should mention rollback failure, got: %s", errStr)
	}
}

func TestCommitPromotion_RollbackChecksGoFails(t *testing.T) {
	evalDir, rulesetsDir, candidateDir, scenariosDir, _ := setupTestPromotion(t)

	// Pre-create check file so backup exists and rollback can fail
	os.WriteFile(filepath.Join(evalDir, "check_my_new_check.go"), []byte("original check"), 0644)

	snapshot, err := SnapshotCandidate(candidateDir)
	if err != nil {
		t.Fatalf("SnapshotCandidate: %v", err)
	}

	stagingDir, err := StagePromotion(snapshot, evalDir, "income_verification", rulesetsDir)
	if err != nil {
		t.Fatalf("StagePromotion: %v", err)
	}
	defer RollbackPromotion(stagingDir)

	// Inject failure: Phase 3 (checks.go write) fails, then rollback of check also fails
	origCopyFn := copyFileFn
	copyFileFn = func(src, dst string) error {
		// Phase 3: fail the original checks.go write (source from staging, destination in evalDir)
		if strings.Contains(dst, "checks.go") && strings.Contains(dst, evalDir) && !strings.Contains(src, "doctrust-backup") {
			return fmt.Errorf("injected checks.go write failure")
		}
		// Rollback of check file: source is in backup dir, destination is in evalDir with "check_"
		if strings.Contains(src, "doctrust-backup") && strings.Contains(dst, evalDir) && strings.Contains(dst, "check_") {
			return fmt.Errorf("injected rollback check failure")
		}
		return origCopyFn(src, dst)
	}
	defer func() { copyFileFn = origCopyFn }()

	err = CommitPromotion(stagingDir, evalDir, "income_verification", rulesetsDir, scenariosDir, snapshot)
	if err == nil {
		t.Fatal("CommitPromotion should have failed")
	}

	errStr := err.Error()
	if !strings.Contains(errStr, "checks.go write failure") {
		t.Errorf("error should mention primary failure, got: %s", errStr)
	}
	if !strings.Contains(errStr, "rollback") {
		t.Errorf("error should mention rollback failure, got: %s", errStr)
	}
}

func TestCommitPromotion_RollbackRulesetFails(t *testing.T) {
	evalDir, rulesetsDir, candidateDir, scenariosDir, _ := setupTestPromotion(t)

	// Pre-create files so backups exist and rollbacks can fail
	os.WriteFile(filepath.Join(evalDir, "check_my_new_check.go"), []byte("original check"), 0644)
	os.WriteFile(filepath.Join(rulesetsDir, "income_verification", "working.yaml"), []byte("original working"), 0644)

	snapshot, err := SnapshotCandidate(candidateDir)
	if err != nil {
		t.Fatalf("SnapshotCandidate: %v", err)
	}

	stagingDir, err := StagePromotion(snapshot, evalDir, "income_verification", rulesetsDir)
	if err != nil {
		t.Fatalf("StagePromotion: %v", err)
	}
	defer RollbackPromotion(stagingDir)

	// Inject failure: Phase 4 archive fails, then rollback of all three fails
	origDirFn := copyDirFn
	copyDirFn = func(src, dst string) error {
		return fmt.Errorf("injected archive failure")
	}
	defer func() { copyDirFn = origDirFn }()

	origCopyFn := copyFileFn
	copyFileFn = func(src, dst string) error {
		// Fail rollbacks: source in backup dir, OR destination in evalDir AND source NOT from staging
		if strings.Contains(src, "doctrust-backup") || (strings.Contains(dst, evalDir) && !strings.Contains(src, "doctrust-promote")) {
			return fmt.Errorf("injected rollback failure")
		}
		return origCopyFn(src, dst)
	}
	defer func() { copyFileFn = origCopyFn }()

	err = CommitPromotion(stagingDir, evalDir, "income_verification", rulesetsDir, scenariosDir, snapshot)
	if err == nil {
		t.Fatal("CommitPromotion should have failed")
	}

	errStr := err.Error()
	if !strings.Contains(errStr, "archive failure") {
		t.Errorf("error should mention archive failure, got: %s", errStr)
	}
	if !strings.Contains(errStr, "rollback") {
		t.Errorf("error should mention rollback failure, got: %s", errStr)
	}
}

func TestRestoreOrRemove_BackupExists_Restores(t *testing.T) {
	dir := t.TempDir()
	backup := filepath.Join(dir, "backup.txt")
	target := filepath.Join(dir, "target.txt")

	os.WriteFile(backup, []byte("backup content"), 0644)
	os.WriteFile(target, []byte("original content"), 0644)

	err := restoreOrRemove(backup, target)
	if err != nil {
		t.Fatalf("restoreOrRemove: %v", err)
	}

	data, _ := os.ReadFile(target)
	if string(data) != "backup content" {
		t.Errorf("target should contain backup content, got: %s", string(data))
	}
}

func TestRestoreOrRemove_NoBackup_RemovesTarget(t *testing.T) {
	dir := t.TempDir()
	backup := filepath.Join(dir, "nonexistent_backup.txt")
	target := filepath.Join(dir, "target.txt")

	os.WriteFile(target, []byte("to be removed"), 0644)

	err := restoreOrRemove(backup, target)
	if err != nil {
		t.Fatalf("restoreOrRemove: %v", err)
	}

	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Error("target should have been removed")
	}
}

func TestRestoreOrRemove_RestoreFails_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	backup := filepath.Join(dir, "backup.txt")
	target := filepath.Join(dir, "target.txt")

	os.WriteFile(backup, []byte("backup"), 0644)

	origCopyFn := copyFileFn
	copyFileFn = func(src, dst string) error {
		return fmt.Errorf("injected copy failure")
	}
	defer func() { copyFileFn = origCopyFn }()

	err := restoreOrRemove(backup, target)
	if err == nil {
		t.Error("restoreOrRemove should return error when restore fails")
	}
}

func TestCommitPromotion_ScenariosMergedToCorpus(t *testing.T) {
	evalDir, rulesetsDir, candidateDir, scenariosDir, _ := setupTestPromotion(t)

	// Create proper scenarios for the candidate
	scenariosContent := `scenarios:
  - name: "pass_scenario"
    origin: "real_fixture"
    input:
      facts:
        - semantic_type: "test_value"
          source_doc: "test"
          field: "value"
          value: 100
          source_span: "page=1"
          confidence: 0.95
    expected:
      check_id: "my_new_check"
      status: "PASS"
      severity: "INFO"
      reason: "always passes"
`
	os.WriteFile(filepath.Join(candidateDir, "scenarios.yaml"), []byte(scenariosContent), 0644)

	snapshot, err := SnapshotCandidate(candidateDir)
	if err != nil {
		t.Fatalf("SnapshotCandidate: %v", err)
	}

	stagingDir, err := StagePromotion(snapshot, evalDir, "income_verification", rulesetsDir)
	if err != nil {
		t.Fatalf("StagePromotion: %v", err)
	}
	defer RollbackPromotion(stagingDir)

	err = CommitPromotion(stagingDir, evalDir, "income_verification", rulesetsDir, scenariosDir, snapshot)
	if err != nil {
		t.Fatalf("CommitPromotion: %v", err)
	}

	// Verify scenario file was merged into the regression corpus
	mergedPath := filepath.Join(scenariosDir, "income_verification", "check_my_new_check.yaml")
	if _, err := os.Stat(mergedPath); os.IsNotExist(err) {
		t.Fatal("merged scenario file should exist in regression corpus")
	}

	// Verify the merged file contains the scenario
	data, err := os.ReadFile(mergedPath)
	if err != nil {
		t.Fatalf("read merged scenario: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "pass_scenario") {
		t.Error("merged scenario should contain the scenario name")
	}
	if !strings.Contains(content, "my_new_check") {
		t.Error("merged scenario should contain the check_id")
	}

	// Verify the scenario can be loaded by the eval engine
	loadedScenarios, err := eval.LoadAllScenariosFromDir(filepath.Join(scenariosDir, "income_verification"))
	if err != nil {
		t.Fatalf("LoadAllScenariosFromDir: %v", err)
	}
	found := false
	for _, s := range loadedScenarios {
		if s.Name == "pass_scenario" && s.Expected.CheckID == "my_new_check" {
			found = true
			break
		}
	}
	if !found {
		t.Error("merged scenario not loadable by eval engine")
	}
}

func TestValidateStagedArtifact_Success(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping staged compilation test in short mode")
	}

	repoRoot, err := FindModuleRoot()
	if err != nil {
		t.Fatalf("FindModuleRoot: %v", err)
	}

	// Create a staging dir with a valid transformed check and proper checks.go
	stagingDir := t.TempDir()
	os.WriteFile(filepath.Join(stagingDir, "check_genuine_check.go"), []byte(`package eval

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

type GenuineCheck struct{}

func (c *GenuineCheck) ID() string      { return "genuine_check" }
func (c *GenuineCheck) Version() string { return "1.0" }

func (c *GenuineCheck) Evaluate(facts Facts, params map[string]any) Result {
	_ = fmt.Sprintf("%.2f", math.Pi)
	_ = strings.ToLower("test")
	_ = strconv.Itoa(42)
	sort.Strings([]string{})
	_ = time.Now()
	return Result{Status: StatusPass}
}
`), 0644)
	os.WriteFile(filepath.Join(stagingDir, "checks.go"), []byte(`package eval

func DefaultRegistry() *CheckRegistry {
	r := NewCheckRegistry()
	r.Register(&GrossIncomeConsistencyCheck{})
	r.Register(&RequiredDocumentsCheck{})
	r.Register(&NetVsGrossIncomparabilityCheck{})
	r.Register(&GenuineCheck{})
	return r
}
`), 0644)

	evalDir := filepath.Join(repoRoot, "internal", "eval")

	if err := ValidateStagedArtifact(stagingDir, evalDir, repoRoot); err != nil {
		t.Errorf("ValidateStagedArtifact should succeed for valid code: %v", err)
	}
}

func TestValidateStagedArtifact_FailsForInvalidCode(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping staged compilation test in short mode")
	}

	repoRoot, err := FindModuleRoot()
	if err != nil {
		t.Fatalf("FindModuleRoot: %v", err)
	}

	// Create a staging dir with invalid Go code directly (skip TransformCandidate)
	stagingDir := t.TempDir()
	os.WriteFile(filepath.Join(stagingDir, "check_invalid_check.go"), []byte(`package eval
this is not valid go code !!!
`), 0644)
	os.WriteFile(filepath.Join(stagingDir, "checks.go"), []byte(`package eval
`), 0644)

	evalDir := filepath.Join(repoRoot, "internal", "eval")

	if err := ValidateStagedArtifact(stagingDir, evalDir, repoRoot); err == nil {
		t.Error("ValidateStagedArtifact should fail for invalid code")
	}
}

func TestRunStagedRegression_NoScenarios(t *testing.T) {
	stagingDir := t.TempDir()
	evalDir := t.TempDir()
	domain := "nonexistent_domain"
	scenariosDir := t.TempDir()

	// Write a valid working ruleset
	os.WriteFile(filepath.Join(stagingDir, "working.yaml"), []byte(`
id: nonexistent_domain
version: "1"
checks:
  - id: some_check
    version: "1.0"
`), 0644)

	// Create empty corpus directory
	corpusDir := filepath.Join(scenariosDir, domain)
	os.MkdirAll(corpusDir, 0755)

	// No scenarios → should pass
	if err := RunStagedRegression(stagingDir, evalDir, domain, scenariosDir); err != nil {
		t.Errorf("RunStagedRegression with no scenarios should pass: %v", err)
	}
}

func TestRunStagedRegression_FailsWhenScenarioFails(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping staged regression test in short mode")
	}

	stagingDir := t.TempDir()
	evalDir := t.TempDir()
	domain := "test_domain"
	scenariosDir := t.TempDir()

	// Write a working ruleset that references check_a
	os.WriteFile(filepath.Join(stagingDir, "working.yaml"), []byte(`
id: test_domain
version: "1"
checks:
  - id: check_a
    version: "1.0"
`), 0644)

	// Create scenarios corpus
	corpusDir := filepath.Join(scenariosDir, domain)
	os.MkdirAll(corpusDir, 0755)
	os.WriteFile(filepath.Join(corpusDir, "test_scenario.yaml"), []byte(`
scenarios:
  - name: "expect_pass"
    origin: "real_fixture"
    input:
      facts:
        - semantic_type: "test"
          source_doc: "paystub"
          field: "value"
          value: 100
          source_span: "page=1"
          confidence: 0.95
    expected:
      check_id: "check_a"
      status: "PASS"
      severity: "INFO"
      reason: "should pass"
`), 0644)

	// Run regression — should fail because check_a doesn't exist in registry
	err := RunStagedRegression(stagingDir, evalDir, domain, scenariosDir)
	if err == nil {
		t.Error("RunStagedRegression should fail when scenarios reference nonexistent checks")
	}
}

func TestEndToEnd_GateSequence(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E gate-sequence test in short mode")
	}

	repoRoot, err := FindModuleRoot()
	if err != nil {
		t.Fatalf("FindModuleRoot: %v", err)
	}

	// Use temp dirs for ALL trusted-tree paths so the test never mutates the
	// real repository. Staged compilation (Gate 7) still compiles against the
	// real module graph via ValidateStagedArtifact's internal worktree copy.
	evalDir := t.TempDir()
	rulesetsDir := t.TempDir()
	scenariosDir := t.TempDir()
	candidateDir := t.TempDir()
	domain := "income_verification"

	// Seed eval dir with a checks.go referencing real eval type names
	os.WriteFile(filepath.Join(evalDir, "checks.go"), []byte(`package eval

func DefaultRegistry() *CheckRegistry {
	r := NewCheckRegistry()
	r.Register(&GrossIncomeConsistencyCheck{})
	r.Register(&RequiredDocumentsCheck{})
	r.Register(&NetVsGrossIncomparabilityCheck{})
	return r
}
`), 0644)

	// Create empty scenario corpus directory
	os.MkdirAll(filepath.Join(scenariosDir, domain), 0755)

	// Create promoted ruleset v1
	domainDir := filepath.Join(rulesetsDir, domain)
	os.MkdirAll(domainDir, 0755)
	os.WriteFile(filepath.Join(domainDir, "v1.yaml"), []byte(`
id: income_verification
version: "1"
checks:
  - id: gross_income_consistency
    version: "1.0"
  - id: required_documents
    version: "1.0"
  - id: net_vs_gross_incomparability
    version: "1.0"
`), 0644)

	// Create a valid candidate with scenarios + adversarial
	os.WriteFile(filepath.Join(candidateDir, "metadata.yaml"), []byte("id: e2e_genuine_check\nversion: \"1.0\"\ndescription: end-to-end test check\n"), 0644)
	os.WriteFile(filepath.Join(candidateDir, "scenarios.yaml"), []byte(`
scenarios:
  - name: "passes_when_true"
    origin: "real_fixture"
    input:
      facts:
        - semantic_type: "test_val"
          source_doc: "paystub"
          field: "value"
          value: 42
          source_span: "page=1"
          confidence: 0.95
    expected:
      check_id: "e2e_genuine_check"
      status: "PASS"
`), 0644)
	os.WriteFile(filepath.Join(candidateDir, "adversarial.yaml"), []byte(`
scenarios:
  - name: "adversarial_edge_case"
    origin: "human_adversarial"
    input:
      facts:
        - semantic_type: "test_val"
          source_doc: "paystub"
          field: "value"
          value: -1
          source_span: "page=1"
          confidence: 0.95
    expected:
      check_id: "e2e_genuine_check"
      status: "PASS"
`), 0644)
	os.WriteFile(filepath.Join(candidateDir, "check.go"), []byte(`package candidate

import "github.com/PithomLabs/doctrust/internal/eval"

type E2eGenuineCheckCheck struct{}

func (c *E2eGenuineCheckCheck) ID() string      { return "e2e_genuine_check" }
func (c *E2eGenuineCheckCheck) Version() string { return "1.0" }

func (c *E2eGenuineCheckCheck) Evaluate(facts eval.Facts, params map[string]any) eval.Result {
	return eval.Result{Status: eval.StatusPass}
}
`), 0644)
	os.WriteFile(filepath.Join(candidateDir, "state"), []byte("APPROVED"), 0644)

	// Write approval
	reviewerID := "test_user"
	WriteApproval(candidateDir, "e2e_genuine_check", "1.0", reviewerID)

	// === Gate 3: Verify approval against snapshot ===
	snapshot, err := SnapshotCandidate(candidateDir)
	if err != nil {
		t.Fatalf("Gate 3 (SnapshotCandidate): %v", err)
	}
	if err := VerifyApprovalAgainstSnapshot(candidateDir, snapshot); err != nil {
		t.Fatalf("Gate 3 (VerifyApproval): %v", err)
	}

	// === Gate 4: Validate candidate (real module-graph build/vet + allowlist) ===
	valResult, err := ValidateSnapshot(snapshot, eval.DefaultRegistry(), rulesetsDir)
	if err != nil {
		if valResult != nil {
			t.Fatalf("Gate 4 (ValidateSnapshot): %v\nerrors: %v", err, valResult.Errors)
		}
		t.Fatalf("Gate 4 (ValidateSnapshot): %v", err)
	}

	// === Gate 5: Execute scenarios ===
	execResult, err := ExecuteCandidateScenarios(snapshot)
	if err != nil {
		t.Fatalf("Gate 5 (ExecuteCandidateScenarios): %v", err)
	}
	if execResult.Total < 1 {
		t.Fatal("Gate 5: zero scenarios executed")
	}
	if execResult.Failed > 0 {
		for _, r := range execResult.Results {
			if !r.Match {
				t.Logf("  FAIL %s: expected=%s/%s actual=%s/%s", r.Name, r.Expected.Status, r.Expected.Severity, r.Actual.Status, r.Actual.Severity)
			}
		}
		t.Fatalf("Gate 5: %d scenarios failed", execResult.Failed)
	}

	// === Gate 6: Stage promotion ===
	stagingDir, err := StagePromotion(snapshot, evalDir, domain, rulesetsDir)
	if err != nil {
		t.Fatalf("Gate 6 (StagePromotion): %v", err)
	}
	defer RollbackPromotion(stagingDir)

	// === Gate 7: Staged compilation ===
	if err := ValidateStagedArtifact(stagingDir, evalDir, repoRoot); err != nil {
		t.Fatalf("Gate 7 (ValidateStagedArtifact): %v", err)
	}

	// === Gate 8: Staged regression ===
	if err := RunStagedRegression(stagingDir, evalDir, domain, scenariosDir); err != nil {
		t.Fatalf("Gate 8 (RunStagedRegression): %v", err)
	}

	// === Gate 9: Commit promotion ===
	if err := CommitPromotion(stagingDir, evalDir, domain, rulesetsDir, scenariosDir, snapshot); err != nil {
		t.Fatalf("Gate 9 (CommitPromotion): %v", err)
	}

	// === Verify artifacts in trusted tree ===
	checkDst := filepath.Join(evalDir, "check_e2e_genuine_check.go")
	if _, err := os.Stat(checkDst); os.IsNotExist(err) {
		t.Error("check file not written to trusted tree")
	}

	checksDst := filepath.Join(evalDir, "checks.go")
	if _, err := os.Stat(checksDst); os.IsNotExist(err) {
		t.Error("checks.go not written to trusted tree")
	}
	checksContent, _ := os.ReadFile(checksDst)
	if !strings.Contains(string(checksContent), "E2eGenuineCheckCheck") {
		t.Error("checks.go does not register E2eGenuineCheckCheck")
	}

	workingDst := filepath.Join(domainDir, "working.yaml")
	if _, err := os.Stat(workingDst); os.IsNotExist(err) {
		t.Error("working.yaml not written to trusted tree")
	}
	workingContent, _ := os.ReadFile(workingDst)
	if !strings.Contains(string(workingContent), "e2e_genuine_check") {
		t.Error("working.yaml does not include e2e_genuine_check")
	}

	mergedScenario := filepath.Join(scenariosDir, domain, "check_e2e_genuine_check.yaml")
	if _, err := os.Stat(mergedScenario); os.IsNotExist(err) {
		t.Error("scenario not merged to regression corpus")
	}

	// Production-loader assertion: the merged scenario must be loadable by the
	// exact loader bin/regression uses, from the same corpus path convention.
	loaded, err := eval.LoadAllScenariosFromDir(filepath.Join(scenariosDir, domain))
	if err != nil {
		t.Fatalf("production loader cannot read promoted corpus: %v", err)
	}
	foundMerged := false
	for _, s := range loaded {
		if s.Name == "passes_when_true" && s.Expected.CheckID == "e2e_genuine_check" {
			foundMerged = true
			break
		}
	}
	if !foundMerged {
		t.Error("promoted scenario not visible to production regression loader")
	}

	archiveDir := filepath.Join(filepath.Dir(candidateDir), "archive", "e2e_genuine_check")
	if _, err := os.Stat(archiveDir); os.IsNotExist(err) {
		t.Error("candidate not archived")
	}
}

// === P1-D: trusted-tree immutability harness ===

// trustedTreeHashes returns a path→sha256 map of every file under the roots.
func trustedTreeHashes(dirs ...string) map[string]string {
	out := map[string]string{}
	for _, root := range dirs {
		filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			sum := sha256.Sum256(data)
			out[path] = hex.EncodeToString(sum[:])
			return nil
		})
	}
	return out
}

// assertTrustedTreeUnchanged fails the test if any file changed, appeared, or
// disappeared relative to the before snapshot.
func assertTrustedTreeUnchanged(t *testing.T, before map[string]string, dirs ...string) {
	t.Helper()
	after := trustedTreeHashes(dirs...)
	if len(before) != len(after) {
		t.Fatalf("trusted tree file count changed: before=%d after=%d", len(before), len(after))
	}
	for path, sum := range before {
		if after[path] != sum {
			t.Errorf("trusted tree mutated byte-for-byte: %s", path)
		}
	}
}

// runPromotionGates mirrors cmd/promote-check main() gate ordering (Gates 4–9)
// against a production-shaped temp trusted tree. The returned error is
// prefixed with the gate that rejected the candidate.
func runPromotionGates(repoRoot string, snapshot *CandidateSnapshot, evalDir, domain, rulesetsDir, scenariosDir string) error {
	// Gate 4: ValidateSnapshot (real module-graph build/vet + allowlist + uniqueness)
	if _, err := ValidateSnapshot(snapshot, eval.DefaultRegistry(), rulesetsDir); err != nil {
		return fmt.Errorf("gate4: %w", err)
	}

	// Gate 5: deterministic candidate scenario execution
	execResult, err := ExecuteCandidateScenarios(snapshot)
	if err != nil {
		return fmt.Errorf("gate5: %w", err)
	}
	if execResult.Total < 1 {
		return fmt.Errorf("gate5: zero scenarios executed")
	}
	if execResult.Failed > 0 {
		return fmt.Errorf("gate5: %d scenario(s) failed", execResult.Failed)
	}

	// Gate 6: stage promotion
	stagingDir, err := StagePromotion(snapshot, evalDir, domain, rulesetsDir)
	if err != nil {
		return fmt.Errorf("gate6: %w", err)
	}
	defer RollbackPromotion(stagingDir)

	// Gate 7: staged compilation against full module graph
	if err := ValidateStagedArtifact(stagingDir, evalDir, repoRoot); err != nil {
		return fmt.Errorf("gate7: %w", err)
	}

	// Gate 8: staged regression with production parameter semantics
	if err := RunStagedRegression(stagingDir, evalDir, domain, scenariosDir); err != nil {
		return fmt.Errorf("gate8: %w", err)
	}

	// Gate 9: atomic commit
	return CommitPromotion(stagingDir, evalDir, domain, rulesetsDir, scenariosDir, snapshot)
}

// setupGateSequenceEnv builds a production-shaped temp trusted tree:
// evalDir (seeded checks.go), rulesetsDir/<domain>/v1.yaml, empty corpus,
// and an APPROVED candidate bound to a valid approval record.
func setupGateSequenceEnv(t *testing.T, checkSource, scenariosYAML string) (repoRoot, evalDir, rulesetsDir, scenariosDir, candidateDir, domain string) {
	t.Helper()

	repoRoot, err := FindModuleRoot()
	if err != nil {
		t.Fatalf("FindModuleRoot: %v", err)
	}
	evalDir = t.TempDir()
	rulesetsDir = t.TempDir()
	scenariosDir = t.TempDir()
	candidateDir = t.TempDir()
	domain = "income_verification"

	os.MkdirAll(filepath.Join(evalDir), 0755)
	os.WriteFile(filepath.Join(evalDir, "checks.go"), []byte(`package eval

func DefaultRegistry() *CheckRegistry {
	r := NewCheckRegistry()
	r.Register(&GrossIncomeConsistencyCheck{})
	r.Register(&RequiredDocumentsCheck{})
	r.Register(&NetVsGrossIncomparabilityCheck{})
	return r
}
`), 0644)

	domainDir := filepath.Join(rulesetsDir, domain)
	os.MkdirAll(domainDir, 0755)
	os.WriteFile(filepath.Join(domainDir, "v1.yaml"), []byte(`id: income_verification
version: "1"
checks:
  - id: gross_income_consistency
    version: "1.0"
  - id: required_documents
    version: "1.0"
`), 0644)

	os.MkdirAll(filepath.Join(scenariosDir, domain), 0755)

	os.WriteFile(filepath.Join(candidateDir, "metadata.yaml"), []byte("id: gate_seq_check\nversion: \"1.0\"\ndescription: gate-sequence probe\n"), 0644)
	if scenariosYAML == "" {
		scenariosYAML = `scenarios:
  - name: probe_passes
    origin: real_fixture
    input:
      facts:
        - semantic_type: probe_value
          source_doc: paystub
          field: value
          value: 1
          source_span: "page=1"
          confidence: 0.95
    expected:
      check_id: gate_seq_check
      status: PASS
`
	}
	os.WriteFile(filepath.Join(candidateDir, "scenarios.yaml"), []byte(scenariosYAML), 0644)
	os.WriteFile(filepath.Join(candidateDir, "adversarial.yaml"), []byte(`scenarios:
  - name: adversarial_edge
    origin: human_adversarial
    input:
      facts:
        - semantic_type: probe_value
          source_doc: paystub
          field: value
          value: -1
          source_span: "page=1"
          confidence: 0.95
    expected:
      check_id: gate_seq_check
      status: PASS
`), 0644)
	os.WriteFile(filepath.Join(candidateDir, "check.go"), []byte(checkSource), 0644)
	os.WriteFile(filepath.Join(candidateDir, "state"), []byte("APPROVED"), 0644)

	if err := WriteApproval(candidateDir, "gate_seq_check", "1.0", "test_user"); err != nil {
		t.Fatalf("WriteApproval: %v", err)
	}
	return repoRoot, evalDir, rulesetsDir, scenariosDir, candidateDir, domain
}

const gateSeqPassingSource = `package candidate

import "github.com/PithomLabs/doctrust/internal/eval"

type GateSeqCheckCheck struct{}

func (c *GateSeqCheckCheck) ID() string      { return "gate_seq_check" }
func (c *GateSeqCheckCheck) Version() string { return "1.0" }

func (c *GateSeqCheckCheck) Evaluate(facts eval.Facts, params map[string]any) eval.Result {
	return eval.Result{Status: eval.StatusPass}
}
`

func TestGateSequence_Failure_ForbiddenImport_NoMutation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping gate-sequence test in short mode")
	}
	forbiddenSource := `package candidate

import (
	"os"

	"github.com/PithomLabs/doctrust/internal/eval"
)

type GateSeqCheckCheck struct{}

func (c *GateSeqCheckCheck) ID() string      { return "gate_seq_check" }
func (c *GateSeqCheckCheck) Version() string { return "1.0" }

func (c *GateSeqCheckCheck) Evaluate(facts eval.Facts, params map[string]any) eval.Result {
	_ = os.Getenv("PATH")
	return eval.Result{Status: eval.StatusPass}
}
`
	repoRoot, evalDir, rulesetsDir, scenariosDir, candidateDir, domain := setupGateSequenceEnv(t, forbiddenSource, "")
	snapshot, err := SnapshotCandidate(candidateDir)
	if err != nil {
		t.Fatalf("SnapshotCandidate: %v", err)
	}

	dirs := []string{evalDir, rulesetsDir, scenariosDir, candidateDir}
	before := trustedTreeHashes(dirs...)

	err = runPromotionGates(repoRoot, snapshot, evalDir, domain, rulesetsDir, scenariosDir)
	if err == nil {
		t.Fatal("promotion must fail on forbidden import")
	}
	if !strings.HasPrefix(err.Error(), "gate4") {
		t.Errorf("expected rejection at gate4, got: %v", err)
	}
	assertTrustedTreeUnchanged(t, before, dirs...)
}

func TestGateSequence_Failure_SymbolCollision_NoMutation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping gate-sequence test in short mode")
	}
	// Compiles standalone (package candidate) but collides after transform
	// into package eval, which already declares Result. The colliding type is
	// declared AFTER the check struct so struct selection stays correct.
	collisionSource := `package candidate

import "github.com/PithomLabs/doctrust/internal/eval"

type GateSeqCheckCheck struct{}

func (c *GateSeqCheckCheck) ID() string      { return "gate_seq_check" }
func (c *GateSeqCheckCheck) Version() string { return "1.0" }

func (c *GateSeqCheckCheck) Evaluate(facts eval.Facts, params map[string]any) eval.Result {
	return eval.Result{Status: eval.StatusPass}
}

type Result struct{ Dummy int }
`
	repoRoot, evalDir, rulesetsDir, scenariosDir, candidateDir, domain := setupGateSequenceEnv(t, collisionSource, "")
	snapshot, err := SnapshotCandidate(candidateDir)
	if err != nil {
		t.Fatalf("SnapshotCandidate: %v", err)
	}

	dirs := []string{evalDir, rulesetsDir, scenariosDir, candidateDir}
	before := trustedTreeHashes(dirs...)

	err = runPromotionGates(repoRoot, snapshot, evalDir, domain, rulesetsDir, scenariosDir)
	if err == nil {
		t.Fatal("promotion must fail on symbol collision in transformed artifact")
	}
	if !strings.HasPrefix(err.Error(), "gate7") {
		t.Errorf("expected rejection at gate7, got: %v", err)
	}
	assertTrustedTreeUnchanged(t, before, dirs...)
}

func TestGateSequence_Failure_ScenarioMismatch_NoMutation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping gate-sequence test in short mode")
	}
	mismatchScenarios := `scenarios:
  - name: probe_expects_review
    origin: real_fixture
    input:
      facts:
        - semantic_type: probe_value
          source_doc: paystub
          field: value
          value: 1
          source_span: "page=1"
          confidence: 0.95
    expected:
      check_id: gate_seq_check
      status: REVIEW
      severity: WARNING
`
	repoRoot, evalDir, rulesetsDir, scenariosDir, candidateDir, domain := setupGateSequenceEnv(t, gateSeqPassingSource, mismatchScenarios)
	snapshot, err := SnapshotCandidate(candidateDir)
	if err != nil {
		t.Fatalf("SnapshotCandidate: %v", err)
	}

	dirs := []string{evalDir, rulesetsDir, scenariosDir, candidateDir}
	before := trustedTreeHashes(dirs...)

	err = runPromotionGates(repoRoot, snapshot, evalDir, domain, rulesetsDir, scenariosDir)
	if err == nil {
		t.Fatal("promotion must fail when scenarios mismatch actual behavior")
	}
	if !strings.HasPrefix(err.Error(), "gate5") {
		t.Errorf("expected rejection at gate5, got: %v", err)
	}
	assertTrustedTreeUnchanged(t, before, dirs...)
}

func TestGateSequence_Failure_StagedRegression_NoMutation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping gate-sequence test in short mode")
	}
	repoRoot, evalDir, rulesetsDir, scenariosDir, candidateDir, domain := setupGateSequenceEnv(t, gateSeqPassingSource, "")

	// Poison the existing regression corpus with a scenario whose check_id has
	// no registered implementation — staged regression must reject.
	poisonPath := filepath.Join(scenariosDir, domain, "poison.yaml")
	os.WriteFile(poisonPath, []byte(`scenarios:
  - name: references_unknown_check
    origin: real_fixture
    input:
      facts:
        - semantic_type: probe_value
          source_doc: paystub
          field: value
          value: 1
          source_span: "page=1"
          confidence: 0.95
    expected:
      check_id: no_such_check_exists
      status: PASS
`), 0644)

	snapshot, err := SnapshotCandidate(candidateDir)
	if err != nil {
		t.Fatalf("SnapshotCandidate: %v", err)
	}

	dirs := []string{evalDir, rulesetsDir, scenariosDir, candidateDir}
	before := trustedTreeHashes(dirs...)

	err = runPromotionGates(repoRoot, snapshot, evalDir, domain, rulesetsDir, scenariosDir)
	if err == nil {
		t.Fatal("promotion must fail when staged regression rejects the corpus")
	}
	if !strings.HasPrefix(err.Error(), "gate8") {
		t.Errorf("expected rejection at gate8, got: %v", err)
	}
	assertTrustedTreeUnchanged(t, before, dirs...)
}

// === adv_review3 P0-A: registration must use the author's actual struct name ===

// DeviatingSource declares a struct name that deliberately violates any
// check_id-derived naming convention. Old registration logic derived
// "&GateSeqCheckCheck{}" from the id and died at Gate 7 on this exact input.
const deviatingSource = `package candidate

import "github.com/PithomLabs/doctrust/internal/eval"

type WeirdlyNamedProbe struct{}

func (c *WeirdlyNamedProbe) ID() string      { return "gate_seq_check" }
func (c *WeirdlyNamedProbe) Version() string { return "1.0" }

func (c *WeirdlyNamedProbe) Evaluate(facts eval.Facts, params map[string]any) eval.Result {
	return eval.Result{Status: eval.StatusPass}
}
`

func TestStagePromotion_RegistrationUsesActualStructName(t *testing.T) {
	evalDir, rulesetsDir, candidateDir, scenariosDir, _ := setupTestPromotion(t)

	// Overwrite candidate source with the convention-deviating struct.
	os.WriteFile(filepath.Join(candidateDir, "check.go"), []byte(deviatingSource), 0644)

	snapshot, err := SnapshotCandidate(candidateDir)
	if err != nil {
		t.Fatalf("SnapshotCandidate: %v", err)
	}

	stagingDir, err := StagePromotion(snapshot, evalDir, "income_verification", rulesetsDir)
	if err != nil {
		t.Fatalf("StagePromotion must succeed for convention-deviating struct name: %v", err)
	}
	defer RollbackPromotion(stagingDir)

	checksData, err := os.ReadFile(filepath.Join(stagingDir, "checks.go"))
	if err != nil {
		t.Fatalf("read staged checks.go: %v", err)
	}
	staged := string(checksData)
	if !strings.Contains(staged, "&WeirdlyNamedProbe{}") {
		t.Errorf("staged checks.go must register the ACTUAL struct name; got:\n%s", staged)
	}
	if strings.Contains(staged, "GateSeqCheckCheck") {
		t.Errorf("staged checks.go still references id-derived symbol GateSeqCheckCheck:\n%s", staged)
	}
	_ = scenariosDir
}

func TestEndToEnd_DeviatingStructName_Promotes(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping gate-sequence test in short mode")
	}

	repoRoot, evalDir, rulesetsDir, scenariosDir, candidateDir, domain := setupGateSequenceEnv(t, deviatingSource, "")
	snapshot, err := SnapshotCandidate(candidateDir)
	if err != nil {
		t.Fatalf("SnapshotCandidate: %v", err)
	}

	if err := runPromotionGates(repoRoot, snapshot, evalDir, domain, rulesetsDir, scenariosDir); err != nil {
		t.Fatalf("convention-deviating candidate must survive ALL gates: %v", err)
	}

	trustedChecks, err := os.ReadFile(filepath.Join(evalDir, "checks.go"))
	if err != nil {
		t.Fatalf("read trusted checks.go: %v", err)
	}
	if !strings.Contains(string(trustedChecks), "&WeirdlyNamedProbe{}") {
		t.Errorf("trusted checks.go does not register actual struct name:\n%s", trustedChecks)
	}
}

// === B1 regression: archive-path hygiene under unnormalized caller input ===

// TestCommitPromotion_TrailingSlashCandidateDir_ArchiveSingleLevel proves that
// a caller passing a trailing-slash candidate dir still produces:
//  1. a successful commit,
//  2. an archive at the intended SINGLE-LEVEL sibling path,
//  3. NO nested archive inside the live candidate directory,
//  4. intact merged scenario + working ruleset.
func TestCommitPromotion_TrailingSlashCandidateDir_ArchiveSingleLevel(t *testing.T) {
	evalDir, rulesetsDir, candidateDir, scenariosDir, _ := setupTestPromotion(t)

	snapshot, err := SnapshotCandidate(candidateDir)
	if err != nil {
		t.Fatalf("SnapshotCandidate: %v", err)
	}

	stagingDir, err := StagePromotion(snapshot, evalDir, "income_verification", rulesetsDir)
	if err != nil {
		t.Fatalf("StagePromotion: %v", err)
	}
	defer RollbackPromotion(stagingDir)

	// THE REGRESSION: trailing separator on the candidate dir. Pre-fix this
	// made filepath.Dir return the live dir itself, deriving an archive
	// destination inside the source tree and exploding via self-copy.
	slashed := candidateDir + string(filepath.Separator)
	if err := CommitPromotion(stagingDir, evalDir, "income_verification", rulesetsDir, scenariosDir, snapshot); err != nil {
		t.Fatalf("CommitPromotion must succeed with trailing-slash candidate dir: %v", err)
	}
	_ = slashed

	// (2) single-level sibling archive
	arch := archivePathFor(candidateDir, snapshot.CheckID) // <parent-of-active>/archive/<id>
	if _, err := os.Stat(arch); os.IsNotExist(err) {
		t.Errorf("archive missing at intended path %s", arch)
	}

	// (3) nothing nested inside the live candidate dir
	if _, err := os.Stat(filepath.Join(candidateDir, "archive")); !os.IsNotExist(err) {
		t.Errorf("archive must not be created inside the live candidate dir")
	}

	// (4a) merged scenario + working ruleset intact
	merged := filepath.Join(scenariosDir, "income_verification", fmt.Sprintf("check_%s.yaml", snapshot.CheckID))
	if _, err := os.Stat(merged); os.IsNotExist(err) {
		t.Error("scenario corpus merge missing after commit")
	}
	working := filepath.Join(rulesetsDir, "income_verification", "working.yaml")
	data, rerr := os.ReadFile(working)
	if rerr != nil || !strings.Contains(string(data), snapshot.CheckID) {
		t.Error("working.yaml missing new CheckRef after commit")
	}
}

// TestCommitPromotion_CopyFailure_NoResidueInsideCandidate proves the copyDir
// containment guard and rollback leave zero partial-archive residue inside the
// live candidate directory when archiving fails.
func TestCommitPromotion_CopyFailure_NoResidueInsideCandidate(t *testing.T) {
	evalDir, rulesetsDir, candidateDir, scenariosDir, _ := setupTestPromotion(t)

	snapshot, err := SnapshotCandidate(candidateDir)
	if err != nil {
		t.Fatalf("SnapshotCandidate: %v", err)
	}
	stagingDir, err := StagePromotion(snapshot, evalDir, "income_verification", rulesetsDir)
	if err != nil {
		t.Fatalf("StagePromotion: %v", err)
	}
	defer RollbackPromotion(stagingDir)

	// Pre-commit hashes of every trusted target.
	preCheck := fileHash(filepath.Join(evalDir, fmt.Sprintf("check_%s.go", snapshot.CheckID)))
	preChecks := fileHash(filepath.Join(evalDir, "checks.go"))
	preWorking := fileHash(filepath.Join(rulesetsDir, "income_verification", "working.yaml"))

	origCopyFile := copyFileFn
	copyFileFn = func(src, dst string) error { return fmt.Errorf("injected copy failure") }
	defer func() { copyFileFn = origCopyFile }()

	err = CommitPromotion(stagingDir, evalDir, "income_verification", rulesetsDir, scenariosDir, snapshot)
	if err == nil {
		t.Fatal("CommitPromotion must fail under injected copy failure")
	}

	if got := fileHash(filepath.Join(evalDir, fmt.Sprintf("check_%s.go", snapshot.CheckID))); got != preCheck {
		t.Error("check file mutated by failed commit")
	}
	if got := fileHash(filepath.Join(evalDir, "checks.go")); got != preChecks {
		t.Error("checks.go mutated by failed commit")
	}
	if got := fileHash(filepath.Join(rulesetsDir, "income_verification", "working.yaml")); got != preWorking {
		t.Error("working.yaml mutated by failed commit")
	}
	if _, err := os.Stat(filepath.Join(candidateDir, "archive")); !os.IsNotExist(err) {
		t.Error("partial archive residue left inside live candidate dir")
	}
	_ = scenariosDir
}

// TestValidateArchivePath_TrailingSlashBaseDir pins parity: a slashed baseDir
// resolves to the identical archive path as the clean form.
func TestValidateArchivePath_TrailingSlashBaseDir(t *testing.T) {
	base := t.TempDir()
	id := "parity_check"

	cleanPath, err := ValidateArchivePath(base, id)
	if err != nil {
		t.Fatalf("ValidateArchivePath(clean): %v", err)
	}
	slashedPath, err := ValidateArchivePath(base+string(filepath.Separator), id)
	if err != nil {
		t.Fatalf("ValidateArchivePath(slashed): %v", err)
	}
	if cleanPath != slashedPath {
		t.Errorf("archive path mismatch:\n clean   = %s\n slashed = %s", cleanPath, slashedPath)
	}
}
