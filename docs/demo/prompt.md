Use this prompt:

````text id="f9g4k2"
# FINAL PHASE — JUDGE-FACING DEMO PACKAGE
## No new engineering

Phase 1–3 are FROZEN.

Final product decision:

- The deterministic DocTrust engine + MCP workflow is the critical product/demo.
- LLM authoring is a VALIDATED PROTOTYPE / OPTIONAL CAPABILITY.
- Do NOT make an external LLM a dependency of the live demo.
- Do NOT reopen the LLM-authoring experiment.
- Do NOT modify Phase 1/2/3 code, trust gates, MCP contracts, evaluator semantics,
  promotion architecture, or Ruleset semantics.
- Do NOT add features.

Your job is now to produce the final judge-facing demo package.

## PRIMARY DEMO STORY

The demo must prove:

```text
Real document case
→ deterministic DocTrust evaluation
→ finding
→ exact document evidence / bounding box
→ human review
→ tamper-evident audit artifact
→ attempted bad policy change
→ deterministic rejection
→ trusted state unchanged
````

Core message:

> **DocTrust gives autonomous agents trusted, evidence-traceable compliance
> decisions with human authority and tamper-evident policy control.**

The LLM-authoring capability should appear only as a brief optional capability:

> "We also built and validated an AI authoring prototype. It feeds the same
> trust funnel, but the compliance runtime never depends on an LLM."

Do NOT make that prototype part of the critical happy path.

---

# 1. INSPECT THE FROZEN DEMO SURFACE

Read the current:

* README.md
* AGENTS.md
* plans7/plan.md
* plans7/rehearsal_report.md
* final implementation/freeze reports
* actual CLI help/output
* MCP tool definitions
* existing demo/rehearsal scripts

Determine the exact commands and outputs supported by the CURRENT tree.

Do not invent commands.

---

# 2. BUILD THE 2–3 MINUTE DEMO SCRIPT

Create/update ONE judge-facing script only if necessary.

Use a clean isolated sandbox.

The live sequence should be:

```text id="g8b0s2"
START
 ↓
fresh MCP/runtime
 ↓
evaluate_case
 ↓
get_findings
 ↓
get_evidence
 ↓
request_human_review
 ↓
get_audit_artifact
 ↓
attempt bad policy change
 ↓
review / deterministic mismatch
 ↓
promotion gate rejection
 ↓
trusted-tree hash unchanged
 ↓
DONE
```

No OpenRouter dependency.

No live LLM generation.

No model selection.

No waiting for external inference.

---

# 3. ACT 1 — EVALUATE REAL DOCUMENT

Use the existing bundled/demo case and the already-promoted Ruleset.

Run:

```text id="9u0l2h"
evaluate_case
```

Capture:

* decision
* ruleset ID
* ruleset version
* case ID
* finding count

Presenter message:

> "Hermes asks DocTrust to evaluate the document. The agent doesn't make the
> compliance decision itself; DocTrust applies a trusted, versioned policy."

---

# 4. ACT 2 — SHOW FINDING + EVIDENCE

Run:

```text id="p9o4hr"
get_findings
```

Select the most understandable finding.

Then:

```text id="v48xqk"
get_evidence
```

Show the actual page/bounding-box evidence in the document/UI if the current
demo surface supports it.

Presenter message:

> "The result isn't a black-box classification. We can trace the finding back
> to the exact evidence in the document."

Do not manufacture or hardcode evidence.

---

# 5. ACT 3 — HUMAN AUTHORITY

Run:

```text id="8sbkwp"
request_human_review
```

Use a real REVIEW finding.

Show:

* proposed disposition
* human action
* human note
* resulting status

Then explain:

> "The agent can surface the issue, but a human remains the authority for the
> disposition."

Do not imply the human review is simulated if the current runtime actually records it.

---

# 6. ACT 4 — AUDIT ARTIFACT

Run:

```text id="e7wkq6"
get_audit_artifact
```

Show the most important fields:

```text id="0fl4nq"
Ruleset ID
Version
Ruleset hash
Decision
Finding
Evidence
Human review
Artifact hash
```

Presenter message:

> "The decision and human action become part of a tamper-evident audit record."

Do not overwhelm the judge with implementation detail.

---

# 7. ACT 5 — BAD POLICY CHANGE

This replaces the live LLM-authoring act.

Use the already-prepared, valid candidate fixture with:

* fresh unique Check ID
* valid structure
* valid imports
* valid metadata
* valid adversarial provenance
* deliberately incorrect scenario expectation

Example concept:

```text id="f4r2nu"
Actual deterministic result: REVIEW
Expected candidate result: PASS
```

Run:

```text id="5urcpi"
review-check
```

Show:

```text
Expected   Actual
PASS       REVIEW
```

Approval must be blocked.

Presenter message:

> "It doesn't matter who wrote the proposed rule. DocTrust independently
> executes it. The expectation is wrong, so the change cannot become trusted."

---

# 8. ACT 6 — DEFENSE IN DEPTH

Use the scratch-only approved candidate path to make the same mismatch reach
the promotion gate.

Run:

```text id="4n7qdx"
promote-check
```

Expected:

```text
Gate 5 fails
promotion blocked
```

Then prove:

```text id="5y0e0p"
SHA256(before)
==
SHA256(after)
```

for the trusted:

* internal/eval tree
* rulesets
* scenarios

Presenter message:

> "And even if someone bypassed the first approval surface, the promotion gate
> independently rejects the change. Trusted state did not move."

Explicitly label this as:

> **Defense-in-depth verification of the promotion gate.**

---

# 9. CLOSE THE DEMO

Return to the successful runtime state.

Make clear:

```text id="y9p7hj"
bad rule was never promoted
trusted Ruleset remains unchanged
```

Final line:

> **"DocTrust doesn't trust the agent to write the law. It makes sure the policy
> the agent consumes is trusted."**

---

# 10. OPTIONAL LLM SLIDE

Prepare ONE optional slide or 20-second verbal aside:

```text
AI authoring prototype:
Plain English
→ constrained candidate
→ same DocTrust trust funnel
→ trusted Ruleset
```

Say:

> "We also validated an AI-assisted authoring prototype. We deliberately keep
> it outside today's critical runtime path because compliance infrastructure
> should not depend on an external model being available."

Do NOT demonstrate model selection, retries, free-model failures, or Go-generation
internals unless a judge specifically asks.

---

# 11. JUDGE Q&A

Prepare concise, evidence-grounded answers to:

1. What exactly makes this different from an LLM simply answering a compliance question?
2. How do you trace a decision to source evidence?
3. Can an agent bypass the human review?
4. Can someone sneak a bad rule into production?
5. What happens if promotion fails halfway through?
6. How do you know the agent is using the intended Ruleset version?
7. Why is the engine provider-agnostic?
8. What does the audit artifact contain?
9. Why isn't an LLM required at runtime?
10. How will AI-assisted rule authoring work eventually?
11. Why generate rules at all?
12. What happens if the proposed rule is wrong?

Answers must use the actual implementation, not hypothetical future behavior.

---

# 12. DEMO SAFETY RULES

The live demo MUST:

* use a clean isolated sandbox
* use existing frozen fixtures
* not modify the real repository
* not depend on OpenRouter
* not depend on Hermes availability
* not use mocks
* not use canned compliance outputs
* not hardcode success/failure results
* clean all scratch state afterward
* verify `git status --short`

If Hermes itself is unavailable, use the real MCP protocol and label it:

> **protocol-equivalent — Hermes not available in this environment**

Never claim Hermes ran if it did not.

---

# 13. REQUIRED DELIVERABLE

Update/create:

```text
plans7/rehearsal_report.md
```

with:

### Demo status

PASS/FAIL for:

* evaluate_case
* findings
* evidence
* human review
* audit artifact
* bad policy rejection
* promotion-gate rejection
* trusted-tree immutability
* cleanup

### Exact demo commands

### 2–3 minute presenter script

### Judge Q&A

### Optional LLM-authoring note

### Actual remaining limitations

Do not turn limitations into new engineering tasks.

---

# 14. FINAL RULE

After producing the package:

**STOP ENGINEERING.**

No new features.
No new prompt experiments.
No new model experiments.
No architecture changes.
No more trust-funnel redesign.

The product is now the deterministic compliance engine + MCP + evidence +
human authority + audit + safe policy change.

The LLM authoring prototype is optional future capability.

The next step after this package is the actual judge rehearsal.

```
```

