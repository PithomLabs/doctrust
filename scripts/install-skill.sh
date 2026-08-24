#!/usr/bin/env bash
# Deploy the canonical Compliance Skill (source of truth) to the Hermes
# runtime location. Idempotent; fails if the runtime copy has diverged from
# canonical (never two independently editable copies).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SRC="$ROOT/skills/compliance-check-artifact/SKILL.md"
DST_DIR="${HERMES_SKILLS_DIR:-$HOME/.hermes}/skills/compliance-check-artifact"
DST="$DST_DIR/SKILL.md"

[ -f "$SRC" ] || { echo "FATAL: canonical skill missing: $SRC"; exit 1; }

if [ -f "$DST" ]; then
    if ! cmp -s "$SRC" "$DST"; then
        if [ "${FORCE_DEPLOY:-0}" != "1" ]; then
            echo "FATAL: runtime copy diverged from canonical: $DST"
            echo "       inspect the diff, then re-run with FORCE_DEPLOY=1 to overwrite."
            diff "$SRC" "$DST" | head -20 || true
            exit 1
        fi
    fi
fi

mkdir -p "$DST_DIR"
cp "$SRC" "$DST"
echo "deployed: $DST"
