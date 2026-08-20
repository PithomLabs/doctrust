package eval

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"
)

type RulesetManifest struct {
	ID         string     `json:"id"`
	Version    string     `json:"version"`
	Checks     []CheckRef `json:"checks"`
	Hash       string     `json:"hash"`
	PromotedAt time.Time  `json:"promoted_at"`
}

func (r *Ruleset) ComputeHash() (string, error) {
	// Create a canonical representation for hashing
	type canonical struct {
		ID      string     `json:"id"`
		Version string     `json:"version"`
		Checks  []CheckRef `json:"checks"`
	}
	c := canonical{
		ID:      r.ID,
		Version: r.Version,
		Checks:  r.Checks,
	}

	// Sort checks for deterministic ordering
	checks := make([]CheckRef, len(r.Checks))
	copy(checks, r.Checks)
	sort.Slice(checks, func(i, j int) bool { return checks[i].ID < checks[j].ID })
	c.Checks = checks

	data, err := json.Marshal(c)
	if err != nil {
		return "", fmt.Errorf("marshal ruleset for hash: %w", err)
	}

	hash := sha256.Sum256(data)
	return fmt.Sprintf("%x", hash), nil
}

func (r *Ruleset) Manifest() (*RulesetManifest, error) {
	hash, err := r.ComputeHash()
	if err != nil {
		return nil, err
	}
	return &RulesetManifest{
		ID:         r.ID,
		Version:   r.Version,
		Checks:    r.Checks,
		Hash:      hash,
		PromotedAt: time.Now().UTC(),
	}, nil
}

func (m *RulesetManifest) Save(path string) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

func LoadManifest(path string) (*RulesetManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	var m RulesetManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("unmarshal manifest: %w", err)
	}
	return &m, nil
}