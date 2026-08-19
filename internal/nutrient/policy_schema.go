package nutrient

import (
	"encoding/json"
	"os"
)

type NutrientAdapter interface {
	ToExtractFieldsSchema(schema *ExtractionSchema) map[string]map[string]any
}

type ExtractionSchema struct {
	PolicyID string                   `json:"policy_id"`
	Schemas  map[string]DocExtraction `json:"extraction_schemas"`
}

type DocExtraction struct {
	Fields           []FieldSpec       `json:"fields"`
	SemanticMappings []SemanticMapping `json:"semantic_mappings"`
}

type FieldSpec struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type SemanticMapping struct {
	Field        string `json:"field"`
	SemanticType string `json:"semantic_type"`
}

type DefaultNutrientAdapter struct{}

func (a *DefaultNutrientAdapter) ToExtractFieldsSchema(schema *ExtractionSchema) map[string]map[string]any {
	result := make(map[string]map[string]any)
	for docType, docExtraction := range schema.Schemas {
		props := make(map[string]any)
		for _, field := range docExtraction.Fields {
			props[field.Name] = map[string]any{
				"type":        "string",
				"description": field.Name,
			}
		}
		result[docType] = map[string]any{
			"type":       "object",
			"properties": props,
		}
	}
	return result
}

// LoadExtractionSchema loads an extraction.json file and returns an ExtractionSchema.
func LoadExtractionSchema(path string) (*ExtractionSchema, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var schema ExtractionSchema
	if err := json.Unmarshal(data, &schema); err != nil {
		return nil, err
	}
	return &schema, nil
}
