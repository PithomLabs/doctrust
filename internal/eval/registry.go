package eval

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type Registry struct {
	rulesetsDir string
}

func NewRegistry(rulesetsDir string) *Registry {
	return &Registry{rulesetsDir: rulesetsDir}
}

func (reg *Registry) LoadPromoted(id string) (Ruleset, error) {
	dir := filepath.Join(reg.rulesetsDir, id)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return Ruleset{}, fmt.Errorf("read ruleset dir: %w", err)
	}

	var versions []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".yaml")
		if name == "working" {
			continue
		}
		versions = append(versions, name)
	}

	if len(versions) == 0 {
		return Ruleset{}, fmt.Errorf("no promoted versions found for %s", id)
	}

	// Sort versions naturally (v1, v2, v10, etc.)
	sort.Slice(versions, func(i, j int) bool {
		return versionNumber(versions[i]) < versionNumber(versions[j])
	})
	latest := versions[len(versions)-1]

	path := filepath.Join(dir, latest+".yaml")
	return LoadRuleset(path)
}

func (reg *Registry) LoadWorking(id string) (Ruleset, error) {
	path := filepath.Join(reg.rulesetsDir, id, "working.yaml")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		// Return empty ruleset with same ID if no working draft exists
		return Ruleset{ID: id, Version: "draft"}, nil
	}
	return LoadRuleset(path)
}

func (reg *Registry) SaveWorking(rs Ruleset) error {
	dir := filepath.Join(reg.rulesetsDir, rs.ID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("mkdir ruleset dir: %w", err)
	}

	path := filepath.Join(dir, "working.yaml")
	data, err := yaml.Marshal(rs)
	if err != nil {
		return fmt.Errorf("marshal working ruleset: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

func (reg *Registry) Promote(rs Ruleset) error {
	if err := rs.Validate(); err != nil {
		return fmt.Errorf("invalid ruleset: %w", err)
	}

	dir := filepath.Join(reg.rulesetsDir, rs.ID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("mkdir ruleset dir: %w", err)
	}

	// Determine next version number
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read ruleset dir: %w", err)
	}

	maxVersion := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".yaml")
		if name == "working" {
			continue
		}
		// Parse version number from filename (e.g., "v1" -> 1)
		if strings.HasPrefix(name, "v") {
			var v int
			if _, err := fmt.Sscanf(name, "v%d", &v); err == nil && v > maxVersion {
				maxVersion = v
			}
		}
	}

	nextVersion := maxVersion + 1
	promotedVersion := fmt.Sprintf("v%d", nextVersion)
	rs.Version = fmt.Sprintf("%d", nextVersion) // version field is the number, filename has v prefix

	promotedPath := filepath.Join(dir, promotedVersion+".yaml")
	data, err := yaml.Marshal(rs)
	if err != nil {
		return fmt.Errorf("marshal promoted ruleset: %w", err)
	}
	if err := os.WriteFile(promotedPath, data, 0644); err != nil {
		return fmt.Errorf("write promoted ruleset: %w", err)
	}

	// Save manifest
	manifest, err := rs.Manifest()
	if err != nil {
		return fmt.Errorf("compute manifest: %w", err)
	}
	manifestPath := filepath.Join(reg.rulesetsDir, rs.ID, promotedVersion+".manifest.json")
	if err := manifest.Save(manifestPath); err != nil {
		return fmt.Errorf("save manifest: %w", err)
	}

	// Clear working draft after promotion
	workingPath := filepath.Join(dir, "working.yaml")
	_ = os.Remove(workingPath) // ignore error if not exists

	return nil
}

func (reg *Registry) ListRulesets() ([]string, error) {
	entries, err := os.ReadDir(reg.rulesetsDir)
	if err != nil {
		return nil, fmt.Errorf("read rulesets dir: %w", err)
	}

	var ids []string
	for _, e := range entries {
		if e.IsDir() {
			ids = append(ids, e.Name())
		}
	}
	return ids, nil
}

// versionNumber extracts the numeric suffix from a version string (e.g., "v10" -> 10).
func versionNumber(v string) int {
	var n int
	fmt.Sscanf(strings.TrimPrefix(v, "v"), "%d", &n)
	return n
}