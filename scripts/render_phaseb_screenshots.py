#!/usr/bin/env python3
"""Render Phase-B screenshots S01-S08 from golden run artifacts.

Uses the same Pillow + DejaVu Sans Mono pipeline as capture_screenshots.sh.
Dark background (#0d1117), light text (#e6edf3), 2x resolution.
"""
import os
import json
from PIL import Image, ImageDraw, ImageFont

REPO = "/home/chaschel/Desktop/biz/nutrient/doctrust"
ASSETS = os.path.join(REPO, "demo", "assets")
FONT = "/usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf"
FONT_BOLD = "/usr/share/fonts/truetype/dejavu/DejaVuSansMono-Bold.ttf"
FS = 20
PAD = 32
LINE_SPACING = 8
SCALE = 2
BG = "#0d1117"
FG = "#e6edf3"
FG_DIM = "#8b949e"
FG_ACCENT = "#58a6ff"
FG_WARN = "#f0883e"
FG_SUCCESS = "#3fb950"
FG_ERROR = "#f85149"

os.makedirs(ASSETS, exist_ok=True)

font = ImageFont.truetype(FONT, FS)
font_bold = ImageFont.truetype(FONT_BOLD, FS)
font_sm = ImageFont.truetype(FONT, FS - 2)


def render_text_block(lines, width=None):
    """Render text lines to an image. Returns PIL Image."""
    if width is None:
        w = max((font.getbbox(l)[2] for l in lines), default=600) + PAD * 2
    else:
        w = width
    h = (FS + LINE_SPACING) * len(lines) + PAD * 2
    img = Image.new("RGB", (w, h), BG)
    d = ImageDraw.Draw(img)
    y = PAD
    for l in lines:
        d.text((PAD, y), l, font=font, fill=FG)
        y += FS + LINE_SPACING
    return img


def render_rich_block(blocks, width=900):
    """Render rich text blocks (with color/size variation). Returns PIL Image.
    blocks: list of (text, color, font) tuples.
    """
    h = PAD
    for text, color, fnt in blocks:
        lines = text.split("\n")
        h += (fnt.size + LINE_SPACING) * len(lines)
    h += PAD
    img = Image.new("RGB", (width, h), BG)
    d = ImageDraw.Draw(img)
    y = PAD
    for text, color, fnt in blocks:
        lines = text.split("\n")
        for l in lines:
            d.text((PAD, y), l, font=fnt, fill=color)
            y += fnt.size + LINE_SPACING
    return img


def save(img, name):
    path = os.path.join(ASSETS, f"{name}.png")
    img.save(path)
    print(f"wrote {path} {img.size}")
    return path


# === S01: Business Consequence ===
def render_s01():
    blocks = [
        ("DocTrust", FG_ACCENT, font_bold),
        ("", FG, font),
        ("Shipment 1047", FG, font_bold),
        ("Trade Document Compliance", FG, font),
        ("", FG, font),
        ("A shipment is about to be released.", FG, font),
        ("Did these documents actually satisfy", FG, font),
        ("the approved policy?", FG, font),
    ]
    img = render_rich_block(blocks, width=700)
    return save(img, "S01-business-context")


# === S02: The Ask + Nutrient (Phase-5 initial state) ===
def render_s02():
    lines = [
        'User request:',
        '  "Check Shipment 1047 against the approved',
        '   release policy"',
        "",
        "Ruleset: shipment_release v1",
        "  - required_shipment_documents v1.0",
        "  - gross_weight_reconciliation v1.0",
        "",
        "Initial evidence state:",
        "  Documents available: 2 of 4",
        "    [x] commercial_invoice",
        "    [x] bill_of_lading",
        "    [ ] packing_list (not yet available)",
        "    [ ] certificate_of_origin (not yet available)",
        "",
        "Case ID: shipment_3a2ea0ec5ce1a41b",
    ]
    img = render_text_block(lines, width=700)
    return save(img, "S02-the-ask-nutrient")


# === S03: Insufficient Evidence (Phase-5) ===
def render_s03():
    lines = [
        "=== DocTrust Disposition: REVIEW — BLOCKING ===",
        "",
        "| Check                       | Status | Severity |",
        "|-----------------------------|--------|----------|",
        "| required_shipment_documents | REVIEW | BLOCKING |",
        "| gross_weight_reconciliation | REVIEW | BLOCKING |",
        "",
        "Insufficient evidence:",
        "  2 of 4 required shipment documents present",
        "",
        "Present:",
        "  [x] commercial_invoice",
        "  [x] bill_of_lading",
        "",
        "Missing:",
        "  [ ] packing_list",
        "  [ ] certificate_of_origin",
    ]
    img = render_text_block(lines, width=700)
    return save(img, "S03-insufficient-evidence")


# === S04: Adaptive Investigation (Phase-5, exact turn1.txt wording) ===
def render_s04():
    lines = [
        "HERMES / AGENT",
        "",
        "To resolve:",
        "  obtain the missing required documents",
        "",
        "  * packing list",
        "  * certificate_of_origin",
        "",
        "through the authorized evidence provider",
        "",
        "-> extend snapshot",
        "-> re-evaluate",
        "",
        "Released:",
        "  certificate_of_origin, packing_list",
    ]
    img = render_text_block(lines, width=700)
    return save(img, "S04-adaptive-investigation")


# === S05: Evidence Mismatch ===
def render_s05():
    lines = [
        "=== Gross Weight Evidence (KG) ===",
        "",
        "  Commercial Invoice      4,650",
        "  Packing List            4,650",
        "  Certificate of Origin   4,650",
        "  Bill of Lading          5,150  <-- OUTLIER",
        "",
        "=== DocTrust ===",
        "  gross_weight_reconciliation: REVIEW / BLOCKING",
        "  Condition: all_equal FAILED",
        "  Outlier: bill_of_lading (5150 vs 4650)",
        "",
        "Evidence References:",
        "  B/L  @ page=1 bbox=[459.0,288.0,50.4,7.9]",
        "  CO   @ page=1 bbox=[493.9,380.9,53.6,7.9]",
        "  CI   @ page=1 bbox=[511.9,349.9,50.8,6.8]",
        "  PL   @ page=2 bbox=[423.0,466.9,33.8,6.8]",
    ]
    img = render_text_block(lines, width=700)
    return save(img, "S05-evidence-mismatch")


# === S06: Human Authority Boundary ===
def render_s06():
    lines = [
        "=== MCP tools/list: 5 tools ===",
        "  evaluate_case, get_audit_artifact,",
        "  get_evidence, get_findings, get_ruleset",
        "",
        "Agent attempts request_human_review:",
        '  -> ERROR: unknown tool "request_human_review"',
        "",
        "=== Human TTY Session ===",
        "  case: 56cf311f09d45996",
        "  Ruleset: shipment_release v1",
        "",
        "  [0] required_shipment_documents  PASS/INFO",
        "      action: confirm",
        '      note: "Required documents present and corroborated."',
        "",
        "  [1] gross_weight_reconciliation  REVIEW/BLOCKING",
        "      action: reject",
        '      note: "HOLD - gross weight mismatch"',
        "",
        "  Signed: 2 Ed25519 records (key_id=owner)",
        "  FINAL DISPOSITION: FAIL",
    ]
    img = render_text_block(lines, width=750)
    return save(img, "S06-human-authority")


# === S07: Audit Trail ===
def render_s07():
    lines = [
        "=== AUDIT ARTIFACT ===",
        "",
        "  version:          1.0",
        "  policy_id:        shipment_release",
        "  ruleset_version:  1",
        "  ruleset_hash:     d3f1a945867942c3...",
        "  final_disposition: FAIL",
        "  artifact_hash:    b2cc19a7309968b3...",
        "",
        "  Documents:  4",
        "  Decisions:  1 (state: REVIEW)",
        "  Reviews:    2",
        "    [0] confirm — owner — human-tty",
        "    [1] reject  — owner — human-tty",
        "",
        "  Created:  2026-08-24T17:59:03Z",
        "  Completed: 2026-08-24T17:59:03Z",
    ]
    img = render_text_block(lines, width=700)
    return save(img, "S07-audit-trail")


# === S08: Feasibility Close (Graphical) ===
def render_s08():
    width, height = 800, 500
    img = Image.new("RGB", (width, height), BG)
    d = ImageDraw.Draw(img)

    # Title
    d.text((PAD, PAD), "DOCTRUST", font=font_bold, fill=FG_ACCENT)

    # Vertical line from title
    cx = width // 2
    d.line([(cx, 70), (cx, 140)], fill=FG_DIM, width=2)

    # Horizontal line
    d.line([(cx - 200, 140), (cx + 200, 140)], fill=FG_DIM, width=2)

    # Three branches
    branches = [
        (cx - 200, "SHIPMENT", "NOW", FG_SUCCESS),
        (cx, "KYC", "ROADMAP", FG_WARN),
        (cx + 200, "INSURANCE", "ROADMAP", FG_WARN),
    ]
    for bx, label, sub, color in branches:
        d.line([(cx, 140), (bx, 200)], fill=FG_DIM, width=2)
        d.line([(bx, 200), (bx, 240)], fill=FG_DIM, width=2)
        # Box
        d.rectangle([(bx - 80, 240), (bx + 80, 290)], outline=color, width=2)
        bbox = font_bold.getbbox(label)
        tw = bbox[2] - bbox[0]
        d.text((bx - tw // 2, 250), label, font=font_bold, fill=color)
        bbox_sub = font_sm.getbbox(sub)
        tw_sub = bbox_sub[2] - bbox_sub[0]
        d.text((bx - tw_sub // 2, 300), sub, font=font_sm, fill=FG_DIM)

    # Bottom text
    d.text((PAD, 360), "same authority", font=font_sm, fill=FG_DIM)
    d.text((PAD, 385), "same evidence", font=font_sm, fill=FG_DIM)
    d.text((PAD, 410), "same audit", font=font_sm, fill=FG_DIM)

    # Nutrient note
    d.text((PAD, 450), "Nutrient = first provider", font=font_sm, fill=FG_ACCENT)

    return save(img, "S08-feasibility-close")


if __name__ == "__main__":
    render_s01()
    render_s02()
    render_s03()
    render_s04()
    render_s05()
    render_s06()
    render_s07()
    render_s08()
    print("All S01-S08 screenshots rendered.")
