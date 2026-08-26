#!/usr/bin/env bash
# judge-demo.sh — LLM-free, six-act judge-facing demo of the DocTrust trust funnel.
# Canonical in-repo location: scripts/judge-demo.sh
# Evidence package: docs/demo/rehearsal_report.md
# Runs the REAL binaries inside an isolated sandbox. Nothing mocked, nothing canned.
set -uo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
LOGROOT="/tmp/judge-demo-logs"
mkdir -p "$LOGROOT"
LOG="$LOGROOT/demo_$(date +%H%M%S).log"
SB=""; MCP_PID=""

log()  { printf '\n\033[1;36m== %s ==\033[0m\n' "$*" | tee -a "$LOG"; }
pass() { printf '\033[1;32mPASS: %s\033[0m\033[0m\n' "$*" | tee -a "$LOG"; }
fail() { printf '\n\033[1;31mFAIL: %s\033[0m\n' "$*" | tee -a "$LOG"; }

CLEANED=0
cleanup() {
  [ "$CLEANED" = "1" ] && return 0
  CLEANED=1
  [ -n "$MCP_PID" ] && kill "$MCP_PID" 2>/dev/null
  [ -n "$SB" ] && rm -rf "$SB"
  log "Cleanup done."
}
trap cleanup EXIT

log "SETUP — isolated sandbox + build"
SB=$(mktemp -d /tmp/judge-demo-XXXXXX)
rsync -a --exclude .git --exclude bin --exclude candidates \
      --exclude '.doctrust-*' --exclude .env "$REPO/" "$SB/"
cd "$SB" || exit 1
make build >/dev/null 2>&1 && make mcp >/dev/null 2>&1 || { fail "build"; exit 1; }
pass "build (12 binaries incl. doctrust-mcp)"

# ============================================================== ACTS 1–6 ====
log "ACTS 1–6 — agent MCP journey: request → evidence → REVIEW → agent stops at authority boundary"

run_mcp() { # requests arrive on stdin from the feeder subshell
  timeout 150 bin/doctrust-mcp --rulesets-dir rulesets --snapshot-root "$SB/demo" 2>/dev/null
}

# --- Single-session MCP driver: writer reacts to live responses ---
LATEST_PROMOTED=$(ls rulesets/income_verification/v*.yaml | grep -E 'v[0-9]+\.yaml$' | sed -E 's/.*v([0-9]+)\.yaml/\1/' | sort -n | tail -1)
echo "ACT1 pre-assert: sandbox promoted latest = v$LATEST_PROMOTED" | tee -a "$LOG"
REQ="$LOGROOT/mcp_$$.json"

(
  send() { printf '%s\n' "$1"; }

  send '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"judge-demo","version":"1"}}}'
  sleep 1
  send '{"jsonrpc":"2.0","method":"notifications/initialized"}'
  sleep 0.3

  # ACT 1: User request — evaluate the case
  send '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"evaluate_case","arguments":{"snapshot_path":"income_verification/evidence_snapshot.json"}}}'
  for i in $(seq 1 240); do grep -q '"case_id"' "$REQ" 2>/dev/null && break; sleep 0.25; done

  CID=$(python3 - "$REQ" <<'PY'
import json,sys
for line in open(sys.argv[1]):
    try: o=json.loads(line)
    except Exception: continue
    if o.get('id')==2:
        d=json.loads(o['result']['content'][0]['text'])
        print(d.get('case_id','')); break
PY
)
  [ -n "$CID" ] || exit 3
  sleep 0.5

  # ACT 2: Get findings — shows REVIEW
  send "{\"jsonrpc\":\"2.0\",\"id\":3,\"method\":\"tools/call\",\"params\":{\"name\":\"get_findings\",\"arguments\":{\"case_id\":\"$CID\"}}}"
  for i in $(seq 1 120); do grep -q '"findings"' "$REQ" 2>/dev/null && break; sleep 0.25; done

  RIDX=$(python3 - "$REQ" <<'PY'
import json,sys
best=None
for line in open(sys.argv[1]):
    try: o=json.loads(line)
    except Exception: continue
    if o.get('id')==3:
        for f in json.loads(o['result']['content'][0]['text']).get('findings',[]):
            if f.get('status')=='REVIEW': best=f['index']
print('-1' if best is None else best)
PY
)
  [ "$RIDX" != "-1" ] || exit 4
  sleep 0.5

  # ACT 3: Get evidence for the REVIEW finding
  send "{\"jsonrpc\":\"2.0\",\"id\":4,\"method\":\"tools/call\",\"params\":{\"name\":\"get_evidence\",\"arguments\":{\"case_id\":\"$CID\",\"finding_index\":$RIDX}}}"
  sleep 1

  # ACT 4: Get audit artifact — proves REVIEW remains unresolved without human
  send "{\"jsonrpc\":\"2.0\",\"id\":5,\"method\":\"tools/call\",\"params\":{\"name\":\"get_audit_artifact\",\"arguments\":{\"case_id\":\"$CID\"}}}"
  sleep 2
) | run_mcp > "$REQ" &
MCP_PID=$!
wait $MCP_PID || true

python3 - "$REQ" "$LATEST_PROMOTED" <<'PY' | tee -a "$LOG"
import json,sys
resp={}
LATEST=sys.argv[2]
for line in open(sys.argv[1]):
    try: o=json.loads(line)
    except Exception: continue
    if isinstance(o.get('id'),int) and o['id']>=2: resp[o['id']]=o

missing=[i for i in (2,3,4,5) if i not in resp]
assert not missing, f"MCP-INCOMPLETE missing={missing}"

def payload(i): return json.loads(resp[i]['result']['content'][0]['text'])

# ACT 1: evaluate_case
ev=payload(2)
if str(ev.get('ruleset_version')) != LATEST:
    print(f"VERSION-ASSERT-FAIL: evaluate_case.ruleset_version={ev.get('ruleset_version')} != promoted latest {LATEST}")
    raise SystemExit(5)
print(f"ACT1 evaluate_case: case_id={ev.get('case_id')} ruleset={ev.get('ruleset_id')} v{ev.get('ruleset_version')} status={ev.get('status')} severity={ev.get('severity')} findings={ev.get('finding_count')}")

# ACT 2: findings
fnd=payload(3).get('findings',[])
ridx=None
print("ACT2 findings:")
for f in fnd:
    sel=" <-- selected (REVIEW)" if f.get('status')=='REVIEW' and ridx is None else ""
    if f.get('status')=='REVIEW' and ridx is None: ridx=f['index']
    print(f"   [{f['index']}] {f['check_id']}: {f['status']}/{f['severity']} - {str(f.get('reason'))[:70]}{sel}")

# ACT 3: evidence
evl=payload(4)
elist=(evl.get('evidence') or evl.get('evidence_refs') or [])
print("ACT3 evidence refs:")
for e in elist[:3]:
    print(f"   field={e.get('field')} doc={e.get('source_doc')} span={e.get('source_span')} conf={e.get('confidence')}")

# ACT 4: audit artifact — must carry ZERO HumanReviewRecords, disposition=REVIEW
art=payload(5); a=art.get('artifact',{})
hr = a.get('human_reviews') or []
fd = a.get('final_disposition')
print(f"ACT4 artifact: ruleset={a.get('ruleset_id')} v{a.get('ruleset_version')} hash={str(art.get('artifact_hash'))[:16]} disposition={fd!r}")
print(f"ACT4 human_reviews entries={len(hr)} (expected 0 — agent cannot author reviews)")
if hr:
    for e in hr[:2]:
        print(f"   UNEXPECTED: finding_index={e.get('finding_index')} action={e.get('action')} note={str(e.get('note'))[:60]!r} resolved_at={e.get('resolved_at')}")
    print("ACT4 FAIL: HumanReviewRecords present after agent-only session — authority boundary broken")
    raise SystemExit(7)
if fd and fd == "PASS":
    print("ACT4 FAIL: final disposition must not be PASS after an agent-only session with unresolved REVIEW findings")
    raise SystemExit(8)
if fd != "REVIEW":
    print(f"ACT4 FAIL: final disposition must be REVIEW (unresolved), got {fd!r}")
    raise SystemExit(9)

print("ACT4 narration: REVIEW remains unresolved — agent stops here; only an authorized human via bin/doctrust-review can resolve it")
assert elist, "no evidence returned"
print("ACTS-PASS: act1_evaluate act2_findings act3_evidence act4_audit_unresolved")
PY
[ ${PIPESTATUS[0]} -eq 0 ] || { fail "Acts 1–6 flow failed"; exit 1; }
pass "Acts 1–6 witnessed live (agent stops at REVIEW / human-authority boundary)"

# ============================================================== ACT 7 ========
log "ACT 7 — defense-in-depth: bad policy rejection at approval gate"
FA="candidates/active/judge_mismatch_check"
mkdir -p "$FA"
cat > "$FA/check.go" <<'GOEOF'
package candidate

import (
	"github.com/PithomLabs/doctrust/internal/evidence"
	"github.com/PithomLabs/doctrust/internal/eval"
)

type JudgeMismatchCheck struct{}

func (c JudgeMismatchCheck) ID() string      { return "judge_mismatch_check" }
func (c JudgeMismatchCheck) Version() string { return "1.0" }

func (c JudgeMismatchCheck) Evaluate(facts eval.Facts, params map[string]any) eval.Result {
	if len(facts["judge_probe_value"]) == 0 {
		return eval.Result{CheckID: c.ID(), Status: eval.StatusReview,
			Severity: eval.SeverityWarning, Reason: "judge probe observation missing",
			Evidence: []evidence.EvidenceRef{}}
	}
	return eval.Result{CheckID: c.ID(), Status: eval.StatusPass,
		Severity: eval.SeverityInfo, Reason: "probe present",
		Evidence: []evidence.EvidenceRef{}}
}
GOEOF
cat > "$FA/metadata.yaml" <<'YMLEOF'
id: judge_mismatch_check
version: "1.0"
description: "DEMO FIXTURE — intentionally invalid policy expectation; not production data"
parameters: {}
YMLEOF
cat > "$FA/scenarios.yaml" <<'YMLEOF'
scenarios:
  - name: expects_pass_but_engine_reviews
    origin: ai
    input:
      facts:
        - semantic_type: unrelated_value
          source_doc: w2
          field: other
          value: 7
          source_span: page=1
          confidence: 0.9
    expected:
      check_id: judge_mismatch_check
      status: PASS
      severity: INFO
      reason: "deliberately wrong: engine will REVIEW because probe fact is missing"
YMLEOF

cat > "$FA/adversarial.yaml" <<'YMLEOF'
scenarios:
  - name: adversarial_missing_probe
    origin: human_adversarial
    input:
      facts:
        - semantic_type: gross_income_taxable
          source_doc: w2
          field: wages_tips_other_compensation
          value: 90000
          source_span: page=1
          confidence: 0.98
    expected:
      check_id: judge_mismatch_check
      status: REVIEW
      severity: WARNING
      reason: "judge probe observation missing"
YMLEOF
echo -n DRAFT > "$FA/state"
printf '%s\n' 'DEMO FIXTURE — intentionally invalid policy expectation; not production data' | tee -a "$LOG"
printf 'a\ny\n' | bin/review-check "$FA" > "$LOG.act7" 2>&1 || true
SEMROW=$(grep -F 'expects_pass_but_engine_reviews: expected=PASS/INFO actual=REVIEW/WARNING' "$LOG.act7" | head -1)
STATE=$(cat "$FA/state" 2>/dev/null || echo DRAFT)
if [ -n "$SEMROW" ] && [ "$STATE" != "APPROVED" ]; then
  pass "Act 7: semantic rejection witnessed — $SEMROW ; approval BLOCKED"
else
  fail "Act 7: approval unexpectedly allowed"; exit 1
fi

# ============================================================== ACT 8 ========
log "ACT 8 — defense-in-depth promotion gate (scratch-approved same-mismatch candidate)"
FB="candidates/active/judge_gate5_probe"
mkdir -p "$FB"
cp "$FA/check.go" "$FB/check.go"
sed 's/judge_mismatch_check/judge_gate5_probe/g' "$FA/check.go" > "$FB/check.go"
sed 's/judge_mismatch_check/judge_gate5_probe/g' "$FA/metadata.yaml" > "$FB/metadata.yaml"
sed 's/judge_mismatch_check/judge_gate5_probe/g' "$FA/scenarios.yaml" > "$FB/scenarios.yaml"
cat > "$FB/adversarial.yaml" <<'YMLEOF'
scenarios:
  - name: judge_gate5_adversarial_edge
    origin: human_adversarial
    input:
      facts:
        - semantic_type: probe_value
          source_doc: paystub
          field: value
          value: -3
          source_span: page=1
          confidence: 0.9
    expected:
      check_id: judge_gate5_probe
      status: REVIEW
      severity: WARNING
      reason: "judge probe observation missing"
YMLEOF
mkdir -p scrathtool
cat > scrathtool/main.go <<'GOEOF'
package main

import (
	"fmt"
	"os"

	"github.com/PithomLabs/doctrust/internal/compiler"
)

// scratch-only demo tooling: simulates an erroneous but content-valid approval so the
// PROMOTION gate can be shown independently rejecting the bad policy. Deleted at cleanup.
func main() {
	dir := os.Args[1]
	id := os.Args[2]
	if err := compiler.SetState(dir, compiler.StateApproved); err != nil {
		fmt.Fprintln(os.Stderr, err); os.Exit(1)
	}
	if err := compiler.WriteApproval(dir, id, "1.0", "defense_in_depth_demo"); err != nil {
		fmt.Fprintln(os.Stderr, err); os.Exit(1)
	}
	fmt.Println("approved-simulated")
}
GOEOF
go run ./scrathtool "$FB" judge_gate5_probe >>"$LOG" 2>&1 || { fail "Act 8 setup"; exit 1; }
TREE_BEFORE=$(for d in internal/eval rulesets scenarios; do find "$d" -type f -print0; done | sort -z | xargs -0 sha256sum 2>/dev/null | sha256sum | cut -d' ' -f1)
PC=$(timeout 300 bin/promote-check --candidate "$FB" --domain income_verification 2>&1)
RC=$?
TREE_AFTER=$(for d in internal/eval rulesets scenarios; do find "$d" -type f -print0; done | sort -z | xargs -0 sha256sum 2>/dev/null | sha256sum | cut -d' ' -f1)
echo "$PC" | tail -6 | tee -a "$LOG"
if [ $RC -ne 0 ] && echo "$PC" | grep -qE "scenario.*failed|Scenarios: .*failed" && [ "$TREE_BEFORE" = "$TREE_AFTER" ]; then
  pass "Act 8: Gate 5 rejected bad policy; SHA256(trusted tree) unchanged"
else
  fail "Act 8 unexpected state rc=$RC trees_equal=$([ "$TREE_BEFORE" = "$TREE_AFTER" ] && echo yes || echo NO)"; exit 1
fi

# ============================================================== CLOSE =======
log "CLOSE — trusted Ruleset unchanged"
LATEST=$(ls rulesets/income_verification/v*.yaml | grep -E 'v[0-9]+\.yaml$' | sed -E 's/.*v([0-9]+)\.yaml/\1/' | sort -n | tail -1)
[ "$LATEST" = "2" ] || { fail "expected latest promoted v2, got v$LATEST"; exit 1; }
pass "bad rule never promoted; trusted Ruleset remains v$LATEST"
echo | tee -a "$LOG"
echo 'FINAL LINE: "DocTrust does not trust the agent to write the law. It makes sure the policy the agent consumes is trusted."' | tee -a "$LOG"
cleanup
pass "cleanup (sandbox removed, processes stopped)"
echo "JUDGE-DEMO-STATUS: evaluate_case=PASS findings=PASS evidence=PASS audit_unresolved=PASS agent_stops=PASS bad_policy_rejection=PASS gate5_rejection=PASS trusted_tree_immutability=PASS cleanup=PASS" | tee -a "$LOG"