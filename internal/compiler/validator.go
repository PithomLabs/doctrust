package compiler

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

type ExpectedFinding struct {
	Rule     string `yaml:"rule"`
	Severity string `yaml:"severity"`
}

type ExpectedCase struct {
	Name             string            `yaml:"name"`
	Description      string            `yaml:"description"`
	InputFile        string            `yaml:"input_file"`
	ExpectedDecision string            `yaml:"expected_decision"`
	ExpectedFindings []ExpectedFinding `yaml:"expected_findings"`
}

type ExpectedCases struct {
	Cases []ExpectedCase `yaml:"cases"`
}

type ValidationResult struct {
	Passed    int
	Failed    int
	Mismatches []FixtureMismatch
}

type FixtureMismatch struct {
	CaseName string
	Field    string
	Expected string
	Actual   string
}

type RegressionResult struct {
	Match     bool
	Mismatches []FixtureMismatch
}

func ValidateRego(ctx context.Context, regoPath string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "opa", "check", regoPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("opa check failed: %w\n%s", err, string(output))
	}
	return nil
}

func RunSupplementaryTests(ctx context.Context, regoPath, testPath string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "opa", "test", regoPath, testPath, "-v")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("opa test failed: %w\n%s", err, string(output))
	}
	return nil
}

func ValidateFixtures(ctx context.Context, policyPath, expectedCasesPath, fixtureRoot string) (*ValidationResult, error) {
	data, err := os.ReadFile(expectedCasesPath)
	if err != nil {
		return nil, fmt.Errorf("read expected cases: %w", err)
	}

	var expected ExpectedCases
	if err := yaml.Unmarshal(data, &expected); err != nil {
		return nil, fmt.Errorf("parse YAML: %w", err)
	}

	result := &ValidationResult{}

	for _, c := range expected.Cases {
		inputPath := filepath.Join(fixtureRoot, c.InputFile)

		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		cmd := exec.CommandContext(ctx, "opa", "eval",
			"-d", policyPath,
			"-i", inputPath,
			"data.doctrust.policy.result")
		out, err := cmd.Output()
		cancel()

		if err != nil {
			result.Failed++
			result.Mismatches = append(result.Mismatches, FixtureMismatch{
				CaseName: c.Name,
				Field:    "opa_error",
				Expected: "success",
				Actual:   err.Error(),
			})
			continue
		}

		var opaOut struct {
			Result []struct {
				Expressions []struct {
					Value struct {
						Decision string `json:"decision"`
						Findings []struct {
							Rule     string `json:"rule"`
							Severity string `json:"severity"`
						} `json:"findings"`
					} `json:"value"`
				} `json:"expressions"`
			} `json:"result"`
		}

		if err := json.Unmarshal(out, &opaOut); err != nil {
			result.Failed++
			continue
		}

		if len(opaOut.Result) == 0 || len(opaOut.Result[0].Expressions) == 0 {
			result.Failed++
			continue
		}

		actual := opaOut.Result[0].Expressions[0].Value

		if actual.Decision != c.ExpectedDecision {
			result.Failed++
			result.Mismatches = append(result.Mismatches, FixtureMismatch{
				CaseName: c.Name,
				Field:    "decision",
				Expected: c.ExpectedDecision,
				Actual:   actual.Decision,
			})
			continue
		}

		if len(actual.Findings) != len(c.ExpectedFindings) {
			result.Failed++
			result.Mismatches = append(result.Mismatches, FixtureMismatch{
				CaseName: c.Name,
				Field:    "findings_count",
				Expected: fmt.Sprintf("%d", len(c.ExpectedFindings)),
				Actual:   fmt.Sprintf("%d", len(actual.Findings)),
			})
			continue
		}

		findingMatch := true
		for i, ef := range c.ExpectedFindings {
			if actual.Findings[i].Rule != ef.Rule {
				result.Mismatches = append(result.Mismatches, FixtureMismatch{
					CaseName: c.Name,
					Field:    fmt.Sprintf("finding[%d].rule", i),
					Expected: ef.Rule,
					Actual:   actual.Findings[i].Rule,
				})
				findingMatch = false
				break
			}
			if actual.Findings[i].Severity != ef.Severity {
				result.Mismatches = append(result.Mismatches, FixtureMismatch{
					CaseName: c.Name,
					Field:    fmt.Sprintf("finding[%d].severity", i),
					Expected: ef.Severity,
					Actual:   actual.Findings[i].Severity,
				})
				findingMatch = false
				break
			}
		}

		if !findingMatch {
			result.Failed++
			continue
		}

		result.Passed++
	}

	return result, nil
}

func RunRegressionCheck(ctx context.Context, generatedPolicyPath, referencePolicyPath, expectedCasesPath, fixtureRoot string) (*RegressionResult, error) {
	generatedResults, err := runPolicyOnFixtures(ctx, generatedPolicyPath, expectedCasesPath, fixtureRoot)
	if err != nil {
		return nil, fmt.Errorf("run generated policy: %w", err)
	}

	referenceResults, err := runPolicyOnFixtures(ctx, referencePolicyPath, expectedCasesPath, fixtureRoot)
	if err != nil {
		return nil, fmt.Errorf("run reference policy: %w", err)
	}

	result := &RegressionResult{Match: true}

	for caseName, gen := range generatedResults {
		ref, ok := referenceResults[caseName]
		if !ok {
			continue
		}

		if gen.Decision != ref.Decision {
			result.Match = false
			result.Mismatches = append(result.Mismatches, FixtureMismatch{
				CaseName: caseName,
				Field:    "decision",
				Expected: ref.Decision,
				Actual:   gen.Decision,
			})
		}

		for i := 0; i < len(gen.Findings) && i < len(ref.Findings); i++ {
			if gen.Findings[i].Rule != ref.Findings[i].Rule {
				result.Match = false
				result.Mismatches = append(result.Mismatches, FixtureMismatch{
					CaseName: caseName,
					Field:    fmt.Sprintf("finding[%d].rule", i),
					Expected: ref.Findings[i].Rule,
					Actual:   gen.Findings[i].Rule,
				})
			}
			if gen.Findings[i].Severity != ref.Findings[i].Severity {
				result.Match = false
				result.Mismatches = append(result.Mismatches, FixtureMismatch{
					CaseName: caseName,
					Field:    fmt.Sprintf("finding[%d].severity", i),
					Expected: ref.Findings[i].Severity,
					Actual:   gen.Findings[i].Severity,
				})
			}
			if gen.Findings[i].ClaimA != ref.Findings[i].ClaimA {
				result.Match = false
				result.Mismatches = append(result.Mismatches, FixtureMismatch{
					CaseName: caseName,
					Field:    fmt.Sprintf("finding[%d].claim_a", i),
					Expected: ref.Findings[i].ClaimA,
					Actual:   gen.Findings[i].ClaimA,
				})
			}
			if gen.Findings[i].ClaimB != ref.Findings[i].ClaimB {
				result.Match = false
				result.Mismatches = append(result.Mismatches, FixtureMismatch{
					CaseName: caseName,
					Field:    fmt.Sprintf("finding[%d].claim_b", i),
					Expected: ref.Findings[i].ClaimB,
					Actual:   gen.Findings[i].ClaimB,
				})
			}
		}
	}

	return result, nil
}

type policyResult struct {
	Decision string
	Findings []struct {
		Rule     string
		Severity string
		ClaimA   string
		ClaimB   string
	}
}

func runPolicyOnFixtures(ctx context.Context, policyPath, expectedCasesPath, fixtureRoot string) (map[string]policyResult, error) {
	data, err := os.ReadFile(expectedCasesPath)
	if err != nil {
		return nil, err
	}

	var expected ExpectedCases
	if err := yaml.Unmarshal(data, &expected); err != nil {
		return nil, err
	}

	results := make(map[string]policyResult)

	for _, c := range expected.Cases {
		inputPath := filepath.Join(fixtureRoot, c.InputFile)

		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		cmd := exec.CommandContext(ctx, "opa", "eval",
			"-d", policyPath,
			"-i", inputPath,
			"data.doctrust.policy.result")
		out, err := cmd.Output()
		cancel()

		if err != nil {
			continue
		}

		var opaOut struct {
			Result []struct {
				Expressions []struct {
					Value struct {
						Decision string `json:"decision"`
						Findings []struct {
							Rule     string `json:"rule"`
							Severity string `json:"severity"`
							ClaimA   string `json:"claim_a"`
							ClaimB   string `json:"claim_b"`
						} `json:"findings"`
					} `json:"value"`
				} `json:"expressions"`
			} `json:"result"`
		}

		if err := json.Unmarshal(out, &opaOut); err != nil {
			continue
		}

		if len(opaOut.Result) == 0 || len(opaOut.Result[0].Expressions) == 0 {
			continue
		}

		actual := opaOut.Result[0].Expressions[0].Value
		var findings []struct {
			Rule     string
			Severity string
			ClaimA   string
			ClaimB   string
		}
		for _, f := range actual.Findings {
			findings = append(findings, struct {
				Rule     string
				Severity string
				ClaimA   string
				ClaimB   string
			}{Rule: f.Rule, Severity: f.Severity, ClaimA: f.ClaimA, ClaimB: f.ClaimB})
		}

		results[c.Name] = policyResult{
			Decision: actual.Decision,
			Findings: findings,
		}
	}

	return results, nil
}
