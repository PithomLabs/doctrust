# DocTrust Rule Authoring & Trust Testing
## Reference for AI Agents and Human Reviewers

> **Purpose:** Define how a plain-English compliance requirement becomes an executable, tested, and trusted DocTrust rule.
>
> **Core principle:** An AI model may propose compliance logic, but it must never be the final authority on whether its own proposal is correct.

---

## 1. The Core Idea

DocTrust separates **rule creation** from **rule trust**.

A non-technical user can express a compliance requirement in ordinary business language:

> Projected annual income from a paystub must not exceed corroborated W-2 wages by more than 5%, unless the difference is explained by documented bonus compensation.

The AI authoring layer translates that requirement into executable artifacts:

```text
Plain-English requirement
        ↓
AI authoring layer
        ↓
Check + Scenarios + Ruleset changes
        ↓
Human semantic review
        ↓
Human-authored adversarial scenario
        ↓
Deterministic execution
        ↓
Expected vs. actual comparison
        ↓
Human approval
        ↓
Promotion
```

The important boundary is:

```text
AI proposes
Human defines meaning
Deterministic engine evaluates
Human approves promotion
```

The AI does **not** grade itself.

---

# 2. What the User Actually Authorizes

The human reviewer is not primarily reviewing Go source code.

The human is reviewing the **meaning of the rule** and the **decision boundaries**.

The human answers questions such as:

- What exactly does this rule mean?
- What should happen at the boundary?
- Which evidence counts?
- Which exceptions are valid?
- What should happen when evidence is missing?
- What unusual case would expose a misunderstanding?

This is domain judgment, not programming.

---

# 3. The Three Different Artifacts

DocTrust separates three concepts.

## 3.1 Check

The **Check** is executable compliance logic.

Conceptually:

```text
Facts + Evidence + parameters
        ↓
Result
```

A Check produces a deterministic result such as:

```text
PASS
REVIEW
FAIL
```

with associated severity, reason, metrics, and evidence references.

In the current implementation, the canonical runtime Check is Go code.

---

## 3.2 Scenario

A **Scenario** is a test case describing facts/evidence and the expected result.

Scenarios are the semantic test fixtures for a Check.

Conceptually:

```yaml
name: variance_within_tolerance
expected: PASS
facts:
  ...
```

The precise YAML schema is defined by the DocTrust scenario contract.

---

## 3.3 Ruleset

A **Ruleset** determines which Checks are active and which parameters/versions apply.

Conceptually:

```text
Ruleset
 ├── Check A v1
 ├── Check B v2
 └── Check C v1
```

A Ruleset can evolve independently of the implementation of every Check.

---

# 4. Why AI-Generated Tests Alone Are Not Enough

Suppose the AI generates:

```text
wrong implementation
+
wrong expected outputs
=
100% green tests
```

This can happen because the same model controls both sides of the comparison.

For example:

```text
AI writes:
    "5.1% variance → PASS"

AI also writes:
    expected = PASS
```

A deterministic runner can correctly report:

```text
expected PASS
actual PASS
```

without proving that the business rule actually means that.

This is a form of **self-selection of test coverage** and **self-selection of expected behavior**.

DocTrust therefore adds independent human judgment.

---

# 5. The Trust Funnel

The complete authoring trust process is:

```text
1. Plain-English requirement
            ↓
2. AI proposes Check
            ↓
3. AI proposes scenarios
            ↓
4. Human reviews semantic meaning
            ↓
5. Human adds an independent adversarial scenario
            ↓
6. Deterministic runner executes scenarios
            ↓
7. Expected vs. actual comparison
            ↓
8. Human resolves mismatches
            ↓
9. Approval bound to exact content/version
            ↓
10. Promotion
```

| Stage | Main question |
|---|---|
| AI authoring | Can the requirement be translated into executable logic quickly? |
| Human semantic review | Did the AI understand what the business rule means? |
| Human adversarial scenario | What important case might the AI have missed? |
| Deterministic execution | Does the implementation actually behave as specified? |
| Mismatch review | Which side is wrong: implementation, scenario, or expectation? |
| Approval | Has the human accepted the exact rule that will be promoted? |
| Promotion | Is the approved artifact now the authoritative runtime rule? |

---

# 6. Step 1 — Start with Plain English

A user writes a requirement in business language.

Example:

> Projected annual income from a paystub must not exceed corroborated W-2 wages by more than 5%, unless the difference is explained by documented bonus compensation.

The AI authoring layer should identify:

### Decision subject

Income consistency.

### Evidence

Potentially:

- paystub gross/projected income
- W-2 wages
- documented bonus compensation

### Parameter

```text
tolerance = 5%
```

### Boundary

The phrase:

> "more than 5%"

creates an important boundary question:

```text
Exactly 5%?
→ PASS or REVIEW?
```

That cannot safely be inferred without human interpretation.

---

# 7. Step 2 — AI Proposes the Check

The model may propose something conceptually like:

```text
Check:
gross_income_consistency

Parameter:
tolerance = 0.05

Behavior:
- within tolerance → PASS
- beyond tolerance → REVIEW
- documented bonus may explain variance
```

The implementation is generated in the canonical runtime format.

In the current DocTrust implementation, that means a Go Check plus associated metadata.

The AI is proposing implementation, not establishing truth.

---

# 8. Step 3 — AI Proposes Scenarios

The AI should generate representative scenarios around the decision boundary.

Example:

| Scenario | Inputs | AI expected result |
|---|---|---|
| Exact match | W-2 $120k, paystub $120k | PASS |
| 4% variance | W-2 $120k, paystub $124.8k | PASS |
| 5% boundary | W-2 $120k, paystub $126k | PASS/REVIEW — human decides |
| 5.1% variance | W-2 $120k, paystub $126.12k | REVIEW |
| 15% variance | W-2 $120k, paystub $138k | REVIEW |
| 15% + documented bonus | W-2 $120k, paystub $138k, bonus $18k | REVIEW/exception behavior — human decides |

The purpose is not merely quantity.

The scenarios should exercise:

- normal cases
- threshold boundaries
- exception behavior
- missing evidence
- contradictory evidence
- unusually large deviations
- evidence-source relationships

---

# 9. Step 4 — Human Semantic Review

The reviewer now asks:

> "Are these the outcomes we actually want?"

This is the first authoritative human gate.

For each proposed scenario, the reviewer validates the expected behavior.

Example:

```text
Scenario:
W-2 = $120,000
Paystub = $126,000

AI expectation:
PASS

Human:
REVIEW
```

The reviewer is allowed to correct the expected result.

This is important:

> **The human is approving the meaning of the scenario, not merely agreeing with the AI's conclusion.**

---

# 10. Boundary Testing

Compliance rules often fail at boundaries.

Examples:

```text
< 5%
= 5%
> 5%
```

or:

```text
document exists
document missing
document malformed
document present but contradictory
```

Human review should deliberately inspect these boundaries.

A useful reviewer question is:

> "What happens one unit before and one unit after the threshold?"

For a percentage threshold:

```text
4.9% → ?
5.0% → ?
5.1% → ?
```

For a required-document rule:

```text
all documents present → ?
one missing → ?
wrong document type → ?
duplicate document → ?
```

---

# 11. Step 5 — Human-Adversarial Scenario

This is the most important independent-coverage safeguard.

The reviewer should create at least one scenario **without copying the AI's scenario set**.

The goal is:

> Find a case that the authoring model might not have considered.

Example:

> The paystub shows $138,000 projected income, W-2 shows $120,000, and the $18,000 difference is documented in a separate bonus statement rather than the paystub itself.

The scenario is explicitly marked as human-authored adversarial coverage.

Conceptually:

```text
origin = human_adversarial
```

This matters because:

```text
AI-created examples
+
human-created adversarial example
```

is much stronger than:

```text
AI-created examples
+
human approval of AI-created examples
```

---

# 12. What Makes an Adversarial Scenario Good?

A useful human-adversarial case should attack a plausible failure mode.

Good examples include:

### Boundary attack

> Exactly at the threshold.

### Near-boundary attack

> Just above or below the threshold.

### Missing-evidence attack

> Required supporting document is absent.

### Alternate-document attack

> The needed semantic fact is present in a different document.

### Contradiction attack

> Two sources disagree.

### Identity attack

> A filename looks like the expected document, but the canonical document type is different.

### Exception attack

> An exception is present but represented differently from the AI's examples.

### Multiplicity attack

> Two sources provide the same fact with different values.

### Ordering attack

> Multiple sources exist and source order could influence selection.

The exact adversarial case depends on the Check.

---

# 13. Step 6 — Deterministic Execution

Once the expected outcomes are human-approved, DocTrust executes the actual Check.

The important distinction is:

```text
EXPECTED
    vs.
ACTUAL
```

For example:

| Scenario | Expected | Actual | Status |
|---|---:|---:|---|
| exact match | PASS | PASS | ✅ |
| 4% variance | PASS | PASS | ✅ |
| 5% boundary | PASS | PASS | ✅ |
| 5.1% variance | REVIEW | REVIEW | ✅ |
| documented bonus | REVIEW | REVIEW | ✅ |
| human-adversarial | REVIEW | REVIEW | ✅ |

This execution is deterministic.

The LLM is not called to judge whether the Check passed.

---

# 14. When a Scenario Fails

A failed scenario does **not** immediately mean the Check is wrong.

There are three possibilities:

```text
A. The Check implementation is wrong
B. The scenario expectation is wrong
C. The human interpretation is wrong
```

Example:

```text
Expected: PASS
Actual:   REVIEW
```

The reviewer investigates.

### Case A — Implementation wrong

The business meaning really is PASS.

Fix the Check.

### Case B — Scenario wrong

The implementation matches the intended rule.

Fix the scenario's expected result.

### Case C — Requirement ambiguous

The organization needs a policy decision.

Human clarification is required before promotion.

DocTrust should not let the AI silently decide.

---

# 15. Never "Fix" a Mismatch by Making the Test Easier

A dangerous anti-pattern is:

```text
Check fails scenario
    ↓
change expected result
    ↓
green
```

That is only valid when the human has independently determined that the original expectation was wrong.

The proper question is:

> "What is the correct business behavior?"

not:

> "How do we make the suite green?"

---

# 16. Expected Results Are the Semantic Contract

The scenario expected result is effectively a lightweight semantic specification.

For a trusted Check:

```text
known input
+
known expected behavior
=
executable semantic contract
```

This is one reason scenarios matter as much as the Check itself.

---

# 17. Why Real Fixtures Matter

Synthetic scenarios are useful for precise boundary testing.

Real document fixtures are useful for validating the seam:

```text
source document
    ↓
extraction
    ↓
normalization
    ↓
EvidenceGraph
    ↓
Facts
    ↓
Check
```

A Check can pass every synthetic unit test while still breaking when real extraction changes.

Therefore DocTrust should maintain both:

```text
Real fixture scenarios
+
Human-adversarial scenarios
```

The current regression suite distinguishes these origins explicitly.

---

# 18. The Regression Gate

After authoring, the candidate Check is tested as part of the relevant Ruleset.

Conceptually:

```text
Ruleset
   ↓
all scenarios
   ↓
expected vs actual
```

The current demo Ruleset has:

```text
14 scenarios
8 real_fixture
6 human_adversarial
```

The exact count may evolve as the Ruleset evolves; the important principle is the origin distinction and complete regression execution.

---

# 19. Promotion Gate

The candidate should not become authoritative simply because:

```text
go test
```

is green.

Promotion should require the trust chain:

```text
human semantic approval
+
human adversarial coverage
+
deterministic scenario validation
+
content/version binding
+
successful promotion
```

The system must ensure:

```text
approved artifact
==
validated snapshot
==
promoted artifact
```

This prevents a later file mutation from changing what was actually promoted.

---

# 20. What the Human Should See

A useful review surface should make the semantic contract visible without requiring the human to inspect Go code.

Example:

```text
────────────────────────────────────────
RULE PROPOSAL
────────────────────────────────────────

Projected income must not exceed
corroborated W-2 wages by more than 5%,
unless explained by documented bonus income.

Generated Check:
gross_income_consistency

Parameter:
tolerance = 5%

────────────────────────────────────────
EXPECTED BEHAVIOR
────────────────────────────────────────

Scenario                         Expected   Actual

Equal income                     PASS       PASS      ✓
4% variance                      PASS       PASS      ✓
5% boundary                      PASS       PASS      ✓
5.1% variance                    REVIEW     REVIEW    ✓
15% variance                     REVIEW     REVIEW    ✓

────────────────────────────────────────
HUMAN-ADVERSARIAL CASE
────────────────────────────────────────

15% variance
+ bonus documented separately

Expected: REVIEW
Actual:   REVIEW

HUMAN COVERAGE: ✓
────────────────────────────────────────

REGRESSION
14 / 14 passed
8 real fixtures
6 human-adversarial

────────────────────────────────────────

[ Reject ]                     [ Approve ]
```

The human does not need to understand the generated Go source to decide whether the rule is trustworthy.

---

# 21. What the AI Agent Should and Should Not Do

## AI may

- translate plain English into candidate rule logic
- propose Checks
- propose scenarios
- propose expected results
- suggest edge cases
- summarize mismatches
- explain why a generated Check produced a result
- identify potential boundary conditions

## AI may not

- declare its own generated Check correct
- silently alter expected results to make tests pass
- create the only adversarial scenarios
- bypass human semantic approval
- promote an unapproved Check
- modify trusted Rulesets directly
- override the deterministic Decision
- manufacture evidence
- decide that a human review requirement is unnecessary

---

# 22. What the Human Owns

The human is authoritative over:

```text
Meaning of the rule
Expected behavior
Exceptions
Boundary semantics
Adversarial coverage
Final approval
Human disposition
```

The machine is authoritative over:

```text
Deterministic execution
Actual result
Scenario comparison
Evidence references
Regression calculation
Hashes/version binding
```

This creates a useful division:

```text
Human
= semantic authority

DocTrust
= execution authority

AI
= authoring assistant
```

---

# 23. The Critical Anti-Circularity Rule

Never allow this:

```text
AI writes rule
    ↓
AI chooses all scenarios
    ↓
AI chooses expected answers
    ↓
AI evaluates results
    ↓
AI says "PASS"
```

Instead:

```text
AI writes rule
    ↓
AI proposes scenarios
    ↓
Human validates expected meaning
    ↓
Human adds independent adversarial case
    ↓
Deterministic engine executes
    ↓
Human reviews discrepancies
    ↓
Human approves promotion
```

The point is to break the circularity between:

```text
author
and
grader
```

---

# 24. How This Appears in an Agentic Workflow

When DocTrust is exposed through MCP, an autonomous agent such as Hermes can orchestrate the process.

Conceptually:

```text
Hermes
  ↓
"Create a compliance rule for this requirement."
  ↓
DocTrust authoring workflow
  ↓
AI-generated Check + scenarios
  ↓
human review
  ↓
deterministic testing
  ↓
promotion
```

After promotion:

```text
Hermes
  ↓
"Evaluate this mortgage package."
  ↓
DocTrust
  ↓
Decision
  ↓
evidence
  ↓
human review
  ↓
audit
```

The key distinction remains:

> **The agent can orchestrate compliance work, but it cannot redefine what compliance means or self-certify its own rules.**

---

# 25. Recommended Demo Sequence

For a compelling demonstration, show the testing loop itself.

### Scene 1 — Plain-English requirement

Human enters:

> Projected income must not exceed W-2 wages by more than 5%, with documented bonus exceptions.

### Scene 2 — AI authors rule

Show:

```text
Check generated
Scenarios generated
Parameter: 5%
```

### Scene 3 — Human reviews semantic boundaries

Show:

```text
4.9% → PASS
5.0% → PASS
5.1% → REVIEW
```

Human confirms the intended meaning.

### Scene 4 — Human adversarial case

Human adds:

> Bonus documented in a separate statement.

Mark:

```text
HUMAN-ADVERSARIAL
```

### Scene 5 — Deterministic test run

Show:

```text
15 / 15 PASS
```

with expected vs actual.

### Scene 6 — Promotion

Show:

```text
Human approved
Check promoted
Ruleset v2
```

### Scene 7 — Real case

Hermes evaluates the mortgage package under the newly promoted rule.

### Scene 8 — Human review

The $18K variance is grounded in actual evidence.

### Scene 9 — Audit

Show Ruleset identity, machine findings, human decision, document hashes, and artifact hash/signature.

The strongest demo line is:

> **"The AI wrote the rule, but it did not get to decide that the rule was correct."**

---

# 26. Testing Checklist for Human Reviewers

Before approving a generated Check, ask:

### Semantic meaning

- Does the Check match the plain-English requirement?
- Are the important terms unambiguous?
- Are exceptions represented correctly?

### Boundaries

- What happens exactly at the threshold?
- What happens just below?
- What happens just above?

### Evidence

- Are the correct evidence types referenced?
- What happens when evidence is missing?
- What happens when sources disagree?
- Are document identities canonical?

### Coverage

- Does the AI's scenario set cover normal cases?
- Does it cover boundary cases?
- Does it cover exception cases?
- Did a human create an independent adversarial scenario?

### Execution

- Does every approved scenario match expected behavior?
- Are failures investigated rather than merely edited away?
- Does the real-fixture regression suite still pass?

### Promotion

- Is the exact approved content the content being promoted?
- Is the version/hash bound correctly?
- Are required human approvals complete?

---

# 27. Testing Checklist for AI Agents

Before proposing promotion, an AI authoring agent should internally ask:

```text
1. What does the rule actually mean?
2. What are its decision boundaries?
3. What assumptions did I make?
4. Which scenarios represent those assumptions?
5. Which adversarial case would most likely break my interpretation?
6. Have I left that case for independent human authorship?
7. Are expected results explicit?
8. Did deterministic execution match every human-approved expectation?
9. Did any mismatch get resolved by domain reasoning rather than by blindly editing tests?
10. Is the exact approved candidate the artifact being promoted?
```

The agent should be encouraged to surface uncertainty rather than conceal it.

---

# 28. Testing as a Trust Contract

The final conceptual model is:

```text
             BUSINESS LANGUAGE
                    │
                    ▼
             AI INTERPRETATION
                    │
                    ▼
          executable Check proposal
                    │
                    ▼
          AI-proposed scenarios
                    │
                    ▼
        HUMAN SEMANTIC AUTHORITY
                    │
              ┌─────┴─────┐
              │           │
       approve meaning   adversarial
              │           │
              └─────┬─────┘
                    ▼
        DETERMINISTIC EXECUTION
                    │
             expected vs actual
                    │
             ┌──────┴──────┐
             │             │
           match        mismatch
             │             │
             │       investigate why
             │             │
             └──────┬──────┘
                    ▼
              HUMAN APPROVAL
                    │
                    ▼
                PROMOTION
                    │
                    ▼
               RUNTIME USE
```

This is the core trust mechanism of AI-assisted compliance authoring.

---

# 29. The Fundamental Rule

The most important rule to preserve is:

> **AI can propose the rule. Humans define what the rule means. Humans independently challenge its boundaries. DocTrust deterministically verifies the implementation. Only the human can approve promotion.**

That is what prevents DocTrust from becoming:

> "An LLM that writes compliance code and then declares itself correct."

Instead, it becomes:

> **A system where AI accelerates compliance-rule creation while deterministic testing and human semantic authority preserve trust.**

---

# 30. Summary

DocTrust's testing process is not merely unit testing.

It is a **semantic trust protocol**:

```text
Plain English
→ AI proposal
→ human meaning review
→ independent adversarial scenario
→ deterministic execution
→ expected/actual comparison
→ mismatch resolution
→ approval
→ promotion
```

The important properties are:

1. **The AI is not its own grader.**
2. **Expected behavior is human-approved.**
3. **At least some coverage is independently human-authored.**
4. **Actual execution is deterministic.**
5. **Mismatches require reasoning, not cosmetic test edits.**
6. **Promotion is gated by the trust process.**
7. **The approved artifact is bound to the exact promoted content.**

That combination is what turns plain-English AI rule authoring into a system suitable for compliance-sensitive workflows.
