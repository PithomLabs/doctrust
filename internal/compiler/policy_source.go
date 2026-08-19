package compiler

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

type PolicySource struct {
	Name              string
	RequiredEvidence  []string
	ExtractionSchemas map[string][]FieldSpec
	SemanticMappings  []SemanticMapping
	Rules             []PolicyRule
	Decisions         []DecisionRule
}

type FieldSpec struct {
	Name string
	Type string
}

type SemanticMapping struct {
	DocType      string
	Field        string
	SemanticType string
}

type PolicyRule struct {
	Type       string // "equality", "variance", "confidence", "incomparable"
	Left       string
	Right      string
	Threshold  float64
	Severities map[string]string
}

type DecisionRule struct {
	State     string // "PASS", "FAIL", "REVIEW", "MISSING_EVIDENCE"
	Condition string
}

var validDocTypes = map[string]bool{
	"paystub":        true,
	"w2":             true,
	"form_1040":      true,
	"bank_statement": true,
}

func ParsePolicySource(path string) (*PolicySource, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open POLICY.md: %w", err)
	}
	defer file.Close()

	ps := &PolicySource{
		ExtractionSchemas: make(map[string][]FieldSpec),
	}

	scanner := bufio.NewScanner(file)
	var currentSection string

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if strings.HasPrefix(line, "# ") && !strings.HasPrefix(line, "## ") {
			ps.Name = strings.TrimSpace(strings.TrimPrefix(line, "# "))
			continue
		}

		if strings.HasPrefix(line, "## ") {
			currentSection = strings.TrimSpace(strings.TrimPrefix(line, "## "))
			continue
		}

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		switch currentSection {
		case "Required Evidence":
			if strings.HasPrefix(line, "- ") {
				docType := strings.TrimSpace(strings.TrimPrefix(line, "- "))
				if !validDocTypes[docType] {
					return nil, fmt.Errorf("unknown document type: %s", docType)
				}
				ps.RequiredEvidence = append(ps.RequiredEvidence, docType)
			}
		case "Extraction Schema":
			if strings.HasPrefix(line, "- ") {
				inner := strings.TrimPrefix(line, "- ")
				parts := strings.SplitN(inner, ":", 2)
				if len(parts) != 2 {
					return nil, fmt.Errorf("invalid extraction schema line: %s", line)
				}
				docType := strings.TrimSpace(parts[0])
				if !validDocTypes[docType] {
					return nil, fmt.Errorf("unknown document type in extraction schema: %s", docType)
				}
				fields := strings.Split(parts[1], ",")
				var specs []FieldSpec
				for _, f := range fields {
					specs = append(specs, FieldSpec{
						Name: strings.TrimSpace(f),
						Type: "currency",
					})
				}
				ps.ExtractionSchemas[docType] = specs
			}
		case "Semantic Classification":
			if strings.HasPrefix(line, "- ") {
				inner := strings.TrimPrefix(line, "- ")
				parts := strings.Split(inner, "→")
				if len(parts) != 2 {
					parts = strings.Split(inner, "->")
				}
				if len(parts) != 2 {
					return nil, fmt.Errorf("invalid semantic mapping: %s", line)
				}
				left := strings.TrimSpace(parts[0])
				right := strings.TrimSpace(parts[1])
				dotParts := strings.SplitN(left, ".", 2)
				if len(dotParts) != 2 {
					return nil, fmt.Errorf("invalid field reference: %s", left)
				}
				ps.SemanticMappings = append(ps.SemanticMappings, SemanticMapping{
					DocType:      dotParts[0],
					Field:        dotParts[1],
					SemanticType: right,
				})
			}
		case "Rules":
			if strings.HasPrefix(line, "- ") {
				rule := parseRule(strings.TrimPrefix(line, "- "))
				if rule != nil {
					ps.Rules = append(ps.Rules, *rule)
				}
			}
		case "Decisions":
			if strings.HasPrefix(line, "- ") {
				decision := parseDecision(strings.TrimPrefix(line, "- "))
				if decision != nil {
					ps.Decisions = append(ps.Decisions, *decision)
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read POLICY.md: %w", err)
	}

	if len(ps.RequiredEvidence) == 0 {
		return nil, fmt.Errorf("missing required section: Required Evidence")
	}

	return ps, nil
}

func parseRule(line string) *PolicyRule {
	if strings.Contains(line, "must equal") {
		parts := strings.SplitN(line, " must equal ", 2)
		if len(parts) == 2 {
			return &PolicyRule{
				Type:  "equality",
				Left:  strings.TrimSpace(parts[0]),
				Right: strings.TrimSpace(parts[1]),
			}
		}
	}

	if strings.Contains(line, "variance over") && strings.Contains(line, "requires human review") {
		parts := strings.SplitN(line, " variance over ", 2)
		if len(parts) == 2 {
			rest := strings.SplitN(parts[1], "% requires", 2)
			if len(rest) == 2 {
				var threshold float64
				fmt.Sscanf(rest[0], "%f", &threshold)
				return &PolicyRule{
					Type:      "variance",
					Left:      strings.TrimSpace(parts[0]),
					Threshold: threshold,
				}
			}
		}
	}

	if strings.HasPrefix(line, "confidence below") && strings.Contains(line, "requires human review") {
		parts := strings.SplitN(line, "confidence below ", 2)
		if len(parts) == 2 {
			rest := strings.SplitN(parts[1], " requires", 2)
			if len(rest) == 2 {
				var threshold float64
				fmt.Sscanf(rest[0], "%f", &threshold)
				return &PolicyRule{
					Type:      "confidence",
					Threshold: threshold,
				}
			}
		}
	}

	if strings.Contains(line, "is incomparable to") {
		parts := strings.SplitN(line, " is incomparable to ", 2)
		if len(parts) == 2 {
			return &PolicyRule{
				Type:  "incomparable",
				Left:  strings.TrimSpace(parts[0]),
				Right: strings.TrimSpace(parts[1]),
			}
		}
	}

	return nil
}

func parseDecision(line string) *DecisionRule {
	for _, state := range []string{"PASS", "FAIL", "REVIEW", "MISSING_EVIDENCE"} {
		prefix := state + " when "
		if strings.HasPrefix(strings.ToUpper(line), prefix) {
			condition := strings.TrimSpace(line[len(prefix):])
			return &DecisionRule{
				State:     state,
				Condition: condition,
			}
		}
	}
	return nil
}
