# Business Case — DocTrust

## One-sentence pitch
DocTrust is an evidence-driven compliance engine for AI agents: it evaluates consequential workflows against approved policy and verifiable evidence, without allowing the agent itself to become the authority that decides compliance.

## Buyer
Compliance, trade-operations, and risk leadership in mid-market and enterprise companies running regulated document workflows — trade compliance, KYC/AML onboarding, insurance claims, mortgage appraisal, e-invoicing.

## Pain
Manual document reconciliation doesn't scale. "AI reads the PDF and tells you if it's fine" tools create unauditable, ungoverned decisions the compliance function cannot defend to a regulator or auditor. Shipment release makes the cost concrete: a single gross-weight mismatch across an invoice, packing list, bill of lading, and certificate that slips through can trigger customs penalties and holds.

## Product
DocTrust is not another PDF extractor, agent harness, or LLM. It is the **compliance layer between an agent and a consequential decision**: evidence + approved Ruleset + deterministic evaluation + human authority when required + tamper-evident audit artifact.

Documents are the starting wedge, not the product boundary. The core engine is designed to become **provider-agnostic, API-agnostic, and agent-agnostic**. Nutrient is one EvidenceProvider; MCP is one agent-facing interface; Hermes is one agent workflow. The compliance engine should remain independent of all three.

## Stakeholder value
- **Compliance officer** → defensible, replayable decisions (rule → fact → evidence → signed human action).
- **Operations team** → agents can investigate without being trusted with authority.
- **Auditor / regulator** → traceable chain with Ruleset hash, evidence page/bbox, identity, and artifact hash.

## Pricing hypothesis
Usage-based per evaluated case, initially tiered by evidence/document volume; enterprise tier adds custom Rulesets, identity binding, audit controls, and policy administration. Hypothesis: land with one painful regulated workflow, then expand ARR by adding Rulesets and evidence sources on the customer's existing trust layer rather than deploying a new backend for every use case.

## Moat
Not PDF extraction and not LLM access. The moat is the **compliance trust architecture**: evidence provenance, immutable Rulesets, deterministic evaluation, explicit authority boundaries, signed human review, tamper-evident audit, and the accumulation of policy/evidence history over time.

The deeper moat is neutrality. If the same compliance engine can consume evidence from different providers, serve different agents, and expose the same decisions through different interfaces, customers are not buying another point integration — they are building a durable compliance control plane on top of it.

## Regulatory timing
Electronic-transferable-records requirements make the shipment scenario a concrete initial wedge. Adjacent opportunities include e-invoicing, KYC/AML, insurance, mortgage/financial verification, and other workflows where organizations must demonstrate policy compliance, oversight, and traceability.

The strategy is not to claim that every regulation mandates DocTrust's architecture. The strategy is to build an engine that can turn applicable regulatory and internal requirements into explicit, testable, auditable controls.

## Go-to-market wedge sequencing
Shipment release (proven) → procurement reconciliation → KYC/onboarding → insurance claims → mortgage/financial verification → regulatory submissions → legal workflows.

Each new vertical should add a **Ruleset + evidence contract + domain-specific control mapping**, not a new compliance backend. The long-term goal is one evidence-driven compliance engine serving many regulated and consequential workflows.

## Competitive framing
Pure extraction tools answer **"what does the document say?"** Generic agent frameworks answer **"how does the agent use tools?"** IAM and policy engines answer **"who is permitted to perform an operation?"**

DocTrust focuses on the compliance question between them:

> **Does the available evidence satisfy the approved policy for this consequential workflow, and can the organization prove how that conclusion was reached?**

That gives DocTrust a role above extraction and alongside agent infrastructure without pretending to replace identity or authorization systems.

## Roadmap

### Near term — generalize DocTrust beyond documents

- **Provider-agnostic evidence:** Nutrient remains the first provider; additional providers plug into the same `EvidenceProvider` seam.
- **Agent-agnostic execution:** Hermes is the initial workflow; independent clients such as Goose demonstrate that the engine is not coupled to one agent.
- **API/protocol agnosticism:** MCP is a demonstrated interface, not the compliance engine itself. The core decision model should remain usable through MCP, APIs, batch execution, or embedded services.
- **Domain expansion:** add Rulesets and evidence contracts for additional regulated and evidence-heavy workflows without changing the core evaluator.

### Longer term — DocTrust as the compliance engine

The product goal is **not "compliance for documents."** The goal is an evidence-driven compliance engine for **anything an AI agent is asked to do that carries consequential policy obligations**.

The reusable pattern is:

```text
Evidence
   ↓
Policy / Ruleset
   ↓
Deterministic Evaluation
   ↓
Compliance State
   ↓
Evidence + Explanation + Audit
```

Documents are simply the first hard, visible evidence source.

### Strategic layer — Solvent

In parallel, Solvent explores the broader authorization problem that starts after compliance has been established:

> **DocTrust asks: "Does this action satisfy policy?"**
>
> **Solvent asks: "Is this action authorized to become real, right now?"**

The long-term Solvent thesis is to explore a vendor-neutral authorization layer for agentic systems — potentially a portable warrant/authority protocol that can sit alongside MCP and A2A rather than replacing either.

The intended division is:

```text
MCP
→ agent ↔ tool

A2A
→ agent ↔ agent

DocTrust
→ evidence ↔ compliance

Solvent
→ compliance / evidence ↔ consequential authority
```

This is deliberately a roadmap thesis, not a claim that Solvent is already an adopted industry standard. DocTrust is the concrete product proving the evidence-and-compliance side of the architecture; Solvent is the longer-term exploration of the authorization layer that could make authority portable, verifiable, and revocable across agent ecosystems.

### The end-state

The company should not depend on one:

- document provider;
- LLM;
- agent framework;
- transport protocol;
- regulated vertical.

The durable asset is the **compliance control plane**: a common way to collect evidence, evaluate policy, explain findings, preserve provenance, and expose a defensible compliance state to whatever agent or application needs it.
