# FINAL DELTA REVIEW — Phase 6 Remediation

Date: 2026-08-25 · Reviewer: senior Go architect (adversarial) · Scope: delta verification of F-1 / F-2 / F-4 / F-5 / F-6 / F-7 and regressions

Authoritative sources: `phase6/ADVERSARIAL_REVIEW.md` (cond. pass, F-1/F-2/F-4 P2), current dirty worktree diff (`git diff HEAD`), `phase6/PHASE6_REPORT.md`, `AGENTS.md`, `plans12/plan2.md` (rev. 2, Ed25519).

---

## 1. Executive verdict

**CONDITIONAL PASS — bounded issue remains**

All three claimed P2 fixes are present and effective for the primary attack paths. Two residual bounded gaps remain: (a) F-1 service-layer identity→key binding not enforced in `LoadAuthorizedReviews`; (b) F-4 sidecar comparison covers 4/7 fields — forged `reason`/`evidence` would not be rejected before display. Both are P2 (integrity misattribution, not direct authority bypass) and are bounded by the existing CLI gate and Ed25519 field binding. No new P0/P1 introduced; regressions green.

Deployable to judge demo after acknowledging the two residuals as documented bounded issues (or applying the 15-LOC patches noted). For a strict PASS, patch F-1 service check and extend `compareFindings` to include `Reason` (and optionally `Evidence` hash) — see §2 and §4.

---

## 2. F-1 — Identity ↔ Signing Key Binding

### Previous vulnerability
```
provision alice (key_id=alice)
  → claim reviewer=owner via --reviewer/DOCTRUST_REVIEWER
  → sign with alice's private key
  → DocTrust verifies against alice.pub (via rec.KeyID)
  → audit: reviewer_identity=owner, key_id=alice  (misattributed)
```
`internal/service/reviews.go:159-168` merged `SignedReview{ReviewerIdentity, KeyID}` without checking equality; `VerifyRecord` (`internal/review/signing.go:117-136`) verifies `ed25519.Verify(pub, canonicalPayload, sig)` where canonical payload *includes* both fields, so a key owner can intentionally set `ReviewerIdentity=victim` and still produce a valid signature for their own `KeyID`.

### Fix claimed
> Encrypted key's `keyID` (json:"key_id" in `internal/review/keyfile.go:40`) is compared against claimed `opts.Reviewer` before record creation.

### Verification — CLI path: FIXED

**Code path:** `cmd/doctrust-review/review_flow.go:132-147`
```go
// 132-142: passphrase → LoadEncryptedPrivateKey(path, pass) returns keyID
137: keyID, pub, priv, err := loadReviewerKey(filepath.Join(opts.KeyDir, opts.Reviewer+".key.enc"), pass)
142: fmt.Fprintf(ioh.out, "signing key unlocked: key_id=%q\n", keyID)
// 144-147: F-1 gate
144: // --- F-1: Validate reviewer identity matches the provisioned key. ---
145: if keyID != opts.Reviewer {
146:   return fmt.Errorf("reviewer identity mismatch: --reviewer=%q but key belongs to %q", opts.Reviewer, keyID)
147: }
149: // ... interactive loop ...
192: rec := review.SignedReview{ ReviewerIdentity: opts.Reviewer, KeyID: keyID, ... }
204: review.SignRecord(priv, &rec, time.Now().UTC())
```

* `keyID` provenance: `internal/review/keyfile.go:40,135-180` — `encryptedKeyFile.KeyID` persisted at `SaveEncryptedPrivateKey(path, keyID, priv, pass)` (`provision.go:60,63`) and returned by `LoadEncryptedPrivateKey`. File `~/.doctrust-reviewer/<name>.key.enc` is named after reviewer but identity is read from inside the container, not the filename.
* Claimed identity provenance: `review_flow.go:56-58,68` — `opts.Reviewer` from `--reviewer` flag or `osUser()` (`tty.go:35`); `grep -R DOCTRUST_REVIEWER cmd/doctrust-review` = 0 hits — env bypass does not exist (only `cmd/review-check` uses that env, unrelated).
* Ordering: comparison occurs *before* the `lineReader` loop (`150`), before `SignRecord` (`204`), before `AppendSignedRecord` (`218`), before `svc.LoadAuthorizedReviews` (`226`). Bypass via `DOCTRUST_REVIEWER` → no code path; via `--reviewer` → loads `attacker.key.enc` and fails the equality check; via alternate review path → no other authoring entry exists (`main.go:11-17` has only `--provision` vs default `runReview`).

**Attack — CLI:**
```
key identity = alice (alice.key.enc, key_id=alice)
claimed reviewer = owner (--reviewer owner → loads owner.key.enc → not alice)
```
Result: `reviewer identity mismatch: --reviewer="owner" but key belongs to "alice"` — REJECT, NO `SignedReview`, NO `AppendSignedRecord`, NO disposition change. Verified by code reading; legitimate path (`alice` key + `--reviewer alice` or `owner` key + `--reviewer owner`) still succeeds (delta rehearsal run shows `key_id="owner"` → 1 record signed in `p6-20260825-012014-676181`).

**Attack — CLI with explicit mismatch:**
```
provision alice → alice.key.enc (key_id alice)
--reviewer owner → loads owner.key.enc? No, loads owner.key.enc, not alice's.
To force mismatch, attacker would need to copy alice.key.enc to owner.key.enc
but file's internal key_id remains alice → comparison keyID(alice) != opts.Reviewer(owner) → REJECT.
```

### Verification — Service path: RESIDUAL P2 REMAINS

**Code path:** `internal/service/reviews.go:115-170`
```go
150: pub, ok := ring[rec.KeyID]
154: if err := review.VerifyRecord(pub, rec); err != nil { return ... }
159: s.case_.reviewStore.AddReview(&review.HumanReview{ReviewerIdentity: rec.ReviewerIdentity, KeyID: rec.KeyID, ...})
```
* No check `if rec.ReviewerIdentity != rec.KeyID { error }`.
* `VerifyRecord` (`signing.go:70-85`) includes `ReviewerIdentity` in canonical payload, so *post-sign tampering* of identity breaks the signature. But an *intentional* mismatch at creation time (key owner constructs `SignedReview{ReviewerIdentity:"victim", KeyID:"attacker"}` and signs with attacker key) produces a **valid** signature (canonical payload contains victim identity, signed by attacker). `VerifyRecord` looks up `ring[attacker]` → verifies successfully → `AddReview` stores `ReviewerIdentity=victim`.

**Reproduction (conceptual, not executed as live exploit per prompt — code-reasoned):**
```go
rec := SignedReview{CaseID: loadedID, SnapshotSHA256: snapHash, FindingIndex:1, Action:reject,
  ReviewerIdentity:"owner", KeyID:"alice", Ruleset: pinned}
SignRecord(alicePriv, &rec, now) // valid for alice's pub
LoadAuthorizedReviews([]SignedReview{rec}, map[string]ed25519.PublicKey{"alice": alicePub})
// → ACCEPT: ring[alice] verifies, audit: reviewer_identity=owner
```

**Severity:** P2 — meaningful integrity weakness (audit misattribution) without direct authority bypass via the declared MCP surface (the agent MCP has no review capability after the fix; this path requires filesystem write access to the reviews sidecar, the F-3 accepted residual). No existing test covers mismatch: `reviews_test.go`, `sidecar_test.go`, `signing_test.go` always set `ReviewerIdentity == KeyID`.

**Required conclusion:** Audit identity is bound to signing authority **for the honest CLI path** (ordering + equality check before signing). It is **caller-asserted metadata** at the service merge layer — a malicious sidecar writer can misattribute. Recommended bounded patch (~5 LOC in `LoadAuthorizedReviews`): `if rec.ReviewerIdentity != rec.KeyID { return fmt.Errorf("reviewer identity %q does not match signing key %q", ...) }` plus test. If design intentionally treats identity as caller-asserted, document in AGENTS.md R29.

### Tests
* Existing: `signing_test.go:64-97` matrix, `sidecar_test.go:33-35` wrong-passphrase closed, `reviews_test.go` legitimate flow. No mismatch test — add one.
* Dirty diff: `review_flow.go:144-147` is the only new identity gate; no service-layer test added for mismatch.

---

## 3. F-2 — Unconditional TTY Gate

### Previous vulnerability
`review_flow.go:40-55` had `requireTTY := fs.Bool("tty", true, ...)` and `if *requireTTY && !isInteractive()` — agent could pass `--tty=false` to disable the human-presence proof.

### Fix claimed
> `--tty` flag removed; `isatty` unconditional.

### Verification — FIXED

**Code path:** `cmd/doctrust-review/review_flow.go:40-54`
```go
40: fs := flag.NewFlagSet("doctrust-review", flag.ExitOnError)
41: snapshot := fs.String("snapshot", "", ...)
...
46: fs.Parse(osStdlib.Args[1:]) // 5 flags only: snapshot, domain, rulesets-dir, reviewer, key-dir
48: if !isInteractive() {
49:  fmt.Fprintln(os.Stderr, "FATAL: human review requires an interactive terminal (TTY).")
52:  osStdlib.Exit(1)
53: }
56: reviewerName := *reviewer // resolved AFTER gate
```
* `grep -R "\-\-tty" doctrust` → 0 hits; `grep -R "tty" cmd/doctrust-review/` → only `TTY` in comments and `isInteractive` symbol; `bin/doctrust-review --help` lists `domain, key-dir, reviewer, rulesets-dir, snapshot` — no `tty` flag.
* Alternate bypass search: `grep -R "os.Getenv\|DOCTRUST_\|FORCE_\|ALLOW_NON_TTY\|BATCH" cmd/doctrust-review` → 0 hits (only `osUser`, `os.UserHomeDir`, `os.Getwd`, `os.Stdin`).
* Ordering: gate is the first statement after `fs.Parse` (`48`), *before* `osUser()` (`56`), before `ReadFile(decisionPath)` (`85`), before `NewDocTrustService`+`LoadCase`+`Evaluate` (`99-110`), before `compareFindings` (`113`), before `secretFn` (`133`), before `loadReviewerKey` (`137`), before `SignRecord` (`204`), before `AppendSignedRecord` (`218`).

**Attack:**
```
stdin=pipe; stdout=pipe; stderr=pipe
$ bin/doctrust-review --snapshot ... --reviewer owner
```
Result: `FATAL: human review requires an interactive terminal` → `Exit(1)`, NO state change. Rehearsal `script -qec` satisfies `isInteractive()` (`ModeCharDevice` on both fds) — legitimate pseudo-TTY still works (`p6-20260825-012014-676181` human_session shows `isInteractive` passed, 1 record signed, `FINAL DISPOSITION: FAIL`).

**Honest property (unchanged):** As prior review noted, `script`/pty satisfies isatty — this is a human-channel presence proof, not biometric identity. The stated guarantee is: *review authority requires the human-review channel, which is TTY-gated and additionally protected by reviewer-key possession*.

---

## 4. F-4 — Decision Sidecar Verification

### Previous vulnerability
```go
for _, f := range sidecar.Findings { fmt.Fprintf(ioh.out, ...) } // untrusted sidecar displayed
// ... interactive input ...
// later: fresh evaluation (too late — human already saw false findings)
```

### Fix claimed
```
load snapshot → fresh evaluation → compare sidecar vs trusted decision → ONLY THEN display
```

### Verification — FIXED for primary fields; residual on reason/evidence

**Code path:** `review_flow.go:97-129`
```go
 97: // F-4: Re-evaluate deterministically BEFORE displaying findings.
 99: svc, err := service.NewDocTrustService(opts.Domain, opts.RulesetsDir)
104: svc.LoadCase(ctx, opts.SnapshotPath)
107: decision, err := svc.Evaluate(ctx)
112: // F-4: Verify sidecar findings match re-evaluation.
113: if err := compareFindings(sidecar.Findings, decision.Results); err != nil {
114:  return fmt.Errorf("DECISION_SIDECAR_MISMATCH: %w", err)
115: }
117: // Display the re-evaluated findings (trusted source, not the sidecar).
118: fmt.Fprintf(ioh.out, "\nHuman authority required — case %s\n", sidecar.LoadcaseID)
122: for i, r := range decision.Results { // ← trusted decision.Results, not sidecar
```

**`compareFindings` (`review_flow.go:249-270`):**
```go
249: func compareFindings(sidecarFindings []service.SidecarFinding, evalResults []eval.Result) error {
250:  if len(sidecarFindings) != len(evalResults) { return count mismatch }
254:  for i, sf := range sidecarFindings {
256:   if sf.CheckID != string(er.CheckID) { return check_id mismatch }
260:   if sf.Status != string(er.Status) { return status mismatch }
264:   if sf.Severity != string(er.Severity) { return severity mismatch }
```

**Attack matrix (7 cases from prompt):**
| # | Mutation | Result |
|---|----------|--------|
| 1 | remove a finding (len 2→1) | `DECISION_SIDECAR_MISMATCH: finding count mismatch: sidecar=1 evaluation=2` → NO REVIEW RECORD |
| 2 | add a fake finding (len 2→3) | same count check → MISMATCH |
| 3 | change PASS↔REVIEW (status) | status mismatch → MISMATCH |
| 4 | change check ID | check_id mismatch → MISMATCH |
| 5 | change reason | **NOT checked** → would PASS (residual) |
| 6 | change evidence/metrics | **NOT checked** → would PASS (residual) |
| 7 | change severity | severity mismatch → MISMATCH |

Most important property — *human sees attacker-controlled findings as trusted* — is fixed: display loop iterates `decision.Results` (fresh evaluation), not `sidecar.Findings`. Sidecar is read only for binding context (`LoadcaseID`, `GraphCaseID`, `SnapshotSHA256`, `Ruleset`). Legitimate sidecar (written by `doctrust-mcp` immediately before TTY) passes: same 2 findings (`required_shipment_documents PASS/INFO`, `gross_weight_reconciliation REVIEW/BLOCKING`) in deterministic order; live rehearsal shows `FINAL DISPOSITION: FAIL` without `DECISION_SIDECAR_MISMATCH`.

**Residual P2:** Forged `reason` or `evidence` inside the sidecar would not be rejected before display, but the human is shown the trusted `decision.Results` instead, so no misled display. The residual matters only if a future change displays reason/evidence from the sidecar without re-adding a check — document that `compareFindings` intentionally covers the security-relevant dimensions (count, identity, disposition) and that reason/evidence are display-only from the trusted evaluation.

---

## 5. F-5 / F-6 / F-7 Cleanup Verification

**F-5 DEBUG-MARKER:** `grep -rn "DEBUG-MARKER" doctrust` → 0 hits. Dirty diff removes `fmt.Fprintln(os.Stderr, "DEBUG-MARKER provision entered")` from `provision.go:13`. No other `DEBUG` strings remain.

**F-6 Public-key output:** `grep -n "public key" cmd/doctrust-review/provision.go` → only inside `if *pubOut != ""` publishing branch. The unconditional `fmt.Printf("public key: %s")` was removed. No private key or passphrase output exists (`grep -i "private key.*%s"`: only the file-path print `private key written: %s`). No sensitive provisioning data in logs.

**F-7 Rehearsal passphrase:** `scripts/rehearse-phase6-human.sh:18-19`
```bash
# Test-only default passphrase; use PHASE6_PASSPHRASE env for real runs.
PASSPHRASE="${PHASE6_PASSPHRASE:-phase6-hold-passphrase}"
```
Explicitly marked test-only, overridable. Not a production credential.

---

## 6. Regression Results

Exact commands (current dirty worktree) and outcomes:

```
$ go vet ./...                                   → EXIT 0
$ make lint-imports                              → Checking service boundary... Checking MCP boundary... PASS
$ go test ./...                                  → 13 pkgs ok (cmd/doctrust-mcp, cmd/promote-check, cmd/review-check, cmd/server, cmd/verify-ruleset, internal/audit, internal/compiler, internal/eval, internal/extraction, internal/opa, internal/review, internal/service, tests/integration)
$ go test ./internal/eval -run TestRunAllScenarios -count=1 -v → 14/14 strict PASS
$ go test ./internal/eval -run TestRunAllShipmentScenarios -count=1 → 6/6 PASS (shipment domain)
$ go test ./internal/review -run TestSignVerify -count=1 -v → 5/5 matrix PASS (valid, modified identity/note/timestamp/index, forged, wrong-key, missing)
$ scripts/rehearse-hermes-shipment.sh 2>&1 | tail → 13/13 assertions PASS (adaptive Hermes still green)
$ scripts/rehearse-phase6-human.sh 2>&1 | tail → ingest PASS, key provisioned, machine REVIEW, PROOF-DENIED (tools/list 5, unknown tool -32602), human TTY wrote signed sidecar, final_disposition FAIL/BLOCKING, artifact hash present
```

No legitimate reviewer record rejected: provision as `owner` (`owner.key.enc`, `keyID=owner`) with `--reviewer owner` passes the F-1 gate, signs, and verifies against `demo/shipment_release/reviewers/owner.pub` (tracked demo key rotated to `6fI5wTUv...` pub). Rehearsal retained run `p6-20260825-012014-676181/human_session.txt` sealed `audit.json` with `reviewer_identity owner, channel human-tty, artifact_hash`.

No new build break: `go build ./...` EXIT 0; `bin/*` present.

---

## F-3 Accepted Residual

No redesign attempted. `AGENTS.md` R29 and `PHASE6_REPORT.md:65-67` document: public-key ring (`<snapshot-root>/reviewers/*.pub`) has no integrity protection beyond local filesystem trust; an unattended shell-capable process with write access can replace a `.pub` file. Deferred: hash-pinning or separate authority key for the ring (explicitly noted as `F-3` accepted residual per `plans12/adv_review.md:134`). Do not overclaim production IAM.

Honest claim remains (as `AGENTS.md` R29 states): **authenticated local human** via passphrase-protected signing key + **cryptographically tamper-evident** records — not real-world identity verification.
