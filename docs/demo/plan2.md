# plan2.md — Judge-Demo Presentation Fixes (prompt3.md × review2.md §8)

Status: APPROVED — executing
Date: 2026-08-23
Scope: `plans8/scripts/judge-demo.sh` edits + regenerated appendix in
`plans7/rehearsal_report.md`. Zero engine/MCP/gate/Ruleset/LLM/architecture changes.

## Fixes

F1 — Act 5 semantic rejection visible (review2 P1-A)
  - Act-5 fixture gains adversarial.yaml: origin human_adversarial, input lacks the
    probe fact, expected REVIEW/WARNING with explicit reason "judge probe observation
    missing" (matches the check's deterministic missing-fact path) so exactly ONE
    mismatch remains (the primary expects_pass_but_engine_reviews scenario).
  - Verification retargeted: require the literal row
      FAIL expects_pass_but_engine_reviews: expected=PASS/INFO actual=REVIEW/WARNING
    in review-check output AND state != APPROVED; captured row goes into the PASS
    message (no empty parens).

F2 — Act 4 human-authority truth (P1-B)
  - Parse artifact key human_reviews (snake_case tag); print each entry's
    finding_index/action/note/resolved_at + final_disposition; narration:
    "confirm upholds the engine's REVIEW disposition."

F3 — Act 6 single mismatch (P2-C)
  - Twin fixture's adversarial expectation gains the explicit matching reason → its row
    passes; exactly one deliberate FAIL row remains; Gate still rejects (≥1 failure).

F4 — version assert + truthful cleanup (P2-D/E)
  - Compute sandbox-promoted latest before Acts; assert evaluate_case.ruleset_version
    == latest; abort on mismatch (assert-before-proceed).
  - Explicit pre-summary cleanup invocation with idempotent-guarded trap ⇒ status line
    prints cleanup=PASS truthfully.

F5 — Appendix regeneration (P2-F)
  - One fresh post-fix run; REPLACE the "Judge Demo Package" section of
    plans7/rehearsal_report.md in place (marked superseding the earlier capture);
    every witnessed row matches screen verbatim; include presenter notes:
    artifact hash varies per run by design (binding, not reproducibility);
    disposition narrated as confirm-upheld.

## Execution order

apply F1–F4 → fresh sandbox run → verify all five fixes live → replace Demo Package
evidence → cleanup + git status --short → FREEZE DEMO (presentation-only from here).
