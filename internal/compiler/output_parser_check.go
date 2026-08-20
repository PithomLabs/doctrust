package compiler

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// ParseCheckOutput parses LLM output into a CheckCandidate.
func ParseCheckOutput(raw string) (*CheckCandidate, error) {
	raw = strings.TrimSpace(raw)

	jsonStr := raw
	if matches := codeFenceRe.FindStringSubmatch(raw); len(matches) > 1 {
		jsonStr = strings.TrimSpace(matches[1])
	}

	var candidate CheckCandidate
	if err := json.Unmarshal([]byte(jsonStr), &candidate); err != nil {
		return nil, fmt.Errorf("parse LLM JSON: %w", err)
	}

	// Validate required fields
	if strings.TrimSpace(candidate.CheckID) == "" {
		return nil, fmt.Errorf("LLM output missing check_id")
	}
	if strings.TrimSpace(candidate.Version) == "" {
		return nil, fmt.Errorf("LLM output missing version")
	}
	if strings.TrimSpace(candidate.GoSource) == "" {
		return nil, fmt.Errorf("LLM output missing go_source")
	}
	if len(candidate.Scenarios) == 0 {
		return nil, fmt.Errorf("LLM output missing scenarios")
	}

	// Validate check_id is snake_case
	if !isSnakeCase(candidate.CheckID) {
		return nil, fmt.Errorf("check_id must be snake_case, got: %s", candidate.CheckID)
	}

	return &candidate, nil
}

var snakeCaseRe = regexp.MustCompile(`^[a-z][a-z0-9]*(_[a-z0-9]+)*$`)

func isSnakeCase(s string) bool {
	return snakeCaseRe.MatchString(s)
}
