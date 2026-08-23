# plan.md — Judge-Facing Demo Package (per plans8/prompt.md)

Status: APPROVED — executing
Date: 2026-08-23
Predecessor: Phase 1–3 FROZEN · LLM authoring = validated prototype, optional capability,
NOT on the demo critical path. No engineering; no OpenRouter; no Hermes dependency;
no mocks; no canned results.

## Deliverables

```
plans8/plan.md                       ← this file
plans8/scripts/judge-demo.sh         ← THE one judge-facing script (LLM-free)
plans7/rehearsal_report.md           ← appended "Judge Demo Package" section per §13:
     demo status PASS/FAIL table · exact commands · 2–3 min presenter script ·
     12 judge Q&A · optional LLM-authoring aside · actual limitations
```

## Decisions (owner-approved)
1. Acts 5–6 mismatch candidate is SCRIPT-GENERATED from a fixed heredoc into the sandbox:
   fresh unique Check ID per invocation, valid imports/metadata/adversarial provenance,
   exactly one deliberate semantic mismatch — Expected: PASS vs Actual: REVIEW.
   Script prints `DEMO FIXTURE — intentionally invalid policy expectation; not production data`.
2. Default evidence surface = pure MCP stdio (protocol-equivalent labeling if Hermes
   absent). Phase-4 web UI (`bin/server`) documented as an OPTIONAL visualization variant
   only — "the compliance decision does not depend on the UI."
3. §13 honored literally: the demo-package section is appended to
   `plans7/rehearsal_report.md` (cross-folder), preserving all prior rehearsal evidence.
4. Acts 5 and 6 use the SAME candidate semantics: Act 5 = human-facing rejection at
   review-check (approval blocked); Act 6 = promotion-layer defense via scratch-approved
   twin reaching Gate 5. Labeled defense-in-depth.

## Live sequence (single configuration, nothing mocked)

```
sandbox rsync (frozen repo) → make build && make mcp
ACT 1  fresh MCP stdio → evaluate_case(demo snapshot)
       capture case_id / ruleset_id / ruleset_version / decision / finding_count
ACT 2  get_findings → select clearest REVIEW finding → get_evidence(index) → page/bbox
ACT 3  request_human_review(confirm + presenter note) → resulting status
ACT 4  get_audit_artifact → ruleset id/version/hash · decision · review · artifact hash
ACT 5  DEMO FIXTURE label → generate mismatch candidate → review-check
       → approval BLOCKED (Expected PASS ≠ Actual REVIEW)
ACT 6  scratch-approved twin → promote-check → Gate 5 rejection
       → sha256(internal/eval ∥ rulesets ∥ scenarios) before == after
CLOSE  promoted Ruleset unchanged (v2 latest) · cleanup · git status --short
```

Acts 1–4 run against the sandbox copy of the REAL corpus (`rulesets/…/v2`,
`demo/income_verification/evidence_snapshot.json`) so Act 3's review-write lands in
scratch and the real repository stays pristine.

## Presenter materials (into plans7/rehearsal_report.md)

- Six-beat script with the specified messages per act + closing line:
  "DocTrust doesn't trust the agent to write the law."
- Optional 20-second LLM-authoring aside (verbatim wording from prompt §10).
- Optional web-UI variant note (visualization surface only).
- 12 judge Q&A answers grounded in implemented mechanisms.
- Safety-rules compliance statement.

## Execution steps

1. Write this plan.md.
2. Build `plans8/scripts/judge-demo.sh` (reusing proven MCP driver + failure-rehearsal
   patterns from plans7).
3. Run once end-to-end in a fresh sandbox; capture every stage's real output as evidence.
4. Append the Demo Package section to `plans7/rehearsal_report.md` using witnessed values.
5. Verify cleanup + `git status --short`; STOP ENGINEERING.
