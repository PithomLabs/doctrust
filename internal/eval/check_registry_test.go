package eval

import (
	"testing"
)

func TestCheckRegistry(t *testing.T) {
	t.Run("register and get", func(t *testing.T) {
		r := NewCheckRegistry()
		check := &GrossIncomeConsistencyCheck{}
		if err := r.Register(check); err != nil {
			t.Fatalf("register: %v", err)
		}
		got, err := r.Get("gross_income_consistency")
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.ID() != "gross_income_consistency" {
			t.Errorf("got ID %s, want gross_income_consistency", got.ID())
		}
	})

	t.Run("duplicate rejection", func(t *testing.T) {
		r := NewCheckRegistry()
		r.Register(&GrossIncomeConsistencyCheck{})
		err := r.Register(&GrossIncomeConsistencyCheck{})
		if err == nil {
			t.Error("duplicate registration should fail")
		}
	})

	t.Run("nil check", func(t *testing.T) {
		r := NewCheckRegistry()
		err := r.Register(nil)
		if err == nil {
			t.Error("nil check should fail")
		}
	})

	t.Run("empty ID", func(t *testing.T) {
		r := NewCheckRegistry()
		err := r.Register(&emptyIDCheck{})
		if err == nil {
			t.Error("empty ID should fail")
		}
	})

	t.Run("get nonexistent", func(t *testing.T) {
		r := NewCheckRegistry()
		_, err := r.Get("nonexistent")
		if err == nil {
			t.Error("get nonexistent should fail")
		}
	})

	t.Run("all returns copy", func(t *testing.T) {
		r := NewCheckRegistry()
		r.Register(&GrossIncomeConsistencyCheck{})
		r.Register(&RequiredDocumentsCheck{})
		all := r.All()
		if len(all) != 2 {
			t.Errorf("got %d checks, want 2", len(all))
		}
		// Modify the copy should not affect registry
		all["new_check"] = &NetVsGrossIncomparabilityCheck{}
		if _, err := r.Get("new_check"); err == nil {
			t.Error("modifying All() copy should not affect registry")
		}
	})

	t.Run("IDs", func(t *testing.T) {
		r := NewCheckRegistry()
		r.Register(&GrossIncomeConsistencyCheck{})
		r.Register(&RequiredDocumentsCheck{})
		ids := r.IDs()
		if len(ids) != 2 {
			t.Errorf("got %d IDs, want 2", len(ids))
		}
	})

	t.Run("default registry has 3 checks", func(t *testing.T) {
		r := DefaultRegistry()
		all := r.All()
		if len(all) != 3 {
			t.Errorf("DefaultRegistry has %d checks, want 3", len(all))
		}
	})
}

type emptyIDCheck struct{}

func (c *emptyIDCheck) ID() string      { return "" }
func (c *emptyIDCheck) Version() string { return "1.0" }
func (c *emptyIDCheck) Evaluate(facts Facts, params map[string]any) Result {
	return Result{}
}
