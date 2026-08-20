package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/doctrust/doctrust/internal/nutrient"
)

func main() {
	data, _ := os.ReadFile("/home/chaschel/Desktop/biz/nutrient/doctrust/.env")
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if i := strings.Index(line, "="); i >= 0 {
			os.Setenv(line[:i], line[i+1:])
		}
	}
	key := os.Getenv("NUTRIENT_DWS_EXTRACTION_API_KEY")
	c := nutrient.NewClient(key, "")

	docs := []struct {
		file  string
		schema map[string]any
	}{
		{"demo/income_verification/1_Paystub_2025.pdf", map[string]any{
			"type": "object", "properties": map[string]any{
				"annualized_gross_ytd": map[string]any{"type": "string", "description": "Annualized YTD gross earnings"},
				"base_salary_ytd":      map[string]any{"type": "string", "description": "YTD base salary"},
				"bonus_ytd":            map[string]any{"type": "string", "description": "YTD bonus"},
			}}},
		{"demo/income_verification/2_W2_Form_2025.pdf", map[string]any{
			"type": "object", "properties": map[string]any{
				"wages_tips_other_compensation": map[string]any{"type": "string", "description": "Box 1 wages"},
			}}},
		{"demo/income_verification/3_Form1040_TaxReturn_2025.pdf", map[string]any{
			"type": "object", "properties": map[string]any{
				"line1z_wages": map[string]any{"type": "string", "description": "Line 1z total wages"},
			}}},
		{"demo/income_verification/4_BankStatement_Q4_2025.pdf", map[string]any{
			"type": "object", "properties": map[string]any{
				"total_deposits": map[string]any{"type": "string", "description": "Total deposits"},
			}}},
	}
	for _, d := range docs {
		res, err := c.ExtractFields(d.file, d.schema, "understand")
		if err != nil {
			fmt.Fprintln(os.Stderr, d.file, err)
			continue
		}
		b, _ := json.MarshalIndent(res, "", "  ")
		fmt.Println("====", d.file, "====")
		fmt.Println(string(b))
	}
}