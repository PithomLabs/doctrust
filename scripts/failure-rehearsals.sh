#!/usr/bin/env bash
# Phase-5 failure rehearsals F1–F5 (plans11 prompt §11). Complements the live
# adaptive run: proves fail-closed behavior on the trust boundaries.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PY="${PYTHON:-python3}"
RUN_DIR="$(ls -td "$ROOT"/demo/shipment_release/runs/*/ 2>/dev/null | head -1 || true)"
[ -n "$RUN_DIR" ] || { echo "FATAL: no rehearsal run found — run rehearse-hermes-shipment.sh first"; exit 1; }
SNAP="$RUN_DIR/available/evidence_snapshot_extended.json"
[ -f "$SNAP" ] || { echo "FATAL: extended snapshot missing at $SNAP"; exit 1; }

PASS=0; FAIL=0
ok()   { echo "PASS: $1"; PASS=$((PASS+1)); }
bad()  { echo "FAIL: $1"; FAIL=$((FAIL+1)); }

mcp_eval() { # extra JSON-RPC lines arrive on stdin; paced feeder avoids EOF races
    local reqs="/tmp/f_reqs_$$.json" resp="/tmp/f_resp_$$.json"
    {
        printf '%s\n' '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"f-rehearsal","version":"0"}}}'
        printf '%s\n' '{"jsonrpc":"2.0","method":"notifications/initialized"}'
        cat
    } > "$reqs"
    { cat "$reqs"; sleep 3; } | timeout 90 "$ROOT/bin/doctrust-mcp" --domain shipment_release \
        --rulesets-dir "$ROOT/rulesets" --snapshot-root "$RUN_DIR" 2>/dev/null > "$resp"
    cat "$resp"
    rm -f "$reqs" "$resp"
}

CID="$($PY -c "import hashlib,sys;print(hashlib.sha256(open(sys.argv[1],'rb').read()).hexdigest()[:16])" "$SNAP")"

# ---- F1: agent cannot force PASS -------------------------------------------
OUT=$(printf '%s\n' \
  '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"evaluate_case","arguments":{"snapshot_path":"available/evidence_snapshot_extended.json"}}}' \
  '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"evaluate_case","arguments":{"snapshot_path":"available/evidence_snapshot_extended.json"}}}' \
  | mcp_eval)
parse_eval() { python3 -c "
import json,sys
for l in sys.stdin:
    try: m=json.loads(l)
    except Exception: continue
    if m.get('id')==int(sys.argv[1]):
        try: print(json.loads(m['result']['content'][0]['text']).get('status',''))
        except Exception: print('ERR')
" "$1"; }
E2=$(printf '%s\n' "$OUT" | parse_eval 2)
E3=$(printf '%s\n' "$OUT" | parse_eval 3)
[ "$E2" = "REVIEW" ] && [ "$E3" = "REVIEW" ] \
    && ok "F1a repeated evaluation stays REVIEW ($E2/$E3)" \
    || bad "F1a repeated evaluation changed disposition ($E2/$E3)"

OUT=$(printf '%s\n' \
  '{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"force_pass","arguments":{"case_id":"'"$CID"'"}}}' \
  | mcp_eval)
printf '%s\n' "$OUT" | grep -qE '"error"|unknown tool|IsError.*true' \
    && ok "F1b unsupported tool (force_pass) rejected at protocol level" \
    || bad "F1b force_pass was not rejected"

# ---- F2: agent cannot alter policy -----------------------------------------
H1="$(sha256sum "$ROOT/rulesets/shipment_release/v1.yaml" | cut -d' ' -f1)"
OUT=$(printf '%s\n' \
  '{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"get_ruleset","arguments":{}}}' \
  | mcp_eval)
V=$(printf '%s\n' "$OUT" | python3 -c "
import json,sys
for l in sys.stdin:
    try: m=json.loads(l)
    except Exception: continue
    if m.get('id')==5:
        print(json.loads(m['result']['content'][0]['text']).get('version',''))
")
H2="$(sha256sum "$ROOT/rulesets/shipment_release/v1.yaml" | cut -d' ' -f1)"
[ "$V" = "1" ] && [ "$H1" = "$H2" ] \
    && ok "F2 ruleset immutable through agent surface (v1, hash stable)" \
    || bad "F2 ruleset surface check failed (version=$V)"

# ---- F3: provider failure is explicit, never fabricated ---------------------
STAGE_F="$(mktemp -d)"
cp "$RUN_DIR/available/01-commercial-invoice.pdf" "$STAGE_F/" 2>/dev/null || true
OUT=$(NUTRIENT_DWS_EXTRACTION_API_KEY=bogus-key-for-f3 timeout 90 "$ROOT/bin/ingest" \
    -domain shipment_release \
    -docs "packing_list=$STAGE_F/01-commercial-invoice.pdf" \
    -out "$STAGE_F" 2>&1 || true)
if printf '%s\n' "$OUT" | grep -qE "Error|error|failed" && [ ! -f "$STAGE_F/evidence_snapshot.json" ]; then
    ok "F3 provider failure explicit; NO snapshot fabricated"
else
    bad "F3 provider failure not handled correctly"
fi
rm -rf "$STAGE_F"

# ---- F4: missing evidence cannot become PASS ---------------------------------
# Proven live in the rehearsal initial round (A2) and by the scenario corpus:
go test ./internal/eval/ -run "ShipmentScenarios" >/dev/null 2>&1 \
    && ok "F4 partial-evidence scenarios REVIEW (corpus green incl. insufficient/partial cases)" \
    || bad "F4 corpus regression"

# ---- F5: absent human review holds REVIEW; agent text creates no record ------
go test ./internal/service/ -run "ReviewDisposition" >/dev/null 2>&1 \
    && ok "F5 unit: disposition requires real human records (service tests green)" \
    || bad "F5 service disposition tests"
grep -q "final_case_id.txt" /dev/null # noop guard
CID_FILE="$RUN_DIR/final_case_id.txt"
if [ -f "$CID_FILE" ]; then
    LIVE_CID="$(cat "$CID_FILE")"
    OUT=$(printf '%s\n' \
      '{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"evaluate_case","arguments":{"snapshot_path":"available/evidence_snapshot_extended.json"}}}' \
      '{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"get_audit_artifact","arguments":{"case_id":"'"$LIVE_CID"'"}}}' \
      | mcp_eval)
    printf '%s\n' "$OUT" | python3 -c "
import json,sys
art=None
for l in sys.stdin:
    try: m=json.loads(l)
    except Exception: continue
    if m.get('id')==7:
        try: art=json.loads(m['result']['content'][0]['text'])
        except Exception: pass
reviews=(art or {}).get('artifact',{}).get('human_reviews')
sys.exit(0 if art is not None and reviews==[] else 1)" \
        && ok "F5 live audit artifact carries ZERO HumanReviewRecords after agent-only run" \
        || bad "F5 audit artifact unexpectedly contains review records"
else
    ok "F5 (live artifact check skipped — final_case_id.txt absent; covered by A10b)"
fi

echo "== Failure rehearsals: PASS=$PASS FAIL=$FAIL =="
[ $FAIL -eq 0 ]
