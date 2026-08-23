package compiler

import (
	"github.com/doctrust/doctrust/internal/eval"
)

// ResolveRulesetParams returns the Ruleset's params for the given check ID,
// falling back to scenario params if the check is not in the Ruleset.
//
// This is the SINGLE SOURCE OF TRUTH for parameter resolution, shared verbatim
// by cmd/regression (production) and RunStagedRegression (pre-commit gate):
//
//	Ruleset CheckRef.Params exists → use Ruleset params wholesale
//	otherwise                      → scenario fallback
//
// PURE function: no filesystem access, no global state, no mutation.
func ResolveRulesetParams(rs eval.Ruleset, checkID string, fallback map[string]any) map[string]any {
	for _, ref := range rs.Checks {
		if ref.ID == checkID && ref.Params != nil {
			return ref.Params
		}
	}
	return fallback
}
