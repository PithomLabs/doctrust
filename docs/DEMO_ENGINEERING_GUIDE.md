# DocTrust Demo Engineering Guide

**Purpose**: Forensic engineering documentation that lets a skeptical engineer verify the demo was real and trace every claim back to source code, execution artifacts, and resulting state.

**Documentation basis**:
- Source revision documented: `c3a6960575f8c24d7bdf5856193e621892a9e780` (commit message: "demo")
- Documentation baseline revision: `<Commit B SHA — filled in Commit C>`
- Frozen execution artifacts:
  - Phase-5: `demo/shipment_release/runs/20260824-185232-341898`
  - Phase-6: `demo/shipment_release/runs/p6-20260825-015819-726232`
- Verification method: static source inspection + frozen execution artifacts
- No demo re-execution performed for this documentation

---

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

The complete forensic walkthrough follows below.

---

## Three Kinds of Truth

Throughout this guide, claims are categorized into three truth types:

| Truth Type | Definition | Example |
|------------|------------|---------|
| **SOURCE TRUTH** | What the code currently does | `GrossWeightReconciliationCheck.Evaluate()` compares all sources for equality |
| **EXECUTION TRUTH** | What the recorded historical run actually did | Phase-6 run returned `gross_weight_reconciliation: REVIEW/BLOCKING` with `bill_of_lading: 5150` |
| **PRESENTATION TRUTH** | How genuine execution states were composed into the judge video | S02–S04 are from Phase-5; S01, S05–S07 are from Phase-6; S08 is constructed |

The video attracts belief. This guide makes that belief independently testable.

---

## 1. Executive Summary

DocTrust is a compliance execution layer that lets AI agents investigate business documents against approved policy without ever letting the agent become the authority that decides compliance.

The demo video proves three things:

1. **PROGRESS**: A substantial working product exists — 18 command packages, 16 compiled binaries, 16 internal packages, 2 promoted Rulesets, and a complete trust funnel from ingestion through audit.

2. **CONCEPT**: AI can investigate consequential documents without becoming the compliance authority. The agent gets `REVIEW` and must stop. A human at a TTY terminal makes per-finding decisions. Ed25519 signatures bind the human's identity to the decision. The audit artifact seals the entire chain.

3. **FEASIBILITY**: The same authority, evidence, and audit architecture can support other regulated document workflows. Shipment release is the first vertical. The provider seam, Ruleset model, and human authority channel are domain-agnostic.

**The agent investigates. DocTrust enforces. A human decides. The audit proves the chain.**

---

## 2. Five Whys

### 2.1 Why does DocTrust exist?

**Problem**: "Let the LLM read the documents and decide" is insufficient for consequential compliance decisions.

In the demo, the gross weight mismatch (4,650 vs 5,150 KG) across four trade documents is the kind of discrepancy that, if missed, becomes a costly compliance problem. An LLM reading the documents might notice the numbers differ, but it has no authority model, no evidence provenance, no audit trail, and no human-in-the-loop guarantee.

DocTrust exists because **deterministic policy evaluation with structured evidence is the only way to make the agent useful without making it the authority**.

**SOURCE TRUTH**: `internal/eval/check_shipment.go:27-165` — `GrossWeightReconciliationCheck.Evaluate()` performs deterministic `all_equal` comparison across sources with tolerance, naming outliers with per-source observations. This is not LLM reasoning; it is code.

**EXECUTION TRUTH**: The Phase-6 run's decision sidecar (`demo/shipment_release/runs/p6-20260825-015819-726232/available/evidence_snapshot.json.decision.json`) records `gross_weight_reconciliation` returning `REVIEW/BLOCKING` with `outliers: ["bill_of_lading"]` and `observations: {bill_of_lading: 5150, certificate_of_origin: 4650, commercial_invoice: 4650, packing_list: 4650}`.

**PRESENTATION TRUTH**: S05 in the demo video shows this mismatch grounded in page/bbox evidence from Nutrient extraction.

---

### 2.2 Why can't the agent itself be the compliance authority?

**Authority separation**: Hermes (the AI agent) can investigate, request documents, and call DocTrust tools. It cannot approve, reject, or finalize compliance decisions.

**SOURCE TRUTH**: `cmd/doctrust-mcp/handlers.go:149-378` — only 5 tools are registered: `evaluate_case`, `get_findings`, `get_evidence`, `get_ruleset`, `get_audit_artifact`. The `request_human_review` tool was removed per R29 (AGENTS.md). `cmd/doctrust-review/main.go:48` — `isInteractive()` checks that both stdin and stdout are `ModeCharDevice`; non-interactive execution is refused.

**EXECUTION TRUTH**: The Phase-6 run's `denied_proof.jsonl` (line 2) records the server returning JSON-RPC error `-32602` ("unknown tool") when the agent attempted `request_human_review`. Line 3 shows `tools/list` returning only 5 tools — `request_human_review` is absent.

**PRESENTATION TRUTH**: S06 in the demo video shows the agent denial, the human TTY session, and the FAIL disposition.

---

### 2.3 Why is an evidence/provider layer required?

**Provider/evidence separation**: DocTrust does not know or care where evidence comes from. The `EvidenceProvider` seam (`internal/provider/provider.go`) defines a generic contract. Nutrient is the first implementation.

**SOURCE TRUTH**:
- `internal/nutrient/client.go` — HTTP client calling Nutrient DWS extraction API. Produces structured, page-and-bbox-grounded fields from PDFs.
- `internal/ingest/evidence_normalizer.go:26` — `Normalize()` converts raw Nutrient extraction results into `evidence.Claim` structs with `SourceSpan` bboxes and confidence scores.
- `internal/service/builder.go:25-46` — `BuildFactsFromSnapshot()` resolves `SourceDoc` from the canonical document type (R16), not the filename. Every fact carries full provenance: `SourceDoc`, `FieldName`, `SourceSpan` (page+bbox), `Confidence`.

**EXECUTION TRUTH**: The Phase-5 extraction report (`runs/20260824-185232-341898/available/extraction_report.json`) documents Nutrient extracting 9 fields from 2 initial documents. The extended extraction report documents 8 additional fields from 2 added documents. The evidence snapshots contain `SourceSpan` bboxes like `page=1;bbox=[459.0,288.0,50.4,7.9]` with `confidence: 0.95`.

**PRESENTATION TRUTH**: S02 shows the DWS one-liner. S05 shows page/bbox evidence references from actual Nutrient extraction.

---

### 2.4 Why does the agent need adaptive investigation?

**Adaptive evidence**: When the initial evidence state is incomplete, the system returns `REVIEW` (not FAIL), and the agent must investigate further.

**SOURCE TRUTH**: `internal/eval/check_shipment.go:186-252` — `RequiredShipmentDocumentsCheck` returns `REVIEW/BLOCKING` when documents are missing, never FAIL. This is the mechanism that drives adaptive investigation. The check is locked to return REVIEW (R25) because insufficient evidence requires human judgment, not a deterministic ruling.

**EXECUTION TRUTH**: Phase-5 run `turn1.txt` records the initial state: 2 of 4 documents available, both checks returning `REVIEW/BLOCKING` with "insufficient evidence". `released.txt` records the agent's explicit request: `certificate_of_origin=04-certificate-of-origin.pdf` and `packing_list=02-packing-list.pdf`. `turn2.txt` records the re-evaluation with all 4 documents: `required_shipment_documents` now `PASS/INFO`, `gross_weight_reconciliation` `REVIEW/BLOCKING` (B/L outlier 5150 vs 4650).

**PRESENTATION TRUTH**: S03 shows insufficient evidence. S04 shows the agent choosing PL + CO.

---

### 2.5 Why is consequential human authority structurally separate?

**Human authority**: The agent cannot approve or reject compliance findings. Only a human at a TTY terminal can sign review records, and those records are Ed25519-signed with full binding.

**SOURCE TRUTH**:
- `cmd/doctrust-review/review_flow.go:78-243` — `runReviewFlow()` re-evaluates from snapshot (F-4), verifies sidecar matches re-evaluation, prompts for passphrase-gated key unlock, processes per-finding confirm/reject with explicit typed consent, signs each record with Ed25519, and finalizes the audit artifact.
- `internal/review/signing.go:89-111` — `SignRecord()` builds a `canonicalPayload` binding case_id, snapshot_sha256, finding_index, action, note, reviewer_identity, channel, key_id, alg, ruleset, resolved_at. Signs with Ed25519.
- `internal/service/reviews.go:115-172` — `LoadAuthorizedReviews()` is fail-closed: every record must pass Ed25519 verification against the public key ring. Missing/forged/wrong-key/content-mismatched signatures fail closed.
- `internal/audit/artifact.go:184-207` — `Finalize()` computes tamper-evident hash via `hashable()` which excludes `Manifest.ArtifactHash` (R15). `SetRuleset()` must be called in both `handleFinalize` and `handleAudit` (R11).

**EXECUTION TRUTH**: Phase-6 run `human_session.txt` records the complete TTY session: passphrase-gated key unlock (`key_id="owner"`), finding [0] confirmed, finding [1] rejected with note "HOLD - gross weight mismatch vs corroborating documents", 2 Ed25519 records signed, `FINAL DISPOSITION: FAIL`. The `.doctrust_reviews.json` contains 2 `HumanReviewRecord` entries with Ed25519 signatures. The `.audit.json` contains the sealed artifact with `final_disposition: FAIL` and `artifact_hash: b2cc19a7...`.

**PRESENTATION TRUTH**: S06 shows MCP denial, TTY session, Ed25519 signing, FAIL disposition. S07 shows the sealed audit artifact.

---

## 3. One How — Forensic Walkthrough

The canonical video path traces two genuine executions composed into one narrative. This section reconstructs exactly what happened.

### 3.1 Phase-5: Progressive Evidence (S02–S04)

**Script**: `scripts/rehearse-hermes-shipment.sh`

**Setup**: CI + B/L placed in `runs/20260824-185232-341898/available/`. PL + CO staged in a temp directory outside `DOCTRUST_SNAPSHOT_ROOT` — genuinely unaddressable by evidence tools.

**Turn 1**: Hermes evaluates CI + B/L.
- Skill: `compliance-check-artifact` (`skills/compliance-check-artifact/SKILL.md`)
- `bin/ingest` extracts 2 documents via Nutrient → `evidence_snapshot.json`
- `bin/doctrust-mcp evaluate_case` runs Ruleset v1 → both checks return `REVIEW/BLOCKING` (insufficient evidence)
- Output: `turn1.txt`

**Release gate**: Python script scans `turn1.txt` for requests of withheld documents. Only copies PL + CO into `available/` after the agent explicitly requested them. Writes `released.txt` with the mapping.

**Turn 2**: Hermes re-evaluates with all 4 documents.
- `bin/ingest --extend-from` produces extended snapshot
- `bin/doctrust-mcp evaluate_case` → `required_shipment_documents: PASS/INFO`, `gross_weight_reconciliation: REVIEW/BLOCKING` (B/L outlier 5150 vs 4650)
- Output: `turn2.txt`, `combined_transcript.txt`

**Assertions A1–A10**: Deterministic verification of the full progressive-evidence contract — initial state was insufficient, agent requested missing docs, extended snapshot has all 4 types, final finding identifies B/L outlier, values are CI=4650/PL=4650/CO=4650/B/L=5150, ruleset unchanged, no `request_human_review` calls.

### 3.2 Phase-6: Human Authority (S01, S05–S07)

**Script**: `scripts/rehearse-phase6-human.sh`

**Step 1 — Ingest**: `bin/ingest -domain shipment_release` extracts all 4 documents via Nutrient → `evidence_snapshot.json`.

**Step 2 — Provision reviewer**: `bin/doctrust-review --provision` generates Ed25519 key pair, encrypts private key with PBKDF2 (600K iterations) + AES-256-GCM, publishes public key to `reviewers/owner.pub`.

**Step 3 — Agent-side evaluation**: `bin/doctrust-mcp evaluate_case` → 2 findings, `REVIEW/BLOCKING`. Decision sidecar written.

**Proof 1 (Denied)**: `tools/list` returns 5 tools — no `request_human_review`. Agent attempt returns JSON-RPC error `-32602`. Written to `denied_proof.jsonl`.

**Proof 2 (Authorized)**: Human at TTY terminal:
1. Passphrase-gated key unlock (`key_id="owner"`)
2. Re-evaluates from snapshot (F-4), verifies sidecar matches
3. Finding [0]: confirm — "Required documents present and corroborated."
4. Finding [1]: reject — "HOLD - gross weight mismatch vs corroborating documents"
5. Signs 2 Ed25519 records
6. Finalizes: `final_disposition = FAIL`

**Artifacts produced**:
- `evidence_snapshot.json.doctrust_reviews.json` — 2 Ed25519-signed records
- `evidence_snapshot.json.audit.json` — sealed audit artifact with tamper-evident hash

### 3.3 Video Composition Provenance

The demo video is assembled from **two genuine historical executions**, not one continuous run:

| Execution | Shots | What It Proves |
|-----------|-------|----------------|
| Phase-5 progressive run (`20260824-185232-341898`) | S02, S03, S04 | Initial evidence state → insufficient evidence → agent investigation → document extension |
| Phase-6 human-authority run (`p6-20260825-015819-726232`) | S01, S05, S06, S07 | DocTrust evaluation → evidence mismatch → human authority → audit |
| Constructed | S08 | Feasibility roadmap (graphical) |

**PRESENTATION TRUTH**: The video composes these genuine execution states into one narrative. It does not claim they occurred in one uninterrupted run. `master_demo.md` Production Notes documents this explicitly.

**Different case IDs**: Phase-5 run case IDs are `shipment_3a2ea0ec5ce1a41b` (initial) and `shipment_ac0d89ce3daa1e14` (extended). Phase-6 run case IDs are `shipment_e067246b18486d8b` (graph) and `56cf311f09d45996` (LoadCase). These are separate genuine executions with different content-derived identifiers.

---

## 4. Architecture at a Glance

```
                    ┌─────────────────────┐
                    │   AI Agent (Hermes)  │
                    │   investigates       │
                    └─────────┬───────────┘
                              │ MCP JSON-RPC (5 tools)
                              ▼
                    ┌─────────────────────┐
                    │   DocTrust MCP       │
                    │   cmd/doctrust-mcp   │
                    └─────────┬───────────┘
                              │
              ┌───────────────┼───────────────┐
              ▼               ▼               ▼
     ┌──────────────┐ ┌──────────────┐ ┌──────────────┐
     │  Service      │ │  Evaluator   │ │  Ruleset     │
     │  Boundary     │ │  (deterministic) │ (frozen)  │
     │  service/     │ │  eval/       │ │  rulesets/   │
     └──────┬───────┘ └──────────────┘ └──────────────┘
            │
            │  EvidenceProvider seam (R13, R23)
            │  internal/service must NOT import internal/nutrient
            ▼
     ┌──────────────┐
     │  Nutrient DWS │
     │  extraction   │
     │  (first provider) │
     └──────┬───────┘
            │
            ▼
     EvidenceGraph
     (structured fields, page+bbox provenance)
            │
            ▼
     Findings → Human TTY → Ed25519 signing → Audit artifact
```

**Key boundaries**:
- Provider boundary (R13): `internal/service` and `cmd/doctrust-mcp` never import `internal/nutrient`, `internal/extraction`, `internal/opa`, or `internal/provider`. Enforced by `make lint-imports`.
- Agent boundary (R29): `request_human_review` is absent from the MCP tool surface. The agent can investigate but cannot approve.
- Human boundary: `cmd/doctrust-review` requires interactive TTY (R29). Non-interactive execution is refused.

---

## 5. Canonical Commands

### 5.1 Primary Demo Path

The video was produced by two scripts executed in sequence:

| Step | Command | What It Proves |
|------|---------|----------------|
| 1 | `scripts/rehearse-hermes-shipment.sh` | Real LLM agent progressive evidence (S02–S04) |
| 2 | `scripts/rehearse-phase6-human.sh` | Ed25519 signing + FAIL disposition (S01, S05–S07) |

### 5.2 Adjacent Entry Points

| Entry | Command | Purpose | Differs from Canonical |
|-------|---------|---------|----------------------|
| PASS demo | `make demo-pass` | Coherent package → PASS | No human review, no outlier |
| REVIEW demo | `make demo-review` | Anomaly package → REVIEW | No Hermes, no progressive evidence |
| Judge demo | `scripts/judge-demo.sh` | LLM-free 8-act trust funnel | Income domain, sandboxed, no Nutrient |
| Regression | `bin/regression --domain shipment_release` | 6/6 scenario corpus | Deterministic, no LLM |

---

## 6. Execution Evidence Index

### E1 — Phase-5 Progressive Run

All artifacts under `demo/shipment_release/runs/20260824-185232-341898/`.

| Artifact | Path | Proves |
|----------|------|--------|
| `turn1.txt` | `turn1.txt` | Agent got REVIEW/BLOCKING for 2/4 docs (insufficient evidence) |
| `turn2.txt` | `turn2.txt` | Agent got REVIEW/BLOCKING for B/L outlier (5150 vs 4650) |
| `combined_transcript.txt` | `combined_transcript.txt` | Full two-turn agent conversation |
| `released.txt` | `released.txt` | Agent explicitly requested PL + CO (filesystem-gated release) |
| `evidence_snapshot.json` | `available/evidence_snapshot.json` | 2-document EvidenceGraph (case `shipment_3a2ea0ec5ce1a41b`) |
| `evidence_snapshot_extended.json` | `available/evidence_snapshot_extended.json` | 4-document EvidenceGraph (case `shipment_ac0d89ce3daa1e14`) |
| `provenance.json` | `available/evidence_snapshot_extended.json.provenance.json` | Extension lineage chain |
| `extraction_report.json` | `available/extraction_report.json` | Nutrient extraction of 2 initial docs |
| `extended_extraction_report.json` | `available/extended_extraction_report.json` | Nutrient extraction of 2 added docs |
| `final_case_id.txt` | `final_case_id.txt` | `fe74ba1a0e76e300` — final evaluation case ID |

### E2 — Phase-6 Human Authority Run

All artifacts under `demo/shipment_release/runs/p6-20260825-015819-726232/`.

| Artifact | Path | Proves |
|----------|------|--------|
| `eval_reqs.jsonl` | `eval_reqs.jsonl` | Agent invoked `evaluate_case` via MCP |
| `eval_resp.jsonl` | `eval_resp.jsonl` | DocTrust returned 2 findings, REVIEW/BLOCKING |
| `denied_proof.jsonl` | `denied_proof.jsonl` | `request_human_review` → error `-32602` (R29 enforced) |
| `denied_reqs.jsonl` | `denied_reqs.jsonl` | Agent attempted the denied tool call |
| `human_input.txt` | `human_input.txt` | Scripted human decisions: confirm [0], reject [1] |
| `human_session.txt` | `human_session.txt` | Complete TTY session: signing, FAIL disposition |
| `.decision.json` | `available/evidence_snapshot.json.decision.json` | Full evaluation state with metrics and evidence refs |
| `.doctrust_reviews.json` | `available/evidence_snapshot.json.doctrust_reviews.json` | 2 Ed25519-signed HumanReviewRecords |
| `.audit.json` | `available/evidence_snapshot.json.audit.json` | Sealed audit artifact (FAIL, `b2cc19a7...`) |
| `keydir/owner.key.enc` | `keydir/owner.key.enc` | Encrypted Ed25519 signing key |

### E3 — Shared Artifacts

| Artifact | Path | Proves |
|----------|------|--------|
| `owner.pub` | `demo/shipment_release/reviewers/owner.pub` | Public key for Ed25519 verification |
| `rulesets/shipment_release/v1.yaml` | `rulesets/shipment_release/v1.yaml` | Frozen promoted Ruleset |
| `rulesets/shipment_release/v1.manifest.json` | `rulesets/shipment_release/v1.manifest.json` | SHA-256 hash of promoted Ruleset |
| `SKILL.md` | `skills/compliance-check-artifact/SKILL.md` | Hermes agent runbook |

---

## 7. Command / File / Function Matrix

### 7.1 Ingestion Pipeline

| Step | Command | Code Location | Input | Output | State Change |
|------|---------|---------------|-------|--------|--------------|
| 1 | `bin/ingest -domain shipment_release -docs "..." -out "$OUT"` | `cmd/ingest/main.go:64` → `shipment_mode.go:79` | 4 PDFs | `evidence_snapshot.json` | PDFs → EvidenceGraph |
| 2 | `loadExtractionKey()` | `shipment_mode.go:30` | env var (primary) / `.env` (fallback) | API key | — |
| 3 | `nutrient.NewClient(key, "")` | `internal/nutrient/client.go` | API key | HTTP client | — |
| 4 | `ingest.BuildShipmentSnapshot(client, opts)` | `internal/ingest/shipment.go:125` | client, docs, options | `*EvidenceGraph, *ExtractionReport` | — |
| 5 | `ShipmentSchemas()` | `internal/ingest/shipment.go:22` | — | per-type extraction schemas | — |
| 6 | `client.ExtractFields(path, schema, mode)` | `internal/nutrient/client.go` | PDF path, schema | `ExtractionResult` (fields + bboxes) | Nutrient API call |
| 7 | `ParseMediaBox(fileBytes)` | `internal/ingest/pdfmedia.go:10` | PDF bytes | (width, height) | — |
| 8 | `normalizer.Normalize(doc, extResult, pdfW, pdfH)` | `internal/ingest/evidence_normalizer.go:26` | doc, extraction result | `[]Claim` | Claims added to graph |
| 9 | `normalizer.BuildRelationships(graph.Claims)` | `internal/ingest/evidence_normalizer.go:54` | all claims | `[]Relationship` | Relationships added |
| 10 | `deriveShipmentCaseID(graph)` | `internal/ingest/shipment.go:275` | full graph | `shipment_<hex>` | Case ID derived |

### 7.2 MCP Server + Evaluator

| Step | Command | Code Location | Input | Output | State Change |
|------|---------|---------------|-------|--------|--------------|
| 11 | `bin/doctrust-mcp --domain shipment_release --rulesets-dir rulesets/ --snapshot-root $OUT` | `cmd/doctrust-mcp/main.go:14` | CLI args | MCP server running | — |
| 12 | `resolveSnapshotRoot()` | `cmd/doctrust-mcp/pathutil.go:10` | `-snapshot-root` flag | canonical path | — |
| 13 | `service.NewDocTrustService(domain, rulesetsDir)` | `internal/service/doctrust.go:47` | domain, dir | `*DocTrustService` | Ruleset loaded, runner created |
| 14 | `eval.NewRegistry(rulesetsDir).LoadPromoted(domain)` | `internal/eval/ruleset.go` | domain | promoted `Ruleset` | — |
| 15 | `registerTools(server, svc, root)` | `cmd/doctrust-mcp/handlers.go:149` | server, svc, root | 5 tools registered | — |
| 16 | `svc.LoadCase(ctx, snapshotPath)` | `internal/service/doctrust.go:66` | snapshot path | case loaded | Snapshot bytes → EvidenceGraph |
| 17 | `svc.Evaluate(ctx)` | `internal/service/doctrust.go:90` | — | `*Decision` | — |
| 18 | `BuildFactsFromSnapshot(snapshot)` | `internal/service/builder.go:25` | EvidenceGraph | `facts.Facts` | Claims → Facts |
| 19 | `runner.Evaluate(ctx, ruleset, f)` | `internal/eval/runner.go:64` | ruleset, facts | `[]Result` | — |
| 20 | `Check.Evaluate(facts, params)` | `internal/eval/check_shipment.go:27` | facts, params | `Result` | — |
| 21 | `DecisionAggregator.Decide(results)` | `internal/eval/aggregator.go` | all results | aggregate `Result` | — |
| 22 | Write `.decision.json` | `cmd/doctrust-mcp/handlers.go:192` | decision sidecar | file on disk | Decision persisted |

### 7.3 Human Review + Signing

| Step | Command | Code Location | Input | Output | State Change |
|------|---------|---------------|-------|--------|--------------|
| 23 | `bin/ingest -domain shipment_release -docs "..." -out "$AVAIL"` | `cmd/ingest/main.go:64` → `shipment_mode.go:79` | 4 PDFs | `evidence_snapshot.json` | — |
| 24 | `bin/doctrust-review --provision --name owner --key-dir $KEYDIR --publish-to $RING` | `cmd/doctrust-review/provision.go:12` | name, key-dir | `owner.key.enc` + `owner.pub` | Ed25519 key pair created |
| 25 | `review.GenerateReviewerKeyPair()` | `internal/review/keyfile.go:82` | — | `(pub, priv)` | — |
| 26 | `review.SaveEncryptedPrivateKey(path, keyID, priv, passphrase)` | `internal/review/keyfile.go:88` | path, key, passphrase | encrypted key file | PBKDF2 + AES-256-GCM |
| 27 | `bin/doctrust-mcp evaluate_case` | `cmd/doctrust-mcp/handlers.go:150` | snapshot path | 2 findings, REVIEW/BLOCKING | — |
| 28 | `tools/list` → no `request_human_review` | `cmd/doctrust-mcp/handlers.go:149` | — | 5 tools | R29 enforced |
| 29 | `tools/call request_human_review` → ERROR | `cmd/doctrust-mcp/handlers.go:149` | — | `-32602` error | Agent denied |
| 30 | `bin/doctrust-review --snapshot $SNAP --domain shipment_release --reviewer owner --key-dir $KEYDIR` | `cmd/doctrust-review/review_flow.go:39` | snapshot, reviewer | human review session | — |
| 31 | `isInteractive()` | `cmd/doctrust-review/tty.go:44` | stdin/stdout | TTY gate | Non-interactive refused |
| 32 | `svc.LoadCase() + svc.Evaluate()` (re-evaluation) | `internal/service/doctrust.go:66,90` | snapshot | fresh findings | F-4: sidecar untrusted |
| 33 | `compareFindings()` | `cmd/doctrust-review/review_flow.go:249` | sidecar vs re-eval | match/mismatch | Tamper detection |
| 34 | `readSecret()` + `loadReviewerKey()` | `cmd/doctrust-review/review_flow.go:133-141` | passphrase | decrypted key | — |
| 35 | `review.SignRecord(priv, &rec, resolvedAt)` | `internal/review/signing.go:89` | private key, record | signed record | Ed25519 signature |
| 36 | `review.AppendSignedRecord(path, caseID, rec)` | `internal/review/signing.go:175` | path, record | sidecar file | 2 records written |
| 37 | `svc.LoadAuthorizedReviews(records, ring)` | `internal/service/reviews.go:115` | records, pub ring | verified reviews | Fail-closed verification |
| 38 | `svc.BuildArtifact()` | `internal/service/doctrust.go:198` | verified reviews | `*Artifact` | — |
| 39 | `artifact.SetRuleset(id, version, hash)` | `internal/audit/artifact.go:116` | ruleset provenance | artifact bound | R11 |
| 40 | `review.ComputeFinalDisposition()` | `internal/review/store.go:82` | findings, reviews | `FAIL` | reject → FAIL |
| 41 | `artifact.Finalize()` | `internal/audit/artifact.go:184` | artifact | tamper-evident hash | R15 |

### 7.4 Audit Generation

The audit artifact (`audit.json`) is produced by `svc.BuildArtifact()` in `internal/service/doctrust.go:198-250`:

1. Creates `audit.NewArtifact(domain, policyHash)` — timestamped container
2. Calls `artifact.SetRuleset(id, version, hash)` — binds Ruleset provenance (R11)
3. Adds documents, decisions, and human reviews
4. Calls `review.ComputeFinalDisposition()` — determines PASS/REVIEW/FAIL
5. Calls `artifact.Finalize()` — computes tamper-evident hash via `hashable()` (R15)

The `hashable()` function (`internal/audit/artifact.go:145`) returns a struct that **excludes `Manifest.ArtifactHash`** to break the self-referential cycle. Invariant: `Finalize().ArtifactHash == Hash()`.

---

## 8. Data Lineage

```
PDF (4 trade documents)
  │
  │  Nutrient DWS extraction
  │  internal/nutrient/client.go
  ▼
Raw extraction result (fields + bboxes + confidence)
  │
  │  EvidenceNormalizer.Normalize()
  │  internal/ingest/evidence_normalizer.go:26
  ▼
evidence.Claim (field, value, source_doc, source_span, confidence)
  │
  │  EvidenceNormalizer.BuildRelationships()
  │  internal/ingest/evidence_normalizer.go:54
  ▼
evidence.EvidenceGraph (documents, claims, relationships)
  │
  │  WriteSnapshot()
  │  internal/ingest/shipment.go:242
  ▼
evidence_snapshot.json
  │
  │  BuildFactsFromSnapshot()
  │  internal/service/builder.go:25
  ▼
facts.Facts (map[semantic_type][]Fact)
  │
  │  runner.Evaluate()
  │  internal/eval/runner.go:64
  ▼
[]eval.Result (check_id, status, severity, reason, evidence)
  │
  │  DecisionAggregator.Decide()
  │  internal/eval/aggregator.go
  ▼
Decision (aggregate status, findings)
  │
  │  WriteFile(.decision.json)
  │  cmd/doctrust-mcp/handlers.go:192
  ▼
Decision sidecar
  │
  │  Human TTY + Ed25519 signing
  │  cmd/doctrust-review/review_flow.go:78
  ▼
doctrust_reviews.json (2 signed records)
  │
  │  svc.BuildArtifact()
  │  internal/service/doctrust.go:198
  ▼
audit.json (sealed, tamper-evident)
```

**For each transition**:
- `evidence_snapshot.json`: Documents carry `Type` (canonical, R16), `Hash`, `Filename`. Claims carry `Field`, `Value`, `ValueType`, `SemanticType`, `Sources[].SourceSpan` (page+bbox), `Sources[].Confidence`.
- `facts.Facts`: Keyed by `semantic_type`. Each `Fact` carries `Value`, `SourceDoc` (canonical type, R16), `FieldName`, `SourceSpan`, `Confidence`.
- `Result`: Carries `CheckID`, `Status`, `Severity`, `Reason`, `Evidence[]EvidenceRef`.
- `audit.json`: Carries `RulesetID/Version/Hash` (R11), `Decisions`, `Documents`, `HumanReviews`, `FinalDisposition`, `Manifest.ArtifactHash` (R15).

---

## 9. Ruleset / Evaluator Trace

**Ruleset**: `rulesets/shipment_release/v1.yaml`

```yaml
id: "shipment_release"
version: "1"
checks:
  - id: "required_shipment_documents"
    version: "1.0"
  - id: "gross_weight_reconciliation"
    version: "1.0"
```

**Loading**: `internal/eval/ruleset.go` — `LoadPromoted(domain)` reads `v1.yaml`, validates, computes hash. The promoted Ruleset is immutable.

**Checks registered**: `internal/eval/checks.go:5` — `DefaultRegistry()` registers both shipment checks:
- `RequiredShipmentDocumentsCheck` (`internal/eval/check_shipment.go:186`)
- `GrossWeightReconciliationCheck` (`internal/eval/check_shipment.go:17`)

**required_shipment_documents** (`internal/eval/check_shipment.go:186-252`):
- Collects all `SourceDoc` values from facts
- Normalizes via `normalizeDocType()` (exact canonical match only)
- Missing documents → `REVIEW/BLOCKING` ("insufficient evidence: N of 4 ...") — never FAIL, never PASS (R25)
- All present → `PASS/INFO`

**gross_weight_reconciliation** (`internal/eval/check_shipment.go:17-165`):
- Params: `semantic_type` (default `shipment.gross_weight`), `sources` (default all 4 types), `tolerance` (default 0.005)
- Groups observations by canonical source
- Conflicting duplicates within one source → `REVIEW`
- Any required source missing → `REVIEW/insufficient`
- Numeric mismatch → `REVIEW/BLOCKING` naming outliers with per-source observations in metrics
- All agree → `PASS/INFO`

**Runner**: `internal/eval/runner.go:64` — `RunRuleset()` iterates `ruleset.Checks`, calls `Check.Evaluate(facts, params)` for each, collects `[]Result`, passes to `DecisionAggregator.Decide()`.

---

## 10. Nutrient / Provider Trace

**Provider seam**: `internal/provider/provider.go` — generic `EvidenceProvider` contract (`ExtractFields` → `[]RawExtraction`).

**Nutrient implementation**: `internal/nutrient/client.go` — HTTP client calling Nutrient DWS extraction API.

**Boundary enforcement** (R13, R23): `internal/service` and `cmd/doctrust-mcp` must NEVER import `internal/nutrient`, `internal/extraction`, `internal/opa`, or `internal/provider`. Enforced by `make lint-imports`.

**Extraction flow**:
1. `cmd/ingest/shipment_mode.go:79` — `runShipmentMode()` creates `nutrient.NewClient(key, "")`
2. `internal/ingest/shipment.go:186` — calls `client.ExtractFields(path, schema, mode)` for each PDF
3. `internal/nutrient/client.go` — sends HTTP request to Nutrient DWS API, receives structured fields + bboxes
4. `internal/ingest/evidence_normalizer.go:26` — `Normalize()` converts to `evidence.Claim` with `SourceSpan`

**Provider extension points**:
- Nutrient = implemented (first provider)
- Foxit = future provider (roadmap)
- Doctavian = future provider (roadmap)

---

## 11. Hermes / MCP Trace

**Skill**: `skills/compliance-check-artifact/SKILL.md` — defines the compliance-check-artifact runbook for Hermes.

**MCP server**: `cmd/doctrust-mcp/main.go:14` — stdio JSON-RPC server with 5 tools.

**Tool surface**:
| Tool | Handler | What It Does |
|------|---------|--------------|
| `evaluate_case` | `handlers.go:150` | Load snapshot, evaluate, write decision sidecar |
| `get_findings` | `handlers.go:219` | Return findings from last evaluation |
| `get_evidence` | `handlers.go:262` | Return evidence refs for a specific finding |
| `get_ruleset` | `handlers.go:311` | Return active Ruleset metadata |
| `get_audit_artifact` | `handlers.go:337` | Generate tamper-evident audit artifact |

**Agent interaction** (Phase-6):
1. Agent sends `initialize` → server responds with capabilities
2. Agent sends `tools/call evaluate_case` → server evaluates, returns `{case_id, status, finding_count}`
3. Agent sends `tools/call get_findings` → returns findings with status/severity/reason
4. Agent sends `tools/call get_evidence` → returns evidence references with bboxes
5. Agent attempts `tools/call request_human_review` → server returns error `-32602` (R29)
6. Agent sends `tools/call get_audit_artifact` → server builds and returns sealed artifact

**Wire protocol evidence**: `eval_reqs.jsonl`, `eval_resp.jsonl`, `denied_proof.jsonl`, `denied_reqs.jsonl` — all from the Phase-6 run.

---

## 12. Adaptive Investigation Trace

**Source**: Phase-5 run (`20260824-185232-341898`)

**Turn 1** (`turn1.txt`):
```
2 of 4 documents available
→ required_shipment_documents: REVIEW/BLOCKING (insufficient)
→ gross_weight_reconciliation: REVIEW/BLOCKING (insufficient)
```

**Agent reasoning**: The transcript shows the agent correctly identified the missing documents (packing list, certificate of origin) and requested them through the authorized evidence provider.

**Release gate** (`released.txt`):
```
certificate_of_origin=04-certificate-of-origin.pdf
packing_list=02-packing-list.pdf
```

The Python release gate in `rehearse-hermes-shipment.sh` scanned `turn1.txt` for regex matches (`packing[_ -]?list`, `certificate[_ -]?of[_ -]?origin`). Only released files after the agent explicitly requested them. Exit code 3 if the agent did NOT request both documents.

**Turn 2** (`turn2.txt`):
```
4 of 4 documents
→ required_shipment_documents: PASS/INFO
→ gross_weight_reconciliation: REVIEW/BLOCKING (B/L outlier: 5150 vs 4650)
```

**Code mechanism**: `internal/eval/check_shipment.go:186-252` — `RequiredShipmentDocumentsCheck` returns `REVIEW/BLOCKING` when documents are missing, never FAIL. This is the structural driver of adaptive investigation: the system cannot resolve insufficient evidence on its own, and the agent cannot convert REVIEW to PASS.

---

## 13. Human Authority Trace

**Source**: Phase-6 run (`p6-20260825-015819-726232`)

**Agent surface** (5 tools, no human-review capability):
```
evaluate_case, get_audit_artifact, get_evidence, get_findings, get_ruleset
```

**Agent denial** (`denied_proof.jsonl`):
```json
{"error":{"code":-32602,"message":"unknown tool \"request_human_review\""}}
```

**Human TTY session** (`human_session.txt`):
1. Passphrase-gated key unlock: `key_id="owner"`
2. Re-evaluates from snapshot (F-4): `svc.LoadCase()` + `svc.Evaluate()`
3. Verifies sidecar matches re-evaluation: `compareFindings()`
4. Finding [0]: confirm — "Required documents present and corroborated."
5. Finding [1]: reject — "HOLD - gross weight mismatch vs corroborating documents"
6. Signs 2 Ed25519 records
7. `FINAL DISPOSITION: FAIL`
8. Artifact hash: `b2cc19a7309968b38d764808cba4b0024841238284e9c6bfc1edbf0229c39ac5`

**Ed25519 signing** (`internal/review/signing.go:89-111`):
- `SignRecord()` builds `canonicalPayload`: deterministic JSON covering case_id, snapshot_sha256, finding_index, action, note, reviewer_identity, channel, key_id, alg, ruleset binding, resolved_at
- `ed25519.Sign(priv, b)` produces the signature

**Verification** (`internal/service/reviews.go:115-172`):
- `LoadAuthorizedReviews()` is fail-closed
- For each record: verifies case binding, snapshot hash, ruleset binding, identity match, finding index range, and Ed25519 signature
- Only after ALL pass, adds reviews to the store

**Disposition** (`internal/review/store.go:82`):
- Any `reject` → `FAIL`
- All `confirm`/`override` → `PASS`
- Unresolved → `REVIEW`

---

## 14. Audit Reconstruction

**T+00** — `bin/ingest` extracts 4 trade documents via Nutrient DWS
- Actor: script
- Artifact: `evidence_snapshot.json`
- State: EvidenceGraph with 4 documents, 17 claims, 17 relationships
- Case ID: `shipment_e067246b18486d8b`

**T+01** — `bin/doctrust-review --provision` generates Ed25519 key pair
- Actor: script
- Artifact: `keydir/owner.key.enc`, `reviewers/owner.pub`
- State: Reviewer identity established

**T+02** — `bin/doctrust-mcp evaluate_case` evaluates snapshot
- Actor: script (MCP server)
- Artifact: `.decision.json`
- State: 2 findings, REVIEW/BLOCKING
- LoadCase ID: `56cf311f09d45996`

**T+03** — Agent attempts `request_human_review`
- Actor: script (agent simulation)
- Artifact: `denied_proof.jsonl`
- State: Error `-32602` — tool absent (R29)

**T+04** — Human at TTY terminal reviews findings
- Actor: human (scripted input)
- Artifact: `.doctrust_reviews.json`
- State: 2 Ed25519-signed records (confirm [0], reject [1])

**T+05** — `bin/doctrust-review` finalizes audit
- Actor: binary (same process as T+04)
- Artifact: `.audit.json`
- State: `final_disposition: FAIL`, `artifact_hash: b2cc19a7...`

**Walk backward from audit**:
```
audit.json
  → human_reviews[1].action = "reject"
  → doctrust_reviews.json (Ed25519 signature verified against owner.pub)
  → decision.json (findings, metrics, evidence refs)
  → evidence_snapshot.json (17 claims, 4 documents)
  → 01-commercial-invoice.pdf, 02-packing-list.pdf, 03-bill-of-lading.pdf, 04-certificate-of-origin.pdf
```

---

## 15. Execution Model

### 15.1 Reproduce Today

What a fresh engineer can run from the current repository:

**Credential setup** (primary path):

```bash
export NUTRIENT_DWS_EXTRACTION_API_KEY=your-key-here
```

The demo scripts inherit this from the shell environment. They do NOT source `.env`. The Go binary's `loadExtractionKey()` checks `os.Getenv("NUTRIENT_DWS_EXTRACTION_API_KEY")` first; `.env` is a fallback only when the variable is unset.

Credential flow:

```text
shell (export)
  ↓
Make → demo script → bin/ingest
  ↓
loadExtractionKey()                    (shipment_mode.go:30)
  ↓
os.Getenv("NUTRIENT_DWS_EXTRACTION_API_KEY")
  ↓
nutrient.NewClient(key, "")            (client.go:32)
  ↓
Authorization: Bearer <key>            (client.go:108)
```

**Available commands**:

| Command | Prerequisites | Expected Result |
|---------|---------------|-----------------|
| `make build` | Go 1.25.7 | 16 binaries in `bin/` |
| `make demo-pass` | `NUTRIENT_DWS_EXTRACTION_API_KEY` (live credits) | PASS status |
| `make demo-review` | `NUTRIENT_DWS_EXTRACTION_API_KEY` (live credits) | REVIEW/BLOCKING |
| `bin/regression --domain shipment_release` | Built binaries | 6/6 scenarios pass |
| `bin/verify-ruleset --domain shipment_release` | Built binaries | Ruleset integrity verified |
| `bin/verify-audit <snapshot>` | Built binaries | Audit artifact integrity verified |

**`.env` fallback** (conditional): The shipment-mode Go code reads `extraction_apikey=...` from `.env` relative to the working directory when the environment variable is unset. This is not the recommended path — the export method works regardless of CWD.

### 15.2 Verify Historical Execution

What can be independently reconstructed from frozen artifacts (no Nutrient credits needed):

| Verification | Artifacts | How |
|--------------|-----------|-----|
| Phase-5 progressive evidence | `turn1.txt`, `turn2.txt`, `released.txt`, snapshots | Read transcripts, verify claims match snapshot data |
| Phase-6 human authority | `denied_proof.jsonl`, `human_session.txt`, `.audit.json` | Verify MCP wire protocol, check audit artifact hash |
| Ed25519 signatures | `.doctrust_reviews.json`, `reviewers/owner.pub` | `bin/verify-audit <snapshot>` |
| Ruleset integrity | `rulesets/shipment_release/v1.yaml`, `v1.manifest.json` | `bin/verify-ruleset --domain shipment_release` |
| Agent denial of human-review | `denied_proof.jsonl` | Parse JSON-RPC error, confirm tool absence |

**This is the primary verification path for judges and engineers.** No live Nutrient run is required.

### 15.3 External Dependencies

What the original Hermes rehearsal depended on outside the repo:

| Dependency | Location | Impact |
|------------|----------|--------|
| PDF fixtures | `/home/chaschel/Desktop/biz/nutrient/doc-generator/examples/shipment-1047/generated` | `rehearse-hermes-shipment.sh` sources fixtures from here, not from `demo/` |
| Hermes binary | `$HOME/.local/bin/hermes` | Required for LLM agent execution |
| Nutrient API key | `NUTRIENT_DWS_EXTRACTION_API_KEY` | Required for live extraction |
| Nutrient quota | 15+ credits per 4-document extraction | The maintainer's hackathon credential is quota-limited. Live re-execution is supported with an operator-supplied `NUTRIENT_DWS_EXTRACTION_API_KEY` with sufficient quota. |
| LLM model | `nvidia/nemotron-3-ultra-550b-a55b:free` | Hermes model for agent reasoning |

The frozen artifacts inside the repo are self-contained for verification. Re-execution requires the external dependencies above.

**Important**: `historical verification ≠ fresh live rerun`. The frozen artifacts are the verification source. Re-execution is optional and requires sufficient Nutrient quota.

---

## 16. Screen-Recording Limitation

> **No continuous screen recording of the original Hermes session was retained. The interaction is independently verifiable through the recorded MCP transcripts, agent transcript, tool surface, evidence snapshots, and audit artifacts.**

The evidence bundle is sufficient without a recording because the verification strength comes from the **combined evidence**, not any single artifact:

- **Agent transcript** (`turn1.txt`, `turn2.txt`) — proves what the agent said and decided
- **MCP requests/responses** (`eval_reqs.jsonl`, `eval_resp.jsonl`) — proves the wire protocol exchange
- **Tool surface** (`denied_proof.jsonl`) — proves `request_human_review` was absent (R29)
- **Evidence snapshots** — proves Nutrient extraction occurred with structured fields and bboxes
- **Deterministic evaluation** — same snapshot + same Ruleset = same findings (reproducible without re-running)
- **Human signed review records** — proves a human at a TTY terminal made per-finding decisions
- **Audit artifact** — proves the full chain was sealed with tamper-evident hash

No single artifact is "cryptographic-level proof" in isolation. The strength is the bundle: every link in the chain has a corresponding artifact, and the artifacts cross-reference each other (case IDs, snapshot hashes, ruleset hashes, finding indices). An engineer can walk backward from the audit artifact to the original PDFs without trusting any single claim.

The missing screen recording is a **presentation gap, not an evidentiary gap**.

---

## 17. Failure Modes

| Failure | Detection | Consequence |
|---------|-----------|-------------|
| Nutrient API unavailable | `bin/ingest` returns error | No snapshot produced; demo cannot start |
| Snapshot path outside jail | `validateSnapshotPath()` rejects | MCP returns error; agent cannot evaluate |
| Ruleset hash mismatch | `verify-ruleset` fails | Trusted Ruleset compromised; rebuild required |
| Ed25519 signature invalid | `LoadAuthorizedReviews()` fails | Review records rejected; disposition blocked |
| Sidecar tampered | `compareFindings()` detects mismatch | Human TTY refuses to proceed |
| Unknown document type | `normalizeDocType()` returns unknown | Check returns REVIEW (unknown → insufficient) |
| Non-numeric gross weight | `GrossWeightReconciliationCheck` detects | REVIEW/BLOCKING with error metrics |
| Conflicting duplicates within one source | `GrossWeightReconciliationCheck` detects | REVIEW/BLOCKING |

---

## 18. Security / Trust Boundaries

| Boundary | Rule | Enforcement |
|----------|------|-------------|
| Provider separation | R13, R23 | `make lint-imports` — `internal/service` and `cmd/doctrust-mcp` never import `internal/nutrient`, `internal/extraction`, `internal/opa`, `internal/provider` |
| Agent authority | R29 | `request_human_review` removed from MCP tool surface; server returns `-32602` |
| Human TTY gate | R29 | `cmd/doctrust-review` `isInteractive()` refuses non-interactive execution |
| Snapshot path jail | R13 | `validateSnapshotRoot()` enforces allowed root; paths must resolve within jail |
| Ruleset immutability | R4, R5 | Promoted Ruleset is immutable; `LoadPromoted` failure → error, never silent fallback |
| Audit integrity | R15 | `hashable()` excludes `Manifest.ArtifactHash` to break self-referential cycle |
| Ruleset provenance | R11 | `artifact.SetRuleset()` must be called in both `handleFinalize` and `handleAudit` |
| Ed25519 fail-closed | R29 | Missing/forged/wrong-key/content-mismatched signatures fail closed |
| No credentials in artifacts | R27 | Passphrase never stored; API key resolved at process runtime only |

---

## 19. How to Independently Verify the Demo

**Step 1**: Inspect the frozen execution artifacts.

```bash
# Phase-5 run (progressive evidence)
cat demo/shipment_release/runs/20260824-185232-341898/turn1.txt
cat demo/shipment_release/runs/20260824-185232-341898/turn2.txt
cat demo/shipment_release/runs/20260824-185232-341898/released.txt

# Phase-6 run (human authority)
cat demo/shipment_release/runs/p6-20260825-015819-726232/denied_proof.jsonl
cat demo/shipment_release/runs/p6-20260825-015819-726232/human_session.txt
cat demo/shipment_release/runs/p6-20260825-015819-726232/available/evidence_snapshot.json.audit.json
```

**Step 2**: Verify the audit artifact hash.

```bash
bin/verify-audit demo/shipment_release/runs/p6-20260825-015819-726232/available/evidence_snapshot.json
```

**Step 3**: Verify Ruleset integrity.

```bash
bin/verify-ruleset --domain shipment_release
```

**Step 4**: Read the source code for the checks.

```bash
# The two shipment checks
cat internal/eval/check_shipment.go

# The service boundary
cat internal/service/doctrust.go

# The human review flow
cat cmd/doctrust-review/review_flow.go

# The audit artifact
cat internal/audit/artifact.go
```

**Step 5**: Verify the MCP tool surface.

```bash
cat cmd/doctrust-mcp/handlers.go  # 5 tools registered, no request_human_review
```

**Step 6**: Verify the provider boundary.

```bash
make lint-imports  # must pass — no nutrient/opa imports from service or mcp
```

---

## 20. Extension Points

| Extension | Mechanism | Current Status |
|-----------|-----------|----------------|
| New check | Add to `DefaultRegistry()` in `internal/eval/checks.go:5` | 5 checks registered |
| New domain | Add `rulesets/<domain>/v1.yaml` + scenarios | 2 domains (income_verification, shipment_release) |
| New provider | Implement `EvidenceProvider` interface in `internal/provider/provider.go` | Nutrient = implemented |
| New document type | Add `types.DocumentType` constant + extraction config | 8 types defined |
| New human channel | Add alternative to TTY in `cmd/doctrust-review/` | TTY = implemented |

---

## 21. Known Limitations

1. **No continuous screen recording** of the original Hermes session. The interaction is verifiable through MCP transcripts, agent transcript, and audit artifacts.

2. **Nutrient quota**. The maintainer's hackathon Nutrient credential is quota-limited. Live re-execution is supported with an operator-supplied `NUTRIENT_DWS_EXTRACTION_API_KEY` with sufficient Nutrient DWS quota. Frozen execution artifacts remain available for quota-free historical verification.

3. **External `doc-generator` dependency**: `rehearse-hermes-shipment.sh` sources PDF fixtures from `/home/chaschel/Desktop/biz/nutrient/doc-generator/examples/shipment-1047/generated`, not from the repository. The frozen artifacts inside the repo are self-contained for verification.

4. **Two-run composition**: The demo video composes two genuine historical executions (Phase-5 + Phase-6) into one narrative. Different case IDs prove they are separate runs.

5. **Human identity is not production IAM**: The reviewer identity is the OS username or `DOCTRUST_REVIEWER` override. This is a hackathon demonstration, not a production identity system.

6. **Foxit and Doctavian are roadmap items only**: Not implemented. The provider seam exists but only Nutrient is wired.

7. **Fixture provenance**. The 5,150 KG bill-of-lading mismatch was deliberately designed into the shipment-1047 fixture scenario to exercise the REVIEW/BLOCKING path. What is being demonstrated is the system's extraction, investigation, evidence reconciliation, authority boundary, and audit behavior — not discovery of a naturally occurring discrepancy.

8. **Attempt history**. The frozen artifacts do not retain a complete attempt count for the historical rehearsals; the documentation therefore makes no claim that the captured executions were first-attempt runs.

---

## 22. Appendix: Source Reference Index

### Go Source Files

| File | Lines | Purpose |
|------|-------|---------|
| `cmd/ingest/main.go` | ~217 | Ingestion entry point |
| `cmd/ingest/shipment_mode.go` | 147 | Shipment ingestion mode |
| `cmd/doctrust-mcp/main.go` | ~50 | MCP server entry point |
| `cmd/doctrust-mcp/handlers.go` | 378 | MCP tool handlers (5 tools) |
| `cmd/doctrust-mcp/pathutil.go` | ~40 | Snapshot path jail |
| `cmd/doctrust-review/main.go` | ~20 | Human review entry point |
| `cmd/doctrust-review/review_flow.go` | 315 | Human TTY review flow |
| `cmd/doctrust-review/provision.go` | ~80 | Key provisioning |
| `cmd/doctrust-review/tty.go` | ~50 | TTY gate |
| `internal/service/doctrust.go` | 266 | Application boundary |
| `internal/service/builder.go` | 61 | Facts builder |
| `internal/service/reviews.go` | ~180 | Review verification |
| `internal/eval/checks.go` | ~20 | Default registry |
| `internal/eval/check_shipment.go` | 253 | Shipment checks |
| `internal/eval/runner.go` | ~100 | Scenario runner |
| `internal/eval/aggregator.go` | ~60 | Decision aggregator |
| `internal/eval/ruleset.go` | ~120 | Ruleset loading |
| `internal/ingest/shipment.go` | 380 | Shipment ingestion |
| `internal/ingest/evidence_normalizer.go` | ~80 | Evidence normalization |
| `internal/ingest/pdfmedia.go` | ~30 | PDF media box parsing |
| `internal/nutrient/client.go` | ~150 | Nutrient API client |
| `internal/provider/provider.go` | ~30 | Provider seam |
| `internal/review/signing.go` | 207 | Ed25519 signing |
| `internal/review/keyfile.go` | ~180 | Key management |
| `internal/review/store.go` | ~120 | Review store + disposition |
| `internal/audit/artifact.go` | 219 | Audit artifact |
| `internal/types/doc_type.go` | ~30 | DocumentType constants |
| `internal/types/evidence_ref.go` | ~20 | EvidenceRef struct |

### Execution Artifacts

| Path | Proves |
|------|--------|
| `demo/shipment_release/runs/20260824-185232-341898/*` | Phase-5 progressive evidence |
| `demo/shipment_release/runs/p6-20260825-015819-726232/*` | Phase-6 human authority |
| `demo/shipment_release/reviewers/owner.pub` | Ed25519 public key |
| `rulesets/shipment_release/v1.yaml` | Frozen promoted Ruleset |
| `rulesets/shipment_release/v1.manifest.json` | Ruleset hash |
| `skills/compliance-check-artifact/SKILL.md` | Hermes runbook |
| `demo/assets/master_demo.md` | Video narration script |
| `demo/assets/S01-S08*.png` | Video screenshots |

### Scripts

| Script | Purpose |
|--------|---------|
| `scripts/rehearse-hermes-shipment.sh` | Phase-5 Hermes rehearsal |
| `scripts/rehearse-phase6-human.sh` | Phase-6 human authority rehearsal |
| `scripts/demo-pass.sh` | One-command PASS demo |
| `scripts/demo-review.sh` | One-command REVIEW demo |
| `scripts/judge-demo.sh` | LLM-free 8-act trust funnel |
| `scripts/render_phaseb_screenshots.py` | Screenshot rendering |
