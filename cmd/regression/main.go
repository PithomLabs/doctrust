package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/PithomLabs/doctrust/internal/compiler"
	"github.com/PithomLabs/doctrust/internal/eval"
)

// paramSource tracks which parameter source was used for audit trail.
type paramSource struct {
	Baseline  string `json:"baseline"`
	Candidate string `json:"candidate"`
}

func main() {
	domain := flag.String("domain", "income_verification", "ruleset domain")
	draftPath := flag.String("draft", "", "candidate working draft YAML (overrides promoted)")
	scenariosDir := flag.String("scenarios", "", "scenarios directory (default: scenarios/<domain>)")
	showJSON := flag.Bool("json", false, "output as JSON")
	scenarioParams := flag.Bool("scenario-params", false, "use scenario-level params instead of Ruleset params (legacy mode)")
	flag.Parse()

	registry := eval.NewRegistry("rulesets")

	// Load the baseline (current promoted version)
	baseline, err := registry.LoadPromoted(*domain)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot load promoted ruleset for %s: %v\n", *domain, err)
		os.Exit(1)
	}

	// Load the candidate
	var candidate eval.Ruleset
	if *draftPath != "" {
		candidate, err = eval.LoadRuleset(*draftPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: cannot load draft ruleset: %v\n", err)
			os.Exit(1)
		}
	} else {
		candidate, err = registry.LoadWorking(*domain)
		if err != nil || candidate.Version == "draft" {
			fmt.Fprintf(os.Stderr, "Error: no working draft found for %s. Use --draft to specify a candidate.\n", *domain)
			os.Exit(1)
		}
	}

	// Load scenarios
	if *scenariosDir == "" {
		*scenariosDir = filepath.Join("scenarios", *domain)
	}
	scenarios, err := eval.LoadAllScenariosFromDir(*scenariosDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot load scenarios: %v\n", err)
		os.Exit(1)
	}
	if len(scenarios) == 0 {
		fmt.Fprintf(os.Stderr, "Error: no scenarios found in %s\n", *scenariosDir)
		os.Exit(1)
	}

	// Register checks
	checks := eval.DefaultRegistry().All()

	// Build runners
	baselineRunner := eval.NewRunner(checks)
	candidateRunner := eval.NewRunner(checks)

	// Resolve params for each scenario based on param source
	ctx := context.Background()
	var baselineResults []eval.ScenarioResult
	var candidateResults []eval.ScenarioResult

	// Track which param source was used per scenario for audit trail
	var paramSources []paramSource

	for _, s := range scenarios {
		var bParams, cParams map[string]any

		if *scenarioParams {
			// Legacy mode: use scenario-level params
			bParams = s.Params
			cParams = s.Params
			paramSources = append(paramSources, paramSource{Baseline: "scenario", Candidate: "scenario"})
		} else {
			// Default: use Ruleset params (the actual Phase 2 behavior).
			// Shared single-source-of-truth resolver — identical semantics to
			// the staged regression gate in internal/compiler.
			bParams = compiler.ResolveRulesetParams(baseline, s.Expected.CheckID, s.Params)
			cParams = compiler.ResolveRulesetParams(candidate, s.Expected.CheckID, s.Params)
			paramSources = append(paramSources, paramSource{Baseline: "ruleset", Candidate: "ruleset"})
		}

		sBaseline := s
		sBaseline.Params = bParams
		baselineResults = append(baselineResults, baselineRunner.RunScenario(ctx, sBaseline))

		sCandidate := s
		sCandidate.Params = cParams
		candidateResults = append(candidateResults, candidateRunner.RunScenario(ctx, sCandidate))
	}

	// Compare and report
	type reportEntry struct {
		Name        string                    `json:"name"`
		Changed     bool                      `json:"changed"`
		Diff        *eval.ScenarioDiffSummary `json:"diff,omitempty"`
		Baseline    eval.ScenarioResult       `json:"baseline"`
		Candidate   eval.ScenarioResult       `json:"candidate"`
		ParamSource paramSource               `json:"param_source"`
	}

	var entries []reportEntry
	changedCount := 0
	passedCount := 0
	failedCount := 0

	for i := range baselineResults {
		b := baselineResults[i]
		c := candidateResults[i]
		diff := eval.DiffResults(c.Actual, b.Actual)

		entry := reportEntry{
			Name:        b.ScenarioName,
			Changed:     diff != nil,
			Baseline:    b,
			Candidate:   c,
			ParamSource: paramSources[i],
		}
		if diff != nil {
			s := diff.ToSummary()
			entry.Diff = &s
			changedCount++
		}
		if c.Passed {
			passedCount++
		} else {
			failedCount++
		}
		entries = append(entries, entry)
	}

	if *showJSON {
		output := map[string]any{
			"baseline": map[string]string{
				"id":      baseline.ID,
				"version": baseline.Version,
			},
			"candidate": map[string]string{
				"id":      candidate.ID,
				"version": candidate.Version,
			},
			"param_source": map[string]string{
				"mode": paramSourceMode(*scenarioParams),
			},
			"summary": map[string]int{
				"total":   len(entries),
				"changed": changedCount,
				"passed":  passedCount,
				"failed":  failedCount,
			},
			"results": entries,
		}
		data, _ := json.MarshalIndent(output, "", "  ")
		fmt.Println(string(data))
		return
	}

	// Human-readable output
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	fmt.Fprintf(w, "REGRESSION REPORT: %s\n", *domain)
	fmt.Fprintf(w, "Param mode: %s\n\n", paramSourceMode(*scenarioParams))

	// Header with params
	baselineParams := summarizeParams(baseline)
	candidateParams := summarizeParams(candidate)
	fmt.Fprintf(w, "Baseline:  %s v%s%s\n", baseline.ID, baseline.Version, baselineParams)
	fmt.Fprintf(w, "Candidate: %s v%s%s\n", candidate.ID, candidate.Version, candidateParams)
	fmt.Fprintf(w, "Scenarios: %d total\n\n", len(entries))

	for _, e := range entries {
		marker := " "
		if e.Changed {
			marker = "*"
		}

		if e.Changed {
			// Rich diff output for changed scenarios
			printChangedEntry(w, e.Name, e.Diff, e.ParamSource)
		} else {
			// Compact output for unchanged scenarios
			status := "PASS"
			if !e.Candidate.Passed {
				status = "FAIL"
			}
			fmt.Fprintf(w, "%s\t[%s]\t%s\n", marker, status, e.Name)
		}
	}

	fmt.Fprintf(w, "\n---\n")
	fmt.Fprintf(w, "Total: %d | Changed: %d | Passed: %d | Failed: %d\n",
		len(entries), changedCount, passedCount, failedCount)

	if changedCount > 0 {
		fmt.Fprintf(w, "\nChanged scenarios marked with *. Review before promoting.\n")
	}

	w.Flush()
}

// summarizeParams returns a human-readable summary of a Ruleset's params.
func summarizeParams(rs eval.Ruleset) string {
	var parts []string
	for _, ref := range rs.Checks {
		if ref.Params != nil {
			for k, v := range ref.Params {
				parts = append(parts, fmt.Sprintf("%s=%v", k, v))
			}
		}
	}
	if len(parts) == 0 {
		return ""
	}
	sort.Strings(parts)
	return " (" + strings.Join(parts, ", ") + ")"
}

// paramSourceMode returns the param source description.
func paramSourceMode(scenarioParams bool) string {
	if scenarioParams {
		return "scenario (legacy)"
	}
	return "ruleset (default)"
}

// printChangedEntry prints a rich Before/After diff for a changed scenario.
func printChangedEntry(w *tabwriter.Writer, name string, diff *eval.ScenarioDiffSummary, ps paramSource) {
	fmt.Fprintf(w, "* %s\n\n", name)

	// Before
	fmt.Fprintf(w, "  Before:  %s / %s\n", diff.Before.Status, diff.Before.Severity)
	if diff.Before.Reason != "" {
		fmt.Fprintf(w, "    reason: %s\n", diff.Before.Reason)
	}

	// After
	fmt.Fprintf(w, "  After:   %s / %s\n", diff.After.Status, diff.After.Severity)
	if diff.After.Reason != "" {
		fmt.Fprintf(w, "    reason: %s\n", diff.After.Reason)
	}

	// Change summary
	var changes []string
	if diff.StatusChanged {
		changes = append(changes, "status")
	}
	if diff.SeverityChanged {
		changes = append(changes, "severity")
	}
	if diff.ReasonChanged {
		changes = append(changes, "reason")
	}
	if diff.EvidenceChanged {
		changes = append(changes, "evidence")
	}
	if len(changes) > 0 {
		fmt.Fprintf(w, "  Changes: %s\n", strings.Join(changes, ", "))
	}

	// Param source for audit
	fmt.Fprintf(w, "  Params:  %s → %s\n", ps.Baseline, ps.Candidate)

	fmt.Fprintf(w, "\n")
}
