package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

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

type OPAExpression struct {
	Value struct {
		Decision string `json:"decision"`
		Findings []struct {
			Rule     string `json:"rule"`
			Severity string `json:"severity"`
		} `json:"findings"`
	} `json:"value"`
}

type OPAResult struct {
	Expressions []OPAExpression `json:"expressions"`
}

type OPAOutput struct {
	Result []OPAResult `json:"result"`
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: validate-fixtures <project-root>\n")
		os.Exit(1)
	}
	root := os.Args[1]

	policyPath := filepath.Join(root, "policies", "income_verification", "policy.rego")
	expectedPath := filepath.Join(root, "policies", "income_verification", "expected_cases.yaml")
	adversarialPath := filepath.Join(root, "policies", "income_verification", "adversarial_cases.yaml")

	var allCases ExpectedCases

	// Load primary fixtures
	data, err := os.ReadFile(expectedPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading %s: %v\n", expectedPath, err)
		os.Exit(1)
	}

	var expected ExpectedCases
	if err := yaml.Unmarshal(data, &expected); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing YAML: %v\n", err)
		os.Exit(1)
	}
	allCases.Cases = append(allCases.Cases, expected.Cases...)

	// Load adversarial fixtures
	if advData, err := os.ReadFile(adversarialPath); err == nil {
		var adversarial ExpectedCases
		if err := yaml.Unmarshal(advData, &adversarial); err == nil {
			allCases.Cases = append(allCases.Cases, adversarial.Cases...)
		}
	}

	pass, fail := 0, 0
	for _, c := range allCases.Cases {
		inputPath := filepath.Join(root, c.InputFile)

		cmd := exec.Command("opa", "eval",
			"-d", policyPath,
			"-i", inputPath,
			"data.doctrust.policy.result")
		out, err := cmd.Output()
		if err != nil {
			fmt.Printf("FAIL: %s (opa error: %v)\n", c.Name, err)
			fail++
			continue
		}

		var opaOut OPAOutput
		if err := json.Unmarshal(out, &opaOut); err != nil {
			fmt.Printf("FAIL: %s (json parse error: %v)\n", c.Name, err)
			fail++
			continue
		}

		if len(opaOut.Result) == 0 || len(opaOut.Result[0].Expressions) == 0 {
			fmt.Printf("FAIL: %s (no result)\n", c.Name)
			fail++
			continue
		}

		actual := opaOut.Result[0].Expressions[0].Value
		actualFindings := actual.Findings

		// Check decision
		if actual.Decision != c.ExpectedDecision {
			fmt.Printf("FAIL: %s (decision: expected=%s, got=%s)\n",
				c.Name, c.ExpectedDecision, actual.Decision)
			fail++
			continue
		}

		// Check findings count
		if len(actualFindings) != len(c.ExpectedFindings) {
			fmt.Printf("FAIL: %s (findings count: expected=%d, got=%d)\n",
				c.Name, len(c.ExpectedFindings), len(actualFindings))
			fail++
			continue
		}

		// Check each expected finding's rule and severity
		findingMatch := true
		for i, ef := range c.ExpectedFindings {
			if actualFindings[i].Rule != ef.Rule {
				fmt.Printf("FAIL: %s (finding[%d].rule: expected=%s, got=%s)\n",
					c.Name, i, ef.Rule, actualFindings[i].Rule)
				findingMatch = false
				break
			}
			if actualFindings[i].Severity != ef.Severity {
				fmt.Printf("FAIL: %s (finding[%d].severity: expected=%s, got=%s)\n",
					c.Name, i, ef.Severity, actualFindings[i].Severity)
				findingMatch = false
				break
			}
		}

		if !findingMatch {
			fail++
			continue
		}

		fmt.Printf("PASS: %s\n", c.Name)
		pass++
	}

	fmt.Printf("\nResults: %d passed, %d failed\n", pass, fail)
	if fail > 0 {
		os.Exit(1)
	}
}
