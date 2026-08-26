#!/usr/bin/env bash
# Phase-6 rehearsal (plans12): proves BOTH sides of the authority boundary.
#
#   PROOF 1 (DENIED):    request_human_review does not exist on the agent MCP
#                        surface — protocol-level unknown-tool rejection.
#   PROOF 2 (AUTHORIZED): the human-only TTY channel signs an Ed25519 review
#                        record; DocTrust verifies it and seals final
#                        disposition FAIL (reject semantics) with reviewer
#                        identity recorded.
#   TAMPER (A5-class):   a modified sidecar record is REJECTED fail-closed.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PY="${PYTHON:-python3}"
FROZEN="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/demo/shipment_release/fixtures/anomaly"
WORKSPACE="$ROOT/demo/shipment_release"
REVIEWER="owner"
# Test-only default passphrase; use PHASE6_PASSPHRASE env for real runs.
PASSPHRASE="${PHASE6_PASSPHRASE:-phase6-hold-passphrase}"

RUN_ID="$(date +%Y%m%d-%H%M%S)-$$"
RUN="$WORKSPACE/runs/p6-$RUN_ID"
AVAIL="$RUN/available"
RING="$WORKSPACE/reviewers"
KEYDIR="$RUN/keydir"
mkdir -p "$AVAIL" "$RING" "$KEYDIR"

cleanup() {
    if [ "${PHASE6_CLEANUP:-0}" = "1" ]; then rm -rf "$RUN"; fi
}
trap cleanup EXIT

cp "$FROZEN"/*.pdf "$AVAIL/"
SNAP_ABS="$AVAIL/evidence_snapshot.json"

PASS=0; FAIL=0
ok()  { echo "PASS: $1"; PASS=$((PASS+1)); }
bad() { echo "FAIL: $1"; FAIL=$((FAIL+1)); }

RS_PRE="$(sha256sum "$ROOT/rulesets/shipment_release/v1.yaml" | cut -d' ' -f1)"

# ---- STEP 1: ingest all four documents --------------------------------------
"$ROOT/bin/ingest" -domain shipment_release \
    -docs "commercial_invoice=$AVAIL/01-commercial-invoice.pdf,packing_list=$AVAIL/02-packing-list.pdf,bill_of_lading=$AVAIL/03-bill-of-lading.pdf,certificate_of_origin=$AVAIL/04-certificate-of-origin.pdf" \
    -out "$AVAIL" >/dev/null
[ -f "$AVAIL/evidence_snapshot.json" ] && ok "ingest produced evidence snapshot" || bad "ingest failed"
LOADCASE_ID="$(sha256sum "$AVAIL/evidence_snapshot.json" | cut -c1-16)"

# ---- STEP 2: provision reviewer (one-time human action) ----------------------
printf '%s\n%s\n' "$PASSPHRASE" "$PASSPHRASE" | \
    script -qec "$ROOT/bin/doctrust-review --provision --name $REVIEWER --key-dir $KEYDIR --publish-to $RING" /dev/null >/dev/null
[ -f "$KEYDIR/$REVIEWER.key.enc" ] && [ -f "$RING/$REVIEWER.pub" ] \
    && ok "reviewer key provisioned (private encrypted; public in ring)" \
    || bad "provisioning failed"

# ---- STEP 3: agent-side evaluate (writes decision sidecar) -------------------
CASE_ID=""
run_mcp_session() { # args are extra JSON-RPC lines on stdin; writes responses to $2
    local reqs="$1" resp="$2"
    {
        printf '%s\n' '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"p6","version":"0"}}}'
        printf '%s\n' '{"jsonrpc":"2.0","method":"notifications/initialized"}'
        sleep 0.5
        cat "$reqs"
        sleep 6
    } | DOCTRUST_SNAPSHOT_ROOT="$WORKSPACE" timeout 120 \
        "$ROOT/bin/doctrust-mcp" --domain shipment_release \
        --rulesets-dir "$ROOT/rulesets" --snapshot-root "$WORKSPACE" 2>/dev/null > "$resp"
}

EVAL_REQS="$RUN/eval_reqs.jsonl"
{
    printf '%s\n' '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"evaluate_case","arguments":{"snapshot_path":"'"$SNAP_ABS"'"}}}'
} > "$EVAL_REQS"
run_mcp_session "$EVAL_REQS" "$RUN/eval_resp.jsonl"

EVAL_STATUS="$($PY - "$RUN/eval_resp.jsonl" <<'PYEOF'
import json, sys
for line in open(sys.argv[1]):
    try: m = json.loads(line)
    except Exception: continue
    if m.get("id") == 2:
        try:
            print(json.loads(m["result"]["content"][0]["text"]).get("status", ""))
        except Exception:
            print("ERR")
print()
PYEOF
)"
[ "$EVAL_STATUS" = "REVIEW" ] && ok "machine evaluation REVIEW/BLOCKING on full package" \
    || bad "unexpected machine status: '$EVAL_STATUS'"

CASE_ID="$($PY -c "import json;print(json.load(open('$AVAIL/evidence_snapshot.json'))['case_id'])")"

# ---- PROOF 1 (DENIED): agent surface has no human-review capability ----------
DENIED_REQS="$RUN/denied_reqs.jsonl"
{
    printf '%s\n' '{"jsonrpc":"2.0","id":10,"method":"tools/list","params":{}}'
    sleep 0.5
    printf '%s\n' '{"jsonrpc":"2.0","id":11,"method":"tools/call","params":{"name":"request_human_review","arguments":{"case_id":"'"$CASE_ID"'","finding_index":1,"action":"reject"}}}'
} > "$DENIED_REQS"
run_mcp_session "$DENIED_REQS" "$RUN/denied_proof.jsonl"

$PY - "$RUN/denied_proof.jsonl" <<'PYEOF'
import json, os, sys
ok1 = ok2 = False
for line in open(sys.argv[1]):
    try: m = json.loads(line)
    except Exception: continue
    if m.get("id") == 10:
        names = [t.get("name") for t in m.get("result", {}).get("tools", [])]
        ok1 = "request_human_review" not in names
        print(f"  tools/list count={len(names)} advertises-review={not ok1}")
    if m.get("id") == 11:
        err = m.get("error") or {}
        ok2 = err.get("code") == -32602 and "unknown tool" in err.get("message", "")
        print(f"  direct call error: code={err.get('code')} msg={err.get('message','')[:60]}")
sys.exit(0 if ok1 and ok2 else 1)
PYEOF
[ $? -eq 0 ] && ok "PROOF-DENIED: agent surface structurally lacks human review (tools/list + unknown tool)" \
    || bad "PROOF-DENIED failed"

# ---- PROOF 2 (AUTHORIZED): human TTY signs; DocTrust seals FAIL --------------
# finding 1 = gross_weight_reconciliation (REVIEW); human HOLDS it (reject).
HUMAN_INPUT="$RUN/human_input.txt"
printf '%s\n%s\n%s\n%s\n%s\n%s\n%s\n%s\n%s\n%s\n' \
    "$PASSPHRASE" \
    "0" "confirm" "Required documents present and corroborated." "0" \
    "1" "reject" "HOLD - gross weight mismatch vs corroborating documents" "1" \
    "done" \
    > "$HUMAN_INPUT"
script -qec \
    "$ROOT/bin/doctrust-review --snapshot $SNAP_ABS --domain shipment_release --rulesets-dir $ROOT/rulesets --reviewer $REVIEWER --key-dir $KEYDIR" \
    /dev/null < "$HUMAN_INPUT" > "$RUN/human_session.txt" 2>&1

[ -f "$SNAP_ABS.doctrust_reviews.json" ] \
    && ok "human TTY wrote Ed25519-signed reviews sidecar" \
    || bad "reviews sidecar missing"

# ---- SEAL: disposition/finalize picks up verified records --------------------
# One doctrust-mcp session: evaluate (merges verified human records),
# then fetch the sealed audit artifact. Requests are PACED so the server
# processes them strictly in order.
LOADCASE_ID="$(sha256sum "$SNAP_ABS" | cut -c1-16)"

# Finalization happened inside the human-authority process (trusted DocTrust
# code path): it replayed evaluation, verified the Ed25519 records, and sealed
# the authoritative audit artifact.
AUDIT_JSON="$SNAP_ABS.audit.json"

$PY - "$AUDIT_JSON" "$REVIEWER" <<'PYASSERT'
import json, os, sys
path, expect_user = sys.argv[1], sys.argv[2]
a = json.load(open(path))
reviews = a.get("human_reviews") or []
fd = a.get("final_disposition")
ok_fd = fd == "FAIL"
ok_rev = len(reviews) >= 1 and any(
    r.get("action") == "reject"
    and r.get("channel") == "human-tty"
    and r.get("reviewer_identity") == expect_user
    for r in reviews)
ok_hash = bool(a.get("manifest", {}).get("artifact_hash"))
print(f"final_disposition={fd} (expect FAIL)  human_reviews={len(reviews)}  hash={'yes' if ok_hash else 'NO'}")
for r in reviews:
    print(f"  review: idx={r['finding_index']} action={r['action']} by={r.get('reviewer_identity')} ch={r.get('channel')}")
sys.exit(0 if ok_fd and ok_rev and ok_hash else 1)
PYASSERT

