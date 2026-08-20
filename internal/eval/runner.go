package eval

import (
	"context"
	"fmt"
)

type ScenarioResult struct {
	Passed       bool
	ScenarioName string
	Actual       Result
	Expected     Result
	Diff         *ScenarioDiff
}

type Runner struct {
	checks map[string]Check
}

func NewRunner(checks map[string]Check) *Runner {
	return &Runner{checks: checks}
}

func (r *Runner) Register(check Check) error {
	if check == nil {
		return fmt.Errorf("check is nil")
	}
	id := check.ID()
	if id == "" {
		return fmt.Errorf("check id is empty")
	}
	if _, exists := r.checks[id]; exists {
		return fmt.Errorf("check %s already registered", id)
	}
	r.checks[id] = check
	return nil
}

func (r *Runner) GetCheck(id string) (Check, error) {
	check, ok := r.checks[id]
	if !ok {
		return nil, fmt.Errorf("check %s not found", id)
	}
	return check, nil
}

func (r *Runner) RunScenario(ctx context.Context, s Scenario) ScenarioResult {
	check, err := r.GetCheck(s.Expected.CheckID)
	if err != nil {
		return ScenarioResult{
			Passed:       false,
			ScenarioName: s.Name,
			Expected:     s.Expected,
		}
	}

	// Convert scenario input to Facts
	facts := ScenarioInputToFacts(s.Input)

	actual := check.Evaluate(facts, s.Params)
	return CompareScenario(s, actual)
}

func (r *Runner) RunRuleset(ctx context.Context, rs Ruleset, facts Facts) ([]Result, error) {
	var results []Result
	for _, ref := range rs.Checks {
		check, ok := r.checks[ref.ID]
		if !ok {
			return nil, fmt.Errorf("check %s not found", ref.ID)
		}
		result := check.Evaluate(facts, ref.Params)
		result.CheckID = ref.ID
		results = append(results, result)
	}
	return results, nil
}

// Evaluate runs all checks in the ruleset and returns the canonical Decision.
// This is the production terminal output of the engine.
func (r *Runner) Evaluate(ctx context.Context, rs Ruleset, f Facts) (Decision, error) {
	results, err := r.RunRuleset(ctx, rs, f)
	if err != nil {
		return Decision{}, err
	}
	agg := &DecisionAggregator{}
	return agg.Decide(results, rs), nil
}

func CompareScenario(s Scenario, actual Result) ScenarioResult {
	expected := s.Expected
	diff := DiffResults(actual, expected)
	return ScenarioResult{
		Passed:       diff == nil,
		ScenarioName: s.Name,
		Actual:       actual,
		Expected:     expected,
		Diff:         diff,
	}
}

func (r *Runner) RunAllScenarios(ctx context.Context, scenarios []Scenario) []ScenarioResult {
	var results []ScenarioResult
	for _, s := range scenarios {
		results = append(results, r.RunScenario(ctx, s))
	}
	return results
}