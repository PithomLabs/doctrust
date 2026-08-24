#!/usr/bin/env bash
set -euo pipefail
# demo-review: anomaly package → REVIEW/BLOCKING → human authority → signed audit
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ANOMALY_DIR="$ROOT/../doc-generator/examples/shipment-1047/generated"
if [ ! -d "$ANOMALY_DIR" ]; then
  ANOMALY_DIR="$ROOT/demo/shipment_release/anomaly-fixture"
fi
OUT="/tmp/doctrust-demo-review-$$"
mkdir -p "$OUT"
echo "== demo-review: ingest anomaly package (B/L 5,150 vs 4,650) =="
"$ROOT/bin/ingest" -domain shipment_release \
  -docs "commercial_invoice=$ANOMALY_DIR/01-commercial-invoice.pdf,packing_list=$ANOMALY_DIR/02-packing-list.pdf,bill_of_lading=$ANOMALY_DIR/03-bill-of-lading.pdf,certificate_of_origin=$ANOMALY_DIR/04-certificate-of-origin.pdf" \
  -out "$OUT" 2>&1 | tail -5

# Evaluate
printf '%s\n' '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"demo-review","version":"0"}}}' > /tmp/demo-review-req.jsonl
printf '%s\n' '{"jsonrpc":"2.0","method":"notifications/initialized"}' >> /tmp/demo-review-req.jsonl
printf '%s\n' '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"evaluate_case","arguments":{"snapshot_path":"'"$OUT/evidence_snapshot.json"'"}}}' >> /tmp/demo-review-req.jsonl
{ cat /tmp/demo-review-req.jsonl; sleep 3; } | DOCTRUST_SNAPSHOT_ROOT="$OUT" timeout 30 "$ROOT/bin/doctrust-mcp" --domain shipment_release --rulesets-dir "$ROOT/rulesets" --snapshot-root "$OUT" 2>/dev/null > /tmp/demo-review-resp.jsonl
python3 -c "
import json
for l in open('/tmp/demo-review-resp.jsonl'):
    try: m=json.loads(l)
    except: continue
    if m.get('id')==2:
        d=json.loads(m['result']['content'][0]['text'])
        print(f\"REVIEW case: {d['case_id']} status={d['status']} severity={d['severity']} findings={d['finding_count']}\")
"
echo ""
echo "Human authority required. To resolve:"
echo "  bin/doctrust-review --snapshot $OUT/evidence_snapshot.json --reviewer \$(whoami) --key-dir ~/.doctrust-reviewer"
echo "Or for automated demo (uses test passphrase):"
echo "  PHASE6_PASSPHRASE=phase6-hold-passphrase $ROOT/scripts/rehearse-phase6-human.sh"
echo ""
echo "Audit will be at: $OUT/*.audit.json after human review"
ls -lh "$OUT" | head -10
