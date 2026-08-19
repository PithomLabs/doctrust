package opa

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/open-policy-agent/opa/v1/rego"
)

type EvaluationResult struct {
	Decision string    `json:"decision"`
	Findings []Finding `json:"findings"`
}

type Finding struct {
	Rule     string      `json:"rule"`
	Severity string      `json:"severity"`
	ClaimA   string      `json:"claim_a,omitempty"`
	ClaimB   string      `json:"claim_b,omitempty"`
	Reason   string      `json:"reason,omitempty"`
	ValueA   interface{} `json:"value_a,omitempty"`
	ValueB   interface{} `json:"value_b,omitempty"`
	Sources  []SourceRef `json:"sources,omitempty"`
}

type SourceRef struct {
	DocumentID string  `json:"document_id"`
	Filename   string  `json:"filename"`
	Page       int     `json:"page"`
	Bbox       []int   `json:"bbox,omitempty"`
	Confidence float64 `json:"confidence"`
	FieldName  string  `json:"field_name,omitempty"`
}

func Evaluate(ctx context.Context, snapshotJSON []byte, regoPolicy string) (*EvaluationResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var input interface{}
	if err := json.Unmarshal(snapshotJSON, &input); err != nil {
		return nil, fmt.Errorf("unmarshal snapshot: %w", err)
	}

	rs, err := rego.New(
		rego.Query("data.doctrust.policy.result"),
		rego.Module("policy.rego", regoPolicy),
		rego.Input(input),
	).Eval(ctx)
	if err != nil {
		return nil, fmt.Errorf("opa eval: %w", err)
	}

	if len(rs) == 0 || len(rs[0].Expressions) == 0 {
		return nil, fmt.Errorf("no result from OPA")
	}

	resultBytes, err := json.Marshal(rs[0].Expressions[0].Value)
	if err != nil {
		return nil, fmt.Errorf("marshal result: %w", err)
	}

	var result EvaluationResult
	if err := json.Unmarshal(resultBytes, &result); err != nil {
		return nil, fmt.Errorf("unmarshal result: %w", err)
	}

	return &result, nil
}
