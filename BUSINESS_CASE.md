# Business Case — DocTrust

## One-sentence pitch
DocTrust is a compliance execution layer that lets AI agents investigate business documents against approved policy — without ever letting the agent become the authority that decides compliance.

## Buyer
Compliance, trade-operations, and risk leadership in mid-market and enterprise companies running regulated document workflows — trade compliance, KYC/AML onboarding, insurance claims, mortgage appraisal, e-invoicing.

## Pain
Manual document reconciliation doesn't scale. "AI reads the PDF and tells you if it's fine" tools create unauditable, ungoverned decisions the compliance function cannot defend to a regulator or auditor. Shipment release makes the cost concrete: a single gross-weight mismatch across an invoice, packing list, bill of lading, and certificate that slips through can trigger customs penalties and holds.

## Product
DocTrust is not another PDF extractor, agent harness, or LLM. It is the authority/audit layer between an agent and any consequential decision: approved Ruleset + evidence requirements + deterministic evaluation + human-authority signing + tamper-evident audit artifact.

## Stakeholder value
- **Compliance officer** → defensible, replayable decisions (rule → fact → evidence → signed human action).
- **Operations team** → agents can investigate without being trusted with authority.
- **Auditor / regulator** → traceable chain with Ruleset hash, evidence page/bbox, identity, and artifact hash.

## Pricing hypothesis
Usage-based per evaluated case, tiered by document volume; enterprise tier adds custom Rulesets and SSO/identity binding for the human-authority channel. Hypothesis: land with one vertical (shipment release), expand ARR via additional Rulesets on the same customer's existing evidence pipeline.

## Moat
Not PDF extraction (commoditized) and not LLM access. The moat is the trust architecture itself — immutable Ruleset, no agent-facing approval tool, signed human authority, tamper-evident snapshots — adversarially tested across Phases 5–6, plus the accumulation of Rulesets and audit history on top. Hard to copy quickly; gets harder as policy coverage and evidence grows.

## Regulatory timing
Electronic-transferable-records laws (trade documents — the shipment scenario). France e-invoicing (Sept 1, 2026) and related digital-identity/AML and mortgage-format cutovers are adjacent wedges the same engine can serve via new Rulesets. Pick the shipment hook as the primary narrative; name it explicitly.

## Go-to-market wedge sequencing
Shipment release (proven) → procurement reconciliation → KYC/onboarding → insurance claims → mortgage/financial verification → regulatory submissions → legal workflows.
Each step adds a Ruleset + evidence contract, not a new backend.

## Competitive framing
Pure extraction tools answer "what does the document say," not "does this satisfy policy or who is accountable." Generic guardrail frameworks constrain calls but don't provide deterministic, replayable compliance decisions or a human-authority signing boundary. DocTrust does both.

## Roadmap
Nutrient is the first provider; the EvidenceProvider contract keeps Foxit/Doctavian as short, credible roadmap items (no half-built adapters before submission).
