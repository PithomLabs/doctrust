# DWS Viewer Spike — 2-Hour Time Box

**Decision:** Fallback to existing page/bbox evidence. Viewer is presentation-only and does not replace DocTrust human-authority semantics.

**Investigation (2026-08-25, ~30 min):**
- Checked `nutrient/docs` and `https://api.nutrient.io` Viewer endpoints: Viewer requires a separate `viewer` product credential and embeddable JS viewer setup, not just extraction `NUTRIENT_DWS_EXTRACTION_API_KEY`.
- Current `make setup` validates only extraction key; Viewer would require additional `NUTRIENT_DWS_VIEWER_KEY` and frontend viewer integration (not in current `evidence-mcp`/`doctrust-mcp` scope).
- Existing human authority already shows grounded `evidence @ page/bbox (conf)` lines and audit artifact trace — sufficient for the trust-boundary story.
- Risk: Viewer integration would consume MVP time and add a second credential path without strengthening the authority model (DocTrust remains authoritative).

**Outcome:** Per DR-4 fallback rule, Viewer not adopted for this submission. Existing page/bbox evidence remains the human presentation surface. Revisit post-hackathon if Viewer is needed for premium demo polish.

**Artifact:** `g1/G1_REPORT.md` and `phase5/PHASE5_REPORT.md` already demonstrate grounded evidence.
