# Judge-Demo Screenshots — captured 2026-08-23T23:22:43+08:00

Source run: single fresh execution of `scripts/judge-demo.sh` (frozen demo).
Ruleset: income_verification v2 · Case ID: `e14895b79cd1365c` · All values live-parsed; nothing edited.

**Canonical source:** `docs/demo/screenshots/`
**Demo-package mirror:** `plans8/screenshots/` (must remain byte-identical)

### 01-evaluate-case.png
Caption: A real agent-facing document compliance decision produced by the frozen engine.
What the judge should notice: Ruleset income_verification v2, REVIEW/WARNING decision,
case ID and finding count — all from the live MCP runtime.
Act: 1 — Evaluate

### 02-finding-and-evidence.png
Caption: The finding traces to exact page/bounding-box evidence inside the source documents.
What the judge should notice: paystub, W-2 and 1040 evidence rows with real coordinates.
Act: 2 — Evidence

### 03-human-review.png
Caption: The agent surfaces the issue; a human remains the authority for the disposition.
What the judge should notice: action=confirm recorded with presenter note and resolved_at.
Act: 3 — Human authority

### 04-audit-artifact.png
Caption: Decision and human action are preserved in a tamper-evident audit record.
What the judge should notice: ruleset id/version, disposition, human_reviews entry count,
artifact hash. Hash binds THIS run's contents; it differs per run by design.
Act: 4 — Auditability

### 05-policy-rejection.png
Caption: A deliberately wrong policy expectation is caught before approval.
What the judge should notice: Expected PASS vs deterministic Actual REVIEW — semantic
rejection executed pre-approval (not a provenance shortcut).
Act: 5 — Policy safety

### 06-promotion-defense.png
Caption: Even with a bypassed approval surface, the promotion gate independently rejects
the bad policy and trusted state does not move.
What the judge should notice: Gate 5 failure row, SHA256 tree equality, promoted v2 intact.
Act: 6 — Defense in depth
