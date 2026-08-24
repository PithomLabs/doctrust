# G1 Extraction Differ Report (v2)

snapshot: `g1/evidence_snapshot.json`
ground truth: `/home/chaschel/Desktop/biz/nutrient/doc-generator/examples/shipment-1047/generated/extraction-hints.json` — GROUND TRUTH, NOT Nutrient output, and never a fallback data source

## A. Evidence-contract scope (fields requested from the provider)

| document | provider field | ground-truth field(s) | expected | actual | status | page | bbox | conf |
|---|---|---|---|---|---|---|---|---|
| bill_of_lading | bill_of_lading_number | bl_number | MSL-620984110 | MSL-620984110 | **MATCH** | 1 | yes | 0.95 |
| bill_of_lading | container_number | container_seal_cell | TCKU-918234-0 SEAL: 850474 | TCKU-918234-0 | **MATCH** | 1 | yes | 0.95 |
| bill_of_lading | gross_weight | gross_weight_cell | 5,150.00 KG | 5150 | **MATCH** | 1 | yes | 0.95 |
| certificate_of_origin | certificate_number | cert_number | CO-PH-2026-08912 | CO-PH-2026-08912 | **MATCH** | 1 | yes | 0.95 |
| certificate_of_origin | container_number | co_container_seal | TCKU-918234-0 / SEAL 850474 | TCKU-918234-0 | **MATCH** | 1 | yes | 0.95 |
| certificate_of_origin | total_gross_weight | co_total_gross | 4,650.00 KG | 4650 | **MATCH** | 1 | yes | 0.95 |
| commercial_invoice | container_number | container_seal | TCKU-918234-0 SEAL: 850474 | TCKU-918234-0 | **MATCH** | 1 | yes | 0.95 |
| commercial_invoice | invoice_number | meta_invoice_no | INV-2026-1047 | INV-2026-1047 | **MATCH** | 1 | yes | 0.95 |
| commercial_invoice | shipment_id | meta_shipment_ref | PH-EXP-1047 | PH-EXP-1047 | **MATCH** | 1 | yes | 0.95 |
| commercial_invoice | total_gross_weight | sum_gross_weight | 4,650.00 KG | 4650 | **MATCH** | 1 | yes | 0.95 |
| packing_list | container_number | container_seal | TCKU-918234-0 SEAL: 850474 TYPE 40 GP - SOC | TCKU-918234-0 | **MATCH** | 1 | yes | 0.95 |
| packing_list | packing_list_number | pl_number | PKL-2026-1047 | PKL-2026-1047 | **MATCH** | 1 | yes | 0.95 |
| packing_list | total_gross_weight | sum_gross | 4,650.00 KG | 4650 | **MATCH** | 2 | yes | 0.95 |
| packing_list | total_gross_weight | tot_gross | 4,650.00 | 4650 | **MATCH** | 2 | yes | 0.95 |

Requested-field outcomes: 14/14 MATCH (13 distinct claims); all matches carry page+bbox provenance: YES

## B. Ground-truth coverage beyond the evidence contract

- hint entries total: 285
- covered by the evidence contract: 14
- not requested from the provider: 271 (full-document fixture inventory: line items, crate schedule cells, party blocks, marks, dates, ports…) — reported for visibility, **not scored as extraction failures**

## C. Verdict

**PASS** — every field requested by the evidence contract was recovered correctly by live Nutrient extraction through the trusted normalizer, with page/bbox provenance preserved.
