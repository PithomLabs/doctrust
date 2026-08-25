package compiler

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/PithomLabs/doctrust/internal/nutrient"
)

const testPolicyMD = `# Test Policy

## Required Evidence

- paystub
- w2

## Extraction Schema

- paystub: annualized_gross_ytd
- w2: wages_tips_other_compensation

## Semantic Classification

- paystub.annualized_gross_ytd → gross_income_projected
- w2.wages_tips_other_compensation → gross_income_taxable

## Rules

- paystub.annualized_gross_ytd variance over 5% requires human review
- confidence below 0.8 requires human review

## Decisions

- PASS when all required evidence is present and no review/violation exists
- REVIEW when evidence is ambiguous or requires human verification
- MISSING_EVIDENCE when required evidence is absent
`

func writeTestPolicy(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "POLICY.md")
	if err := os.WriteFile(path, []byte(testPolicyMD), 0644); err != nil {
		t.Fatalf("write test policy: %v", err)
	}
	return path
}

func TestParsePolicySource(t *testing.T) {
	path := writeTestPolicy(t)
	ps, err := ParsePolicySource(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if ps.Name != "Test Policy" {
		t.Errorf("name: expected 'Test Policy', got %q", ps.Name)
	}

	if len(ps.RequiredEvidence) != 2 {
		t.Errorf("required evidence: expected 2, got %d", len(ps.RequiredEvidence))
	}

	if ps.RequiredEvidence[0] != "paystub" || ps.RequiredEvidence[1] != "w2" {
		t.Errorf("required evidence: %v", ps.RequiredEvidence)
	}

	if len(ps.ExtractionSchemas) != 2 {
		t.Errorf("extraction schemas: expected 2 doc types, got %d", len(ps.ExtractionSchemas))
	}

	if len(ps.SemanticMappings) != 2 {
		t.Errorf("semantic mappings: expected 2, got %d", len(ps.SemanticMappings))
	}

	if len(ps.Rules) != 2 {
		t.Errorf("rules: expected 2, got %d", len(ps.Rules))
	}

	if ps.Rules[0].Type != "variance" {
		t.Errorf("rule[0] type: expected 'variance', got %q", ps.Rules[0].Type)
	}

	if ps.Rules[0].Threshold != 5.0 {
		t.Errorf("rule[0] threshold: expected 5.0, got %f", ps.Rules[0].Threshold)
	}
}

func TestParsePolicySource_MissingRequiredSection(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "POLICY.md")
	os.WriteFile(path, []byte("# Policy\n\n## Rules\n\n- confidence below 0.8 requires human review\n"), 0644)

	_, err := ParsePolicySource(path)
	if err == nil {
		t.Error("expected error for missing Required Evidence section")
	}
}

func TestParsePolicySource_UnknownDocType(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "POLICY.md")
	os.WriteFile(path, []byte("# Policy\n\n## Required Evidence\n\n- unknown_type\n"), 0644)

	_, err := ParsePolicySource(path)
	if err == nil {
		t.Error("expected error for unknown document type")
	}
}

func TestBuildPrompt(t *testing.T) {
	path := writeTestPolicy(t)
	ps, err := ParsePolicySource(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	prompt := BuildPrompt(ps)

	if prompt == "" {
		t.Error("prompt is empty")
	}

	if !contains(prompt, "gross_income_projected") {
		t.Error("prompt missing semantic type")
	}

	if !contains(prompt, "variance over 5%") {
		t.Error("prompt missing variance rule")
	}

	if !contains(prompt, "Acceptance Constraints") {
		t.Error("prompt missing acceptance constraints")
	}

	if !contains(prompt, "never branch on fixture names") {
		t.Error("prompt missing anti-memorization instruction")
	}
}

func TestParseOutput_ValidJSON(t *testing.T) {
	raw := `{"policy_rego": "package test", "policy_test_rego": "package test"}`
	cp, err := ParseLLMOutput(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if cp.Rego != "package test" {
		t.Errorf("rego: expected 'package test', got %q", cp.Rego)
	}
}

func TestParseOutput_CodeFences(t *testing.T) {
	raw := "```json\n{\"policy_rego\": \"package test\", \"policy_test_rego\": \"package test\"}\n```"
	cp, err := ParseLLMOutput(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if cp.Rego != "package test" {
		t.Errorf("rego: expected 'package test', got %q", cp.Rego)
	}
}

func TestParseOutput_MissingRego(t *testing.T) {
	raw := `{"policy_test_rego": "package test"}`
	_, err := ParseLLMOutput(raw)
	if err == nil {
		t.Error("expected error for missing policy_rego")
	}
}

func TestParseOutput_MissingTests(t *testing.T) {
	raw := `{"policy_rego": "package test"}`
	_, err := ParseLLMOutput(raw)
	if err == nil {
		t.Error("expected error for missing policy_test_rego")
	}
}

func TestValidateRego_Syntax(t *testing.T) {
	// Find opa binary
	opaPath := findOpa(t)
	if opaPath == "" {
		t.Skip("opa not found in PATH or ~/bin")
	}

	dir := t.TempDir()
	regoPath := filepath.Join(dir, "policy.rego")
	os.WriteFile(regoPath, []byte("package test\nimport rego.v1\ndefault result := true"), 0644)

	err := ValidateRego(context.Background(), regoPath)
	if err != nil {
		t.Errorf("expected valid rego to pass: %v", err)
	}
}

func TestValidateRego_Invalid(t *testing.T) {
	dir := t.TempDir()
	regoPath := filepath.Join(dir, "policy.rego")
	os.WriteFile(regoPath, []byte("package test\nthis is not rego"), 0644)

	err := ValidateRego(context.Background(), regoPath)
	if err == nil {
		t.Error("expected error for invalid rego")
	}
}

func findOpa(t *testing.T) string {
	t.Helper()
	// Check standard PATH
	if path, err := filepath.Abs("opa"); err == nil {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	// Check ~/bin
	home, _ := os.UserHomeDir()
	opaInHome := filepath.Join(home, "bin", "opa")
	if _, err := os.Stat(opaInHome); err == nil {
		return opaInHome
	}
	return ""
}

func TestExtractSchemaMatchesPolicySource(t *testing.T) {
	path := writeTestPolicy(t)
	ps, err := ParsePolicySource(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	schema := BuildExtractionSchema(ps)

	if schema.PolicyID != "Test Policy" {
		t.Errorf("policy_id: expected 'Test Policy', got %q", schema.PolicyID)
	}

	if len(schema.Schemas) != 2 {
		t.Errorf("schemas: expected 2, got %d", len(schema.Schemas))
	}

	paystub, ok := schema.Schemas["paystub"]
	if !ok {
		t.Fatal("missing paystub schema")
	}

	if len(paystub.Fields) != 1 {
		t.Errorf("paystub fields: expected 1, got %d", len(paystub.Fields))
	}

	if paystub.Fields[0].Name != "annualized_gross_ytd" {
		t.Errorf("paystub field: expected 'annualized_gross_ytd', got %q", paystub.Fields[0].Name)
	}
}

func TestNutrientAdapter(t *testing.T) {
	path := writeTestPolicy(t)
	ps, err := ParsePolicySource(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	schema := BuildExtractionSchema(ps)
	adapter := &nutrient.DefaultNutrientAdapter{}
	nutrientSchema := adapter.ToExtractFieldsSchema(schema)

	if len(nutrientSchema) != 2 {
		t.Errorf("nutrient schema: expected 2 doc types, got %d", len(nutrientSchema))
	}

	paystub, ok := nutrientSchema["paystub"]
	if !ok {
		t.Fatal("missing paystub in nutrient schema")
	}

	props, ok := paystub["properties"].(map[string]any)
	if !ok {
		t.Fatal("missing properties in paystub schema")
	}

	if _, ok := props["annualized_gross_ytd"]; !ok {
		t.Error("missing annualized_gross_ytd in paystub properties")
	}
}

func TestVersionMetadata(t *testing.T) {
	v := NewPolicyVersion("hash1", "hash2", "hash3", "hash4", "hash5", "test-model", 1, true)

	if v.PolicyMDHash != "hash1" {
		t.Errorf("policy_md_hash: expected 'hash1', got %q", v.PolicyMDHash)
	}

	if v.AttemptCount != 1 {
		t.Errorf("attempt_count: expected 1, got %d", v.AttemptCount)
	}

	if !v.ValidationPassed {
		t.Error("validation_passed: expected true")
	}
}

func TestExtractAdversarialFixtures(t *testing.T) {
	fixtures := []string{
		"../../fixtures/compiler_adversarial/threshold_reinterpretation.json",
		"../../fixtures/compiler_adversarial/evidence_ordering.json",
		"../../fixtures/compiler_adversarial/confidence_boundary.json",
	}

	for _, f := range fixtures {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Errorf("read %s: %v", f, err)
			continue
		}
		if len(data) == 0 {
			t.Errorf("empty file: %s", f)
		}
	}
}

func TestCheatingRego_CaseID(t *testing.T) {
	opaPath := findOpa(t)
	if opaPath == "" {
		t.Skip("opa not found")
	}

	dir := t.TempDir()
	cheatRego := `package doctrust.policy
import rego.v1

default result := {"decision": "FAIL", "findings": []}

result := {"decision": "PASS", "findings": []} if {
	input.case_id == "test_pass"
}

result := {"decision": "REVIEW", "findings": [{"rule": "cheat", "severity": "low"}]} if {
	input.case_id == "test_conflict"
}
`
	regoPath := filepath.Join(dir, "policy.rego")
	os.WriteFile(regoPath, []byte(cheatRego), 0644)

	if err := ValidateRego(context.Background(), regoPath); err != nil {
		t.Fatalf("cheating rego should pass opa check: %v", err)
	}

	// Run against a fixture — the case-ID cheat should produce wrong results
	// for fixtures that don't match the hardcoded case IDs
	fixturePath := "../../fixtures/pass_bundle.json"
	validation, err := ValidateFixtures(context.Background(), regoPath,
		"../../policies/income_verification/expected_cases.yaml", "../..")
	if err != nil {
		t.Fatalf("validation error: %v", err)
	}

	if validation.Failed == 0 {
		t.Error("expected cheating case-ID rego to fail fixture validation")
	}

	_ = fixturePath
}

func TestCheatingRego_HardcodedValue(t *testing.T) {
	opaPath := findOpa(t)
	if opaPath == "" {
		t.Skip("opa not found")
	}

	dir := t.TempDir()
	cheatRego := `package doctrust.policy
import rego.v1

default result := {"decision": "FAIL", "findings": []}

result := {"decision": "REVIEW", "findings": [{"rule": "paystub_variance", "severity": "medium"}]} if {
	some c in input.claims
	c.value == 138000
}

result := {"decision": "PASS", "findings": []} if {
	some c in input.claims
	c.value == 120000
}
`
	regoPath := filepath.Join(dir, "policy.rego")
	os.WriteFile(regoPath, []byte(cheatRego), 0644)

	if err := ValidateRego(context.Background(), regoPath); err != nil {
		t.Fatalf("cheating rego should pass opa check: %v", err)
	}

	validation, err := ValidateFixtures(context.Background(), regoPath,
		"../../policies/income_verification/expected_cases.yaml", "../..")
	if err != nil {
		t.Fatalf("validation error: %v", err)
	}

	if validation.Failed == 0 {
		t.Error("expected cheating hardcoded-value rego to fail fixture validation")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
