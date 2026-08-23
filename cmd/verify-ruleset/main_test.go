package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/doctrust/doctrust/internal/eval"
)

func TestVerifyRuleset_ValidDomain(t *testing.T) {
	rulesetsDir := t.TempDir()

	// Create a ruleset with a check
	domainDir := filepath.Join(rulesetsDir, "income_verification")
	os.MkdirAll(domainDir, 0755)
	rs := `id: income_verification
version: "1"
checks:
  - id: "gross_income_consistency"
    version: "1.0"
`
	os.WriteFile(filepath.Join(domainDir, "v1.yaml"), []byte(rs), 0644)

	// Build the binary first
	binary := filepath.Join(t.TempDir(), "verify-ruleset")
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Dir = filepath.Join(findModuleRoot(t), "cmd", "verify-ruleset")
	out, err := build.CombinedOutput()
	if err != nil {
		t.Fatalf("go build: %v\noutput: %s", err, out)
	}

	// Run the binary
	cmd := exec.Command(binary, "--domain", "income_verification", "--rulesets-dir", rulesetsDir)
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("verify-ruleset failed: %v\noutput: %s", err, out)
	}
	if !strings.Contains(string(out), "verified successfully") {
		t.Errorf("expected success message, got: %s", out)
	}
}

func TestVerifyRuleset_MissingCheck(t *testing.T) {
	rulesetsDir := t.TempDir()

	// Create a ruleset with a non-existent check
	domainDir := filepath.Join(rulesetsDir, "income_verification")
	os.MkdirAll(domainDir, 0755)
	rs := `id: income_verification
version: "1"
checks:
  - id: "nonexistent_check"
    version: "1.0"
`
	os.WriteFile(filepath.Join(domainDir, "v1.yaml"), []byte(rs), 0644)

	// Build the binary first
	binary := filepath.Join(t.TempDir(), "verify-ruleset")
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Dir = filepath.Join(findModuleRoot(t), "cmd", "verify-ruleset")
	out, err := build.CombinedOutput()
	if err != nil {
		t.Fatalf("go build: %v\noutput: %s", err, out)
	}

	// Run the binary
	cmd := exec.Command(binary, "--domain", "income_verification", "--rulesets-dir", rulesetsDir)
	out, err = cmd.CombinedOutput()
	if err == nil {
		t.Fatal("verify-ruleset should fail for missing check")
	}
	if !strings.Contains(string(out), "not registered") {
		t.Errorf("expected 'not registered' error, got: %s", out)
	}
}

func TestVerifyRuleset_ExpectVersion_Pass(t *testing.T) {
	rulesetsDir := t.TempDir()

	domainDir := filepath.Join(rulesetsDir, "income_verification")
	os.MkdirAll(domainDir, 0755)
	rs := `id: income_verification
version: "3"
checks:
  - id: "gross_income_consistency"
    version: "1.0"
`
	os.WriteFile(filepath.Join(domainDir, "v3.yaml"), []byte(rs), 0644)

	binary := filepath.Join(t.TempDir(), "verify-ruleset")
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Dir = filepath.Join(findModuleRoot(t), "cmd", "verify-ruleset")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\noutput: %s", err, out)
	}

	cmd := exec.Command(binary, "--domain", "income_verification", "--rulesets-dir", rulesetsDir, "--expect-version", "3")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("verify-ruleset should pass with correct version: %v\noutput: %s", err, out)
	}
	if !strings.Contains(string(out), "Version match: PASS") {
		t.Errorf("expected 'Version match: PASS', got: %s", out)
	}
}

func TestVerifyRuleset_ExpectVersion_Fail(t *testing.T) {
	rulesetsDir := t.TempDir()

	domainDir := filepath.Join(rulesetsDir, "income_verification")
	os.MkdirAll(domainDir, 0755)
	rs := `id: income_verification
version: "3"
checks:
  - id: "gross_income_consistency"
    version: "1.0"
`
	os.WriteFile(filepath.Join(domainDir, "v3.yaml"), []byte(rs), 0644)

	binary := filepath.Join(t.TempDir(), "verify-ruleset")
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Dir = filepath.Join(findModuleRoot(t), "cmd", "verify-ruleset")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\noutput: %s", err, out)
	}

	cmd := exec.Command(binary, "--domain", "income_verification", "--rulesets-dir", rulesetsDir, "--expect-version", "5")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("verify-ruleset should fail with wrong version")
	}
	if !strings.Contains(string(out), "version mismatch") {
		t.Errorf("expected 'version mismatch', got: %s", out)
	}
}

func TestVerifyRuleset_ExpectHash_Pass(t *testing.T) {
	rulesetsDir := t.TempDir()

	domainDir := filepath.Join(rulesetsDir, "income_verification")
	os.MkdirAll(domainDir, 0755)
	rs := `id: income_verification
version: "2"
checks:
  - id: "gross_income_consistency"
    version: "1.0"
`
	os.WriteFile(filepath.Join(domainDir, "v2.yaml"), []byte(rs), 0644)

	binary := filepath.Join(t.TempDir(), "verify-ruleset")
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Dir = filepath.Join(findModuleRoot(t), "cmd", "verify-ruleset")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\noutput: %s", err, out)
	}

	// First get the actual hash
	cmd := exec.Command(binary, "--domain", "income_verification", "--rulesets-dir", rulesetsDir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("verify-ruleset: %v\noutput: %s", err, out)
	}
	t.Logf("output: %s", out)

	// Compute hash via Go API directly
	registry := eval.NewRegistry(rulesetsDir)
	loaded, err := registry.LoadPromoted("income_verification")
	if err != nil {
		t.Fatalf("LoadPromoted: %v", err)
	}
	actualHash, err := loaded.ComputeHash()
	if err != nil {
		t.Fatalf("ComputeHash: %v", err)
	}

	// Verify with correct hash
	cmd = exec.Command(binary, "--domain", "income_verification", "--rulesets-dir", rulesetsDir, "--expect-hash", actualHash)
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("verify-ruleset should pass with correct hash: %v\noutput: %s", err, out)
	}
	if !strings.Contains(string(out), "Hash match: PASS") {
		t.Errorf("expected 'Hash match: PASS', got: %s", out)
	}
}

func TestVerifyRuleset_ExpectHash_Fail(t *testing.T) {
	rulesetsDir := t.TempDir()

	domainDir := filepath.Join(rulesetsDir, "income_verification")
	os.MkdirAll(domainDir, 0755)
	rs := `id: income_verification
version: "2"
checks:
  - id: "gross_income_consistency"
    version: "1.0"
`
	os.WriteFile(filepath.Join(domainDir, "v2.yaml"), []byte(rs), 0644)

	binary := filepath.Join(t.TempDir(), "verify-ruleset")
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Dir = filepath.Join(findModuleRoot(t), "cmd", "verify-ruleset")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\noutput: %s", err, out)
	}

	cmd := exec.Command(binary, "--domain", "income_verification", "--rulesets-dir", rulesetsDir, "--expect-hash", "0000000000000000")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("verify-ruleset should fail with wrong hash")
	}
	if !strings.Contains(string(out), "hash mismatch") {
		t.Errorf("expected 'hash mismatch', got: %s", out)
	}
}

func findModuleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("cannot find module root")
		}
		dir = parent
	}
}
