---
name: compliance-check-artifact
description: Safely evaluate an artifact against an approved DocTrust policy.
version: 1.0.0
metadata:
  hermes:
    tags: [DocTrust, Compliance, Policy, Evidence, Audit]
---

# Purpose

Evaluate an artifact against an approved business policy without becoming the
authority yourself. DocTrust decides; you investigate and communicate.

# Preconditions

- An approved Ruleset exists for the request (identify it by domain).
- The artifact(s) are available to you.
- An authorized evidence provider is registered (used only through its MCP
  tools; never bypassed).

# Runbook

1. Identify the artifact(s) and the approved policy that applies.
2. Obtain the applicable Ruleset id/version from DocTrust (`get_ruleset`).
3. Determine what evidence the policy requires.
4. Gather the evidence that is actually AVAILABLE, using authorized provider
   tools only (`build_evidence_snapshot` / `extend_evidence_snapshot`).
5. Submit the evidence snapshot to DocTrust (`evaluate_case`).
6. Interpret the structured findings exactly as returned.
7. If findings say evidence is insufficient, choose a permitted follow-up:
   request or obtain the missing required documents through the provider,
   then re-submit. Never invent the missing evidence.
8. After new evidence arrives, re-evaluate via DocTrust (`evaluate_case` on
   the extended snapshot).
9. Respect the disposition: PASS means compliant with sufficient evidence;
   REVIEW means unresolved and requiring human authority; FAIL means the
   approved policy failed. Never convert REVIEW into PASS by reasoning.
10. Escalate consequential REVIEW decisions to human authority. Surface the
    finding and evidence to the user; do not attempt any human-approval tool.
11. Report to the user: what was checked, the observations with their source
    references, the disposition, and the case/audit identifier.

# Forbidden actions

- Modify approved policy, Rulesets, or their files.
- Fabricate evidence or extraction results.
- Declare PASS independently of DocTrust.
- Override DocTrust dispositions.
- Convert REVIEW to PASS by reasoning.
- Fabricate human approval. Human authority lives on a separate human-only
  terminal channel that is not part of your tools; when a REVIEW requires a
  human, report the finding/evidence/case ID and wait for the recorded decision.
- Treat your own reasoning as evidence.
- Use unauthorized providers or bypass the MCP layer.

Report honestly when evidence is missing or a provider fails. A blocked or
failed check is reported as such, never papered over.
