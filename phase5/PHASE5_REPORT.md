# PHASE5_REPORT.md — Real Hermes Orchestration

Date: 2026-08-24 · Implements `plans11/prompt.md` per `plans11/plan.md`.
**Result: PASS — 13/13 automated assertions; genuine adaptive run completed
end-to-end with live Nutrient extraction and frozen-DocTrust evaluation.**

## Skill

| Item | Value |
|---|---|
| Canonical (source of truth) | `doctrust/skills/compliance-check-artifact/SKILL.md` |
| Derived runtime copy | `~/.hermes/skills/compliance-check-artifact/SKILL.md` |
| Deploy tooling | `doctrust/scripts/install-skill.sh` (idempotent; fails on divergence) |
| Load verification | `hermes skills list` → `compliance-check-artif… enabled` **plus** both live turns ran with `--skills compliance-check-artifact`; the agent's outputs follow the skill's runbook and forbidden-actions list verbatim |

## MCP

Persistent registrations in `~/.hermes/config.yaml` (`hermes mcp add`):
- `doctrust` → `bin/doctrust-mcp --domain shipment_release --rulesets-dir …/rulesets --snapshot-root …/demo/shipment_release` — 6 tools, connect-verified.
- `evidence` → `bin/evidence-mcp --snapshot-root …/demo/shipment_release --env-file …/doctrust/.env` — 2 tools, connect-verified.
- P5-7 honored: config carries only the `.env` PATH; secret-leak grep on
  `~/.hermes/config.yaml`: zero matches. Fixed trusted root:
  `demo/shipment_release/`, per-run sandboxes under `runs/<ts>/`.

## Adaptive workflow (real transcript, retained)

Run dir: `demo/shipment_release/runs/20260824-165349-229360/`
(`turn1.txt`, `turn2.txt`, `combined_transcript.txt`, `session_archive/`,
snapshots, provenance sidecar). Model: `nvidia/nemotron-3-ultra-550b-a55b:free`.

**Turn 1** — user request named ONLY the commercial invoice + bill of lading;
the sandbox `available/` directory genuinely contained only those two files
(PL + CO staged outside the snapshot root, unaddressable). Hermes built the
CI+B/L snapshot via `evidence-mcp`, evaluated via DocTrust:

> "**Conclusion:** The two documents you provided (commercial invoice + bill
> of lading) are **not sufficient** to satisfy the approved `shipment_release`
> policy. The policy requires **four** shipment documents total — you're
> missing the **certificate of origin** and **packing list**."

DocTrust: REVIEW — `required_shipment_documents` insufficient (2 of 4).

**Agent decision (its own words):** requested exactly the two missing standard
companion documents. Harness released precisely those from staging
(`released.txt`: packing_list=02-packing-list.pdf,
certificate_of_origin=04-certificate-of-origin.pdf) — a genuine availability
transition, enforced by the filesystem (P5-6), not by prompt instruction.

**Turn 2** — resumed session; agent ran `extend_evidence_snapshot` (live
Nutrient extraction of the released PDFs) and re-evaluated:

> "`gross_weight_reconciliation` | **REVIEW** | gross weight reconciliation
> failed: commercial_invoice, packing_list, certificate_of_origin,
> bill_of_lading differ (commercial_invoice=4650.00 KG, packing_list=4650.00
> KG, certificate_of_origin=4650.00 KG, bill_of…"

> "**Next step (per policy):** This REVIEW disposition with BLOCKING severity
> must be escalated to human authority… The case/audit ID is `1efb11169d1fa137`."

The agent explicitly declined to convert REVIEW to PASS and surfaced the audit
reference — the skill's forbidden actions held.

## Extended round

| Item | Value |
|---|---|
| Initial case (graph id) | `shipment_3a2ea0ec5ce1a41b` (CI+B/L only) |
| Extended case (graph id) | `shipment_33617040a4a1695e` |
| Previous case ID | recorded in sidecar `evidence_snapshot_extended.json.provenance.json` + tool output |
| Observations | CI 4650 · PL 4650 · CO 4650 · B/L **5150** KG (outlier: bill_of_lading) |
| DocTrust result | REVIEW / BLOCKING (`gross_weight_reconciliation`) |
| Provenance | every observation page+bbox (G1-proven extractor; unchanged contract) |

## Trust-boundary assertions

- Ruleset `shipment_release/v1.yaml` + manifest hashes identical pre/post run (A9).
- Agent cannot force PASS: repeated evaluations stay REVIEW (F1a); unknown
  tools rejected at protocol level — `"unknown tool \"force_pass\""` (F1b);
  no mutating tool exists on the MCP surface (F2).
- Provider failure is explicit: forced bad key ⇒ error, NO snapshot fabricated (F3).
- Missing evidence never PASS: live initial round + scenario corpus incl.
  `partial_evidence_insufficient` (F4).
- No fabricated human approval: transcript shows ZERO `request_human_review`
  calls (A10); authoritative audit check A10b confirms zero HumanReviewRecords
  for the final case; unit/service tests prove absent reviews hold REVIEW (F5).
- Frozen PDFs sha256-identical pre/post run.

## Test results

- Rehearsal assertions: **13/13 PASS** (A1–A10b, listed in run transcript tail).
- Failure rehearsals: **6/6 PASS** (`scripts/failure-rehearsals.sh`).
- Go suite: all packages green; `make lint-imports` PASS; shipment scenario
  corpus 6/6.
- Note: an earlier rehearsal attempt hit free-tier LLM rate limits
  (`max_retries_exhausted` request dump retained by Hermes); the recorded run
  completed without substitution — per P5-3 this would have been reported as
  an infrastructure failure had it not recovered.

## Limitations

- Human authorization is not exercised in Phase 5; `request_human_review`
  remains a Phase-6 human-only operation. Current enforcement is skill +
  transcript assertion + audit-artifact verification; caller-authentication
  (cryptographic human-channel separation) is designed for Phase 6.
- Session call-level capture depends on Hermes' request-dump artifacts (only
  written on provider retries); A10 therefore combines dumps, transcript
  prose analysis, and the authoritative A10b audit check.
- Evidence contract remains weight + reference fields (14 ground-truth-covered
  requests); broader field coverage is future work, not a Phase-5 gap.
- Free-tier model availability varies; runs pin `nemotron-3-ultra-550b-a55b:free`
  and fail honestly rather than substituting.

## HARD STOP

Phase 5 complete after the first successful end-to-end adaptive run. Not
started (per boundary): final human-review flow, final audit demo, judge
package, Foxit, Doctavian, policy authoring. Next planning phase builds on
this real result.
