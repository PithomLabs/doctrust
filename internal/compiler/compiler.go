package compiler

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/doctrust/doctrust/internal/nutrient"
	"gopkg.in/yaml.v3"
)

type Compiler struct {
	llmClient    LLMClient
	adapter      nutrient.NutrientAdapter
	maxAttempts  int
}

type CompileResult struct {
	Rego       string
	Tests      string
	Extraction *nutrient.ExtractionSchema
	Version    *PolicyVersion
	Attempts   int
	OutputDir  string
}

func NewCompiler(llmClient LLMClient) *Compiler {
	return &Compiler{
		llmClient:   llmClient,
		adapter:     &nutrient.DefaultNutrientAdapter{},
		maxAttempts: 3,
	}
}

func (c *Compiler) Compile(ctx context.Context, policyMDPath string) (*CompileResult, error) {
	policyDir := filepath.Dir(policyMDPath)
	policyName := filepath.Base(policyDir)

	policyMDBytes, err := os.ReadFile(policyMDPath)
	if err != nil {
		return nil, fmt.Errorf("read POLICY.md: %w", err)
	}

	ps, err := ParsePolicySource(policyMDPath)
	if err != nil {
		return nil, fmt.Errorf("parse POLICY.md: %w", err)
	}

	extractionSchema := BuildExtractionSchema(ps)
	prompt := BuildPrompt(ps)

	expectedCasesPath := filepath.Join(policyDir, "expected_cases.yaml")
	adversarialCasesPath := filepath.Join(policyDir, "adversarial_cases.yaml")
	referencePolicyPath := filepath.Join(policyDir, "policy.rego")

	outputDir := filepath.Join("compiled", policyName)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("create output dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(outputDir, "attempts"), 0755); err != nil {
		return nil, fmt.Errorf("create attempts dir: %w", err)
	}

	// Compute canonical fixture-set hash (immutable acceptance inputs)
	fixtureSetHash, err := computeFixtureSetHash(expectedCasesPath, adversarialCasesPath)
	if err != nil {
		return nil, fmt.Errorf("compute fixture hash: %w", err)
	}

	var lastErr error
	var lastRego, lastTests string

	for attempt := 1; attempt <= c.maxAttempts; attempt++ {
		fmt.Fprintf(os.Stderr, "Attempt %d/%d...\n", attempt, c.maxAttempts)

		rawOutput, err := c.llmClient.Generate(ctx, prompt)
		if err != nil {
			lastErr = fmt.Errorf("LLM error: %w", err)
			fmt.Fprintf(os.Stderr, "LLM error: %v\n", err)
			continue
		}

		attemptPath := filepath.Join(outputDir, "attempts", fmt.Sprintf("attempt_%d.rego", attempt))
		if err := os.WriteFile(attemptPath, []byte(rawOutput), 0644); err != nil {
			return nil, fmt.Errorf("write attempt file: %w", err)
		}

		compiled, err := ParseLLMOutput(rawOutput)
		if err != nil {
			lastErr = fmt.Errorf("parse output: %w", err)
			fmt.Fprintf(os.Stderr, "Parse error: %v\n", err)
			continue
		}

		regoPath := filepath.Join(outputDir, "policy.rego")
		testPath := filepath.Join(outputDir, "policy_test.rego")

		if err := os.WriteFile(regoPath, []byte(compiled.Rego), 0644); err != nil {
			return nil, fmt.Errorf("write policy.rego: %w", err)
		}
		if err := os.WriteFile(testPath, []byte(compiled.Tests), 0644); err != nil {
			return nil, fmt.Errorf("write policy_test.rego: %w", err)
		}

		if err := ValidateRego(ctx, regoPath); err != nil {
			lastErr = fmt.Errorf("opa check: %w", err)
			fmt.Fprintf(os.Stderr, "opa check failed: %v\n", err)
			prompt = enhancePromptWithCorrection(prompt, fmt.Sprintf("The generated Rego failed opa check:\n%v\n\nPlease fix the Rego syntax.", err))
			continue
		}

		if err := RunSupplementaryTests(ctx, regoPath, testPath); err != nil {
			lastErr = fmt.Errorf("opa test: %w", err)
			fmt.Fprintf(os.Stderr, "opa test failed: %v\n", err)
			prompt = enhancePromptWithCorrection(prompt, fmt.Sprintf("The generated supplementary tests failed:\n%v\n\nPlease fix the tests.", err))
			continue
		}

		// Merge primary + adversarial fixture cases (source files are immutable)
		mergedCasesPath := filepath.Join(outputDir, "merged_cases.yaml")
		if err := mergeFixtureCases(expectedCasesPath, adversarialCasesPath, mergedCasesPath); err != nil {
			lastErr = fmt.Errorf("merge fixtures: %w", err)
			fmt.Fprintf(os.Stderr, "Fixture merge failed: %v\n", err)
			continue
		}

		validation, err := ValidateFixtures(ctx, regoPath, mergedCasesPath, ".")
		if err != nil {
			lastErr = fmt.Errorf("fixture validation: %w", err)
			fmt.Fprintf(os.Stderr, "Fixture validation error: %v\n", err)
			os.Remove(mergedCasesPath)
			prompt = enhancePromptWithCorrection(prompt, fmt.Sprintf("Fixture validation failed:\n%v\n\nPlease fix the Rego.", err))
			continue
		}

		if validation.Failed > 0 {
			lastErr = fmt.Errorf("fixture validation: %d/%d failed", validation.Failed, validation.Passed+validation.Failed)
			fmt.Fprintf(os.Stderr, "Fixtures: %d/%d passed\n", validation.Passed, validation.Passed+validation.Failed)
			for _, m := range validation.Mismatches {
				fmt.Fprintf(os.Stderr, "  - %s: %s expected=%s got=%s\n", m.CaseName, m.Field, m.Expected, m.Actual)
			}
			os.Remove(mergedCasesPath)
			prompt = enhancePromptWithCorrection(prompt, fmt.Sprintf("Some fixtures did not produce the expected results:\n%v\n\nPlease fix the Rego to match the expected behavior.", formatMismatches(validation.Mismatches)))
			continue
		}

		fmt.Fprintf(os.Stderr, "Fixtures: %d/%d passed\n", validation.Passed, validation.Passed+validation.Failed)

		// Behavioral regression check — hard gate, not a warning
		regression, err := RunRegressionCheck(ctx, regoPath, referencePolicyPath, mergedCasesPath, ".")
		os.Remove(mergedCasesPath)
		if err != nil {
			lastErr = fmt.Errorf("regression check: %w", err)
			fmt.Fprintf(os.Stderr, "Regression check error: %v\n", err)
			prompt = enhancePromptWithCorrection(prompt, fmt.Sprintf("Regression check failed:\n%v\n\nPlease fix the Rego to match reference behavior.", err))
			continue
		}
		if !regression.Match {
			lastErr = fmt.Errorf("regression: %d mismatches", len(regression.Mismatches))
			fmt.Fprintf(os.Stderr, "Behavioral regression: %d mismatches\n", len(regression.Mismatches))
			for _, m := range regression.Mismatches {
				fmt.Fprintf(os.Stderr, "  - %s: %s expected=%s got=%s\n", m.CaseName, m.Field, m.Expected, m.Actual)
			}
			prompt = enhancePromptWithCorrection(prompt, fmt.Sprintf("Behavioral regression check failed. Your generated policy produces different results than the reference for these cases:\n%v\n\nPlease fix the Rego to match the reference behavior exactly.", formatMismatches(regression.Mismatches)))
			continue
		}

		fmt.Fprintf(os.Stderr, "Behavioral regression: all match\n")

		lastRego = compiled.Rego
		lastTests = compiled.Tests

		// Write extraction.json
		extractionBytes, err := json.MarshalIndent(extractionSchema, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("marshal extraction: %w", err)
		}
		if err := os.WriteFile(filepath.Join(outputDir, "extraction.json"), extractionBytes, 0644); err != nil {
			return nil, fmt.Errorf("write extraction.json: %w", err)
		}

		// Write version metadata
		policyMDHash := ComputeHash(policyMDBytes)
		promptHash := ComputeHash([]byte(prompt))
		regoHash := ComputeHash([]byte(compiled.Rego))
		extractionHash := ComputeHash(extractionBytes)

		version := NewPolicyVersion(
			policyMDHash, promptHash, regoHash, fixtureSetHash, extractionHash,
			getModelName(), attempt, true,
		)
		versionBytes, err := json.MarshalIndent(version, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("marshal version: %w", err)
		}
		if err := os.WriteFile(filepath.Join(outputDir, "policy_version.json"), versionBytes, 0644); err != nil {
			return nil, fmt.Errorf("write policy_version.json: %w", err)
		}

		return &CompileResult{
			Rego:       lastRego,
			Tests:      lastTests,
			Extraction: extractionSchema,
			Version:    version,
			Attempts:   attempt,
			OutputDir:  outputDir,
		}, nil
	}

	return nil, fmt.Errorf("COMPILATION_ERROR after %d attempts: %w", c.maxAttempts, lastErr)
}

// computeFixtureSetHash produces a canonical SHA-256 of the immutable acceptance inputs.
func computeFixtureSetHash(primaryPath, adversarialPath string) (string, error) {
	primaryBytes, err := os.ReadFile(primaryPath)
	if err != nil {
		return "", fmt.Errorf("read primary fixtures: %w", err)
	}
	adversarialBytes, err := os.ReadFile(adversarialPath)
	if err != nil {
		return "", fmt.Errorf("read adversarial fixtures: %w", err)
	}

	manifest := struct {
		Version           int    `json:"version"`
		ExpectedCasesSHA  string `json:"expected_cases_sha256"`
		AdversarialSHA    string `json:"adversarial_cases_sha256"`
	}{
		Version:          1,
		ExpectedCasesSHA: ComputeHash(primaryBytes),
		AdversarialSHA:   ComputeHash(adversarialBytes),
	}

	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		return "", fmt.Errorf("marshal fixture manifest: %w", err)
	}

	return ComputeHash(manifestBytes), nil
}

func enhancePromptWithCorrection(original, correction string) string {
	return original + "\n\n## Previous Attempt Failed\n\n" + correction + "\n\nPlease generate corrected Rego and tests."
}

func formatMismatches(mismatches []FixtureMismatch) string {
	var sb strings.Builder
	for _, m := range mismatches {
		sb.WriteString(fmt.Sprintf("- Case: %s, Field: %s, Expected: %s, Got: %s\n", m.CaseName, m.Field, m.Expected, m.Actual))
	}
	return sb.String()
}

func getModelName() string {
	model := os.Getenv("OPENROUTER_MODEL")
	if model == "" {
		model = "anthropic/claude-sonnet-4-20250514"
	}
	return model
}

func BuildExtractionSchema(ps *PolicySource) *nutrient.ExtractionSchema {
	schemas := make(map[string]nutrient.DocExtraction)
	for docType, fields := range ps.ExtractionSchemas {
		schemas[docType] = nutrient.DocExtraction{
			Fields:           convertFieldSpecs(fields),
			SemanticMappings: filterMappings(ps.SemanticMappings, docType),
		}
	}
	return &nutrient.ExtractionSchema{
		PolicyID: ps.Name,
		Schemas:  schemas,
	}
}

func convertFieldSpecs(specs []FieldSpec) []nutrient.FieldSpec {
	result := make([]nutrient.FieldSpec, len(specs))
	for i, s := range specs {
		result[i] = nutrient.FieldSpec{Name: s.Name, Type: s.Type}
	}
	return result
}

func filterMappings(mappings []SemanticMapping, docType string) []nutrient.SemanticMapping {
	var result []nutrient.SemanticMapping
	for _, m := range mappings {
		if m.DocType == docType {
			result = append(result, nutrient.SemanticMapping{
				Field:        m.Field,
				SemanticType: m.SemanticType,
			})
		}
	}
	return result
}

func mergeFixtureCases(primary, adversarial, output string) error {
	primaryData, err := os.ReadFile(primary)
	if err != nil {
		return err
	}

	adversarialData, err := os.ReadFile(adversarial)
	if err != nil {
		return err
	}

	var primaryCases, adversarialCases ExpectedCases
	if err := yaml.Unmarshal(primaryData, &primaryCases); err != nil {
		return err
	}
	if err := yaml.Unmarshal(adversarialData, &adversarialCases); err != nil {
		return err
	}

	merged := ExpectedCases{
		Cases: append(primaryCases.Cases, adversarialCases.Cases...),
	}

	outBytes, err := yaml.Marshal(merged)
	if err != nil {
		return err
	}

	return os.WriteFile(output, outBytes, 0644)
}
