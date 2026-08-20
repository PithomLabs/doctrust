package eval

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

func LoadRuleset(path string) (Ruleset, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Ruleset{}, fmt.Errorf("read ruleset: %w", err)
	}

	var rs Ruleset
	if err := yaml.Unmarshal(data, &rs); err != nil {
		return Ruleset{}, fmt.Errorf("unmarshal ruleset: %w", err)
	}
	return rs, nil
}

func LoadRulesetFromDir(dir string) ([]Ruleset, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read ruleset dir: %w", err)
	}

	var all []Ruleset
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}
		rs, err := LoadRuleset(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		all = append(all, rs)
	}
	return all, nil
}

func (r *Ruleset) Validate() error {
	if r.ID == "" {
		return fmt.Errorf("ruleset id is required")
	}
	if r.Version == "" {
		return fmt.Errorf("ruleset version is required")
	}
	if len(r.Checks) == 0 {
		return fmt.Errorf("ruleset must have at least one check")
	}
	for _, ref := range r.Checks {
		if ref.ID == "" {
			return fmt.Errorf("check reference id is required")
		}
		if ref.Version == "" {
			return fmt.Errorf("check reference version is required for %s", ref.ID)
		}
	}
	return nil
}