# Judge-Facing Demo Package

LLM-free six-act demo of the DocTrust trust funnel. Frozen 2026-08-23.

## Run it

```bash
bash scripts/judge-demo.sh          # one command: all six acts + close, isolated sandbox
```

Optional visual variant between Acts 2b and 3: `bin/server` (Phase-4 UI) —
visualization surface ONLY; the compliance decision does not depend on the UI.

## Contents

| File | Purpose |
|---|---|
| `judge-demo.sh` → installed at `scripts/judge-demo.sh` | THE demo script (canonical location). Six acts: evaluate_case → findings/evidence(bbox) → human review → audit artifact → bad-rule approval blocked → Gate-5 defense-in-depth + SHA-256 immutability → close. |
| `rehearsal_report.md` | Canonical evidence package: witnessed status table, exact commands, 2–3 min presenter script, optional LLM aside, 12 judge Q&A, limitations. §"Judge Demo Package" supersedes earlier capture (2026-08-23 evening). |
| `plan.md`, `plan2.md` | Approved planning records (initial package; five presentation fixes from adversarial review2). |
| `prompt.md`, `review2.md` | Source specification + adversarial review driving the fixes. |

## Provenance note

Originals were authored outside this repository (`../plans8`, `../plans7`) and are
mirrored here as of 2026-08-23. Historical paths inside documents (e.g.,
`plans8/scripts/judge-demo.sh`, `plans7/rehearsal_report.md`) refer to those original
locations; the canonical in-repo script is `scripts/judge-demo.sh`.
