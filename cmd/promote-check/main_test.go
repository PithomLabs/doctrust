package main

// adv_review4 P1-A: promote-check must exit cleanly (code 1, explanatory
// message) when ValidateSnapshot rejects early with a nil result — never
// SIGSEGV. Drives the REAL binary through the exact production path that
// crashed on duplicate Check-ID retries.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PithomLabs/doctrust/internal/compiler"
)

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

func buildPromoteCheck(t *testing.T) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "promote-check")
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Dir = filepath.Join(findModuleRoot(t), "cmd", "promote-check")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\noutput: %s", err, out)
	}
	return binary
}

// TestDuplicateCheckID_ExitsCleanly reproduces the pre-fix crash scenario:
// re-running promote-check on a candidate whose check_id already exists in the
// domain ruleset. Pre-fix this panicked with SIGSEGV at the valResult.Errors
// dereference; post-fix it must print a clean validation error and exit 1.
func TestDuplicateCheckID_ExitsCleanly(t *testing.T) {
	binary := buildPromoteCheck(t)

	rulesetsDir := t.TempDir()
	candidateDir := t.TempDir()

	// Promoted ruleset ALREADY contains the candidate's check ID — the
	// uniqueness gate inside ValidateSnapshot fires before any compilation,
	// so no module-graph work is needed for this test.
	domainDir := filepath.Join(rulesetsDir, "income_verification")
	if err := os.MkdirAll(domainDir, 0755); err != nil {
		t.Fatalf("mkdir domain: %v", err)
	}
	promoted := `id: income_verification
version: "1"
checks:
  - id: gross_income_consistency
    version: "1.0"
  - id: duplicate_probe_check
    version: "1.0"
`
	if err := os.WriteFile(filepath.Join(domainDir, "v1.yaml"), []byte(promoted), 0644); err != nil {
		t.Fatalf("write v1.yaml: %v", err)
	}

	// Fully conforming candidate so the run passes Gates 1-3 and reaches Gate 4.
	files := map[string]string{
		"check.go": `package candidate

import (
	"strings"

	"github.com/PithomLabs/doctrust/internal/eval"
)

type DuplicateProbeCheck struct{}

func (c *DuplicateProbeCheck) ID() string      { return "duplicate_probe_check" }
func (c *DuplicateProbeCheck) Version() string { return "1.0" }

func (c *DuplicateProbeCheck) Evaluate(facts eval.Facts, params map[string]any) eval.Result {
	_ = strings.TrimSpace("x")
	return eval.Result{Status: eval.StatusPass}
}
`,
		"metadata.yaml":  "id: duplicate_probe_check\nversion: \"1.0\"\ndescription: duplicate-id retry probe\n",
		"scenarios.yaml": "scenarios: []\n",
		"adversarial.yaml": `scenarios:
  - name: adversarial_edge
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
      check_id: duplicate_probe_check
      status: PASS
`,
		"state": "APPROVED",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(candidateDir, name), []byte(content), 0644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	if err := compiler.WriteApproval(candidateDir, "duplicate_probe_check", "1.0", "test_user"); err != nil {
		t.Fatalf("WriteApproval: %v", err)
	}

	cmd := exec.Command(binary,
		"--candidate", candidateDir,
		"--domain", "income_verification",
		"--rulesets-dir", rulesetsDir,
	)
	cmd.Stdin = strings.NewReader("")
	out, err := cmd.CombinedOutput()
	output := string(out)

	// Exit code must be a clean failure (1), not a signal death (pre-fix: 2).
	if err == nil {
		t.Fatalf("duplicate Check-ID retry must fail; output:\n%s", output)
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("expected ExitError, got %T: %v\noutput:\n%s", err, err, output)
	}
	if exitErr.ExitCode() != 1 {
		t.Errorf("exit code = %d, want 1; output:\n%s", exitErr.ExitCode(), output)
	}

	// Clean explanatory message, not a stack trace.
	if !strings.Contains(output, "Validation failed") {
		t.Errorf("missing 'Validation failed'; output:\n%s", output)
	}
	if !strings.Contains(output, "already exists in ruleset") {
		t.Errorf("missing 'already exists in ruleset'; output:\n%s", output)
	}
	for _, panicMarker := range []string{"panic:", "runtime error", "goroutine", "SIGSEGV"} {
		if strings.Contains(output, panicMarker) {
			t.Errorf("crash marker %q in output:\n%s", panicMarker, output)
		}
	}
}
