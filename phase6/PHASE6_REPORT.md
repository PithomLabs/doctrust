# PHASE6_REPORT.md — Human Authority Channel

Date: 2026-08-24 · Implements `plans12/plan2.md` (Ed25519 revision).
**Result: PASS** — both required proof points demonstrated live:

1. **DENIED agent attempt**: `request_human_review` does not exist on the
   agent MCP surface; a direct call returns protocol-level
   `unknown tool "request_human_review"` (-32602) and `tools/list` advertises
   only the 5 agent tools.
2. **AUTHORIZED human resolution**: through the human-only TTY channel the
   reviewer confirmed finding 0 and HOLD/rejected finding 1; every record was
   Ed25519-signed with the passphrase-protected private key; DocTrust verified
   the signatures against the reviewers ring and sealed
   **final disposition = FAIL** with reviewer identity, channel, and artifact
   hash recorded.

## Architecture delivered

```text
Agent MCP surface (5 tools)          Human TTY channel (cmd/doctrust-review)
evaluate_case · get_findings ·       provision (--provision, one-time)
get_evidence · get_ruleset ·         review: replay-verify decision →
get_audit_artifact                   display findings/citations → typed index
        ❌ no review capability      consent → action → Ed25519 signature
                 ▲                            │ signed records (sidecar)
                 │ fail-closed merge ─────────┘ (public-key ring verification)
           ReviewStore → ComputeFinalDisposition → Artifact.Finalize()
```

- Agent surface: 5 tools (`request_human_review` removed entirely — unknown-
  tool rejection, not denial logic). Frozen evaluation semantics untouched:
  reject→FAIL · confirm/override-on-all→PASS · unresolved→REVIEW.
- Records bind case ID, snapshot SHA-256, finding index, action, identity,
  note, resolved_at, ruleset id/version/hash; canonical serialization;
  scrypt/AES-GCM passphrase-encrypted private keys outside any snapshot root;
  public keys published to the reviewers ring.
- The human TTY can AUTHOR records but can never finalize — finalization is
  performed by DocTrust service code inside the human-channel process via the
  same fail-closed verification used everywhere else.

## Verification

| Proof | Result |
|---|---|
| PROOF-DENIED (A1/A7) | tools/list: 5 tools, no review capability; direct call ⇒ -32602 |
| PROOF-AUTHORIZED (A8/A9) | reject by `owner` via human-tty ⇒ final disposition FAIL; identity+channel in artifact; hash present |
| TAMPER (A2-class) | modified sidecar note ⇒ REVIEWS_REJECTED fail-closed at merge |
| Signature matrix (unit) | valid ✓; modified action/identity/note/timestamp/index/case/snapshot/ruleset ✗; missing/forged/wrong-key ✗ |
| Keyfile round-trip | wrong passphrase fails closed |
| Service merge flow | out-of-range index rejected before signature matters; confirm-all seals PASS on control fixture |
| Phase-5 regression | `rehearse-hermes-shipment.sh` 13/13 green after the change |
| Failure rehearsals F1–F5 | green |
| Go suite | 13 packages ok · `make lint-imports` PASS |

## Boundary statement

```text
Agent MCP   ❌ human review (capability absent — structural denial)
Human TTY   ✅ authenticated local human + Ed25519 tamper-evident records
bin/server /api/review   ⚠️ localhost/debug surface, OUTSIDE the trusted path
```

Honest phrasing: this proves an **authenticated local human** with
**cryptographically tamper-evident** decisions — not production IAM, and not
real-world identity verification. Documented residual: an unattended
shell-capable process can execute arbitrary local commands; that is outside
the local-demo threat model.

## Docs updated

AGENTS.md: additive R29 (human-channel-only rule, key lifecycle, sidecar/ring
contracts) + forbidden-pattern appends. README.md: Phase-6 row, quickstart
human-authority commands, limitations bullet rewritten. SKILL.md: escalation
wording now states the human channel is outside the agent's tools.

## Deferred (not committed)

Multi-user identity management · web approval UI · `/api/review` hardening ·
broader evidence-contract coverage · OS-level access-control story for the
reviewers ring.

## HARD STOP

Phase 6 complete per plan2.md §12 steps 0–8. Next: judge-package/demo
rehearsal planning — no further architecture work.
