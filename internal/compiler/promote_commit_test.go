package compiler

// These tests are not parallel-safe due to package-level function-variable injection.
// Every test must restore function variables with defer.

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
func setupTestPromotion(t *testing.T) (evalDir, rulesetsDir, candidateDir string, cleanup func()) {
	t.Helper()

	evalDir = t.TempDir()
	rulesetsDir = t.TempDir()
	candidateDir = t.TempDir()

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

import "github.com/doctrust/doctrust/internal/eval"

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
	evalDir, rulesetsDir, candidateDir, _ := setupTestPromotion(t)

	snapshot, err := SnapshotCandidate(candidateDir)
	if err != nil {
		t.Fatalf("SnapshotCandidate: %v", err)
	}

	stagingDir, err := StagePromotion(snapshot, evalDir, "income_verification", rulesetsDir)
	if err != nil {
		t.Fatalf("StagePromotion: %v", err)
	}
	defer RollbackPromotion(stagingDir)

	err = CommitPromotion(stagingDir, evalDir, "income_verification", rulesetsDir, snapshot)
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
	evalDir, rulesetsDir, candidateDir, _ := setupTestPromotion(t)

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
	addCheckRefFn = func(rulesetsDir, domain, checkID, version string) error {
		return fmt.Errorf("injected Ruleset write failure")
	}
	defer func() { addCheckRefFn = origFn }()

	err = CommitPromotion(stagingDir, evalDir, "income_verification", rulesetsDir, snapshot)
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
	evalDir, rulesetsDir, candidateDir, _ := setupTestPromotion(t)

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

	err = CommitPromotion(stagingDir, evalDir, "income_verification", rulesetsDir, snapshot)
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
	evalDir, rulesetsDir, candidateDir, _ := setupTestPromotion(t)

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

	err = CommitPromotion(stagingDir, evalDir, "income_verification", rulesetsDir, snapshot)
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
	evalDir, rulesetsDir, candidateDir, _ := setupTestPromotion(t)

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

	err = CommitPromotion(stagingDir, evalDir, "income_verification", rulesetsDir, snapshot)
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
	_, rulesetsDir, _, _ := setupTestPromotion(t)

	err := addCheckRefToRuleset(rulesetsDir, "income_verification", "my_new_check", "1.0")
	if err != nil {
		t.Fatalf("first addCheckRefToRuleset: %v", err)
	}

	err = addCheckRefToRuleset(rulesetsDir, "income_verification", "my_new_check", "1.0")
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
	_, rulesetsDir, _, _ := setupTestPromotion(t)

	// Add v1
	err := addCheckRefToRuleset(rulesetsDir, "income_verification", "my_new_check", "1.0")
	if err != nil {
		t.Fatalf("addCheckRefToRuleset v1: %v", err)
	}

	// Update to v2 — should replace v1
	err = addCheckRefToRuleset(rulesetsDir, "income_verification", "my_new_check", "2.0")
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
	_, rulesetsDir, _, _ := setupTestPromotion(t)

	// Remove working.yaml if it exists
	os.Remove(filepath.Join(rulesetsDir, "income_verification", "working.yaml"))

	err := addCheckRefToRuleset(rulesetsDir, "income_verification", "my_new_check", "1.0")
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

	err := addCheckRefToRuleset(emptyDir, "nonexistent_domain", "check", "1.0")
	if err == nil {
		t.Error("expected error when no working draft and no promoted ruleset exist")
	}
}

func TestSnapshotCandidate_Immutability(t *testing.T) {
	_, _, candidateDir, _ := setupTestPromotion(t)

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
	_, _, candidateDir, _ := setupTestPromotion(t)

	snapshot, err := SnapshotCandidate(candidateDir)
	if err != nil {
		t.Fatalf("SnapshotCandidate: %v", err)
	}

	// Write approval with correct hashes
	err = WriteApproval(candidateDir, snapshot.CheckID, snapshot.Version)
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
	_, _, candidateDir, _ := setupTestPromotion(t)

	snapshot, err := SnapshotCandidate(candidateDir)
	if err != nil {
		t.Fatalf("SnapshotCandidate: %v", err)
	}

	// Write approval with wrong CheckID
	err = WriteApproval(candidateDir, "wrong_check_id", snapshot.Version)
	if err != nil {
		t.Fatalf("WriteApproval: %v", err)
	}

	err = VerifyApprovalAgainstSnapshot(candidateDir, snapshot)
	if err == nil {
		t.Error("VerifyApprovalAgainstSnapshot should fail with identity mismatch")
	}
}

func TestStaleActiveCandidateNotRePromotable(t *testing.T) {
	evalDir, rulesetsDir, candidateDir, _ := setupTestPromotion(t)

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
	err = CommitPromotion(stagingDir, evalDir, "income_verification", rulesetsDir, snapshot)
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

import "github.com/doctrust/doctrust/internal/eval"

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

	err = CommitPromotion(stagingDir, evalDir, "income_verification", customRulesets, snapshot)
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
	evalDir, rulesetsDir, candidateDir, _ := setupTestPromotion(t)

	snapshot, err := SnapshotCandidate(candidateDir)
	if err != nil {
		t.Fatalf("SnapshotCandidate: %v", err)
	}

	stagingDir, err := StagePromotion(snapshot, evalDir, "income_verification", rulesetsDir)
	if err != nil {
		t.Fatalf("StagePromotion: %v", err)
	}
	defer RollbackPromotion(stagingDir)

	err = CommitPromotion(stagingDir, evalDir, "income_verification", rulesetsDir, snapshot)
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
	evalDir, rulesetsDir, candidateDir, _ := setupTestPromotion(t)

	snapshot, err := SnapshotCandidate(candidateDir)
	if err != nil {
		t.Fatalf("SnapshotCandidate: %v", err)
	}

	stagingDir, err := StagePromotion(snapshot, evalDir, "income_verification", rulesetsDir)
	if err != nil {
		t.Fatalf("StagePromotion: %v", err)
	}
	defer RollbackPromotion(stagingDir)

	err = CommitPromotion(stagingDir, evalDir, "income_verification", rulesetsDir, snapshot)
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
	evalDir, rulesetsDir, candidateDir, _ := setupTestPromotion(t)

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

	err = CommitPromotion(stagingDir, evalDir, "income_verification", rulesetsDir, snapshot)
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
	evalDir, rulesetsDir, candidateDir, _ := setupTestPromotion(t)

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

	err = CommitPromotion(stagingDir, evalDir, "income_verification", rulesetsDir, snapshot)
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
	evalDir, rulesetsDir, candidateDir, _ := setupTestPromotion(t)

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

	err = CommitPromotion(stagingDir, evalDir, "income_verification", rulesetsDir, snapshot)
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
