package compiler

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

type CompiledPolicy struct {
	Rego   string `json:"policy_rego"`
	Tests  string `json:"policy_test_rego"`
}

var codeFenceRe = regexp.MustCompile("(?s)```(?:json)?\\s*\n?(.*?)\n?\\s*```")

func ParseLLMOutput(raw string) (*CompiledPolicy, error) {
	raw = strings.TrimSpace(raw)

	jsonStr := raw
	if matches := codeFenceRe.FindStringSubmatch(raw); len(matches) > 1 {
		jsonStr = strings.TrimSpace(matches[1])
	}

	var cp CompiledPolicy
	if err := json.Unmarshal([]byte(jsonStr), &cp); err != nil {
		return nil, fmt.Errorf("parse LLM JSON: %w", err)
	}

	if strings.TrimSpace(cp.Rego) == "" {
		return nil, fmt.Errorf("LLM output missing policy_rego")
	}

	if strings.TrimSpace(cp.Tests) == "" {
		return nil, fmt.Errorf("LLM output missing policy_test_rego")
	}

	return &cp, nil
}
