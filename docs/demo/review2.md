# review2.md — Adversarial Judge-Demo Review — Final Freeze

Method per prompt2.md §1: I executed the actual `plans8/scripts/judge-demo.sh` end-to-end (exit 0) in its fresh sandbox, parsed the live output, verified every script assumption against the real MCP implementation (`handlers.go` tool registry, CLI flags, `review.ActionConfirm`, `demo/income_verification/evidence_snapshot.json`), traced the two on-screen anomalies to their root causes in code, and confirmed the repository was pristine before and after (`git status --short` = 0 entries; sandbox and processes cleaned; logs intentionally retained under `/tmp/judge-demo-logs/`).

---

## 1. Executive Verdict

## **CONDITIONAL PASS — FIX BEFORE DEMO**

The demo is genuinely LLM-free, sandboxed, and live: every displayed decision, finding, bbox, hash, and version came from real execution in my run, and the trust mechanics (Gate 5 rejection, SHA-256 tree equality, promoted v2 unchanged) were witnessed, not narrated. But two presentation-layer defects make the screen contradict the narration at the demo's two most important beats — human authority (Act 4 prints `reviews=0` after a review was recorded) and safe policy change (Act 5 blocks for the *wrong reason*, never showing the Expected-vs-Actual mismatch the script promises). Both fixes are script/fixture edits inside `judge-demo.sh`; no engine, gate, or contract code changes. After those two edits and one rerun, this is an APPROVE.

---

## 2. Findings

### P1-A — Act 5 does not demonstrate semantic rejection; it blocks for missing adversarial provenance, and the package claims otherwise

- **SEVERITY:** P1 (misleading demo output; directly contradicts prompt2 §6's requirement that "the rejection must specifically demonstrate semantic validation")
- **LOCATION:** `plans8/scripts/judge-demo.sh:122-178` — the Act-5 fixture writes `check.go`, `metadata.yaml`, `scenarios.yaml` but **no `adversarial.yaml`**; `cmd/review-check/main.go:83-88` executes candidate scenarios only `if hasAdv`.
- **CLAIM:** "Expected PASS ≠ Actual REVIEW → approval blocked" (script PASS message; `plans7/rehearsal_report.md:194` claims this was "witnessed").
- **ACTUAL (live run):** With no adversarial file, review-check never executes anything — it prints "⚠ None authored yet" and offers only edit/reject/quit. Approval is blocked because human adversarial provenance is missing, not because the expectation is semantically wrong. The script's own PASS line shows the tell: `approval blocked — Expected PASS ≠ Actual REVIEW ()` — empty parentheses where the FAIL detail should be, because the grep found nothing (nothing ran). The rehearsal appendix's line "Expected PASS != Actual REVIEW/WARNING - approval BLOCKED" asserts a witnessed value that never appeared on screen.
- **REPRODUCTION:** Run `judge-demo.sh`; inspect the Act-5 block: no Expected-vs-Actual table, empty `()` in the PASS line.
- **IMPACT:** The act's entire message — "DocTrust independently executes it; the expectation is wrong" — is not demonstrated. A judge who asks "where did it show me the wrong expectation?" breaks the beat. The package also records evidence that wasn't produced, which is exactly the class of inaccuracy this product exists to prevent.
- **MINIMUM FIX:** Add an `adversarial.yaml` to the Act-5 fixture (same shape as the Act-6 twin, with an explicit expected `reason` so exactly one mismatch remains — see P2-C). Then review-check executes pre-approval, prints the real `FAIL expects_pass_but_engine_reviews: expected=PASS/INFO actual=REVIEW/WARNING` row, and blocks with the semantic reason. Adjust the Act-5 PASS-condition grep to match that row.

### P1-B — Act 4 prints `reviews=0` after Act 3 recorded a review; the screen contradicts the human-authority narration

- **SEVERITY:** P1 (misleading demo output at the human-authority beat)
- **LOCATION:** `judge-demo.sh:109` — parser reads `a.get('HumanReviews')`; the artifact's JSON tag is `human_reviews` (`internal/audit/artifact.go:20`). `service.BuildArtifact` does populate it (`doctrust.go:235-241`, `artifact.AddHumanReview`).
- **CLAIM:** "The decision and human action become part of a tamper-evident audit record" (Act 4 narration; appendix line 192 "note preserved in artifact flow").
- **ACTUAL (live run):** Act 3 records the review (response carries `review_id`, `resolved_at`); Act 4 then displays `reviews=0 disposition='REVIEW'`. The review IS inside the artifact — the script just reads the wrong key — so the judge-visible output denies exactly what the act claims.
- **REPRODUCTION:** Live run output, Act 4 line: `reviews=0`.
- **IMPACT:** Sharp judges catch the contradiction; the strongest trust beat (human authority reaching the audit record) displays as if it failed. Also nothing on screen shows a disposition delta, so "resulting status changes correctly" (§4) is invisible.
- **MINIMUM FIX:** Parse `human_reviews`; print the recorded entry (finding_index/action/note/resolved_at) and the final disposition, narrated as "confirm upholds the REVIEW disposition" so the (correct) unchanged disposition reads as a decision, not a no-op.

### P2-C — Act 6 shows a FAIL row with matching statuses; plan's "exactly one deliberate mismatch" is not met

- **SEVERITY:** P2
- **LOCATION:** Act-6 fixture `adversarial.yaml` (no expected `reason`); `eval` comparison includes reason.
- **ACTUAL (live):** `FAIL judge_gate5_adversarial_edge: expected=REVIEW/WARNING actual=REVIEW/WARNING` — statuses/severities match; the row fails on the unstated reason expectation. `plan.md` Decision 1 promised "exactly one deliberate semantic mismatch"; the run shows two failures, one of which looks like a checker bug to anyone without reason-comparison context.
- **IMPACT:** Invites the worst kind of judge question ("your gate says FAIL for identical outputs?") and costs implementation-detail explanation time.
- **MINIMUM FIX:** Set the fixture's expected `reason: "judge probe observation missing"` (making the adversarial case genuinely pass, 1/2, single mismatch) — or keep it and pre-empt with one narration line about reason-level comparison.

### P2-D — No explicit assertion that the MCP-reported Ruleset version equals the promoted latest

- **SEVERITY:** P2. ACT 1 prints `v2` and CLOSE asserts latest promoted is `2`, but nothing asserts `evaluate_case.ruleset_version == manifest/latest` in one place (prompt2 §2). One `assert` line in the Act-1 parser closes it.

### P2-E — Final status line prints `cleanup=PENDING`

- **SEVERITY:** P2. The trap cleans up immediately after the summary is printed; the label reads as unfinished work. Print the status line after cleanup, or label it `cleanup=auto(EXIT-trap)`.

### P2-F — Appendix "witnessed values" overstate the screen

- **SEVERITY:** P2 (folded into P1-A/P1-B). `plans7/rehearsal_report.md:192,194` record values the run never displayed. After the two fixes, regenerate this section from a fresh run so every "witnessed" row matches the screen verbatim. Also note for the presenter: the artifact hash legitimately changes between runs (timestamps are inside the hashed material — `d18120…` vs my run's `1691a6…`); present binding, not reproducibility, of the hash.

### Verified non-issues (attacked, held)

- **Canned-output attack (§1):** all displayed values (case_id, bboxes, hashes, counts) parsed from live responses; rerunning produces a different case_id and artifact hash while decisions stay deterministic — no replay possible.
- **Stale/wrong Ruleset (§2):** single fresh MCP process loads promoted v2 at startup; `ruleset_version` flows through every act; CLOSE re-asserts v2.
- **Evidence chain (§3):** live bbox rows correspond to the bundled PDFs' actual coordinates (paystub/w2/1040); `get_evidence` keyed by the same `case_id` + selected REVIEW index.
- **Human authority mechanics (§4):** `request_human_review` validates index bounds, records action/note/resolved_at; invalid actions are typed enums (`confirm|reject`).
- **Act 6 (§7):** scratch-approved twin genuinely reaches Gate 5 through the full promote path (validation 2 passed → execution), rejected 0/2, `SHA256(internal/eval ∥ rulesets ∥ scenarios)` equal before/after, promoted v2 intact, no partial files. The UI-bypass question is answered: yes, a bypassed approval surface still cannot inject bad policy — witnessed live.
- **Provider-agnostic (§9):** engine consumed only the normalized snapshot; `make lint-imports` clean; no vendor dependency anywhere on the demo path.
- **LLM-free (§14):** no OpenRouter, no model, no retries anywhere in the script; the optional aside wording matches the sanctioned prototype framing.
- **Cleanup/reproducibility (§15):** sandbox removed, MCP killed, `git status --short` = 0; second run reproducible (deterministic acts, fresh IDs/hashes where expected).

---

## 3. Act-by-act attack results

| Act | Verdict | Notes |
|---|---|---|
| 1 evaluate_case | PASS | Real ruleset v2, real case, REVIEW/3 findings; add version-equality assert (P2-D) |
| 2 findings→evidence | PASS | Real bbox trace into bundled PDFs; clearest REVIEW finding auto-selected |
| 3 human review | PASS (mechanics) / WEAK (display) | Recorded correctly; screen shows only key names, no values (feeds P1-B) |
| 4 audit artifact | FAIL (display) | `reviews=0` contradicts Act 3 (P1-B); ruleset identity/hash/disposition correct |
| 5 bad policy | FAIL (narrative) | Blocked for missing adversarial provenance; semantic mismatch never shown (P1-A) |
| 6 defense-in-depth | PASS | Gate 5 rejection + SHA equality live; confusing second FAIL row (P2-C) |
| Close | PASS | v2 asserted; final line as scripted; cleanup=PENDING label (P2-E) |

## 4. Trust/evidence verification table

| Invariant | Verified? | Evidence |
|---|---|---|
| Live execution, no canned results | YES | Fresh case_id/hash per run; all values parsed from MCP responses |
| Ruleset shown == Ruleset loaded | YES | v2 in Acts 1/4 and Close; explicit assert recommended (P2-D) |
| Finding → document bbox trace | YES | Three real bbox rows witnessed against bundled PDFs |
| Human review recorded | YES | review_id/resolved_at returned; artifact contains it (screen hides it — P1-B) |
| Disposition reflects human action | YES | `ComputeFinalDisposition` over reviews; confirm ⇒ REVIEW upheld (display fix needed) |
| Artifact carries ruleset identity/hash | YES | id/version/hash witnessed; hash binding per `hashable()` |
| Semantic policy rejection demonstrated | **NO (Act 5)** | Block reason = missing adversarial file; mismatch never executed (P1-A) |
| Promotion-gate independence | YES | Scratch-approved twin rejected at Gate 5 live |
| Trusted-tree immutability | YES | SHA-256 before == after, live |
| Promoted Ruleset unchanged | YES | v2 asserted at Close |
| Repo pristine / reproducible | YES | 0 modified files; sandbox/processes cleaned |
| LLM-free critical path | YES | No OpenRouter/model/latency dependency anywhere |
| Protocol-equivalent labeling | YES | Appendix states Hermes absent; stdio MCP used |

## 5. Judge comprehension risks

1. Act 4's `reviews=0` — the single most damaging on-screen artifact (P1-B).
2. Act 5's missing Expected-vs-Actual table — narration references a table that never appears (P1-A).
3. Act 6's identical-status FAIL row (P2-C).
4. Artifact hash changes between runs — pre-empt with "the hash binds this run's contents; it will differ per run by design."
5. `final_disposition=REVIEW` equal to the decision status — narrate as "human confirmed the engine's REVIEW," not as a state change.

## 6. Judge Q&A (grounded, condensed)

1. **vs an LLM answering?** Versioned deterministic checks over hash-bound evidence; no model in the runtime path; same input ⇒ same decision.
2. **Trace to document?** Every finding carries EvidenceRefs with document/page/bbox/confidence — live bbox rows shown in Act 2.
3. **Agent bypassing review?** Dispositions require recorded human action; the artifact embeds it; nothing the agent calls can finalize without it.
4. **Bad human rule?** Acts 5–6: deterministic execution against declared expectations blocks it at approval and again at promotion (Act 6 live-proven even with a bypassed approval surface).
5. **Halfway promotion failure?** Backup/restore per write target, scenario file included; byte-identical trusted tree (injection-tested; SHA equality shown live).
6. **Intended Ruleset?** Startup loads latest promoted; every response carries `ruleset_version`; `verify-ruleset` asserts version+hash against the manifest.
7. **Provider-agnostic?** Locked import boundary (`lint-imports`); engine consumes normalized snapshots from any extractor.
8. **Tamper-evident how?** Finalized manifest hash over canonical artifact content; post-finalization byte changes break the hash (Act 4 + regression suite).
9. **No LLM at runtime?** Architecture rule: LLM exists only in the optional authoring prototype, upstream of every gate shown.
10. **AI authoring eventually?** Same funnel — candidate → human adversarial review → deterministic verification → staged promotion (prototype validated end-to-end 2026-08-23).
11. **Why generate at all?** Lowers authoring cost for compliance teams; generation is convenience upstream, never an authority shortcut.
12. **Wrong proposed rule?** Today's Acts 5–6: rejected by the human-facing surface and independently by the promotion gate; trusted tree byte-identical.

## 7. Demo scorecard

| Criterion | Score |
|---|---:|
| Technical credibility | 9 |
| Demo reliability | 9 |
| Differentiation | 8 |
| Judge comprehension | 7 |
| Evidence traceability | 9 |
| Human authority story | 6 |
| Agent/MCP story | 9 |
| Auditability | 8 |
| Policy-change safety | 9 |
| **Overall winning potential** | **8** |

**Single biggest remaining weakness:** Act 5's rejection reason is not what the narration claims — the semantic mismatch is never shown (P1-A), with Act 4's `reviews=0` a close second (P1-B).

## 8. Exact minimum changes

1. Add `adversarial.yaml` to the Act-5 fixture (explicit expected reason; single mismatch); retarget the Act-5 PASS grep to the real FAIL row.
2. Fix the Act-4 parser key (`human_reviews`), print the review entry + disposition with "confirm upholds REVIEW" narration.
3. Act-6 fixture: set expected reason (or narrate reason-sensitivity) so only one FAIL row shows.
4. Add the Act-1 `ruleset_version == promoted latest` assert; fix the `cleanup=PENDING` label.
5. Regenerate the `plans7/rehearsal_report.md` Demo Package values from one fresh post-fix run so every "witnessed" row matches the screen.

All five are edits inside `judge-demo.sh` + the appendix text. No engine/gate/MCP changes.

## 9. Final recommendation

**CONDITIONAL PASS.** The six-act structure is the right product story and the machinery behind it is real — I watched Gate 5 eat a bypassed-approval bad policy while the trusted tree's SHA-256 stood still. Fix the two screen-vs-narration contradictions (Act 5's invisible mismatch, Act 4's `reviews=0`), apply the three small polish items, rerun once, regenerate the appendix from that run — then **freeze the demo. No further engineering. Rehearse presentation only.**
