package eval

// DefaultRegistry returns the registry with all known checks.
// New checks added by the authoring pipeline are registered here during promotion.
func DefaultRegistry() *CheckRegistry {
	r := NewCheckRegistry()
	r.Register(&GrossIncomeConsistencyCheck{})
	r.Register(&RequiredDocumentsCheck{})
	r.Register(&NetVsGrossIncomparabilityCheck{})
	return r
}
