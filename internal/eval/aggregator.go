package eval

import "github.com/doctrust/doctrust/internal/types"

type DecisionAggregator struct{}

func (a *DecisionAggregator) Aggregate(results []Result) (status string, blockedBy []string) {
	hasBlockingFail := false
	hasReview := false

	for _, r := range results {
		switch r.Status {
		case StatusFail:
			if r.Severity == SeverityBlocking {
				hasBlockingFail = true
				blockedBy = append(blockedBy, r.CheckID)
			}
		case StatusReview:
			hasReview = true
		}
	}

	if hasBlockingFail {
		return "FAIL", blockedBy
	}
	if hasReview {
		return "REVIEW", nil
	}
	return "PASS", nil
}

// Decide aggregates results into a Decision with ruleset provenance.
// The results slice and nested Evidence are defensively copied to make
// Decision an immutable terminal result.
func (a *DecisionAggregator) Decide(results []Result, rs Ruleset) Decision {
	status, _ := a.Aggregate(results)
	copied := make([]Result, len(results))
	for i, r := range results {
		copied[i] = r
		if len(r.Evidence) > 0 {
			copied[i].Evidence = make([]types.EvidenceRef, len(r.Evidence))
			copy(copied[i].Evidence, r.Evidence)
		}
	}
	return Decision{
		RulesetID:      rs.ID,
		RulesetVersion: rs.Version,
		Status:         Status(status),
		Results:        copied,
	}
}