# Phase-B Report

**Date**: 2026-08-25
**Status**: COMPLETE — STOP for owner review

---

## 1. Golden Run Result

### Selected Golden Execution

**Phase-6 human-authority run**: `demo/shipment_release/runs/p6-20260825-015819-726232/`

This is a **genuinely captured, previously verified execution** demonstrating the complete Phase-6 trust funnel:
- 4 trade documents ingested via Nutrient extraction
- DocTrust evaluation: REVIEW/BLOCKING (B/L outlier: 5,150 vs 4,650 KG)
- Agent denied `request_human_review` (R29 enforcement proven)
- Human TTY session: confirm finding 0, reject finding 1
- Ed25519-signed review records
- Final disposition: FAIL
- Tamper-evident audit artifact produced

### Second Independent Run

**Unavailable**. The current Nutrient account is quota-blocked (5 credits available, 15 required per extraction). A second independent live run was not possible.

This is documented honestly — the video does not claim two independent runs occurred.

---

## 2. Provenance Split

The final demo intentionally combines **two genuine historical executions**:

| Execution | Shots | What It Proves |
|-----------|-------|----------------|
| **Phase-5 progressive run** (`runs/20260824-185232-341898/`) | S02, S03, S04 | Initial evidence state → insufficient evidence → agent investigation → document extension |
| **Phase-6 human-authority run** (`runs/p6-20260825-015819-726232/`) | S01, S05, S06, S07 | DocTrust evaluation → evidence mismatch → human authority → audit |
| **Constructed** | S08 | Feasibility roadmap (graphical) |

**S02–S04** are from the verified Phase-5 progressive-evidence execution.
**S01, S05–S07** are from the verified Phase-6 human-authority execution.
**S08** is a constructed graphical roadmap.

The final video intentionally composes these genuine execution states into one narrative; it does not claim they occurred in one uninterrupted run.

---

## 3. Final Shot List

| Shot | ID | Timestamp | Source | Asset |
|------|----|-----------|--------|-------|
| S01 | business-context | 00:00–00:10 | Phase-6 (context) | S01-business-context.png |
| S02 | the-ask-nutrient | 00:10–00:35 | Phase-5 (initial state) | S02-the-ask-nutrient.png |
| S03 | insufficient-evidence | 00:35–00:50 | Phase-5 (progressive) | S03-insufficient-evidence.png |
| S04 | adaptive-investigation | 00:50–01:10 | Phase-5 (progressive) | S04-adaptive-investigation.png |
| S05 | evidence-mismatch | 01:10–01:30 | Phase-6 (evidence) | S05-evidence-mismatch.png |
| S05→S06 | transition | 01:30 | — | (narration only) |
| S06 | human-authority | 01:30–01:55 | Phase-6 (human) | S06-human-authority.png |
| S07 | audit-trail | 01:55–02:10 | Phase-6 (audit) | S07-audit-trail.png |
| S08 | feasibility-close | 02:10–02:30 | Constructed | S08-feasibility-close.png |

---

## 4. Final Narration Duration

**Estimated**: ~2:15 (based on master_demo.md word count: ~350 words at ~160 wpm)

Actual duration will be determined by owner's TTS generation.

---

## 5. TTS Verification

**Not yet generated**. Owner handles TTS after reviewing master_demo.md.

The master_demo.md contains the exact verbatim narration for each shot, including:
- The Nutrient DWS one-liner (S02, verbatim from README/DevPost)
- The S05→S06 transition sentence
- The feasibility close (S08)

No ad-libbing or paraphrasing after master_demo.md is frozen.

---

## 6. Screenshot/Timeline Mapping

| Shot | Duration | Screenshot | Notes |
|------|----------|------------|-------|
| S01 | 10s | S01-business-context.png | Title card |
| S02 | 25s | S02-the-ask-nutrient.png | MCP request/response + DWS one-liner |
| S03 | 15s | S03-insufficient-evidence.png | 2 of 4 docs, REVIEW/BLOCKING |
| S04 | 20s | S04-adaptive-investigation.png | Agent chooses PL + CO |
| S05 | 20s | S05-evidence-mismatch.png | 4,650/4,650/4,650/5,150 |
| S05→S06 | — | — | Connective narration only |
| S06 | 25s | S06-human-authority.png | MCP denial + TTY + signed records |
| S07 | 15s | S07-audit-trail.png | Audit artifact fields |
| S08 | 20s | S08-feasibility-close.png | Graphical roadmap |
| **Total** | **~150s** | | |

---

## 7. Deviations from Plan

| Planned | Actual | Reason |
|---------|--------|--------|
| 2 golden runs | 1 golden run + 1 progressive run | Nutient quota blocks second independent run |
| 8 continuous shots from one run | 2 provenance sources (Phase-5 + Phase-6) | Honest composition of genuine executions |
| S08 terminal-style | S08 graphical | Stronger visual impact for feasibility close |

---

## 8. Final Judge-Facing One-Sentence Takeaway

> **DocTrust is a compliance execution layer that lets AI agents investigate business documents against approved policy — without ever letting the agent become the authority that decides compliance.**

---

## 9. Deliverables

| File | Status |
|------|--------|
| `master_demo.md` | ✅ Written |
| `demo/assets/S01-business-context.png` | ✅ Rendered |
| `demo/assets/S02-the-ask-nutrient.png` | ✅ Rendered |
| `demo/assets/S03-insufficient-evidence.png` | ✅ Rendered |
| `demo/assets/S04-adaptive-investigation.png` | ✅ Rendered |
| `demo/assets/S05-evidence-mismatch.png` | ✅ Rendered |
| `demo/assets/S06-human-authority.png` | ✅ Rendered |
| `demo/assets/S07-audit-trail.png` | ✅ Rendered |
| `demo/assets/S08-feasibility-close.png` | ✅ Rendered |
| `demo/assets/README.md` | ✅ Written |
| `phaseB/PHASEB_REPORT.md` | ✅ Written |

---

## 10. Hard Stop

**STOP for owner review.**

After owner approval:
1. Owner generates TTS from master_demo.md narration
2. Owner plots S01–S08 screenshots against TTS timeline
3. Owner assembles final 2–4 minute video
4. Owner reviews against final quality gate

No TTS generated. No video assembled. No DevPost submission. No MVP changes.
