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

// Decision is the canonical terminal output of the evaluation engine.
// It contains the ruleset provenance, aggregate status, and all check results.
type Decision struct {
	RulesetID      string   `json:"ruleset_id" yaml:"ruleset_id"`
	RulesetVersion string   `json:"ruleset_version" yaml:"ruleset_version"`
	Status         Status   `json:"status" yaml:"status"`
	Results        []Result `json:"results" yaml:"results"`
}

type Scenario struct {
	Name     string         `yaml:"name"`
	Origin   string         `yaml:"origin"` // "ai" | "human_adversarial" | "real_fixture"
	Input    ScenarioInput  `yaml:"input"`
	Params   map[string]any `yaml:"params,omitempty"`
	Expected Result         `yaml:"expected"`
}