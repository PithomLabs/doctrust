# DocTrust — Universal Document Compliance Engine

Nutrient extraction → frozen evidence → deterministic evaluation → human review → audit artifact.

**Demo target:** September 4, 2026

---

## Architecture

```
Nutrient DWS extracts evidence from source PDFs
        ↓
Evidence snapshot (frozen, SHA-256 bound to documents)
        ↓
Eval engine evaluates against promoted Ruleset (deterministic, no LLM)
        ↓
Machine finding = REVIEW  (structured, reproducible, auditable)
        ↓
Human verifies source in Nutrient Viewer
        ↓
Final disposition = PASS  (human judgment, not re-evaluation)
```

### Core Invariants

- **Ingestion-agnostic**: no Nutrient dependency inside `internal/eval`
- **Zero LLM at runtime**: deterministic checks only
- **No hot reload**: promoted Ruleset is immutable at startup
- **No fallback to reference policy**: promoted Ruleset is authoritative
- **No silent failures**: unknown documents → explicit state, never silently dropped
- **Map key owns semantic identity**: `Facts[string]Fact` where string is the semantic type
- **EvidenceRef is output/projection**: `facts.Fact` is the canonical observation

---

## Quick Start

```bash
# Build all binaries
make build

# Run the test suite (14/14 strict scenarios)
make test

# Run scenario regression (default: uses Ruleset params)
make regression

# Promote working draft to next immutable version
make promote

# Inspect registry state
make registry
```

---

## Directory Structure

```
doctrust/
  cmd/                          # CLI commands
    regression/                 # Scenario regression (Phase 2)
    promote/                    # Ruleset promotion (Phase 2)
    registry/                   # Registry inspection (Phase 2)
    server/                     # HTTP API (Phase 4)
    eval/                       # OPA standalone evaluator (reference)
    compare/                    # OPA vs eval cross-validation (reference)
    validate-fixtures/          # OPA fixture validation (reference)
    ingest/                     # Nutrient extraction pipeline
    compile-policy/             # POLICY.md → Rego compiler (Phase 3)
  internal/
    eval/                       # Core eval engine (locked architecture)
    facts/                      # Canonical facts model
    evidence/                   # EvidenceRef, EvidenceGraph
    opa/                        # OPA SDK wrapper (reference)
    compiler/                   # LLM policy compiler (Phase 3)
    nutrient/                   # Nutrient DWS client
    audit/                      # Audit artifact + PDF report
    review/                     # Human review store + disposition
    extraction/                 # Extraction config (shared)
  rulesets/                     # Ruleset registry (versioned)
    income_verification/
      v1.yaml                   # Promoted v1 (tolerance=5%)
      v1.manifest.json          # SHA-256 hash + timestamp
      v2.yaml                   # Promoted v2 (tolerance=3%)
      v2.manifest.json
  scenarios/                    # Scenario YAML files
    income_verification/
      check_gross_income_variance.yaml  (6 scenarios)
      check_required_docs.yaml          (5 scenarios)
      check_net_vs_gross.yaml           (3 scenarios)
  fixtures/                     # Test fixtures
  policies/                     # Rego policies (reference/migration)
  web/                          # Server UI templates
  scripts/                      # PDF generation
```

---

## Eval Engine

### Checks

Each check implements the `Check` interface:

```go
type Check interface {
    ID() string
    Version() string
    Evaluate(facts Facts, params map[string]any) Result
}
```

**Three registered checks:**

| Check ID | Purpose | Key Params |
|----------|---------|------------|
| `gross_income_consistency` | Compares paystub projected gross vs W-2 taxable income | `tolerance` (default 5%), `bonus_field` |
| `required_documents` | Verifies all required document types are present | none |
| `net_vs_gross_incomparability` | Semantic guard: net cash flow ≠ gross taxable income | none |

### Ruleset

An ordered list of check references with per-check parameter overrides:

```yaml
id: "income_verification"
version: "2"
checks:
  - id: "gross_income_consistency"
    version: "1.0"
    params:
      tolerance: 0.03
      bonus_field: "bonus_compensation"
  - id: "required_documents"
    version: "1.0"
  - id: "net_vs_gross_incomparability"
    version: "1.0"
```

### Registry

Filesystem-based versioning:

```
rulesets/income_verification/
  working.yaml                 # Mutable draft (promote deletes this)
  v1.yaml                      # Immutable promoted v1
  v1.manifest.json             # SHA-256 hash + timestamp
  v2.yaml                      # Immutable promoted v2
  v2.manifest.json
```

- `LoadPromoted(id)` → latest version
- `LoadWorking(id)` → draft (or empty ruleset)
- `SaveWorking(rs)` → writes `working.yaml`
- `Promote(rs)` → validates, assigns next version, writes YAML + manifest, deletes draft

### Runner

```go
runner := eval.NewRunner(checks)
result := runner.RunScenario(ctx, scenario)    // single scenario
results := runner.RunAllScenarios(ctx, all)    // all scenarios
results, err := runner.RunRuleset(ctx, rs, f)  // ruleset against facts
```

### Aggregator

`DecisionAggregator` reduces multiple check results to a single decision:

1. **FAIL** if any result is `FAIL` with `BLOCKING` severity
2. **REVIEW** if any result is `REVIEW`
3. **PASS** otherwise

### Diff

`DiffResults(after, before Result)` compares two results using set semantics for evidence. Returns `nil` if identical, `*ScenarioDiff` with boolean flags for each changed dimension.

---

## Ruleset Evolution Workflow

### Step 1: Inspect current state

```bash
bin/registry
# income_verification: v1 (3 checks)
```

### Step 2: Create working draft

```bash
cp rulesets/income_verification/v1.yaml rulesets/income_verification/working.yaml
# Edit: tolerance 0.05 → 0.03
```

### Step 3: Run regression

```bash
bin/regression --domain income_verification
```

Output:
```
REGRESSION REPORT: income_verification
Param mode: ruleset (default)

Baseline:  income_verification v1 (tolerance=0.05)
Candidate: income_verification v1 (tolerance=0.03)
Scenarios: 14 total

* pass_within_threshold

  Before:  PASS / INFO
    reason: Paystub gross within tolerance of W-2/1040
  After:   REVIEW / WARNING
    reason: Paystub projected gross exceeds corroborated taxable income by 4.2% (tolerance: 3.0%)
  Changes: status, severity, reason
  Params:  ruleset → ruleset

---
Total: 14 | Changed: 4 | Passed: 10 | Failed: 4
```

### Step 4: Approve and promote

```bash
bin/promote --domain income_verification
# Promoted: income_verification v2
```

### Step 5: Runtime uses new version

```bash
bin/registry
# income_verification: v2 (3 checks, latest)
```

---

## CLI Reference

| Command | Purpose |
|---------|---------|
| `bin/regression --domain <domain>` | Run scenario regression (default: Ruleset params) |
| `bin/regression --domain <domain> --scenario-params` | Legacy mode (scenario-level params) |
| `bin/regression --domain <domain> --json` | JSON output with audit trail |
| `bin/promote --domain <domain>` | Promote working draft to next version |
| `bin/promote --domain <domain> --dry-run` | Validate only, don't promote |
| `bin/registry` | List all rulesets with versions |
| `bin/registry --domain <domain>` | Show details for one ruleset |
| `bin/server --domain <domain>` | HTTP API server |
| `bin/eval --policy <path> <snapshot>` | OPA standalone evaluator (reference) |
| `bin/compare <snapshot>` | Cross-validate OPA vs eval engine (reference) |

---

## Verification Commands

```bash
go build ./...                                          # must compile
go vet ./...                                            # must be clean
go test ./...                                           # all tests pass
go test ./internal/eval/... -v -run TestRunAllScenarios # 14/14 strict
bin/regression --domain income_verification             # baseline = 0 changed
bin/registry                                            # shows all versions
```

---

## Phase Status

| Phase | Status | Description |
|-------|--------|-------------|
| Phase 1.3 | Frozen | Nutrient integration, evidence normalization, 14/14 strict scenarios |
| Phase 2 | Frozen | Regression CLI, promote CLI, registry CLI, Ruleset params default |
| Phase 3 | Pending | LLM Policy Compiler (POLICY.md → Rego) |
| Phase 4 | Pending | Human Review + Nutrient Viewer integration |
| Phase 5 | Pending | Signing + Audit Artifact |

---

## Dependencies

- Go 1.25.7
- `github.com/open-policy-agent/opa` v1.4.2 (OPA SDK, reference/migration tooling)
- `github.com/jung-kurt/gofpdf/v2` v2.17.3 (PDF generation for audit reports)
- `gopkg.in/yaml.v3` v3.0.1 (YAML parsing for rulesets and scenarios)
