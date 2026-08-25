package eval

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/PithomLabs/doctrust/internal/facts"
	"gopkg.in/yaml.v3"
)

// ScenarioFact is a YAML-only deserialization struct.
// It carries SemanticType solely for grouping into the canonical Facts map.
// It is never used at runtime beyond the conversion in ScenarioInputToFacts.
type ScenarioFact struct {
	SemanticType string  `yaml:"semantic_type"`
	Value        any     `yaml:"value"`
	SourceDoc    string  `yaml:"source_doc"`
	FieldName    string  `yaml:"field"`
	SourceSpan   string  `yaml:"source_span"`
	Confidence   float64 `yaml:"confidence"`
}

// ScenarioInput holds the YAML-deserialized input for a scenario.
type ScenarioInput struct {
	Facts []ScenarioFact `yaml:"facts"`
}

func LoadScenario(path string) (Scenario, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Scenario{}, fmt.Errorf("read scenario: %w", err)
	}

	var wrapper struct {
		Scenarios []Scenario `yaml:"scenarios"`
	}
	if err := yaml.Unmarshal(data, &wrapper); err != nil {
		return Scenario{}, fmt.Errorf("unmarshal scenario: %w", err)
	}

	if len(wrapper.Scenarios) == 0 {
		return Scenario{}, fmt.Errorf("no scenarios found in %s", path)
	}

	// For single scenario files, return first; caller can iterate if multiple
	return wrapper.Scenarios[0], nil
}

func LoadAllScenarios(path string) ([]Scenario, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read scenarios: %w", err)
	}

	var wrapper struct {
		Scenarios []Scenario `yaml:"scenarios"`
	}
	if err := yaml.Unmarshal(data, &wrapper); err != nil {
		return nil, fmt.Errorf("unmarshal scenarios: %w", err)
	}

	return wrapper.Scenarios, nil
}

func LoadAllScenariosFromDir(dir string) ([]Scenario, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read scenario dir: %w", err)
	}

	var all []Scenario
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}
		scenarios, err := LoadAllScenarios(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		all = append(all, scenarios...)
	}
	return all, nil
}

// ScenarioInputToFacts converts YAML-deserialized ScenarioFacts into canonical Facts.
// SemanticType is used solely as the map key; it is not carried into facts.Fact.
func ScenarioInputToFacts(input ScenarioInput) facts.Facts {
	f := make(facts.Facts)
	for _, sf := range input.Facts {
		fact := facts.Fact{
			Value:      sf.Value,
			SourceDoc:  sf.SourceDoc,
			FieldName:  sf.FieldName,
			SourceSpan: sf.SourceSpan,
			Confidence: sf.Confidence,
		}
		f[sf.SemanticType] = append(f[sf.SemanticType], fact)
	}
	return f
}
