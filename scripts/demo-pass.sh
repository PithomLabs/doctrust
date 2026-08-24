#!/usr/bin/env bash
set -euo pipefail
# demo-pass: coherent PASS package — all four gross weights agree at 4,650
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PASS_DIR="$ROOT/demo/shipment_release/fixtures/pass"
if [ ! -d "$PASS_DIR" ]; then
  PASS_DIR="$ROOT/../doc-generator/examples/shipment-1047/generated-pass"
fi
if [ ! -d "$PASS_DIR" ]; then
  PASS_DIR="$ROOT/demo/shipment_release/pass-fixture"
fi
OUT="/tmp/doctrust-demo-pass-$$"
mkdir -p "$OUT"
echo "== demo-pass: ingest coherent PASS package =="
"$ROOT/bin/ingest" -domain shipment_release \
  -docs "commercial_invoice=$PASS_DIR/01-commercial-invoice.pdf,packing_list=$PASS_DIR/02-packing-list.pdf,bill_of_lading=$PASS_DIR/03-bill-of-lading.pdf,certificate_of_origin=$PASS_DIR/04-certificate-of-origin.pdf" \
  -out "$OUT" 2>&1 | tail -5
# Evaluate via doctrust-mcp (single tool call)
echo "== demo-pass: evaluate =="
printf '%s\n' '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"demo-pass","version":"0"}}}' > /tmp/demo-pass-req.jsonl
printf '%s\n' '{"jsonrpc":"2.0","method":"notifications/initialized"}' >> /tmp/demo-pass-req.jsonl
printf '%s\n' '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"evaluate_case","arguments":{"snapshot_path":"'"$OUT/evidence_snapshot.json"'"}}}' >> /tmp/demo-pass-req.jsonl
{ cat /tmp/demo-pass-req.jsonl; sleep 3; } | DOCTRUST_SNAPSHOT_ROOT="$OUT" timeout 30 "$ROOT/bin/doctrust-mcp" --domain shipment_release --rulesets-dir "$ROOT/rulesets" --snapshot-root "$OUT" 2>/dev/null > /tmp/demo-pass-resp.jsonl
python3 -c "
import json
for line in open('/tmp/demo-pass-resp.jsonl'):
    import json as j
    try: m=j.loads(line)
    except: continue
    if m.get('id')==2:
        txt=m['result']['content'][0]['text']
        d=j.loads(txt)
        print(f\"PASS case: {d['case_id']} status={d['status']} findings={d['finding_count']}\")
        assert d['status']=='PASS', 'expected PASS'
"
AUDIT="$OUT/evidence_snapshot.json.audit.json"
# Also try get_audit_artifact via second session if needed
echo "audit: $OUT/evidence_snapshot.json"
ls -lh "$OUT" | head -10
echo "demo-pass complete — audit artifact at $OUT"
