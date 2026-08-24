# plan3.md — Screenshot Package Capture (prompt4.md)

Status: APPROVED — executing
Date: 2026-08-23

## Decisions (owner-approved)
1. Terminal-render PNGs via Pillow + DejaVu Sans Mono: slices of the frozen run's actual
   output, ANSI-stripped, high-res (2x), tightly cropped per act. No styled HTML layer.
2. Act 2 bonus attempt: real Phase-4 web UI (bin/server + headless google-chrome) as an
   optional visualization of the same live evidence; any failure falls back to the
   terminal render without blocking.
3. Paths: canonical `docs/demo/screenshots/` inside the doctrust repo; byte-identical
   mirror at `plans8/screenshots/` (prompt's literal path). README states both and that
   they must remain byte-identical.

## Constraints
- One fresh source run is the SOLE source of truth; no rerun-until-pretty.
- capture tooling NEVER alters judge-demo.sh or demo state to improve a screenshot;
  it consumes recorded live output only (Act 2 UI = optional visualization of same evidence).
- No secrets/temp paths/unrelated noise in crops; simple cropping only.

## Artifacts
```
docs/demo/screenshots/{01..06}-*.png + README.md   (canonical)
plans8/screenshots/                                (byte-identical mirror)
docs/demo/capture_screenshots.sh                   (reproducible tooling, tracked)
docs/demo/rehearsal_report.md                      += Screenshot Package addendum (§12)
plans8/plan3.md                                    ← this file (+ repo mirror docs/demo/plan3.md)
```

## Execution steps
1. Write plan3.md (both locations).
2. Build capture_screenshots.sh: fresh run of scripts/judge-demo.sh (tee colored log) →
   slice six act excerpts by stable markers → ANSI-strip → secret-scan → Pillow render →
   README generation from live values → mirror → cleanup verification + git status.
3. Execute once. Attempt web-UI shot for Act 2 (bin/server against sandbox state +
   headless chrome); fallback silently to terminal 02 if imperfect.
4. Quality gates: six PNGs present/non-trivial dims; no secrets or /tmp identifiers in
   crops; values spot-checked vs run log (case_id e14895b79cd1365c, ruleset v2,
   REVIEW/WARNING, Gate-5 row, SHA equality); cmp mirror == canonical; git status shows
   only intended additions.
5. Append §12 report addendum. STOP — presentation assets frozen too.
