# Phase-B Demo Assets — S01–S08 Screenshots

**Generated**: 2026-08-25
**Source runs**:
- Phase-5: `demo/shipment_release/runs/20260824-185232-341898/`
- Phase-6: `demo/shipment_release/runs/p6-20260825-015819-726232/`

---

## Shot-to-Asset Mapping

### S01-business-context.png
- **Shot**: S01 — BUSINESS CONSEQUENCE (00:00–00:10)
- **Source**: Title card constructed from anomaly fixtures
- **Provenance**: Phase-6 run context
- **Content**: "Shipment 1047 — Trade Document Compliance"
- **Narration**: "A shipment is about to be released..."

### S02-the-ask-nutrient.png
- **Shot**: S02 — THE ASK + NUTRIENT (00:10–00:35)
- **Source**: `20260824-185232-341898/available/evidence_snapshot.json`, `turn1.txt`
- **Provenance**: Phase-5 progressive-evidence execution (initial state)
- **Content**: User request, Ruleset, initial evidence state (2 of 4 docs)
- **Narration**: "We start with the evidence that's available..." + DWS one-liner

### S03-insufficient-evidence.png
- **Shot**: S03 — INSUFFICIENT EVIDENCE (00:35–00:50)
- **Source**: `20260824-185232-341898/turn1.txt`, decision state
- **Provenance**: Phase-5 progressive-evidence execution
- **Content**: REVIEW/BLOCKING: 2 of 4 docs present, missing PL + CO
- **Narration**: "Only two of the four required documents are available..."

### S04-adaptive-investigation.png
- **Shot**: S04 — GENUINE ADAPTIVE INVESTIGATION (00:50–01:10)
- **Source**: `20260824-185232-341898/turn1.txt`, `released.txt`
- **Provenance**: Phase-5 progressive-evidence execution
- **Content**: Agent reasoning, document release (CO + PL)
- **Narration**: "The agent identifies the missing documents..."

### S05-evidence-mismatch.png
- **Shot**: S05 — REAL NUTRIENT EVIDENCE + MISMATCH (01:10–01:30)
- **Source**: `p6-20260825-015819-726232/evidence_snapshot.json`, `decision.json`
- **Provenance**: Phase-6 human-authority execution
- **Content**: Evidence table (4,650/4,650/4,650/5,150), outlier identification
- **Narration**: "Now the evidence agrees three ways..."

### S06-human-authority.png
- **Shot**: S06 — HUMAN AUTHORITY BOUNDARY (01:30–01:55)
- **Source**: `p6-20260825-015819-726232/denied_proof.jsonl`, `human_session.txt`, `doctrust_reviews.json`
- **Provenance**: Phase-6 human-authority execution
- **Content**: MCP denial, TTY session, signed review records, FAIL disposition
- **Narration**: "Hermes can investigate this case, but it cannot approve it."

### S07-audit-trail.png
- **Shot**: S07 — AUDIT (01:55–02:10)
- **Source**: `p6-20260825-015819-726232/audit.json`
- **Provenance**: Phase-6 human-authority execution
- **Content**: Audit artifact fields (ruleset, findings, reviews, disposition, hash)
- **Narration**: "The complete chain... is preserved in a tamper-evident audit artifact."

### S08-feasibility-close.png
- **Shot**: S08 — FEASIBILITY / COMPANY CLOSE (02:10–02:30)
- **Source**: Constructed graphical roadmap
- **Provenance**: N/A (constructed)
- **Content**: DocTrust → Shipment (now) / KYC (roadmap) / Insurance (roadmap)
- **Narration**: "Today this runs shipment release..."

---

## Rendering Details

- **Engine**: Pillow (PIL) 10.2.0
- **Font**: DejaVu Sans Mono (20pt), DejaVu Sans Mono Bold (20pt)
- **Background**: #0d1117 (dark theme)
- **Text**: #e6edf3 (light)
- **Resolution**: 2x upscale via LANCZOS
- **Script**: `scripts/render_phaseb_screenshots.py`

---

## Provenance Split

| Shot | Execution Source |
|------|-----------------|
| S01 | Phase-6 run (context) |
| S02 | Phase-5 run (initial evidence state) |
| S03 | Phase-5 run (progressive evidence) |
| S04 | Phase-5 run (agent investigation) |
| S05 | Phase-6 run (evidence + mismatch) |
| S06 | Phase-6 run (human authority) |
| S07 | Phase-6 run (audit) |
| S08 | Constructed (graphical) |

**Note**: S02–S04 are from the verified Phase-5 progressive-evidence execution. S01, S05–S07 are from the verified Phase-6 human-authority execution. S08 is a constructed graphical roadmap. The final video intentionally composes these genuine execution states into one narrative; it does not claim they occurred in one uninterrupted run.
