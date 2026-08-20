package eval

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