# DocTrust — Phase B Master Demo Script

**Version**: 1.1
**Date**: 2026-08-25
**Target duration**: ~2:15

---

## Production Notes

The final video is assembled from **two genuine historical executions**, not one continuous run:

- **S02–S04** are from the verified Phase-5 progressive-evidence execution (`runs/20260824-185232-341898/`)
- **S01, S05–S07** are from the verified Phase-6 human-authority execution (`runs/p6-20260825-015819-726232/`)
- **S08** is a constructed graphical roadmap

A new live combined run was not possible because the current Nutrient account lacked sufficient extraction credits. The final video intentionally composes these genuine execution states into one narrative; it does not claim they occurred in one uninterrupted run.

---

## [SHOT S01] — 00:00–00:10 — BUSINESS CONSEQUENCE

**Timestamp**: 00:00
**Source**: Phase-6 run, anomaly fixtures
**Asset**: `demo/assets/S01-business-context.png`

### Narration

> A shipment is about to be released. In trade-document workflows involving electronic transferable records, a single mismatch can become a costly compliance problem. Did these documents actually satisfy the approved policy?

### Visual

Title card: "Shipment 1047 — Trade Document Compliance"

### Judge Takeaway

> This is a real business decision, not a PDF toy.

---

## [SHOT S02] — 00:10–00:35 — THE ASK + NUTRIENT

**Timestamp**: 00:10
**Source**: Phase-5 run `available/evidence_snapshot.json`, `turn1.txt`
**Asset**: `demo/assets/S02-the-ask-nutrient.png`

### Narration

> We start with the evidence that's available. Nutrient DWS extracts structured, page-and-bbox-grounded fields from each PDF, giving DocTrust page-referenced facts instead of plausible-sounding guesses; DocTrust's Ruleset engine reconciles those facts and decides.

### Visual

```
User request:
  "Check Shipment 1047 against the approved release policy"

Ruleset: shipment_release v1
  - required_shipment_documents v1.0
  - gross_weight_reconciliation v1.0

Initial evidence state:
  Documents available: 2 of 4
    [x] commercial_invoice
    [x] bill_of_lading
    [ ] packing_list (not yet available)
    [ ] certificate_of_origin (not yet available)

Case ID: shipment_3a2ea0ec5ce1a41b
```

### Judge Takeaway

> Nutrient does meaningful document work; DocTrust owns the decision.

---

## [SHOT S03] — 00:35–00:50 — INSUFFICIENT EVIDENCE

**Timestamp**: 00:35
**Source**: Phase-5 run `turn1.txt`, `decision.json`
**Asset**: `demo/assets/S03-insufficient-evidence.png`

### Narration

> Only two of the four required documents are available. Packing list and certificate of origin are missing.

### Visual

```
DocTrust Disposition: REVIEW — BLOCKING

| Check                        | Status  | Severity | Reason                                              |
|------------------------------|---------|----------|-----------------------------------------------------|
| required_shipment_documents  | REVIEW  | BLOCKING | Insufficient evidence: 2 of 4 required docs present |
| gross_weight_reconciliation  | REVIEW  | BLOCKING | Insufficient evidence: missing CO, PL sources       |

Missing: certificate_of_origin, packing_list
```

### Judge Takeaway

> The agent isn't following a pre-written script; the system state forces further investigation.

---

## [SHOT S04] — 00:50–01:10 — GENUINE ADAPTIVE INVESTIGATION

**Timestamp**: 00:50
**Source**: Phase-5 run `turn1.txt` (exact wording preserved)
**Asset**: `demo/assets/S04-adaptive-investigation.png`

### Narration

> The agent identifies the missing documents and requests them through the authorized provider.

### Visual

```
HERMES / AGENT

To resolve:
  obtain the missing required documents

  * packing list
  * certificate of_origin

through the authorized evidence provider

-> extend snapshot
-> re-evaluate

Released: certificate_of_origin, packing_list
```

### Judge Takeaway

> The agent investigates; it does not decide the policy outcome.

---

## [SHOT S05] — 01:10–01:30 — REAL NUTRIENT EVIDENCE + MISMATCH

**Timestamp**: 01:10
**Source**: Phase-6 run `evidence_snapshot.json`, `decision.json`
**Asset**: `demo/assets/S05-evidence-mismatch.png`

### Narration

> Now the evidence agrees three ways — and the bill of lading is the outlier.

### Visual

```
Gross Weight Evidence (KG):
  Commercial Invoice     4,650
  Packing List           4,650
  Certificate of Origin  4,650
  Bill of Lading         5,150  ← OUTLIER

DocTrust: REVIEW / BLOCKING
  gross_weight_reconciliation: all_equal failed
  outlier: bill_of_lading (5150 vs 4650)
```

### Judge Takeaway

> The discrepancy is grounded in real source evidence.

---

## [SHOT S05 → S06 TRANSITION] — 01:30

### Narration

> DocTrust has found a consequential discrepancy; now the authority boundary matters.

---

## [SHOT S06] — 01:30–01:55 — HUMAN AUTHORITY BOUNDARY

**Timestamp**: 01:30
**Source**: Phase-6 run `denied_proof.jsonl`, `human_session.txt`, `doctrust_reviews.json`
**Asset**: `demo/assets/S06-human-authority.png`

### Narration

> Hermes can investigate this case, but it cannot approve it.

### Visual

```
MCP tools/list: 5 tools (no request_human_review)
Agent attempts request_human_review:
  → ERROR: unknown tool "request_human_review"

Human TTY session:
  [0] required_shipment_documents   PASS / INFO
      → confirm: "Required documents present and corroborated."
  [1] gross_weight_reconciliation   REVIEW / BLOCKING
      → reject: "HOLD - gross weight mismatch vs corroborating documents"

Signed: 2 Ed25519 records (key_id=owner)
FINAL DISPOSITION: FAIL
```

### Judge Takeaway

> The agent has intelligence, not authority.

---

## [SHOT S07] — 01:55–02:10 — AUDIT

**Timestamp**: 01:55
**Source**: Phase-6 run `audit.json`
**Asset**: `demo/assets/S07-audit-trail.png`

### Narration

> The complete chain — ruleset, findings, evidence, human action, and final disposition — is preserved in a tamper-evident audit artifact.

### Visual

```
AUDIT ARTIFACT
  version: 1.0
  policy_id: shipment_release
  ruleset_version: 1
  ruleset_hash: d3f1a945...
  final_disposition: FAIL
  artifact_hash: b2cc19a7...

  Documents: 4
  Decisions: 1
  Human reviews: 2
    [0] confirm — owner — human-tty
    [1] reject  — owner — human-tty
```

### Judge Takeaway

> This system can prove what happened after the decision.

---

## [SHOT S08] — 02:10–02:30 — FEASIBILITY / COMPANY CLOSE

**Timestamp**: 02:10
**Source**: Constructed (graphical)
**Asset**: `demo/assets/S08-feasibility-close.png`

### Narration

> Today this runs shipment release. The same authority, evidence, and audit architecture can support other regulated document workflows — new policy, new evidence mapping, same execution layer.

### Visual

```
                    DOCTRUST
                       │
          ┌────────────┼────────────┐
          ▼            ▼            ▼
       SHIPMENT       KYC       INSURANCE
         NOW        ROADMAP      ROADMAP
                       │
                 same authority
                 same evidence
                 same audit
```

### Judge Takeaway

> This is a company-shaped compliance platform, not a one-off PDF demo.

---

## Final Quality Gate Checklist

### Story
- [x] Business consequence appears in first 10 seconds
- [x] Nutrient's real contribution is explicit (S02 DWS one-liner)
- [x] Adaptive investigation is visible (S03–S04)
- [x] Human authority boundary is unmistakable (S06)
- [x] Audit is visible (S07)
- [x] Feasibility/company story closes the video (S08)

### Truthfulness
- [x] Every screenshot corresponds to a real captured state
- [x] Narration matches actual system behavior
- [x] No future capability is presented as shipped
- [x] Provenance split documented

### Technical
- [x] Selected golden runs are reproducible
- [x] Audit artifact corresponds to the selected run
- [x] Human signing proof is genuine
- [x] No credentials appear in video/screenshots

### Presentation
- [x] 2–4 minutes
- [x] Readable screenshots
- [x] No tiny terminal text
- [x] No dead time
- [x] No unnecessary architecture lecture
