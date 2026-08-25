package compiler

import (
	"fmt"
	"strings"

	"github.com/PithomLabs/doctrust/internal/eval"
)

// goCheckSystemPrompt is the system prompt for Go Check generation.
const goCheckSystemPrompt = `You are a compliance check author. You generate deterministic Go code
implementing the eval.Check interface.

You MUST return valid JSON with exactly these fields:
{
  "check_id": "snake_case identifier",
  "version": "1.0",
  "description": "what this check verifies",
  "go_source": "complete Go source implementing eval.Check",
  "parameters": {"param_name": default_value},
  "scenarios": [...],
  "adversarial_hint": "description of an edge case for human to author"
}

Constraints:
- Package must be "candidate"
- Must import "github.com/PithomLabs/doctrust/internal/eval"
- Must implement eval.Check interface: ID(), Version(), Evaluate(facts eval.Facts, params map[string]any) eval.Result
- Use eval.StatusPass, eval.StatusReview, eval.StatusFail
- Use eval.SeverityInfo, eval.SeverityWarning, eval.SeverityBlocking
- Build evidence with []evidence.EvidenceRef
- Access observations by their canonical semantic-type map key, e.g. facts["base_salary"]; each value is a []Fact slice
- Never call an LLM or external service
- Never import anything outside the standard library and eval/evidence packages
- Each scenario must use the canonical YAML scenario format
- Include at least 3 scenarios: pass, review/fail, and boundary case`

// GoCheckSystemPrompt returns the system prompt for Go Check generation.
func GoCheckSystemPrompt() string {
	return goCheckSystemPrompt
}

// BuildCheckPrompt builds the user prompt for Go Check generation.
func BuildCheckPrompt(intent string, registry *eval.CheckRegistry, factsSchema map[string][]string) string {
	var sb strings.Builder

	sb.WriteString("## Intent\n")
	sb.WriteString(intent)
	sb.WriteString("\n\n")

	// Available facts
	sb.WriteString("## Available Facts (semantic-type map keys)\n")
	for semanticType, fields := range factsSchema {
		for _, field := range fields {
			fmt.Fprintf(&sb, "- %s (%s) — access via facts[\"%s\"]\n", semanticType, field, semanticType)
		}
	}
	sb.WriteString("\n")

	// Existing check examples
	sb.WriteString("## Existing Check Examples\n\n")

	checks := registry.All()
	exampleCount := 0
	for id, check := range checks {
		if exampleCount >= 2 {
			break
		}
		sb.WriteString(fmt.Sprintf("### Example: %s (v%s)\n", check.ID(), check.Version()))
		sb.WriteString(fmt.Sprintf("Check ID: %s\n", id))
		sb.WriteString("Note: This check implements the eval.Check interface.\n\n")
		exampleCount++
	}

	// Check interface
	sb.WriteString("## Check Interface\n")
	sb.WriteString("```go\ntype Check interface {\n")
	sb.WriteString("    ID() string\n")
	sb.WriteString("    Version() string\n")
	sb.WriteString("    Evaluate(facts Facts, params map[string]any) Result\n")
	sb.WriteString("}\n```\n\n")

	// Result contract
	sb.WriteString("## Result Contract\n")
	sb.WriteString("```go\ntype Result struct {\n")
	sb.WriteString("    CheckID  string\n")
	sb.WriteString("    Status   Status   // PASS | REVIEW | FAIL\n")
	sb.WriteString("    Severity Severity // INFO | WARNING | BLOCKING\n")
	sb.WriteString("    Reason   string\n")
	sb.WriteString("    Evidence []evidence.EvidenceRef\n")
	sb.WriteString("    Metrics  map[string]any\n")
	sb.WriteString("}\n```\n\n")

	// Scenario format
	sb.WriteString("## Scenario YAML Format\n")
	sb.WriteString("```yaml\n")
	sb.WriteString("scenarios:\n")
	sb.WriteString("  - name: \"descriptive_name\"\n")
	sb.WriteString("    origin: \"ai\"\n")
	sb.WriteString("    input:\n")
	sb.WriteString("      facts:\n")
	sb.WriteString("        - semantic_type: \"key\"\n")
	sb.WriteString("          source_doc: \"doc_type\"\n")
	sb.WriteString("          field: \"field_name\"\n")
	sb.WriteString("          value: 123\n")
	sb.WriteString("          source_span: \"page=1\"\n")
	sb.WriteString("          confidence: 0.95\n")
	sb.WriteString("    params:\n")
	sb.WriteString("      param: value\n")
	sb.WriteString("    expected:\n")
	sb.WriteString("      check_id: \"check_id\"\n")
	sb.WriteString("      status: \"PASS|REVIEW|FAIL\"\n")
	sb.WriteString("      severity: \"INFO|WARNING|BLOCKING\"\n")
	sb.WriteString("      reason: \"explanation\"\n")
	sb.WriteString("      evidence:\n")
	sb.WriteString("        - field: \"semantic_type\"\n")
	sb.WriteString("          source_doc: \"doc_type\"\n")
	sb.WriteString("          source_span: \"page=1\"\n")
	sb.WriteString("          confidence: 0.95\n")
	sb.WriteString("```\n")

	return sb.String()
}
