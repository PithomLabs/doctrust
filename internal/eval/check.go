package eval

import (
	"github.com/doctrust/doctrust/internal/evidence"
	"github.com/doctrust/doctrust/internal/facts"
)

// Facts is a type alias for the canonical facts.Facts.
// The map key is the semantic type; the slice contains all observations of that type.
type Facts = facts.Facts

// Fact is a type alias for the canonical facts.Fact.
type Fact = facts.Fact

type Severity string

const (
	SeverityInfo     Severity = "INFO"
	SeverityWarning  Severity = "WARNING"
	SeverityBlocking Severity = "BLOCKING"
)

type Status string

const (
	StatusPass   Status = "PASS"
	StatusReview Status = "REVIEW"
	StatusFail   Status = "FAIL"
)

type Check interface {
	ID() string
	Version() string
	Evaluate(facts Facts, params map[string]any) Result
}

type Result struct {
	CheckID  string                `json:"check_id" yaml:"check_id"`
	Status   Status                `json:"status" yaml:"status"`
	Severity Severity              `json:"severity" yaml:"severity"`
	Reason   string                `json:"reason" yaml:"reason"`
	Evidence []evidence.EvidenceRef `json:"evidence" yaml:"evidence"`
	Metrics  map[string]any        `json:"metrics,omitempty" yaml:"metrics,omitempty"`
}

type SourceRef struct {
	DocumentID string    `json:"document_id"`
	Filename   string    `json:"filename"`
	Page       int       `json:"page"`
	Bbox       []int     `json:"bbox,omitempty"`
	Confidence float64   `json:"confidence"`
	FieldName  string    `json:"field_name,omitempty"`
}

type CheckRef struct {
	ID      string         `yaml:"id"`
	Version string         `yaml:"version"`
	Params  map[string]any `yaml:"params,omitempty"`
}

type Ruleset struct {
	ID       string      `yaml:"id"`
	Version  string      `yaml:"version"` // immutable once promoted
	Checks   []CheckRef  `yaml:"checks"`
}

type Aggregator interface {
	Aggregate([]Result) (status string, blockedBy []string)
}

type EnrichedFinding struct {
	Rule     string      `json:"rule"`
	Severity string       `json:"severity"`
	ClaimA   string       `json:"claim_a"`
	ClaimB   string       `json:"claim_b"`
	ValueA   interface{}  `json:"value_a"`
	ValueB   interface{}  `json:"value_b"`
	Sources  []SourceRef  `json:"sources,omitempty"`
}

type EnrichedResult struct {
	Decision string          `json:"decision"`
	Findings []EnrichedFinding `json:"findings"`
}

type Scenario struct {
	Name     string         `yaml:"name"`
	Origin   string         `yaml:"origin"` // "ai" | "human_adversarial" | "real_fixture"
	Input    ScenarioInput  `yaml:"input"`
	Params   map[string]any `yaml:"params,omitempty"`
	Expected Result         `yaml:"expected"`
}