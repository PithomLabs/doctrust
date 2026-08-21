# AGENTS.md — Reference for AI Agents

This file is the authoritative reference for AI agents working on this codebase. It defines locked architecture rules, canonical types, and forbidden patterns.

**Phase status:** Phases 1-4 frozen. Phase 1 corrective refactor complete (service layer migration, artifact hash fix, lifecycle invariant). See `plans4/` for development history.

---

## Locked Architecture Rules (NON-NEGOTIABLE)

These are frozen. Do not modify, reinterpret, or "improve" them.

1. **One canonical fact model**: `facts.Fact` is the observation. `evidence.EvidenceRef` is the output/projection. Map key owns semantic identity.
2. **ScenarioFact is YAML-only**: never stored in runtime types. Converted to `facts.Fact` via `ScenarioInputToFacts()`.
3. **No hardcoded scenario mapping**: scenarios map to checks via `expected.check_id`.
4. **No reference-policy fallback**: promoted Ruleset is authoritative. `LoadPromoted` failure → error, never silent fallback.
5. **No hot reload**: promoted Ruleset is immutable at server startup.
6. **No LLM at runtime**: deterministic checks only. LLM is compile-time only (Phase 3).
7. **No Nutrient dependency inside `internal/eval`**: eval engine works with any evidence source.
8. **Ingestion-agnostic**: the engine does not know or care where evidence comes from.
9. **No silent failures**: unknown documents → explicit state, never silently dropped.
10. **Evidence comparison uses set semantics**: not list order. `evidenceEqual()` compares Field+SourceDoc+SourceSpan+Confidence.
11. **Audit artifact carries Ruleset provenance**: `Artifact.SetRuleset()` must be called in both `handleFinalize` and `handleAudit`. The ruleset hash is computed from the same promoted Ruleset used for evaluation.
12. **Current coordinate contract**: evidence bboxes are treated as viewer-compatible top-left PDF-point coordinates; no transform is currently applied. Any change to the extraction/viewer coordinate contract requires an explicit verification and regression test.
13. **Provider boundary**: `internal/service` and `cmd/doctrust-mcp` must NEVER import `internal/nutrient`, `internal/extraction`, or `internal/opa`. Enforced by `make lint-imports`.
14. **Handler lifecycle**: Only `handleEvaluate` may call `LoadCase` + `Evaluate`. Later handlers (`handleReview`, `handleDisposition`, `handleFinalize`, `handleAudit`) must return an error if no case is loaded. They operate on the pinned case state.
15. **Artifact hash integrity**: `Finalize()` and `Hash()` use `hashable()` canonical form that excludes `Manifest.ArtifactHash` to break the self-referential cycle. `Finalize().ArtifactHash == Hash()` is guaranteed.
16. **SourceDoc canonicalization**: `Fact.SourceDoc` uses the canonical document TYPE from `snapshot.Documents[].Type` (e.g., "paystub", "w2"), NOT the filename. Enforced by `service.BuildFactsFromSnapshot`.

---

## Canonical Flow

```
ScenarioFact (YAML-only)
    ↓ ScenarioInputToFacts()
facts.Fact (canonical observation)
    ↓ collected into
facts.Facts (map[string][]Fact)
    ↓ Check.Evaluate()
Result (Status, Severity, Reason, Evidence)
    ↓ DecisionAggregator
Final decision (PASS | REVIEW | FAIL)
```

---

## Key Types

### internal/types (leaf package — zero internal imports)

```go
// DocumentType is the canonical document type identifier.
type DocumentType string

const (
    DocTypePaystub  DocumentType = "paystub"
    DocTypeW2       DocumentType = "w2"
    DocType1040     DocumentType = "form_1040"
    DocTypeBankStmt DocumentType = "bank_statement"
    DocTypeUnknown  DocumentType = "unknown"
)

// EvidenceRef is the pointer from a check result back to source evidence.
type EvidenceRef struct {
    Field      string  `json:"field" yaml:"field"`
    SourceDoc  string  `json:"source_doc" yaml:"source_doc"`
    SourceSpan string  `json:"source_span" yaml:"source_span"`
    Confidence float64 `json:"confidence" yaml:"confidence"`
}
```

### internal/evidence (model types only — no Nutrient/extraction deps)

```go
// Re-exported from types for convenience.
type DocumentType = types.DocumentType
type EvidenceRef = types.EvidenceRef

// EvidenceGraph is the complete evidence snapshot for a case.
type EvidenceGraph struct {
    CaseID        string         `json:"case_id"`
    Documents     []Document     `json:"documents"`
    Claims        []Claim        `json:"claims"`
    Relationships []Relationship `json:"relationships"`
    CreatedAt     time.Time      `json:"created_at"`
}

type Document struct {
    ID       string       `json:"id"`
    Filename string       `json:"filename"`
    Hash     string       `json:"hash"`
    Type     DocumentType `json:"type"`
}

type Claim struct {
    ID           string      `json:"id"`
    Field        string      `json:"field"`
    SemanticType string      `json:"semantic_type"`
    Value        any         `json:"value"`
    ValueType    string      `json:"value_type"`
    Sources      []Source    `json:"sources"`
    Status       ClaimStatus `json:"status"`
}
```

### internal/facts (Facts/Fact types)

```go
// Facts maps semantic type → all observations of that type.
type Facts map[string][]Fact

// Fact is a single canonical observation with full provenance.
type Fact struct {
    Value      any       // the extracted value
    SourceDoc  string    // canonical document type (paystub, w2, form_1040, bank_statement)
    FieldName  string    // field name within the document
    SourceSpan string    // page + bounding box (e.g. "page=1;bbox=[508,274,50,8]")
    Confidence float64   // [0..1]
}
```

### eval/check.go

```go
type Check interface {
    ID() string
    Version() string
    Evaluate(facts Facts, params map[string]any) Result
}

type Result struct {
    CheckID  string
    Status   Status     // PASS | REVIEW | FAIL
    Severity Severity   // INFO | WARNING | BLOCKING
    Reason   string
    Evidence []types.EvidenceRef
    Metrics  map[string]any
}

type Ruleset struct {
    ID      string
    Version string       // immutable once promoted
    Checks  []CheckRef
}

type CheckRef struct {
    ID      string
    Version string
    Params  map[string]any
}
```

### eval/runner.go

```go
type ScenarioResult struct {
    Passed       bool
    ScenarioName string
    Actual       Result
    Expected     Result
    Diff         *ScenarioDiff
}
```

### eval/diff.go

```go
type ScenarioDiff struct {
    Before           Result
    After            Result
    Changed          bool
    StatusChanged    bool
    SeverityChanged  bool
    ReasonChanged    bool
    EvidenceChanged  bool
}
```

### audit/artifact.go

```go
type Artifact struct {
    Version          string                 `json:"version"`
    PolicyID         string                 `json:"policy_id"`
    PolicyHash       string                 `json:"policy_hash"`
    RulesetID        string                 `json:"ruleset_id"`
    RulesetVersion   string                 `json:"ruleset_version"`
    RulesetHash      string                 `json:"ruleset_hash"`
    Decisions        []Decision             `json:"decisions"`
    Documents        []DocumentRecord       `json:"documents"`
    HumanReviews     []HumanReviewRecord    `json:"human_reviews,omitempty"`
    FinalDisposition string                 `json:"final_disposition,omitempty"`
    CreatedAt        time.Time              `json:"created_at"`
    CompletedAt      *time.Time             `json:"completed_at,omitempty"`
    Signatures       []Signature            `json:"signatures,omitempty"`
    Manifest         Manifest               `json:"manifest"`
}

func (a *Artifact) SetRuleset(id, version, hash string)
func (a *Artifact) Finalize()  // computes Manifest.ArtifactHash via hashable()
```

### internal/service (application boundary)

```go
// DocTrustService is the application boundary.
// It depends ONLY on: eval, facts, evidence (model), audit, review, types.
// It must NOT import: nutrient, extraction, opa.
type DocTrustService struct { /* unexported */ }

func NewDocTrustService(domain, rulesetsDir string) (*DocTrustService, error)
func (s *DocTrustService) LoadCase(ctx context.Context, snapshotPath string) error
func (s *DocTrustService) Evaluate(ctx context.Context) (*eval.Decision, error)
func (s *DocTrustService) GetDecision() *eval.Decision
func (s *DocTrustService) GetFinding(index int) (*eval.Result, error)
func (s *DocTrustService) GetEvidence(findingIndex int) ([]types.EvidenceRef, error)
func (s *DocTrustService) GetRuleset() RulesetInfo
func (s *DocTrustService) GetReviews() ([]*review.HumanReview, error)
func (s *DocTrustService) RequestHumanReview(findingIndex int, action review.FindingAction, note string) (string, error)
func (s *DocTrustService) BuildArtifact() (*audit.Artifact, error)

// BuildFactsFromSnapshot converts an EvidenceGraph into canonical Facts.
// SourceDoc is resolved from snapshot.Documents[].Type (canonical), NOT filename.
func BuildFactsFromSnapshot(snapshot *evidence.EvidenceGraph) (facts.Facts, error)
```

---

## Adding a New Check

1. Create `internal/eval/check_<name>.go`
2. Implement the `Check` interface:

```go
type MyCheck struct{}

func (c *MyCheck) ID() string      { return "my_check" }
func (c *MyCheck) Version() string { return "1.0" }
func (c *MyCheck) Evaluate(facts Facts, params map[string]any) Result {
    // Use facts["semantic_type"] to get observations
    // Use params["key"].(float64) for parameters
    // Return Result with Status, Severity, Reason, Evidence
}
```

3. Register in ALL of these files:
   - `cmd/regression/main.go` (line ~71)
   - `cmd/compare/main.go` (if exists)
   - `cmd/server/main.go` uses `eval.DefaultRegistry().All()` (no explicit registration needed)
4. Add scenario YAML files in `scenarios/<domain>/`
5. Add test in `internal/eval/eval_test.go`

**Check registration pattern** (must be identical in all locations):

```go
checks := map[string]eval.Check{
    "gross_income_consistency":     &eval.GrossIncomeConsistencyCheck{},
    "required_documents":           &eval.RequiredDocumentsCheck{},
    "net_vs_gross_incomparability": &eval.NetVsGrossIncomparabilityCheck{},
    "my_new_check":                 &eval.MyCheck{},  // add here
}
```

---

## Ruleset YAML Format

```yaml
id: "income_verification"          # domain identifier
version: "2"                       # set by Promote(), immutable after
checks:
  - id: "gross_income_consistency" # must match Check.ID()
    version: "1.0"                 # check version
    params:                        # optional per-check params
      tolerance: 0.03
      bonus_field: "bonus_compensation"
  - id: "required_documents"
    version: "1.0"
  - id: "net_vs_gross_incomparability"
    version: "1.0"
```

**Validation rules** (`Ruleset.Validate()`):
- `ID` must not be empty
- `Version` must not be empty
- At least 1 check
- Each CheckRef must have non-empty `ID` and `Version`

---

## Scenario YAML Format

```yaml
scenarios:
  - name: "pass_within_threshold"        # human-readable name
    origin: "real_fixture"                # real_fixture | human_adversarial | ai
    input:
      facts:
        - semantic_type: "gross_income_projected"  # Facts map key
          source_doc: "paystub"                     # canonical doc type
          field: "annualized_gross_ytd"             # field name
          value: 125000                             # numeric or string
          source_span: "page=1;bbox=[508,274,50,8]" # page + bbox
          confidence: 0.95                          # [0..1]
    params:                                # optional check params
      tolerance: 0.05                      # overrides ruleset params if used
    expected:
      check_id: "gross_income_consistency" # maps to Check.ID()
      status: "PASS"                       # PASS | REVIEW | FAIL
      severity: "INFO"                     # INFO | WARNING | BLOCKING
      reason: "Paystub gross within tolerance of W-2/1040"
      evidence:                            # set semantics comparison
        - field: "gross_income_projected"
          source_doc: "paystub"
          source_span: "page=1;bbox=[508,274,50,8]"
          confidence: 0.95
        - field: "gross_income_taxable"
          source_doc: "w2"
          source_span: "page=1;bbox=[224,120,44,8]"
          confidence: 0.95
```

**Evidence comparison rules:**
- Same set of unique field names (no extra, no missing)
- For each expected entry, at least one actual entry satisfies:
  - If expected `SourceDoc != ""`, must match exactly
  - If expected `SourceSpan != ""`, must match exactly
  - If expected `Confidence != 0`, within 0.01 tolerance
- Empty expected fields are treated as "don't care"

---

## Registry Lifecycle

```
rulesets/<domain>/working.yaml       # mutable draft
    ↓ Promote()
rulesets/<domain>/v<N>.yaml          # immutable promoted
rulesets/<domain>/v<N>.manifest.json # SHA-256 hash + timestamp
    ↓
working.yaml is deleted
```

**Version numbering:** `v1`, `v2`, `v3`, ... (natural sort via `versionNumber()`)

**Manifest content:**
```json
{
  "id": "income_verification",
  "version": "2",
  "checks": [...],
  "hash": "c63b2bca18da64e8...",
  "promoted_at": "2026-08-20T12:00:00Z"
}
```

---

## Regression Command

```bash
# Default: uses Ruleset params (the actual Phase 2 behavior)
bin/regression --domain income_verification

# Legacy: uses scenario-level params (for backwards compatibility)
bin/regression --domain income_verification --scenario-params

# JSON output with param_source audit trail
bin/regression --domain income_verification --json

# Compare against explicit draft file
bin/regression --domain income_verification --draft path/to/draft.yaml
```

**Param resolution (default mode):**
For each scenario, find the matching check in the Ruleset by `expected.check_id`. Use the Ruleset's params for that check. Fall back to scenario params only if the check is not in the Ruleset.

**Param resolution (legacy mode):**
Always use scenario-level params. Ruleset params are ignored.

---

## Forbidden Patterns

These are violations. Do not do them.

- ❌ Adding `context.Context` to `Check.Evaluate()` interface
- ❌ Using `results[0].Status` for aggregate decisions (use `DecisionAggregator`)
- ❌ Comparing evidence by list order (use set semantics via `evidenceEqual()`)
- ❌ Falling back to reference policy on Ruleset load failure
- ❌ Hardcoding check IDs in scenarios (use `expected.check_id`)
- ❌ Calling Nutrient API from `internal/eval`
- ❌ Modifying a promoted Ruleset YAML (immutable)
- ❌ Using substring matching for document type normalization (use exact canonical matches)
- ❌ Silently dropping unknown documents
- ❌ Returning `results[0]` as the aggregate decision
- ❌ Using `make test` or `make test-policy` without actual test files
- ❌ Returning bbox coordinates without verifying they match the viewer coordinate contract
- ❌ Accepting `finding_index` without validating against current findings
- ❌ Omitting `Artifact.SetRuleset()` in finalize or audit handlers
- ❌ Using `PSPDFKit.Annotations.RectangleAnnotation` (use `NutrientViewer.Annotations.RectangleAnnotation`)
- ❌ Importing `internal/nutrient`, `internal/extraction`, or `internal/opa` from `internal/service` or `cmd/doctrust-mcp`
- ❌ Calling `LoadCase` or `Evaluate` from any handler other than `handleEvaluate`
- ❌ Using `SourceDoc: src.Filename` in Fact construction (must use canonical document type)
- ❌ Using `Hash()` after `Finalize()` without the `hashable()` canonical form

---

## Verification Commands

```bash
# Build and vet
go build ./...
go vet ./...

# Full test suite
go test ./...

# 14/14 strict scenario regression
go test ./internal/eval/... -v -run TestRunAllScenarios

# Regression (ruleset params, the default)
bin/regression --domain income_verification

# Registry inspection
bin/registry

# Dry-run promotion
bin/promote --domain income_verification --dry-run

# Provider boundary check
make lint-imports

# Service tests
go test ./internal/service/... -v

# Artifact hash integrity
go test ./internal/audit/... -v -run TestArtifact
```

---

## Server Trust Tests

```bash
# Phase 4 trust property tests
go test ./cmd/server/... -v

# Specific trust tests
go test ./cmd/server/... -v -run TestHandleRuleset
go test ./cmd/server/... -v -run TestHandleEvaluate
go test ./cmd/server/... -v -run TestHandleFinalize
go test ./cmd/server/... -v -run TestHandleReview
go test ./cmd/server/... -v -run TestLifecycle_NoReEvaluation
```

**Test coverage:**

| Test | Trust property |
|------|----------------|
| `TestHandleRuleset_ReturnsLoadedRuleset` | Ruleset identity matches promoted Ruleset |
| `TestHandleEvaluate_ReturnsEnrichedFields` | Enriched fields present in response |
| `TestHandleEvaluate_BboxFromSnapshot` | Bbox values flow from snapshot to response |
| `TestHandleFinalize_ArtifactContainsRulesetProvenance` | Artifact has Ruleset ID/version/hash |
| `TestHandleReview_InvalidFindingIndex` | Negative index rejected (400) |
| `TestHandleReview_OutOfRangeFindingIndex` | Out-of-range index rejected (400) |
| `TestHandleReview_ValidFindingIndex` | Valid review stored correctly |
| `TestParseSourceSpan` | SourceSpan parsing correctness |
| `TestBuildFactsFromSnapshot_UsesCanonicalBuilder` | HTTP path uses service builder |
| `TestLifecycle_NoReEvaluation` | Later handlers never re-load or re-evaluate |

---

## Existing Checks Reference

### gross_income_consistency (v1.0)

Compares paystub projected gross income against W-2 taxable income.

**Params:**
- `tolerance` (float64, default 0.05): maximum allowed variance ratio
- `bonus_field` (string, default "bonus_compensation"): semantic type for bonus lookup

**Logic:**
1. Extract `gross_income_projected` (paystub) and `gross_income_taxable` (W-2 by document identity)
2. Compute variance: `|paystub - w2| / w2`
3. If bonus present and variance > tolerance → REVIEW with bonus context
4. If variance > tolerance → REVIEW with percentage
5. If within tolerance → PASS

**Metrics returned:** `paystub_gross`, `w2_gross`, `variance_pct`, `tolerance_pct`

### required_documents (v1.0)

Verifies all required document types are present.

**Required:** `["paystub", "w2", "form_1040"]`

**Logic:**
1. Collect all `SourceDoc` values from all facts
2. Normalize via `normalizeDocType()` (exact canonical match only)
3. If any required doc missing → FAIL/BLOCKING
4. If all present → PASS/INFO

### net_vs_gross_incomparability (v1.0)

Semantic guard preventing false contradictions between net cash flow and gross taxable income.

**Logic:**
1. Check if `net_cash_flow` exists in facts
2. Check if `gross_income_taxable` exists in facts
3. If either missing → REVIEW/WARNING
4. If both present → PASS/INFO ("correctly treated as incomparable")

**Never produces FAIL.** Its purpose is to confirm the system recognizes semantic incomparability.
