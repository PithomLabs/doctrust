# G1 — Live Nutrient Extraction Proof: Report

Date: 2026-08-24 · Gate per `plans10/plan.md` §14 · Verdict: **PASS** (evidence
contract) with the end-to-end disposition demonstrated: **REVIEW / BLOCKING**.

## Chain executed (real API, real authority)

```text
4 frozen PDFs (doc-generator/examples/shipment-1047/generated/)
    ↓ bin/evidence-mcp path → cmd/ingest shipment mode
Nutrient DWS api.nutrient.io  (extraction/extract, citations enabled)
    ↓ trusted normalizer (internal/extraction configs + internal/ingest)
g1/evidence_snapshot.json        case_id(derived): shipment_a78341babe979f4f
    ↓ compare vs ground truth          scripts/g1_differ.py
g1/g1_extraction_report.md       14/14 requested fields MATCH, all page+bbox
    ↓ frozen doctrust-mcp evaluate_case (ruleset shipment_release v1)
status REVIEW · severity BLOCKING · findings 2
```

## A. Evidence-contract results (requested fields)

14/14 ground-truth-covered requests MATCHED across the four documents:
- Gross weights: CI 4,650.00 KG · PL 4,650.00 KG (summary + TOTALS row) ·
  CO 4,650.00 KG · **B/L 5,150.00 KG** — the deliberate anomaly is fully
  recoverable from live extraction.
- References: INV-2026-1047 · PKL-2026-1047 · MSL-620984110 ·
  CO-PH-2026-08912 · PH-EXP-1047.
- Container TCKU-918234-0 and seal 850474 recovered on all four documents.
- Every match carries page + bbox provenance (confidence ≈0.95).

## B. Coverage beyond the evidence contract

285 ground-truth hint entries exist; 14 are covered by the current evidence
contract; 271 (line items, 20-row crate schedule cells, party blocks, marks,
dates, ports) were NOT requested from the provider in this phase and are
reported for visibility only — not scored as failures, never backfilled.

## C. Defect found and fixed by G1 (the gate doing its job)

Live values arrive as "4,650.00 KG". The trusted normalizer's currency parser
stripped `$`/commas but not unit suffixes, leaving facts as non-numeric
strings. Fixed in `internal/extraction.ParseCurrencyValue` (+ unit test);
ingestion re-run; facts now numeric. This defect was invisible until a real
provider run exercised the actual production normalization path.

## D. End-to-end evaluation on live evidence

`doctrust-mcp --domain shipment_release evaluate_case`:

```json
{"case_id":"5c5256ddf35d13d4","status":"REVIEW","severity":"BLOCKING",
 "ruleset_id":"shipment_release","ruleset_version":"1","finding_count":2}
```

- finding 0 `required_shipment_documents` PASS/INFO — all 4 documents present.
- finding 1 `gross_weight_reconciliation` REVIEW/BLOCKING — observations:
  commercial_invoice=4650 · packing_list=4650 · certificate_of_origin=4650 ·
  bill_of_lading=5150 (KG); outliers: [bill_of_lading].

The agent does not decide this; the promoted Ruleset did.

## E. Artifacts

- `doctrust/g1/evidence_snapshot.json` (normalized evidence, hash-bound)
- `doctrust/g1/extraction_report.json` (per-document provider outcomes)
- `doctrust/g1/g1_extraction_report.md` (differ report v2)
- Code: provider seam (`internal/provider`), trade-doc types + configs,
  ingest shipment mode (`--extend-from` supported), thin `cmd/evidence-mcp`,
  Path-B checks `gross_weight_reconciliation` + `required_shipment_documents`,
  promoted `rulesets/shipment_release/v1.yaml` (hash d3f1a945…), scenario
  corpus 6/6 PASS, D7 domain-threading exception (tested), doc-generator
  `generated-pass/` control package (`--control` validator PASS).
- Full Go suite green (12 packages) · `make lint-imports` PASS (R13 intact).

## STOP

Per plan §14/GATE G1: stopping here for owner review. Hermes orchestration
(Phase 5+), compliance skill installation, human-review flow, audit trace
verification, and the demo harness proceed only after owner acceptance of
this report.
