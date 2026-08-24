package eval

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"
)

// TestRunAllShipmentScenarios runs the shipment_release scenario corpus
// against the shipment checks (mirrors TestRunAllScenarios).
func TestRunAllShipmentScenarios(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	projectRoot := filepath.Dir(filepath.Dir(filepath.Dir(thisFile)))
	scenariosDir := filepath.Join(projectRoot, "scenarios", "shipment_release")

	scenarios, err := LoadAllScenariosFromDir(scenariosDir)
	if err != nil {
		t.Fatalf("load scenarios: %v", err)
	}
	if len(scenarios) < 6 {
		t.Fatalf("expected at least 6 shipment scenarios, got %d", len(scenarios))
	}

	runner := NewRunner(DefaultRegistry().All())
	results := runner.RunAllScenarios(context.Background(), scenarios)

	passed, failed := 0, 0
	for _, r := range results {
		if r.Passed {
			passed++
			continue
		}
		failed++
		t.Errorf("FAIL: %s — StatusChanged=%v ReasonChanged=%v EvidenceChanged=%v",
			r.ScenarioName,
			r.Diff != nil && r.Diff.StatusChanged,
			r.Diff != nil && r.Diff.ReasonChanged,
			r.Diff != nil && r.Diff.EvidenceChanged)
		t.Logf("  actual:   status=%s severity=%s reason=%q", r.Actual.Status, r.Actual.Severity, r.Actual.Reason)
		t.Logf("  expected: status=%s severity=%s reason=%q", r.Expected.Status, r.Expected.Severity, r.Expected.Reason)
	}

	if passed == len(scenarios) {
		t.Logf("all %d shipment scenarios pass", passed)
	} else {
		t.Fatalf("results: %d passed, %d failed out of %d total", passed, failed, len(scenarios))
	}
}
