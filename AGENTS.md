# AGENTS.md — Reference for AI Agents

This file is the authoritative reference for AI agents working on this codebase. It defines locked architecture rules, canonical types, and forbidden patterns.

**Phase status:** Phases 1–4 frozen. **Phase 3 candidate-lifecycle trust funnel FROZEN** (adv_review4 P1-A fixed; real LLM authoring rehearsed end-to-end). See `plans6/` and `plans6/imp.md` for development history, adversarial reviews, and freeze evidence; `plans4/` for earlier phases.

---

## Locked Architecture Rules (NON-NEGOTIABLE)

These are frozen. Do not modify, reinterpret, or "improve" them.

1. **One canonical fact model**: `facts.Fact` is the observation. `evidence.EvidenceRef` is the output/projection. Map key owns semantic identity.
2. **ScenarioFact is YAML-only**: never stored in runtime types. Converted to `facts.Fact` via `ScenarioInputToFacts()`.
3. **No hardcoded scenario mapping**: scenarios map to checks via `expected.check_id`.
4. **No reference-policy fallback**: promoted Ruleset is authoritative. `LoadPromoted` failure → error, never silent fallback.
5. **No hot reload**: promoted Ruleset is immutable at server startup.
6. **No LLM at runtime**: deterministic checks only. LLM is compile-time only (candidate authoring).
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
17. **Registration symbol comes from AST extraction**: promoted checks register under `extractCheckStructName(snapshot.GoSource)` — the struct name the author actually wrote. Deriving type names from `check_id` is forbidden; the old `toTypeName` helper was deleted for exactly this reason. Natural LLM output owes nothing to any naming convention.
18. **Generated-check imports are allowlisted**: positive default-deny (`allowedImports` in `import_allowlist.go`; explicit denylist checked first). Candidate Go source may import only: `fmt`, `math`, `sort`, `strconv`, `strings`, `time`, and `internal/{eval,evidence,types,facts}`.
19. **ONE parameter resolver**: `compiler.ResolveRulesetParams` (Ruleset params win wholesale if present, else scenario fallback). Both staged regression and `cmd/regression` call it. Duplicating the algorithm anywhere breaks the invariant: staged regression PASS ≡ post-promotion regression PASS.
20. **Promotion gate order is fixed**: ValidateSnapshot → ExecuteCandidateScenarios → StagePromotion → ValidateStagedArtifact → RunStagedRegression → CommitPromotion. Any gate failure must leave the trusted tree (`internal/eval`, `rulesets/`, `scenarios/`) byte-for-byte unchanged.
21. **Approval is human-gated and content-bound**: approval requires an explicit trimmed `y`/`Y` after full adversarial YAML reprint; any other input cancels with zero state mutation. approval.json binds SHA-256 hashes of all candidate files plus reviewer identity (OS user or `DOCTRUST_REVIEWER`).
22. **The Gate-4 import-path assertion can never be waived**: if `go list -deps` itself errors, Gate 4 FAILS. The assertion exists to catch module-graph divergence — precisely when it must not be skipped.

---

## Canonical Flow

Upstream of the engine (agent side, plans10/11): an AI agent running the
`compliance-check-artifact` skill obtains evidence through `evidence-mcp`
(→ ingest → Nutrient), producing the `evidence_snapshot.json` consumed below.
The agent never constructs facts itself; raw provider output is normalized
inside the trusted ingest path.

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
    DocTypePaystub             DocumentType = "paystub"
    DocTypeW2                  DocumentType = "w2"
    DocType1040                DocumentType = "form_1040"
    DocTypeBankStmt            DocumentType = "bank_statement"
    DocTypeUnknown             DocumentType = "unknown"
    DocTypeCommercialInvoice   DocumentType = "commercial_invoice"
    DocTypePackingList         DocumentType = "packing_list"
    DocTypeBillOfLading        DocumentType = "bill_of_lading"
    DocTypeCertificateOfOrigin DocumentType = "certificate_of_origin"
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

Registration is **centralized**: `internal/eval/checks.go::DefaultRegistry()`.
All consumers (`cmd/regression`, `cmd/server`, `cmd/doctrust-mcp`) call
`eval.DefaultRegistry().All()` — no caller-side edits are needed.

### Path A: authored through the pipeline (recommended for new logic)

1. `bin/author-check --domain <domain> --intent "<text>"` (or hand-write the
   candidate files in the layout defined under
   [Candidate Lifecycle (Trust Funnel)](#candidate-lifecycle-trust-funnel--frozen))
2. Author the human adversarial scenario, then approve via
   `bin/review-check <candidate-dir>` with an explicit `y`
3. `bin/promote-check --candidate <dir> --domain <domain>` — transform,
   registration insert, Ruleset update, and scenario-corpus merge happen
   automatically inside the trust funnel

### Path B: hand-written check

1. Create `internal/eval/check_<name>.go` implementing the `Check` interface:

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

2. Register in `internal/eval/checks.go::DefaultRegistry()`:

```go
func DefaultRegistry() *CheckRegistry {
    r := NewCheckRegistry()
    r.Register(&GrossIncomeConsistencyCheck{})
    r.Register(&RequiredDocumentsCheck{})
    r.Register(&NetVsGrossIncomparabilityCheck{})
    r.Register(&MyCheck{}) // add here
    return r
}
```

3. Add scenario YAML files in `scenarios/<domain>/`
4. Add tests in `internal/eval/eval_test.go`
5. Rebuild (`make build`) before verify/runtime — registration compiles into
   every binary that links DefaultRegistry

Either path ends the same way: the check is registered in DefaultRegistry, its
scenarios live in the regression corpus, and its params flow from the promoted
Ruleset through `compiler.ResolveRulesetParams`.

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

## Candidate Lifecycle (Trust Funnel) — FROZEN

New check logic reaches the runtime only through this pipeline. All gates are
fail-closed; any rejection leaves `internal/eval`, `rulesets/`, `scenarios/`,
and the candidate directory byte-for-byte unchanged.

### State machine

```
DRAFT → HUMAN_REVIEW → APPROVED → PROMOTED
                     ↘ REJECTED
```

States live in `candidates/active/<check_id>/state`. After commit, the candidate
is archived byte-identically to `candidates/archive/<check_id>/` and marked
`PROMOTED`; the active directory is removed.

### Candidate directory layout

| File | Written by | Purpose |
|------|-----------|---------|
| `check.go` | author-check / human | candidate Go source (`package candidate`, imports eval via canonical path) |
| `metadata.yaml` | author-check / human | id, version, description, parameters (approved params source of truth) |
| `scenarios.yaml` | author-check / human | deterministic scenarios incl. expected results |
| `adversarial.yaml` | **human only** | ≥1 scenario with `origin: human_adversarial` |
| `state` | pipeline | current lifecycle state |
| `approval.json` | review-check | SHA-256 hashes of all four artifacts + reviewer identity + timestamp |

### Promotion gates (promote-check main(), fixed order)

| # | Function | Rejects on |
|---|----------|-----------|
| 1 | `GetState` | state ≠ APPROVED |
| 2 | `HasAdversarial` (filesystem) | no human_adversarial scenario |
| 3 | `SnapshotCandidate` + `VerifyApprovalAgainstSnapshot` | approval identity/content mismatch vs snapshot bytes |
| 4 | `ValidateSnapshot` | forbidden imports; duplicate Check-ID (registry or any ruleset); build/vet failure; nested go.mod in validation worktree; `go list -deps` assertion error or missing canonical import |
| 5 | `ExecuteCandidateScenarios` | compile failure, zero scenarios, any Expected≠Actual |
| 6 | `StagePromotion` | AST transform failure, registration-insert failure, no ruleset for domain |
| 7 | `ValidateStagedArtifact` | transformed artifact fails to compile inside full module-graph worktree (e.g., symbol collision) |
| 8 | `RunStagedRegression` | any corpus scenario fails under staged ruleset (production param semantics) |
| 9 | `CommitPromotion` | I/O failure → automatic rollback restores prior bytes |

Gate 4 early rejections return `(nil, err)` — callers must nil-guard detail printing.

Single-read invariant: everything downstream of Gate 3 consumes snapshot bytes;
the candidate directory is never re-read after approval verification.

### Key compiler API (internal/compiler)

```go
func SnapshotCandidate(candidateDir string) (*CandidateSnapshot, error)
// CandidateSnapshot: Dir, CheckID, Version, GoSource, Metadata, Scenarios,
//                    Adversarial, Parameters (from metadata.yaml)

func ValidateSnapshot(snapshot *CandidateSnapshot, registry *eval.CheckRegistry,
    rulesetsDir string) (*CandidateValidationResult, error)

func ExecuteCandidateScenarios(snapshot *CandidateSnapshot) (*CandidateExecutionResult, error)

func StagePromotion(snapshot *CandidateSnapshot, evalDir, domain,
    rulesetsDir string) (stagingDir string, err error)

func ValidateStagedArtifact(stagingDir, evalDir, repoRoot string) error
func RunStagedRegression(stagingDir, evalDir, domain, scenariosDir string) error
func CommitPromotion(stagingDir, evalDir, domain, rulesetsDir, scenariosRoot string,
    snapshot *CandidateSnapshot) error
func RollbackPromotion(stagingDir string) error

func ResolveRulesetParams(rs eval.Ruleset, checkID string,
    fallback map[string]any) map[string]any   // PURE — single source of truth

func applyCheckRef(rs eval.Ruleset, checkID, version string,
    params map[string]any) eval.Ruleset       // PURE — replace-or-append

func ValidateImports(goSource []byte) ([]ImportViolation, error)
func InsertCheckRegistration(checksGoPath, typeName string) error
func extractCheckStructName(goSource string) (string, error)
```

Semantics that must not drift:
- **Staging builds the working Ruleset from LoadWorking→LoadPromoted baseline +
  `applyCheckRef(rs, CheckID, Version, snapshot.Parameters)`** — identical inputs
  to what CommitPromotion writes; existing CheckRefs and Params survive.
- **ValidateStagedArtifact** copies the real module tree into a `.doctrust-staged-*`
  worktree, overlays staged files onto `internal/eval/`, compiles there.
- **ValidateSnapshot** does the same in a `.doctrust-validate-*` worktree
  (`copyModuleTree`, nested-go.mod tripwire, candidate compiled as its own package),
  then asserts the canonical import appears in `go list -deps ./candidate/`.
- **CommitPromotion** backs up every write target (incl. merged scenario file)
  and restores them on failure; rollback-of-rollback errors are joined, not swallowed.

### Rebuild requirement

Check registration compiles into `DefaultRegistry()` (`internal/eval/checks.go`).
After promoting a NEW Go check: `bin/promote` → `make build` → `verify-ruleset`
→ restart runtime/MCP. Stale binaries fail verify-ruleset closed ("not registered").
Ruleset-only changes need relaunch only.

### Environment variables

| Variable | Scope | Purpose |
|----------|-------|---------|
| `OPENROUTER_API_KEY` | authoring only | required by author-check; fails closed |
| `OPENROUTER_MODEL` | authoring only | optional model override (default claude-sonnet-4) |
| `DOCTRUST_REVIEWER` | review only | overrides OS username recorded as reviewer |
| `NUTRIENT_DWS_EXTRACTION_API_KEY` | ingest / evidence-mcp | Nutrient extraction credential; resolves from process env or `.env` via evidence-mcp `--env-file` (P5-7: never persisted into MCP registration config) |
| `DOCTRUST_SNAPSHOT_ROOT` | doctrust-mcp / evidence-mcp | allowed root for snapshot/document paths (path jail) |

### Frozen vs deferred (post-adv_review4)

Frozen: everything above. Deferred P2s — do NOT reopen without explicit instruction:
legacy `compiler.go` Nutrient linkage (P2-B); OpenRouter env docs were added to
README (P2-C closed); draft-sentinel ambiguity (P2-D); exact executed-count ==
expected-count (P2-E); auto manifest cross-check (P2-F); param shadowing cleanup
(P2-G); first-exported-struct selection (P2-H); orphaned temp worktrees on SIGKILL
(P2-I); Makefile verify convenience flags (P2-J).

Phase-6 deferred items (planned, NOT committed — do not present as shipped):
caller-authenticated human-only channel for `request_human_review`;
persistence of reviews/artifacts beyond a single case lifetime; multi-case
service operation; broader shipment evidence-contract field coverage beyond
gross weights + references + container/seal. See
`phase5/PHASE5_REPORT.md` Limitations.

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
- ❌ Deriving a registration type name from `check_id` (use `extractCheckStructName` on the candidate source)
- ❌ Duplicating parameter-resolution logic (call `compiler.ResolveRulesetParams`)
- ❌ Skipping or reordering promotion gates in `promote-check` main()
- ❌ Writing approval.json without an explicit trimmed `y`/`Y` adversarial confirmation
- ❌ Mutating any trusted-tree path when a gate fails (assert byte-for-byte equality in tests)
- ❌ Importing non-allowlisted packages in candidate Go source
- ❌ Treating a `go list -deps` error as a pass in Gate 4
- ❌ Running verify-ruleset or the runtime without rebuilding after promoting a new Go check
- ❌ Importing `internal/provider` from `internal/service` or `cmd/doctrust-mcp`
- ❌ Persisting provider credentials into MCP registration configuration
- ❌ Editing the derived Hermes skill copy independently of the canonical source
- ❌ Simulating evidence availability in rehearsals instead of real filesystem gating

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

# 6/6 shipment scenarios (shipment_release domain)
go test ./internal/eval/... -v -run TestRunAllShipmentScenarios

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

# Candidate lifecycle suites (trust funnel)
go test ./internal/compiler/...          # gates, staged build/regression, E2E + failure paths
go test ./cmd/promote-check/...          # duplicate-ID binary regression (P1-A)
go test ./cmd/review-check/...           # adversarial confirmation subprocess tests
go test ./cmd/verify-ruleset/...         # version/hash assertion exit codes
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

## Phase 5 Additions (shipment domain, provider seam, agent orchestration) — LOCKED

These rules are locked with the same authority as R1–R22. The existing rules
above remain verbatim; this section extends them to the shipment domain and
the agent-facing surfaces added in plans10/plans11.

23. **Provider seam**: `internal/provider` defines the generic
    `EvidenceProvider` contract (`ExtractFields` → `[]RawExtraction`).
    Sanctioned consumers are the ingest path ONLY (`cmd/ingest`,
    `cmd/evidence-mcp`). In addition to R13, `internal/service` and
    `cmd/doctrust-mcp` must NEVER import `internal/provider`. Enforced by
    `make lint-imports`.
24. **Trade-document canonical types**: `commercial_invoice`, `packing_list`,
    `bill_of_lading`, `certificate_of_origin` are canonical
    `types.DocumentType` values; the SourceDoc rule (R16) applies unchanged.
    Shipment snapshots additionally carry a graph-level `case_id`
    (`shipment_<sha256[:16]>`) derived from canonical snapshot CONTENT, while
    `LoadCase` continues to derive its case_id from raw snapshot BYTES. Both
    identifiers are recorded (snapshot field vs tool/audit output) and must
    not be conflated.
25. **Shipment check semantics are locked**:
    `required_shipment_documents` returns REVIEW/BLOCKING when required trade
    documents are missing — never FAIL and never PASS (the progressive-evidence
    workflow depends on insufficient-evidence REVIEW).
    `gross_weight_reconciliation` evaluates `all_equal` across the configured
    sources for semantic type `shipment.gross_weight` with tolerance default
    0.005 KG; missing sources ⇒ REVIEW/insufficient; mismatch ⇒ REVIEW naming
    outliers in reason and metrics; conflicting duplicate observations within
    one source ⇒ REVIEW.
26. **Compliance Skill deployment**: `skills/<name>/SKILL.md` is the CANONICAL
    source of truth. Runtime copies under `~/.hermes/skills/` are DERIVED and
    deployed only via `scripts/install-skill.sh`; independently editing the
    runtime copy is forbidden. Skill loading must be verified by both
    `hermes skills list` AND a live filtered run.
27. **Secrets never enter MCP registration configuration or logs.**
    Provider credentials resolve at process runtime from the launch environment
    or an `--env-file` PATH (e.g., `doctrust/.env`). Registrations carry only
    file PATHS. Credential values must never appear in plans, transcripts,
    screenshots, reports, or source control.
28. **Rehearsal evidence availability is filesystem-enforced.** Documents
    withheld from an adaptive run live OUTSIDE `DOCTRUST_SNAPSHOT_ROOT` until
    released; prompt-only unavailability is forbidden. Release happens only
    after the agent's documented choice following a genuine insufficient-
    evidence REVIEW.

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

### required_shipment_documents (v1.0) — shipment domain

Verifies the four trade-document types contributed evidence. **Missing
documents ⇒ REVIEW/BLOCKING** ("insufficient evidence: N of 4 … missing: …") —
never FAIL, never PASS; this REVIEW drives the progressive-evidence workflow.

**Params:** `required` (list; defaults to all four trade-document types).

### gross_weight_reconciliation (v1.0) — shipment domain

Reconciles `shipment.gross_weight` observations across the required trade
documents (`all_equal`).

**Params:** `semantic_type` (default `shipment.gross_weight`), `sources`
(default: all four trade types), `unit` (default KG), `tolerance`
(default 0.005).

**Logic:** group observations by canonical source; conflicting duplicates
within one source ⇒ REVIEW; any required source missing ⇒ REVIEW/insufficient
with `missing_sources` metrics; numeric mismatch ⇒ REVIEW/BLOCKING naming
outliers with per-source observations in metrics; agreement ⇒ PASS.

**Metrics:** `condition`, `unit`, `missing_sources` / `observations`,
`outliers`, `value`, `source_count`.

Scenario corpus: `scenarios/shipment_release/check_shipment_release.yaml`
(6 scenarios), executed by `TestRunAllShipmentScenarios`.
