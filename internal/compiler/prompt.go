package compiler

import (
	"fmt"
	"sort"
	"strings"
)

func BuildPrompt(ps *PolicySource) string {
	var sb strings.Builder

	// Build document list from ExtractionSchemas (not hardcoded)
	docTypes := make([]string, 0, len(ps.ExtractionSchemas))
	for docType := range ps.ExtractionSchemas {
		docTypes = append(docTypes, docType)
	}
	sort.Strings(docTypes)

	sb.WriteString("## Evidence Schema\n\n")
	sb.WriteString("Documents: ")
	sb.WriteString(strings.Join(docTypes, ", "))
	sb.WriteString("\n\n")

	sb.WriteString("### Fields by Document Type\n\n")
	for _, docType := range docTypes {
		fields := ps.ExtractionSchemas[docType]
		sb.WriteString(fmt.Sprintf("%s:\n", docType))
		for _, f := range fields {
			mapping := findSemanticMapping(ps.SemanticMappings, docType, f.Name)
			if mapping != "" {
				sb.WriteString(fmt.Sprintf("  - %s (%s) → %s\n", f.Name, f.Type, mapping))
			} else {
				sb.WriteString(fmt.Sprintf("  - %s (%s)\n", f.Name, f.Type))
			}
		}
		sb.WriteString("\n")
	}

	sb.WriteString("### Semantic Relationships\n\n")
	for _, rule := range ps.Rules {
		if rule.Type == "incomparable" {
			sb.WriteString(fmt.Sprintf("- %s is incomparable to %s\n", rule.Left, rule.Right))
		}
	}
	sb.WriteString("\n")

	sb.WriteString("## Policy Definition\n\n")
	for _, rule := range ps.Rules {
		switch rule.Type {
		case "equality":
			sb.WriteString(fmt.Sprintf("- %s must equal %s\n", rule.Left, rule.Right))
		case "variance":
			sb.WriteString(fmt.Sprintf("- %s variance over %.0f%% requires human review\n", rule.Left, rule.Threshold))
		case "confidence":
			sb.WriteString(fmt.Sprintf("- confidence below %.1f requires human review\n", rule.Threshold))
		}
	}
	sb.WriteString("\n")

	sb.WriteString("## Required Evidence\n\n")
	requiredSet := make(map[string]bool)
	for _, doc := range ps.RequiredEvidence {
		requiredSet[doc] = true
	}
	for _, doc := range ps.RequiredEvidence {
		sb.WriteString(fmt.Sprintf("- %s is required\n", doc))
	}
	// Optional docs: in schemas but not in required
	for _, docType := range docTypes {
		if !requiredSet[docType] {
			sb.WriteString(fmt.Sprintf("- %s is optional (contextual)\n", docType))
		}
	}
	sb.WriteString("\n")

	// Generate acceptance constraints from parsed Decisions
	sb.WriteString("## Acceptance Constraints\n\n")
	sb.WriteString("The following scenarios represent required behavioral outcomes.\n")
	sb.WriteString("Implement the general policy semantics from the Policy Definition above.\n")
	sb.WriteString("These threshold values are policy semantics derived from POLICY.md.\n")
	sb.WriteString("Implement the general rule; never branch on fixture names, case IDs,\n")
	sb.WriteString("fixture-specific values, or expected-output literals.\n")
	sb.WriteString("The acceptance cases are behavioral constraints, not case-specific exceptions.\n\n")

	for _, d := range ps.Decisions {
		sb.WriteString(fmt.Sprintf("- %s when %s\n", d.State, d.Condition))
	}
	sb.WriteString("\n")

	sb.WriteString("## Additional Rules\n\n")
	sb.WriteString("- W-2 wages = 0 → REVIEW (zero-denominator, variance undefined)\n")
	sb.WriteString("- Required evidence check happens before variance checks\n")

	return sb.String()
}

func findSemanticMapping(mappings []SemanticMapping, docType, field string) string {
	for _, m := range mappings {
		if m.DocType == docType && m.Field == field {
			return m.SemanticType
		}
	}
	return ""
}
