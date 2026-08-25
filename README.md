# DocTrust — Universal Document Compliance Engine

> DocTrust is a compliance execution layer that lets AI agents investigate business documents against approved policy — without ever letting the agent become the authority that decides compliance.

**Demo target:** September 4, 2026 · **Business case:** [`BUSINESS_CASE.md`](BUSINESS_CASE.md)

## Problem

Regulated document workflows — trade compliance, KYC/AML, insurance claims, mortgage appraisal, e-invoicing — either rely on manual reconciliation that doesn't scale, or on “AI reads the PDF” tools whose decisions are unauditable and indefensible to a regulator. DocTrust solves the authority gap: agents can investigate, but only approved Rulesets evaluated deterministically (plus a signed human authority when required) can decide compliance. Shipment release — a single gross-weight mismatch across four trade documents triggering customs penalties — makes the cost concrete and is the first vertical wedge.

Nutrient extraction → frozen evidence → deterministic evaluation → human review → audit artifact.
AI-authored rules enter through a human-gated trust funnel before reaching the runtime.
The agent-facing runtime ships as MCP tools (`doctrust-mcp` · `evidence-mcp`),
operated by AI agents (Hermes or any MCP-capable harness) under a compliance skill:
**the agent investigates; DocTrust enforces the approved policy.**

---

## Architecture

Two live compliance domains ship today: `income_verification` (authoring
reference corpus) and `shipment_release` (live demo on the frozen Shipment-1047
PDF fixtures).

Canonical agent-side interaction (verified end-to-end in Phase 5):

```
END USER
    ↓  "Do these shipment PDFs pass the release policy?"
HERMES / AI AGENT   — runs the compliance-check-artifact skill
    ├──► evidence-mcp ──► Nutrient DWS ──► normalized evidence snapshot
    │        (progressive: only available documents are submitted;
    │         missing required documents surface at evaluation)
    └──► doctrust-mcp ──► deterministic Ruleset evaluation
                              ↓
                     PASS / REVIEW / FAIL  (+ structured findings,
                     page/bbox provenance, case/audit IDs)
    ↓
REVIEW escalates to human authority · audit artifact sealed
```

Authoring path (how check logic enters the trusted tree):

```
OpenRouter author-check generates a candidate check
        ↓
Human authors adversarial scenario + explicit y/N confirmation
        ↓
promote-check trust funnel (9 gates, single-read snapshot)
        ↓
Staged build + staged regression (production-identical semantics)
        ↓
Atomic promotion: Go check + registry + Ruleset draft + scenario corpus
        ↓
bin/promote freezes immutable Ruleset version + manifest
        ↓
make build → verify-ruleset → restart runtime / MCP
        ↓
Hermes/agent evaluate_case against the promoted Ruleset

─────────────────────────────────────────────────────────────
Runtime flow (per case):
─────────────────────────────────────────────────────────────
Nutrient DWS extracts evidence from source PDFs
        ↓
Evidence snapshot (frozen, SHA-256 bound to documents)
        ↓
DocTrustService (provider-agnostic, no Nutrient deps)
        ↓
BuildFactsFromSnapshot (canonical SourceDoc, BBox pass-through)
        ↓
Eval engine evaluates against promoted Ruleset (deterministic, no LLM)
        ↓
Machine Decision = PASS / REVIEW / FAIL  (structured, reproducible, auditable)
        ↓
Human Review
        ↓
Final Disposition
        ↓
Audit artifact (Ruleset identity, decisions, reviews, SHA-256 hash)
        ↓
Signed PDF report (cryptographic binding)
```

### Core Invariants

- **Ingestion-agnostic**: no Nutrient dependency inside `internal/eval`
- **Provider boundary**: `internal/service` has zero Nutrient/extraction/opa dependencies
- **Zero LLM at runtime**: deterministic checks only
- **No hot reload**: promoted Ruleset is immutable at startup
- **No fallback to reference policy**: promoted Ruleset is authoritative
- **No silent failures**: unknown documents → explicit state, never silently dropped
- **Map key owns semantic identity**: `Facts[string]Fact` where string is the semantic type
- **EvidenceRef is output/projection**: `facts.Fact` is the canonical observation
- **Single evaluation entry point**: Only `/api/evaluate` loads and evaluates; later handlers operate on pinned case
- **SourceDoc canonicalization**: `Fact.SourceDoc` uses document type from snapshot, NOT filename
- **Import allowlist**: generated check source may import only allowlisted packages (positive default-deny)
- **AST-name registration**: promoted checks register under the struct name the author actually wrote — no naming convention is imposed
- **Staged ≡ production**: one parameter resolver (`compiler.ResolveRulesetParams`) feeds both staged regression and production regression
- **Trusted-tree immutability**: any gate failure leaves eval/rulesets/scenarios byte-for-byte unchanged
- **Provider seam**: `internal/provider.EvidenceProvider` is the generic extraction contract; consumers are the ingest path only (`cmd/ingest`, `cmd/evidence-mcp`)
- **Skill canonical/derived contract**: `skills/<name>/SKILL.md` is the source of truth; the `~/.hermes` copy deploys solely via `scripts/install-skill.sh`
- **Secrets stay out of MCP config**: provider credentials resolve from the process environment or an `--env-file` PATH at runtime
- **Rehearsal availability is filesystem-gated**: withheld documents live outside `DOCTRUST_SNAPSHOT_ROOT` until chosen; prompt-only unavailability is forbidden

---

## Quick Start

## 60-Second Verification

For judges and engineers who want to verify the core claims without reading the full forensic walkthrough:

**1. Ruleset integrity**

```bash
bin/verify-ruleset --domain shipment_release
```

Verifies: the promoted shipment_release Ruleset and its manifest integrity.

**2. Audit integrity**

```bash
bin/verify-audit demo/shipment_release/runs/p6-20260825-015819-726232/available/evidence_snapshot.json
```

Verifies: the recorded audit artifact, snapshot binding, Ruleset binding, and Ed25519 signature integrity.

**3. Agent authority boundary**

Inspect:

```text
demo/shipment_release/runs/p6-20260825-015819-726232/denied_proof.jsonl
```

Verifies: the MCP evidence records the attempted `request_human_review` call being rejected because the capability is absent (R29 enforced).

---

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

### Shipment release — live demo path (Phase 5)

```bash
export NUTRIENT_DWS_EXTRACTION_API_KEY=your-key-here    # required for live extraction
make build                                   # 16 binaries incl. evidence-mcp, verify-audit

./scripts/install-skill.sh                   # deploy Compliance Skill to Hermes
hermes mcp add doctrust --command "$PWD/bin/doctrust-mcp" \
    --args --domain shipment_release --rulesets-dir "$PWD/rulesets" --snapshot-root "$PWD/demo/shipment_release"
hermes mcp add evidence --command "$PWD/bin/evidence-mcp" \
    --args --snapshot-root "$PWD/demo/shipment_release" --env-file "$PWD/.env"
# evidence-mcp needs NUTRIENT_DWS_EXTRACTION_API_KEY.
# Either export it in your shell, or pass --env-file pointing to a .env with the key.
# If the binary runs from bin/, it auto-discovers doctrust/.env without the flag.

bin/ingest -domain shipment_release \
    -docs "commercial_invoice=<pdf>,packing_list=<pdf>,bill_of_lading=<pdf>,certificate_of_origin=<pdf>" \
    -report g1/extraction_report.json

./scripts/rehearse-hermes-shipment.sh        # real adaptive Hermes run + assertions A1–A10b
./scripts/failure-rehearsals.sh              # trust-boundary failure rehearsals F1–F5

# Human authority (Phase 6): the agent can NEVER approve/reject.
bin/doctrust-review --provision owner --publish-to demo/shipment_release/reviewers
bin/doctrust-review --snapshot <case>/evidence_snapshot.json --reviewer owner
./scripts/rehearse-phase6-human.sh           # denied agent attempt + authorized human resolution proofs
```

Reports: `g1/G1_REPORT.md` (live extraction proof) ·
`phase5/PHASE5_REPORT.md` (adaptive orchestration) ·
`phase6/PHASE6_REPORT.md` (human authority).

**Video provenance**: The demo composes two genuine historical executions (Phase-5 progressive evidence + Phase-6 human authority) into one narrative. Different case IDs prove they are separate runs. S08 is constructed (roadmap only). For independent verification, see `docs/DEMO_ENGINEERING_GUIDE.md`.

**Audit verification**: S07 shows the sealed audit artifact. Independent verification: `docs/DEMO_ENGINEERING_GUIDE.md`.

**PASS example:**

```bash
make demo-pass
# → PASS — all four gross weights reconcile at 4,650 KG
# audit: <run>/audit.json  (ruleset hash, findings, evidence page/bbox, artifact hash)
```

**REVIEW + human-authority example:**

```bash
make demo-review
# → REVIEW / BLOCKING — B/L 5,150 KG vs 4,650 KG on the other three
# → human sees grounded evidence (page/bbox), enters decision + passphrase
# → Ed25519-signed review, DocTrust verifies, seals FAIL
# → audit artifact with reviewer identity + final disposition
```

### Credentials

The canonical operator path is to export the Nutrient API key before running demos:

```bash
export NUTRIENT_DWS_EXTRACTION_API_KEY=your-key-here
make demo-pass
```

The demo scripts inherit this from the shell environment. They do NOT source `.env`. The Go binary's `loadExtractionKey()` checks `os.Getenv("NUTRIENT_DWS_EXTRACTION_API_KEY")` first; `.env` is a fallback only when the variable is unset.

**Quota note**: The maintainer's hackathon credential is quota-limited. Nutrient charges 15 credits per page in understand mode; the canonical 4-document shipment suite (5 pages total) requires approximately 75 credits. Live re-execution is supported with an operator-supplied `NUTRIENT_DWS_EXTRACTION_API_KEY` with sufficient Nutrient DWS quota. Frozen execution artifacts remain available for quota-free historical verification.

**What DWS does (same line in README, video, and DevPost):**

> Nutrient DWS extracts structured, page-and-bbox-grounded fields from each PDF, giving DocTrust page-referenced facts instead of plausible-sounding guesses; DocTrust's Ruleset engine reconciles those facts and decides.

**Adding a provider:** Implement `internal/provider.EvidenceProvider` (method `ExtractFields`) — see `internal/provider/provider.go` and the Nutrient implementation in `internal/nutrient`. No changes to DocTrust, Ruleset, evaluator, or the skill are required.

**Adding a Ruleset:** New vertical = new Ruleset YAML + evidence mapping (e.g., KYC, insurance) on the same engine — see `rulesets/shipment_release/v1.yaml` as the template.

**Business feasibility:** See [`BUSINESS_CASE.md`](BUSINESS_CASE.md) for the one-sentence pitch, buyer, pricing hypothesis (usage-based per case + enterprise tier for custom Rulesets/identity), and why the trust architecture — not extraction or LLM — is the moat. The wedge is shipment release; expansion is additional Rulesets on the same pipeline.

### Author a new check through the trust funnel

```bash
bin/author-check --domain income_verification --intent "check that base salary is positive"
# → candidates/active/<check_id>/  (LLM-generated check + scenarios)

bin/review-check candidates/active/<check_id>
# author the adversarial scenario, then answer [a] Approve with explicit y

bin/promote-check --candidate candidates/active/<check_id> --domain income_verification
# 9 gates: approval binding, import allowlist, module-graph build/vet,
# deterministic scenario execution, staging, staged build, staged regression, commit

bin/promote --domain income_verification   # freeze immutable version + manifest
make build                                  # REQUIRED: new Go checks compile into the binary
bin/verify-ruleset --domain income_verification --expect-version <N> --expect-hash <hash>
```

---

## Directory Structure

```
doctrust/
  cmd/                          # CLI commands
    author-check/               # LLM candidate generation (Phase 3 trust funnel)
    review-check/               # Interactive review + explicit adversarial confirmation
    promote-check/              # 9-gate promotion pipeline (fail-closed, atomic)
    verify-ruleset/             # Assert promoted Ruleset version/hash vs manifest
    regression/                 # Scenario regression (Phase 2)
    promote/                    # Ruleset promotion (Phase 2)
    registry/                   # Registry inspection (Phase 2)
    server/                     # HTTP API (Phase 4)
    doctrust-mcp/               # MCP stdio server — 5 tools: evaluate_case,
                                #   get_findings, get_evidence, get_ruleset,
                                #   get_audit_artifact
    evidence-mcp/               # MCP stdio provider adapter (thin): builds/
                                #   extends evidence snapshots via ingest path
    eval/                       # OPA standalone evaluator (reference)
    compare/                    # OPA vs eval cross-validation (reference)
    validate-fixtures/          # OPA fixture validation (reference)
    ingest/                     # Nutrient extraction pipeline
    compile-policy/             # POLICY.md → Rego compiler (legacy reference)
  internal/
    types/                      # Leaf shared contracts (DocumentType, EvidenceRef)
    eval/                       # Core eval engine (locked architecture)
    facts/                      # Canonical facts model (Facts, Fact)
    evidence/                   # Evidence model (EvidenceGraph, Document, Claim)
    service/                    # Application boundary (DocTrustService)
    compiler/                   # Candidate authoring & promotion trust funnel
                                #   (snapshot, gates, staged build/regression,
                                #    import allowlist, param resolver)
    ingest/                     # Nutrient-specific ingestion (classifier, normalizer)
    opa/                        # OPA SDK wrapper (reference)
    nutrient/                   # Nutrient DWS client
    audit/                      # Audit artifact + PDF report
    review/                     # Human review store + disposition
    extraction/                 # Extraction config (shared; income + shipment)
    provider/                   # Provider-neutral EvidenceProvider seam
  skills/                       # PRODUCT ARTIFACT: agent skills (canonical)
    compliance-check-artifact/SKILL.md
  rulesets/                     # Ruleset registry (versioned, immutable once promoted)
    income_verification/
      v1.yaml / v1.manifest.json
      v2.yaml / v2.manifest.json
    shipment_release/
      v1.yaml                   # required_shipment_documents +
      v1.manifest.json          #   gross_weight_reconciliation (live demo)
  scenarios/                    # Regression corpus — promotions merge new check
    income_verification/        # scenarios here; production loaders read this dir
      check_gross_income_variance.yaml  (6 scenarios)
      check_required_docs.yaml          (5 scenarios)
      check_net_vs_gross.yaml           (3 scenarios)
    shipment_release/           # 6 scenarios (pass / B/L outlier REVIEW /
      check_shipment_release.yaml  #  partial-insufficient / conflicting / …)
  candidates/                   # Authoring workspace (created by author-check)
    active/<check_id>/          # check.go, metadata.yaml, scenarios.yaml,
      adversarial.yaml, state, approval.json
    archive/<check_id>/         # byte-identical snapshot after PROMOTED
  fixtures/                     # Test fixtures
  policies/                     # Rego policies (reference/migration)
  web/                          # Server UI templates
  scripts/                      # PDF generation
  plans4/, plans6/              # Development history (plans, adversarial reviews)
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

**Five registered checks:**

| Check ID | Purpose | Key Params |
|----------|---------|------------|
| `gross_income_consistency` | Compares paystub projected gross vs W-2 taxable income | `tolerance` (default 5%), `bonus_field` |
| `required_documents` | Verifies all required document types are present (income domain) | none |
| `net_vs_gross_incomparability` | Semantic guard: net cash flow ≠ gross taxable income | none |
| `gross_weight_reconciliation` | Shipment gross weight must agree (`all_equal`) across required trade documents; missing sources → REVIEW (insufficient evidence); mismatch → REVIEW naming outliers | `semantic_type`, `sources`, `condition`, `unit`, `tolerance` (default 0.005) |
| `required_shipment_documents` | Verifies the four trade-document types are present; missing → REVIEW/BLOCKING (never FAIL — progressive-evidence dependency) | `required` (list of document types) |

Trade documents: `commercial_invoice`, `packing_list`, `bill_of_lading`,
`certificate_of_origin`. All four expose their gross weight under the shared
semantic type `shipment.gross_weight`; identity references and container/seal
are captured per document for corroboration.

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

> Hand-editing a draft is the manual path for param tweaks on existing checks.
> For **new check logic**, use the governed path instead — see
> [Candidate Check Lifecycle](#candidate-check-lifecycle-ai-authoring).

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

> **Rebuild after promoting new Go checks.** Check registration compiles into
> `DefaultRegistry()`, so a promotion that adds a Go check requires
> `make build` before `bin/verify-ruleset` or any runtime restart.

### Step 5: Runtime uses new version

```bash
bin/registry
# income_verification: v2 (3 checks, latest)
```

---

## Candidate Check Lifecycle (AI Authoring)

New check logic enters the runtime only through this funnel. Every gate is
fail-closed: a rejection leaves `internal/eval`, `rulesets/`, and `scenarios/`
byte-for-byte unchanged.

```
DRAFT → HUMAN_REVIEW → APPROVED → PROMOTED
                     ↘ REJECTED
```

1. **author-check** generates a candidate from a natural-language intent
   (OpenRouter; requires `OPENROUTER_API_KEY`).
2. **Human adversarial review** — you author at least one `human_adversarial`
   scenario against the check's actual behavior, then approve in
   **review-check**. Approval reprints the full adversarial YAML and demands an
   explicit `y`/`Y`; anything else cancels with zero state change.
3. **Approval binding** — approval.json records SHA-256 hashes of all candidate
   files plus reviewer identity (`OS user` or `DOCTRUST_REVIEWER` override).
4. **promote-check** runs nine gates on a single-read snapshot:
   state APPROVED → adversarial present → approval verified against bytes →
   import allowlist + module-graph build/vet + Check-ID uniqueness →
   deterministic scenario execution → staging → staged build of the transformed
   check against the full module graph → staged regression over the real corpus
   (same parameter semantics as production) → atomic commit.
5. **Atomic commit** writes the Go check, updated registry, Ruleset draft
   (preserving existing CheckRefs and their params), merges scenarios into
   `scenarios/<domain>/`, and archives the candidate byte-identically.
6. **bin/promote** freezes the next immutable version + manifest.
7. **Rebuild**: new Go checks compile into `DefaultRegistry()` — run
   `make build` before verifying or restarting anything (stale binaries fail
   verify-ruleset closed).
8. **verify-ruleset** asserts version and manifest hash; restart the server/MCP
   process so it loads the promoted version at startup.

Registration uses the struct name extracted by AST from the author's source —
no naming convention is required or checked. Generated code may import only
allowlisted packages (`fmt`, `math`, `sort`, `strconv`, `strings`, `time`,
`internal/{eval,evidence,types,facts}`).

### Environment variables

| Variable | Required | Purpose |
|----------|----------|---------|
| `NUTRIENT_DWS_EXTRACTION_API_KEY` | for live extraction | Nutrient DWS extraction credential; resolves from process env first, falls back to `.env` via `loadExtractionKey()` |
| `OPENROUTER_API_KEY` | for author-check | LLM generation; fails closed if absent |
| `OPENROUTER_MODEL` | no | Model override (default: claude-sonnet-4) |
| `DOCTRUST_REVIEWER` | no | Reviewer identity recorded in approval.json |

---

## CLI Reference

| Command | Purpose |
|---------|---------|
| `bin/author-check --intent "<text>" --domain <domain>` | Generate candidate check + scenarios via LLM |
| `bin/review-check <candidate-dir>` | Interactive review; explicit adversarial y/N confirmation gates approval |
| `bin/promote-check --candidate <dir> --domain <domain>` | Run all nine trust gates and atomically promote |
| `bin/verify-ruleset --domain <domain> [--expect-version N] [--expect-hash H]` | Assert promoted Ruleset identity vs manifest |
| `bin/regression --domain <domain>` | Run scenario regression (default: Ruleset params) |
| `bin/regression --domain <domain> --scenario-params` | Legacy mode (scenario-level params) |
| `bin/regression --domain <domain> --json` | JSON output with audit trail |
| `bin/promote --domain <domain>` | Promote working draft to next version |
| `bin/promote --domain <domain> --dry-run` | Validate only, don't promote |
| `bin/registry` | List all rulesets with versions |
| `bin/registry --domain <domain>` | Show details for one ruleset |
| `bin/server --domain <domain>` | HTTP API server |
| `bin/doctrust-mcp --domain <domain> [--rulesets-dir D] [--snapshot-root D]` | MCP stdio server (5 tools: evaluate_case, get_findings, get_evidence, get_ruleset, get_audit_artifact) |
| `bin/evidence-mcp [--snapshot-root D] [--env-file F]` | MCP stdio provider adapter: build_evidence_snapshot / extend_evidence_snapshot (credentials load at runtime from env or .env file — never stored in agent config) |
| `bin/ingest <dir>` | Income-domain extraction pipeline (legacy positional mode) |
| `bin/ingest -domain shipment_release -docs type=path[,…] [--extend-from S] [--report F]` | Shipment evidence pipeline; extension produces a new content-derived case + provenance sidecar |
| `bin/verify-audit <snapshot>` | Verify audit artifact integrity (snapshot hash, Ruleset binding, Ed25519 reviews) |
| `scripts/rehearse-hermes-shipment.sh` | Real adaptive Hermes run + assertions A1–A10b |
| `scripts/failure-rehearsals.sh` | Trust-boundary failure rehearsals F1–F5 |
| `scripts/install-skill.sh` | Deploy canonical Compliance Skill to the Hermes runtime |
| `bin/eval --policy <path> <snapshot>` | OPA standalone evaluator (reference) |
| `bin/compare <snapshot>` | Cross-validate OPA vs eval engine (reference) |

Makefile conveniences: `make review-candidate CANDIDATE=<dir>`,
`make promote-candidate CANDIDATE=<dir> DOMAIN=<domain>`,
`make verify-ruleset DOMAIN=<domain>`.

---

## Verification Commands

```bash
go build ./...                                          # must compile
go vet ./...                                            # must be clean
go test ./...                                           # all tests pass
go test -race ./...                                     # race detector
go test ./internal/eval/... -v -run TestRunAllScenarios # 14/14 strict
go test ./internal/eval/... -v -run TestRunAllShipmentScenarios # 6/6 shipment scenarios
go test ./internal/service/... -v                       # service layer tests
go test ./internal/audit/... -v -run TestArtifact       # artifact hash integrity
bin/regression --domain income_verification             # baseline = 0 changed
bin/registry                                            # shows all versions
make lint-imports                                       # provider boundary check

# Phase 5 — agent orchestration
./scripts/rehearse-hermes-shipment.sh                   # live adaptive run + A1–A10b
./scripts/failure-rehearsals.sh                         # F1–F5 fail-closed proofs

# Candidate lifecycle suites (trust funnel)
go test ./internal/compiler/...                         # gates, staged build/regression, E2E + failure paths
go test ./cmd/promote-check/...                         # binary-level duplicate-ID regression
go test ./cmd/review-check/...                          # adversarial confirmation subprocess tests
go test ./cmd/verify-ruleset/...                        # version/hash assertion exit codes
```

---

## Phase 4 — Trust Properties

| Property | Protection |
|----------|------------|
| Ruleset identity in audit | `Artifact.SetRuleset()` called in `handleFinalize` and `handleAudit` via `svc.BuildArtifact()` |
| Bbox grounding | `BuildFactsFromSnapshot` passes through viewer-scaled bboxes with no transform |
| finding_index validation | `svc.RequestHumanReview()` validates index against pinned `decision.Results` |
| Server trust tests | 10 tests in `cmd/server/main_test.go` |
| Coordinate contract | Documented in `BuildFactsFromSnapshot` and `navigateToFinding` |
| Provider boundary | `make lint-imports` verifies `internal/service` has no nutrient/extraction/opa deps |
| Handler lifecycle | `TestLifecycle_NoReEvaluation` proves later handlers never re-load or re-evaluate |
| Artifact hash integrity | `TestArtifact_Finalize_HashIntegrity` proves `Finalize().ArtifactHash == Hash()` |
| SourceDoc canonicalization | `TestBuildFactsFromSnapshot_SourceDocCanonical` proves production filenames produce canonical types |

---

## Phase Status

| Phase | Status | Description |
|-------|--------|-------------|
| Phase 1 | Frozen | Nutrient integration, evidence normalization, 14/14 strict scenarios |
| Phase 1 (corrective) | Frozen | Service layer migration, artifact hash fix, lifecycle invariant, provider boundary |
| Phase 2 | Frozen | Regression CLI, promote CLI, registry CLI, Ruleset params default |
| Phase 3 | Frozen | Candidate Authoring & Trust Funnel — AI-generated candidate → human adversarial review → deterministic execution → staged build/regression → atomic promotion → durable regression coverage → verified runtime Ruleset → MCP/agent evaluation |
| Phase 4 | Frozen | Enriched evaluation UI, audit artifact with ruleset provenance, bbox grounding, server trust tests |
| Shipment evidence pipeline (plans10) | COMPLETE | Provider seam (`internal/provider`), shipment domain + promoted Ruleset v1, thin evidence-mcp, **G1 gate passed**: live Nutrient extraction matched ground truth 14/14 with page/bbox provenance — see [g1/G1_REPORT.md](g1/G1_REPORT.md) |
| Agent orchestration & Compliance Skill (plans11 / Phase 5) | COMPLETE | Canonical skill + Hermes registrations + genuine adaptive investigation on live evidence; 13/13 assertions, F1–F5 rehearsals green — see [phase5/PHASE5_REPORT.md](phase5/PHASE5_REPORT.md) |
| Human authority channel (plans12 / Phase 6) | COMPLETE | `request_human_review` removed from the agent surface (structurally denied); human-only Ed25519-signed review TTY (`bin/doctrust-review`); fail-closed signature verification — see [phase6/PHASE6_REPORT.md](phase6/PHASE6_REPORT.md) |

---

## Known Limitations & Deferred Items

Verified-state documentation — what the current proof covers and where it ends:

- **Review store is in-memory, single-case-per-process.** Human reviews and
  audit artifacts are not persisted across process restarts; the demo
  finalizes within one session.
- **Human review authority (Phase 6)** is enforced structurally:
  `request_human_review` does not exist on the agent MCP surface, and the
  human-only TTY channel signs every decision with Ed25519 (fail-closed
  verification). Deferred: multi-user identity management and web approval UI.
- **Vestigial `MISSING_EVIDENCE`** exists only in the legacy Rego reference
  path; the Go engine uses PASS/REVIEW/FAIL exclusively.
- **Empty scaffolds**: `internal/policy/`, `internal/server/`,
  `internal/workflow/` are reserved but unbuilt; orphaned root binaries
  (`test_nutrient`, etc.) and the `cmd/inspect` debug CLI await cleanup.
- **Evidence contract scope**: shipment extraction currently requests gross
  weights + document references + container/seal (14 ground-truth-covered
  fields). Full-document field coverage (line items, crate cells, party
  blocks) is future work.
- **Agent runs depend on free-tier model availability**
  (`nvidia/nemotron-3-ultra-550b-a55b:free`); unavailability is reported as an
  honest infrastructure failure, never silently substituted.
- **Fixture provenance**. The 5,150 KG bill-of-lading mismatch was deliberately designed into the shipment-1047 fixture scenario to exercise the REVIEW/BLOCKING path. What is being demonstrated is the system's extraction, investigation, evidence reconciliation, authority boundary, and audit behavior — not discovery of a naturally occurring discrepancy.
- **Attempt history**. The frozen artifacts do not retain a complete attempt count for the historical rehearsals; the documentation therefore makes no claim that the captured executions were first-attempt runs.
- **Screen recording**. No continuous screen recording of the original Hermes session was retained. The interaction is independently verifiable through the recorded MCP transcripts, agent transcript, tool surface, evidence snapshots, and audit artifacts. The verification strength comes from the cross-linked evidence bundle, not any single artifact.
- **Live integration evidence**. During clean-clone verification we encountered a real Nutrient quota response (HTTP 402) after earlier extraction work had already completed; the raw response is preserved in [`phaseA/MVP_LIVE_VERIFICATION.md`](phaseA/MVP_LIVE_VERIFICATION.md). This demonstrates the live provider integration reached Nutrient and processed earlier documents before the account quota prevented subsequent extraction. The failure is useful evidence because nobody fabricates their own embarrassing failure in a document meant to reassure a skeptic.

---

## Dependencies

- Go 1.25.7
- `github.com/open-policy-agent/opa` v1.4.2 (OPA SDK, reference/migration tooling)
- `github.com/jung-kurt/gofpdf/v2` v2.17.3 (PDF generation for audit reports)
- `gopkg.in/yaml.v3` v3.0.1 (YAML parsing for rulesets and scenarios)
