package eval

import "fmt"

// CheckRegistry maps check IDs to implementations.
// Single source of truth for all consumers (server, regression, tests).
type CheckRegistry struct {
	checks map[string]Check
}

// NewCheckRegistry creates an empty registry.
func NewCheckRegistry() *CheckRegistry {
	return &CheckRegistry{checks: make(map[string]Check)}
}

// Register adds a check to the registry. Returns error on duplicate ID.
func (r *CheckRegistry) Register(c Check) error {
	if c == nil {
		return fmt.Errorf("check is nil")
	}
	id := c.ID()
	if id == "" {
		return fmt.Errorf("check ID is empty")
	}
	if _, exists := r.checks[id]; exists {
		return fmt.Errorf("check %s already registered", id)
	}
	r.checks[id] = c
	return nil
}

// Get retrieves a check by ID.
func (r *CheckRegistry) Get(id string) (Check, error) {
	c, ok := r.checks[id]
	if !ok {
		return nil, fmt.Errorf("check %s not found", id)
	}
	return c, nil
}

// All returns a copy of all registered checks.
func (r *CheckRegistry) All() map[string]Check {
	out := make(map[string]Check, len(r.checks))
	for k, v := range r.checks {
		out[k] = v
	}
	return out
}

// IDs returns all registered check IDs.
func (r *CheckRegistry) IDs() []string {
	ids := make([]string, 0, len(r.checks))
	for id := range r.checks {
		ids = append(ids, id)
	}
	return ids
}
