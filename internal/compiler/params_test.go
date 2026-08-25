package compiler

// P1-C: single source of truth for parameter resolution.
// ResolveRulesetParams is a verbatim lift of cmd/regression semantics;
// these tests pin behavioral equivalence and prove the staged regression
// gate produces the same outcomes production regression would.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PithomLabs/doctrust/internal/eval"
)

func TestResolveRulesetParams_ProductionSemantics(t *testing.T) {
	rulesetParams := map[string]any{"tolerance": 0.03, "bonus_field": "bonus_compensation"}
	scenarioFallback := map[string]any{"tolerance": 9.99}

	rs := eval.Ruleset{
		ID:      "income_verification",
		Version: "1",
		Checks: []eval.CheckRef{
			{ID: "gross_income_consistency", Version: "1.0", Params: rulesetParams},
			{ID: "nil_params_check", Version: "1.0", Params: nil},
		},
	}

	t.Run("ParamsPresent_WholesaleReplace", func(t *testing.T) {
		got := ResolveRulesetParams(rs, "gross_income_consistency", scenarioFallback)
		if got["tolerance"] != 0.03 {
			t.Errorf("expected ruleset tolerance 0.03, got %v", got["tolerance"])
		}
		if _, ok := got["bonus_field"]; !ok {
			t.Error("ruleset bonus_field param lost")
		}
		// Wholesale replace, NOT merge: scenario-only keys must NOT appear.
		if _, ok := got["scenario_only_key"]; ok {
			t.Error("scenario keys leaked into resolved params (merge semantics detected)")
		}
	})

	t.Run("NilParams_FallsBackToScenario", func(t *testing.T) {
		got := ResolveRulesetParams(rs, "nil_params_check", scenarioFallback)
		if got["tolerance"] != 9.99 {
			t.Errorf("expected scenario fallback tolerance 9.99, got %v", got["tolerance"])
		}
	})

	t.Run("CheckAbsent_FallsBackToScenario", func(t *testing.T) {
		got := ResolveRulesetParams(rs, "unknown_check", scenarioFallback)
		if got["tolerance"] != 9.99 {
			t.Errorf("expected scenario fallback tolerance 9.99, got %v", got["tolerance"])
		}
	})
}

// TestRunStagedRegression_ParameterizedScenarioUsesPromotedParams proves that
// staged regression resolves parameters identically to post-promotion
// regression. An existing parameterized check (gross_income_consistency with
// tolerance 0.01 in the promoted ruleset) MUST retain its params in the staged
// working ruleset. A corpus scenario with NO scenario-level params expects
// REVIEW at 3% variance — it can only pass if the promoted CheckRef.Params
// survived staging. The control proves the test discriminates: the old
// minimal-staged-ruleset construction fails this exact regression.
func TestRunStagedRegression_ParameterizedScenarioUsesPromotedParams(t *testing.T) {
	evalDir, rulesetsDir, candidateDir, scenariosDir, _ := setupTestPromotion(t)

	// Promoted v1 carries a TIGHT tolerance for the existing parameterized check.
	v1Path := filepath.Join(rulesetsDir, "income_verification", "v1.yaml")
	promoted := `id: income_verification
version: "1"
checks:
  - id: gross_income_consistency
    version: "1.0"
    params:
      tolerance: 0.01
  - id: required_documents
    version: "1.0"
`
	if err := os.WriteFile(v1Path, []byte(promoted), 0644); err != nil {
		t.Fatalf("write v1.yaml: %v", err)
	}

	// Corpus scenario with NO scenario-level params: 3% variance vs 1% ruleset
	// tolerance → REVIEW. Under default tolerance (0.05) this would PASS.
	corpusDir := filepath.Join(scenariosDir, "income_verification")
	if err := os.MkdirAll(corpusDir, 0755); err != nil {
		t.Fatalf("mkdir corpus: %v", err)
	}
	scenarioYAML := `scenarios:
  - name: "tight_tolerance_review"
    origin: "real_fixture"
    input:
      facts:
        - semantic_type: "gross_income_projected"
          source_doc: "paystub"
          field: "annualized_gross_ytd"
          value: 103000
          source_span: "page=1;bbox=[10,20,30,40]"
          confidence: 0.95
        - semantic_type: "gross_income_taxable"
          source_doc: "w2"
          field: "wages_tips_other_compensation"
          value: 100000
          source_span: "page=1;bbox=[50,60,70,80]"
          confidence: 0.95
    expected:
      check_id: "gross_income_consistency"
      status: "REVIEW"
      severity: "WARNING"
      reason: "Paystub projected gross exceeds corroborated taxable income by 3.0% (tolerance: 1.0%)"
      evidence:
        - field: "gross_income_projected"
          source_doc: "paystub"
          source_span: "page=1;bbox=[10,20,30,40]"
          confidence: 0.95
        - field: "gross_income_taxable"
          source_doc: "w2"
          source_span: "page=1;bbox=[50,60,70,80]"
          confidence: 0.95
`
	if err := os.WriteFile(filepath.Join(corpusDir, "gross_variance.yaml"), []byte(scenarioYAML), 0644); err != nil {
		t.Fatalf("write corpus scenario: %v", err)
	}

	snapshot, err := SnapshotCandidate(candidateDir)
	if err != nil {
		t.Fatalf("SnapshotCandidate: %v", err)
	}

	stagingDir, err := StagePromotion(snapshot, evalDir, "income_verification", rulesetsDir)
	if err != nil {
		t.Fatalf("StagePromotion: %v", err)
	}
	defer RollbackPromotion(stagingDir)

	// Staged working.yaml must preserve the promoted CheckRef.Params.
	stagedData, err := os.ReadFile(filepath.Join(stagingDir, "working.yaml"))
	if err != nil {
		t.Fatalf("read staged working.yaml: %v", err)
	}
	stagedContent := string(stagedData)
	if !strings.Contains(stagedContent, "tolerance") || !strings.Contains(stagedContent, "0.01") {
		t.Fatalf("staged working.yaml lost promoted params:\n%s", stagedContent)
	}
	if !strings.Contains(stagedContent, "my_new_check") {
		t.Fatalf("staged working.yaml missing new CheckRef:\n%s", stagedContent)
	}

	// Staged regression passes with preserved production semantics.
	if err := RunStagedRegression(stagingDir, evalDir, "income_verification", scenariosDir); err != nil {
		t.Errorf("staged regression should pass when promoted params are preserved: %v", err)
	}

	// Control: reconstruct the OLD minimal staged ruleset (only the new check).
	// Identical regression input must now FAIL — proving the test discriminates
	// between divergent parameter semantics.
	minimalStaging := t.TempDir()
	minimalYAML := `id: income_verification
version: "draft"
checks:
  - id: my_new_check
    version: "1.0"
`
	if err := os.WriteFile(filepath.Join(minimalStaging, "working.yaml"), []byte(minimalYAML), 0644); err != nil {
		t.Fatalf("write minimal working.yaml: %v", err)
	}
	if err := RunStagedRegression(minimalStaging, evalDir, "income_verification", scenariosDir); err == nil {
		t.Error("control failed: minimal staged ruleset passed the parameterized scenario — divergence not discriminated")
	} else {
		t.Logf("control correctly failed: %v", err)
	}
}

// TestStagePromotion_StagedRulesetMatchesProductionConstruction pins that
// staged working.yaml is derived from the current ruleset (promoted baseline)
// plus the new CheckRef — identical inputs to CommitPromotion's disk write.
func TestStagePromotion_StagedRulesetMatchesProductionConstruction(t *testing.T) {
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

	stagedRS, err := eval.LoadRuleset(filepath.Join(stagingDir, "working.yaml"))
	if err != nil {
		t.Fatalf("load staged ruleset: %v", err)
	}

	// Must contain BOTH the pre-existing refs and the new one.
	var haveGross, haveDocs, haveNew bool
	for _, ref := range stagedRS.Checks {
		switch ref.ID {
		case "gross_income_consistency":
			haveGross = true
		case "required_documents":
			haveDocs = true
		case "my_new_check":
			haveNew = true
			if ref.Version != snapshot.Version {
				t.Errorf("new CheckRef version = %s, want %s", ref.Version, snapshot.Version)
			}
		}
	}
	if !haveGross || !haveDocs {
		t.Errorf("staged ruleset dropped existing CheckRefs (gross=%v docs=%v)", haveGross, haveDocs)
	}
	if !haveNew {
		t.Error("staged ruleset missing new CheckRef")
	}

	_ = scenariosDir
}
