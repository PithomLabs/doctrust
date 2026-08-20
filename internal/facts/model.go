package facts

// Facts preserves all sources per semantic type with full observation identity
type Facts map[string][]Fact

// Fact represents a canonical observation with full provenance
type Fact struct {
    Value        any
    SourceDoc    string
    FieldName    string
    SourceSpan   string
    Confidence   float64
}