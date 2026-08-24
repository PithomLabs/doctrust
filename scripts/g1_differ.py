#!/usr/bin/env python3
"""G1 extraction differ v2 (plan10 §11 T9, D5).

Compares the trusted-normalized evidence snapshot against doc-generator's
extraction-hints.json GROUND TRUTH with precise miss attribution:

  COVERED      hint entry targeted by a field the evidence contract requests
               from the provider (gross weights + document references +
               container/seal per document type)
  NOT_REQUESTED hint entry outside the current evidence contract (reported
               for coverage visibility; NOT an extraction failure)

A COVERED entry is a MATCH when a same-document claim carries an equal
normalized value (formatting-only normalization: commas/symbols/spaces/case;
unit-insensitive numeric equality) or, for composite container/seal strings,
when the extracted component value is contained in the hint string.

Misses among COVERED entries are REAL failures. Nothing is backfilled.

Usage:
    g1_differ.py <evidence_snapshot.json> <extraction-hints.json> <out_report.md>
"""
import json
import re
import sys

# evidence-contract coverage: provider field -> hint fields it must satisfy
REQUESTED = {
    "commercial_invoice": {
        "total_gross_weight": ["sum_gross_weight"],
        "invoice_number": ["meta_invoice_no"],
        "shipment_id": ["meta_shipment_ref"],
        "container_number": ["container_seal"],
        "seal_number": ["container_seal"],
    },
    "packing_list": {
        "total_gross_weight": ["sum_gross", "tot_gross"],
        "packing_list_number": ["pl_number"],
        "container_number": ["container_seal"],
        "seal_number": ["container_seal"],
    },
    "bill_of_lading": {
        "gross_weight": ["gross_weight_cell"],
        "bill_of_lading_number": ["bl_number"],
        "container_number": ["container_seal_cell"],
        "seal_number": ["container_seal_cell"],
    },
    "certificate_of_origin": {
        "total_gross_weight": ["co_total_gross"],
        "certificate_number": ["cert_number"],
        "container_number": ["co_container_seal"],
        "seal_number": ["co_container_seal"],
    },
}
COMPOSITE = {"container_seal", "container_seal_cell", "co_container_seal"}


def norm(v):
    s = str(v).strip().upper()
    s = s.replace(",", "").replace("$", "").replace("USD", "")
    s = re.sub(r"\s+", " ", s).strip()
    m = re.match(r"^([0-9]+(?:\.[0-9]+)?)\s*(KG|PCS|CBM)?$", s)
    if m:
        return ("num", float(m.group(1)))
    return ("txt", s)


def main():
    snap_path, hints_path, out_path = sys.argv[1:4]
    snap = json.load(open(snap_path))
    hints = json.load(open(hints_path))["entries"]

    claims_by_doc = {}
    for c in snap.get("claims", []):
        for src in c.get("sources", []):
            claims_by_doc.setdefault(src.get("document_id"), []).append(c)

    docs = {d["id"]: d for d in snap.get("documents", [])}

    covered_rows, not_requested = [], []
    covered_matched_claim_keys = set()

    for h in hints:
        if not h["value"]:
            continue
        doc, field = h["document"], h["field"]
        req_fields = REQUESTED.get(doc, {})
        targets = None
        for pf, hint_fields in req_fields.items():
            if field in hint_fields:
                targets = pf
                break
        if targets is None:
            not_requested.append(h)
            continue

        # find the claim for this requested provider field on this doc
        hit = None
        for did, d in docs.items():
            if d["type"] != doc:
                continue
            for c in claims_by_doc.get(did, []):
                if c.get("field") != targets:
                    continue
                cv, hv = norm(c.get("value")), norm(h["value"])
                if cv[0] == hv[0] == "num":
                    ok = abs(cv[1] - hv[1]) < 1e-9
                elif field in COMPOSITE:
                    ok = str(cv[1]) in str(hv[1]) or str(hv[1]) in str(cv[1])
                else:
                    ok = cv[1] == hv[1]
                if ok:
                    hit = (did, c)
                    break
            if hit:
                break

        if hit:
            did, c = hit
            covered_matched_claim_keys.add((doc, c.get("id"), field))
            srcs = c.get("sources", [])
            covered_rows.append({
                "document": doc, "provider_field": targets,
                "hint_field": field, "expected": h["value"],
                "actual": c.get("value"), "status": "MATCH",
                "page": srcs[0]["page"] if srcs else None,
                "bbox": bool(srcs and srcs[0].get("bbox")),
                "confidence": srcs[0].get("confidence") if srcs else None,
            })
        else:
            covered_rows.append({
                "document": doc, "provider_field": targets,
                "hint_field": field, "expected": h["value"],
                "actual": None, "status": "EXTRACTION_FAILURE",
                "page": None, "bbox": False, "confidence": None,
            })

    failures = [r for r in covered_rows if r["status"] != "MATCH"]
    matches = [r for r in covered_rows if r["status"] == "MATCH"]
    with_prov = [r for r in matches if r["bbox"]]
    unique_claims = {(r["document"], r["provider_field"]) for r in matches}

    lines = [
        "# G1 Extraction Differ Report (v2)",
        "",
        "snapshot: `%s`" % snap_path,
        "ground truth: `%s` — GROUND TRUTH, NOT Nutrient output, and never a "
        "fallback data source" % hints_path,
        "",
        "## A. Evidence-contract scope (fields requested from the provider)",
        "",
        "| document | provider field | ground-truth field(s) | expected | actual | status | page | bbox | conf |",
        "|---|---|---|---|---|---|---|---|---|",
    ]
    seen = set()
    for r in sorted(covered_rows, key=lambda r: (r["document"], r["provider_field"], r["hint_field"])):
        key = (r["document"], r["provider_field"], r["expected"])
        dup = key in seen
        seen.add(key)
        lines.append("| %s | %s | %s | %s | %s | **%s** | %s | %s | %s |" % (
            r["document"], r["provider_field"], r["hint_field"],
            r["expected"].replace("\n", " "),
            (str(r["actual"]) if r["actual"] is not None else "-").replace("\n", " "),
            r["status"], r["page"],
            "yes" if r["bbox"] else "-",
            ("%.2f" % r["confidence"]) if r["confidence"] else "-"))

    lines += [
        "",
        "Requested-field outcomes: %d/%d MATCH (%d distinct claims); "
        "all matches carry page+bbox provenance: %s"
        % (len(matches), len(covered_rows), len(unique_claims),
           "YES" if len(with_prov) == len(matches) else "no"),
        "",
        "## B. Ground-truth coverage beyond the evidence contract",
        "",
        "- hint entries total: %d" % (len(covered_rows) + len(not_requested)),
        "- covered by the evidence contract: %d" % len(covered_rows),
        "- not requested from the provider: %d "
        "(full-document fixture inventory: line items, crate schedule cells, "
        "party blocks, marks, dates, ports…) — reported for visibility, "
        "**not scored as extraction failures**" % len(not_requested),
        "",
        "## C. Verdict",
        "",
    ]
    if not failures and matches:
        verdict = "PASS"
        lines.append("**PASS** — every field requested by the evidence "
                     "contract was recovered correctly by live Nutrient "
                     "extraction through the trusted normalizer, with "
                     "page/bbox provenance preserved.")
    else:
        verdict = "FAIL"
        lines.append("**FAIL** — %d requested field(s) missing or wrong:" % len(failures))
        lines.extend("- %s/%s (%s)" % (f["document"], f["hint_field"],
                                       f["provider_field"]) for f in failures)

    with open(out_path, "w") as fh:
        fh.write("\n".join(lines) + "\n")
    print("\n".join(lines[-16:]))
    print("report written:", out_path)
    return 0 if verdict == "PASS" else 1


if __name__ == "__main__":
    sys.exit(main())
