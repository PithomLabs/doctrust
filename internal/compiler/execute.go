package compiler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/doctrust/doctrust/internal/eval"
	"github.com/doctrust/doctrust/internal/types"
	"gopkg.in/yaml.v3"
)

type CandidateExecutionResult struct {
	Passed  int                  `json:"passed"`
	Failed  int                  `json:"failed"`
	Total   int                  `json:"total"`
	Results []ScenarioExecResult `json:"results"`
	Error   string               `json:"error,omitempty"`
}

type ScenarioExecResult struct {
	Name     string      `json:"name"`
	Origin   string      `json:"origin"`
	Expected eval.Result `json:"expected"`
	Actual   eval.Result `json:"actual"`
	Match    bool        `json:"match"`
}

func extractCheckStructName(goSource string) (string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "check.go", goSource, 0)
	if err != nil {
		return "", fmt.Errorf("parse Go source: %w", err)
	}
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}
		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			_, isStruct := typeSpec.Type.(*ast.StructType)
			if !isStruct {
				continue
			}
			if !typeSpec.Name.IsExported() {
				continue
			}
			return typeSpec.Name.Name, nil
		}
	}
	return "", fmt.Errorf("no exported struct type found in Go source")
}

func generateHarnessMain(typeName string) []byte {
	harnessSrc := `package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/doctrust/doctrust/internal/eval"
)

func main() {
	r := eval.NewCheckRegistry()
	c := &TYPE_NAME{}
	if err := r.Register(c); err != nil {
		fmt.Fprintf(os.Stderr, "register: %v\n", err)
		os.Exit(1)
	}
	runner := eval.NewRunner(r.All())

	data, err := os.ReadFile("scenarios.json")
	if err != nil {
		fmt.Fprintf(os.Stderr, "read scenarios: %v\n", err)
		os.Exit(1)
	}
	var scenarios []eval.Scenario
	if err := json.Unmarshal(data, &scenarios); err != nil {
		fmt.Fprintf(os.Stderr, "unmarshal: %v\n", err)
		os.Exit(1)
	}

	if len(scenarios) == 0 {
		os.WriteFile("results.json", []byte("[]"), 0644)
		return
	}

	ctx := context.Background()
	type execResult struct {
		Name     string       ` + "`" + `json:"name"` + "`" + `
		Origin   string       ` + "`" + `json:"origin"` + "`" + `
		Expected eval.Result  ` + "`" + `json:"expected"` + "`" + `
		Actual   eval.Result  ` + "`" + `json:"actual"` + "`" + `
		Match    bool         ` + "`" + `json:"match"` + "`" + `
	}
	var results []execResult

	for _, s := range scenarios {
		sr := runner.RunScenario(ctx, s)
		results = append(results, execResult{
			Name:     sr.ScenarioName,
			Origin:   s.Origin,
			Expected: sr.Expected,
			Actual:   sr.Actual,
			Match:    sr.Passed,
		})
	}

	out, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "marshal: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile("results.json", out, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "write: %v\n", err)
		os.Exit(1)
	}
}
`
	harnessSrc = strings.Replace(harnessSrc, "TYPE_NAME", typeName, 1)
	return []byte(harnessSrc)
}

// FindModuleRoot walks upward from the current working directory until it finds
// a go.mod file, returning the directory containing it. This resolves the
// repository root independently of the caller's CWD depth.
func FindModuleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no go.mod found walking up from %s", dir)
		}
		dir = parent
	}
}

func ExecuteCandidateScenarios(snapshot *CandidateSnapshot) (*CandidateExecutionResult, error) {
	if len(snapshot.GoSource) == 0 {
		return nil, fmt.Errorf("no Go source in snapshot")
	}

	typeName, err := extractCheckStructName(string(snapshot.GoSource))
	if err != nil {
		return nil, fmt.Errorf("extract type name: %w", err)
	}

	repoRoot, err := FindModuleRoot()
	if err != nil {
		return nil, fmt.Errorf("resolve repo root: %w", err)
	}

	tmpDir, err := os.MkdirTemp(repoRoot, ".doctrust-execute-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	candidateSrc := rewritePackageDecl(string(snapshot.GoSource), "main")
	if err := os.WriteFile(filepath.Join(tmpDir, "check.go"), []byte(candidateSrc), 0644); err != nil {
		return nil, fmt.Errorf("write candidate check.go: %w", err)
	}

	scenarios, err := parseCandidateScenarios(snapshot)
	if err != nil {
		return nil, fmt.Errorf("parse scenarios: %w", err)
	}
	scenariosJSON, err := json.Marshal(scenarios)
	if err != nil {
		return nil, fmt.Errorf("marshal scenarios: %w", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "scenarios.json"), scenariosJSON, 0644); err != nil {
		return nil, fmt.Errorf("write scenarios.json: %w", err)
	}

	harness := generateHarnessMain(typeName)
	if err := os.WriteFile(filepath.Join(tmpDir, "main.go"), harness, 0644); err != nil {
		return nil, fmt.Errorf("write harness: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", "run", ".")
	cmd.Dir = tmpDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()

	result := &CandidateExecutionResult{}
	resultsPath := filepath.Join(tmpDir, "results.json")
	if data, readErr := os.ReadFile(resultsPath); readErr == nil {
		var execResults []ScenarioExecResult
		if jsonErr := json.Unmarshal(data, &execResults); jsonErr == nil {
			result.Results = execResults
			result.Total = len(execResults)
			for _, r := range execResults {
				if r.Match {
					result.Passed++
				} else {
					result.Failed++
				}
			}
		}
	}

	if runErr != nil && result.Total == 0 {
		result.Error = fmt.Sprintf("compilation failed: %v\nstderr: %s", runErr, stderr.String())
		return result, fmt.Errorf("candidate compilation failed: %v\n%s", runErr, stderr.String())
	}

	return result, nil
}

func rewritePackageDecl(src string, newPkg string) string {
	lines := splitLines(src)
	for i, line := range lines {
		if len(line) > 8 && line[:8] == "package " {
			lines[i] = "package " + newPkg + "\n"
			break
		}
	}
	return joinLines(lines)
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i+1])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func joinLines(lines []string) string {
	var b bytes.Buffer
	for _, l := range lines {
		b.WriteString(l)
	}
	return b.String()
}

func parseCandidateScenarios(snapshot *CandidateSnapshot) ([]eval.Scenario, error) {
	var all []eval.Scenario
	if len(snapshot.Scenarios) > 0 {
		scenarios, err := parseScenarioYAML(snapshot.Scenarios)
		if err != nil {
			return nil, fmt.Errorf("parse scenarios.yaml: %w", err)
		}
		all = append(all, scenarios...)
	}
	if len(snapshot.Adversarial) > 0 {
		scenarios, err := parseScenarioYAML(snapshot.Adversarial)
		if err != nil {
			return nil, fmt.Errorf("parse adversarial.yaml: %w", err)
		}
		all = append(all, scenarios...)
	}
	return all, nil
}

func parseScenarioYAML(data []byte) ([]eval.Scenario, error) {
	var wrapper struct {
		Scenarios []struct {
			Name   string         `yaml:"name"`
			Origin string         `yaml:"origin"`
			Input  scenarioInput  `yaml:"input"`
			Params map[string]any `yaml:"params"`
			Expected struct {
				CheckID  string              `yaml:"check_id"`
				Status   string              `yaml:"status"`
				Severity string              `yaml:"severity"`
				Reason   string              `yaml:"reason"`
				Evidence []candidateEvidence `yaml:"evidence"`
			} `yaml:"expected"`
		} `yaml:"scenarios"`
	}
	if err := yaml.Unmarshal(data, &wrapper); err != nil {
		return nil, err
	}
	var result []eval.Scenario
	for _, s := range wrapper.Scenarios {
		var facts []eval.ScenarioFact
		for _, f := range s.Input.Facts {
			facts = append(facts, eval.ScenarioFact{
				SemanticType: f.SemanticType,
				Value:        f.Value,
				SourceDoc:    f.SourceDoc,
				FieldName:    f.Field,
				SourceSpan:   f.SourceSpan,
				Confidence:   f.Confidence,
			})
		}
		var evidence []types.EvidenceRef
		for _, e := range s.Expected.Evidence {
			evidence = append(evidence, types.EvidenceRef{
				Field:      e.Field,
				SourceDoc:  e.SourceDoc,
				SourceSpan: e.SourceSpan,
				Confidence: e.Confidence,
			})
		}
		result = append(result, eval.Scenario{
			Name:   s.Name,
			Origin: s.Origin,
			Input: eval.ScenarioInput{
				Facts: facts,
			},
			Params: s.Params,
			Expected: eval.Result{
				CheckID:  s.Expected.CheckID,
				Status:   eval.Status(s.Expected.Status),
				Severity: eval.Severity(s.Expected.Severity),
				Reason:   s.Expected.Reason,
				Evidence: evidence,
			},
		})
	}
	return result, nil
}

type scenarioInput struct {
	Facts []candidateFact `yaml:"facts"`
}

type candidateFact struct {
	SemanticType string  `yaml:"semantic_type"`
	SourceDoc    string  `yaml:"source_doc"`
	Field        string  `yaml:"field"`
	Value        any     `yaml:"value"`
	SourceSpan   string  `yaml:"source_span"`
	Confidence   float64 `yaml:"confidence"`
}

type candidateEvidence struct {
	Field      string  `yaml:"field"`
	SourceDoc  string  `yaml:"source_doc"`
	SourceSpan string  `yaml:"source_span"`
	Confidence float64 `yaml:"confidence"`
}
