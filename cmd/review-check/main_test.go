package main

// P1-B trust property: approval MUST NOT be written without an explicit
// adversarial y/Y confirmation. Drives the real binary over stdin.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func findRepoRoot(t *testing.T) string {
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

const probeSource = `package candidate

import "github.com/PithomLabs/doctrust/internal/eval"

type ReviewProbeCheck struct{}

func (c *ReviewProbeCheck) ID() string      { return "review_probe_check" }
func (c *ReviewProbeCheck) Version() string { return "1.0" }

func (c *ReviewProbeCheck) Evaluate(facts eval.Facts, params map[string]any) eval.Result {
	return eval.Result{Status: eval.StatusPass}
}
`

const probeScenario = `scenarios:
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
      check_id: review_probe_check
      status: PASS
`

const probeAdversarial = `scenarios:
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
      check_id: review_probe_check
      status: PASS
`

// setupInteractiveCandidate writes a fully valid DRAFT candidate whose single
// scenario passes pre-approval execution.
func setupInteractiveCandidate(t *testing.T) string {
	t.Helper()
	candidateDir := t.TempDir()

	files := map[string]string{
		"check.go":         probeSource,
		"metadata.yaml":    "id: review_probe_check\nversion: \"1.0\"\ndescription: review-check interactive probe\n",
		"scenarios.yaml":   probeScenario,
		"adversarial.yaml": probeAdversarial,
		"state":            "DRAFT",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(candidateDir, name), []byte(content), 0644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return candidateDir
}

func buildReviewCheck(t *testing.T) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "review-check")
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Dir = filepath.Join(findRepoRoot(t), "cmd", "review-check")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\noutput: %s", err, out)
	}
	return binary
}

func readState(t *testing.T, candidateDir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(candidateDir, "state"))
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	return strings.TrimSpace(string(data))
}

func approvalExists(t *testing.T, candidateDir string) bool {
	t.Helper()
	_, err := os.Stat(filepath.Join(candidateDir, "approval.json"))
	return err == nil
}

func TestApproval_RequiresExplicitAdversarialConfirmation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping interactive subprocess test in short mode")
	}
	binary := buildReviewCheck(t)

	cases := []struct {
		name       string
		stdin      string
		wantApprov bool
	}{
		{"y_confirms", "a\ny\n", true},
		{"Y_confirms", "a\nY\n", true},
		{"n_cancels", "a\nn\n", false},
		{"empty_cancels", "a\n\n", false},
		{"garbage_cancels", "a\nyes!\n", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			candidateDir := setupInteractiveCandidate(t)

			cmd := exec.Command(binary, candidateDir)
			cmd.Stdin = strings.NewReader(tc.stdin)
			out, err := cmd.CombinedOutput()
			if err != nil && !tc.wantApprov {
				t.Logf("non-zero exit acceptable on cancel path: %v", err)
			}

			gotApprov := approvalExists(t, candidateDir)
			state := readState(t, candidateDir)

			if tc.wantApprov {
				if !gotApprov {
					t.Fatalf("expected approval.json after y-confirmation; output:\n%s", out)
				}
				if state != "APPROVED" {
					t.Errorf("state = %s, want APPROVED", state)
				}
				data, _ := os.ReadFile(filepath.Join(candidateDir, "approval.json"))
				if !strings.Contains(string(data), "review_probe_check") {
					t.Error("approval.json not bound to candidate check identity")
				}
			} else {
				if gotApprov {
					t.Fatalf("approval.json written WITHOUT explicit confirmation; output:\n%s", out)
				}
				if state == "APPROVED" {
					t.Errorf("state advanced to APPROVED without confirmation; output:\n%s", out)
				}
			}
		})
	}
}

func TestApproval_DOCTRUST_REVIEWEROverride(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping interactive subprocess test in short mode")
	}
	binary := buildReviewCheck(t)
	candidateDir := setupInteractiveCandidate(t)

	cmd := exec.Command(binary, candidateDir)
	cmd.Stdin = strings.NewReader("a\ny\n")
	cmd.Env = append(os.Environ(), "DOCTRUST_REVIEWER=demo_reviewer")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("review-check failed: %v\noutput: %s", err, out)
	}

	if !approvalExists(t, candidateDir) {
		t.Fatal("expected approval.json")
	}
	data, _ := os.ReadFile(filepath.Join(candidateDir, "approval.json"))
	if !strings.Contains(string(data), "demo_reviewer") {
		t.Errorf("DOCTRUST_REVIEWER identity missing from approval.json:\n%s", data)
	}
}
