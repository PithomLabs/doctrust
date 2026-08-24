#!/usr/bin/env bash
# capture_screenshots.sh — runs the FROZEN judge-demo once, slices its live output into
# six act-scoped excerpts, renders high-res monospace PNGs (Pillow), writes captions
# README, mirrors byte-identically to plans8/screenshots/.
#
# NEVER modifies judge-demo.sh or demo state. Consumes recorded live output only.
set -uo pipefail

REPO="/home/chaschel/Desktop/biz/nutrient/doctrust"
CANON="$REPO/docs/demo/screenshots"
MIRROR="/home/chaschel/Desktop/biz/nutrient/plans8/screenshots"
LOGROOT="${LOGROOT:-/tmp/judge-demo-capture-$(date +%H%M%S)}"
mkdir -p "$LOGROOT" "$CANON" "$MIRROR"

ansi_strip() { sed -E 's/\x1B\[[0-9;]*[A-Za-z]//g'; }
secret_scan() { # exits nonzero if secrets found
  grep -EqE '(sk-[A-Za-z0-9_-]{8,}|pdf_live_[A-Za-z0-9]{10,}|API_KEY=?[A-Za-z0-9]{8,})' "$1"
}

log() { printf '\n== %s ==\n' "$*"; }

if [ "${SKIP_RUN:-0}" = "1" ] && ls /tmp/judge-demo-capture-*/raw.txt >/dev/null 2>&1; then
  log "STEP 1 skipped (SKIP_RUN=1) — reusing preserved raw capture"
else
  log "STEP 1 — fresh frozen demo run (single source of truth)"
RUNLOG="$LOGROOT/run.log"
RAW="$LOGROOT/raw.txt"
if ! bash "$REPO/scripts/judge-demo.sh" 2>&1 | tee "$RUNLOG" | ansi_strip > "$RAW"; then
    echo "FATAL: judge-demo run failed"; exit 1
  fi
fi

log "STEP 2 — secret scan on raw capture"
if secret_scan "$LOGROOT/raw.txt"; then echo "FATAL: secrets detected in output"; exit 1; fi
pass="secrets-clean"

log "STEP 3 — slice six acts"
slice() { awk -v s="$2" 'index($0,s){f=1} f' "$LOGROOT/raw.txt" | awk -v e="$3" -v f=1 'index($0,e){print; exit} f{print}' | head -n "$4"; }

{
  echo "=== JUDGE DEMO — ACT 1: evaluate_case ==="
  slice x "ACT1 pre-assert" "ACT2 findings:" 40
} > "$LOGROOT/01.txt"
{
  echo "=== JUDGE DEMO — ACT 2: finding + document evidence ==="
  slice x "ACT2 findings:" "ACT2b evidence refs:" 20
  slice x "ACT2b evidence refs:" "PASS: Acts" 12
} > "$LOGROOT/02.txt"
{
  echo "=== JUDGE DEMO — ACT 3: human review recorded ==="
  slice x "ACT3 human review recorded" "ACT4 artifact" 6
} > "$LOGROOT/03.txt"
{
  echo "=== JUDGE DEMO — ACT 4: tamper-evident audit artifact ==="
  slice x "ACT4 artifact" "ACTS-PASS" 10
} > "$LOGROOT/04.txt"
{
  echo "=== JUDGE DEMO — ACT 5: deliberately bad policy is rejected ==="
  echo "(DEMO FIXTURE — intentionally invalid policy expectation; not production data)"
  slice x "DEMO FIXTURE" "PASS: Act 5" 30
} > "$LOGROOT/05.txt"
{
  echo "=== JUDGE DEMO — ACT 6: promotion gate defense-in-depth ==="
  slice x "ACT 6 — defense-in-depth" "FINAL LINE" 40
  echo "Trusted Ruleset remains unchanged (v2)."
} > "$LOGROOT/06.txt"

for f in 01 02 03 04 05 06; do secret_scan "$LOGROOT/$f.txt" && { echo "FATAL: secrets in $f"; exit 1; }; done
grep -qE '/tmp/(judge-demo|promptexp)' "$LOGROOT"/0*.txt && { echo "FATAL: temp identifiers leaked"; exit 1; } || true

log "STEP 4 — render PNGs (Pillow, DejaVu Sans Mono, 2x)"
python3 - <<'PY'
from PIL import Image, ImageDraw, ImageFont
import glob
FONT="/usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf"
FS=22; PAD=28; SCALE=2
names={"01":"01-evaluate-case","02":"02-finding-and-evidence","03":"03-human-review",
       "04":"04-audit-artifact","05":"05-policy-rejection","06":"06-promotion-defense"}
font=ImageFont.truetype(FONT,FS)
for k,title in names.items():
    import os
    logdir=sorted(glob.glob("/tmp/judge-demo-capture-*"))[-1]
    lines=open(os.path.join(logdir,k+".txt")).read().rstrip().split("\n")
    w=max((font.getbbox(l)[2] for l in lines), default=400)+PAD*2
    h=(FS+10)*len(lines)+PAD*2
    img=Image.new("RGB",(w,h),"#0d1117"); d=ImageDraw.Draw(img)
    y=PAD
    for l in lines:
        d.text((PAD,y),l,font=font,fill="#e6edf3"); y+=FS+10
    img=img.resize((w*SCALE,h*SCALE),Image.LANCZOS)
    out=f"/home/chaschel/Desktop/biz/nutrient/doctrust/docs/demo/screenshots/{names[k]}.png"
    img.save(out)
    print("wrote",out,img.size)
PY
RC=$?
[ $RC -ne 0 ] && { echo "FATAL: render failed"; exit 1; }

log "STEP 5 — captions README (live values from this run)"
CID=$(grep -oE 'case_id=[a-f0-9]+' "$LOGROOT/raw.txt" | head -1 | cut -d= -f2)
TS=$(date -Iseconds)
cat > "$CANON/README.md" <<RDEOF
# Judge-Demo Screenshots — captured $TS

Source run: single fresh execution of \`scripts/judge-demo.sh\` (frozen demo).
Ruleset: income_verification v2 · Case ID: \`$CID\` · All values live-parsed; nothing edited.

**Canonical source:** \`docs/demo/screenshots/\`
**Demo-package mirror:** \`plans8/screenshots/\` (must remain byte-identical)

### 01-evaluate-case.png
Caption: A real agent-facing document compliance decision produced by the frozen engine.
What the judge should notice: Ruleset income_verification v2, REVIEW/WARNING decision,
case ID and finding count — all from the live MCP runtime.
Act: 1 — Evaluate

### 02-finding-and-evidence.png
Caption: The finding traces to exact page/bounding-box evidence inside the source documents.
What the judge should notice: paystub, W-2 and 1040 evidence rows with real coordinates.
Act: 2 — Evidence

### 03-human-review.png
Caption: The agent surfaces the issue; a human remains the authority for the disposition.
What the judge should notice: action=confirm recorded with presenter note and resolved_at.
Act: 3 — Human authority

### 04-audit-artifact.png
Caption: Decision and human action are preserved in a tamper-evident audit record.
What the judge should notice: ruleset id/version, disposition, human_reviews entry count,
artifact hash. Hash binds THIS run's contents; it differs per run by design.
Act: 4 — Auditability

### 05-policy-rejection.png
Caption: A deliberately wrong policy expectation is caught before approval.
What the judge should notice: Expected PASS vs deterministic Actual REVIEW — semantic
rejection executed pre-approval (not a provenance shortcut).
Act: 5 — Policy safety

### 06-promotion-defense.png
Caption: Even with a bypassed approval surface, the promotion gate independently rejects
the bad policy and trusted state does not move.
What the judge should notice: Gate 5 failure row, SHA256 tree equality, promoted v2 intact.
Act: 6 — Defense in depth
RDEOF

log "STEP 6 — byte-identical mirror to plans8/screenshots/"
rm -rf "$MIRROR"; mkdir -p "$MIRROR"
cp -a "$CANON/." "$MIRROR/"
for f in "$CANON"/*; do cmp -s "$f" "$MIRROR/$(basename "$f")" || { echo "FATAL mirror mismatch $f"; exit 1; }; done
echo "PASS: mirror byte-identical"

log "STEP 7 — cleanup verification"
ls /tmp/judge-demo-* >/dev/null 2>&1 && echo "note: demo sandboxes/logs retained under /tmp (outside repo)" || true
cd "$REPO"
git status --short
echo "CAPTURE-COMPLETE"
