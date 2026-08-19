package doctrust.policy

import rego.v1

# Decision
default result := {"decision": "PASS", "findings": []}

result := {"decision": "MISSING_EVIDENCE", "findings": findings} if {
	count(missing_required) > 0
	findings = [f | some f in missing_required]
}

result := {"decision": "FAIL", "findings": findings} if {
	count(missing_required) == 0
	count(rule_violations) > 0
	findings = [f | some f in rule_violations]
}

result := {"decision": "REVIEW", "findings": findings} if {
	count(missing_required) == 0
	count(rule_violations) == 0
	count(review_triggers) > 0
	findings = [f | some f in review_triggers]
}

# --- Missing required evidence ---
missing_required contains {
	"rule": "required_evidence",
	"severity": "high",
	"reason": sprintf("Missing required document type: %s", [doc_type]),
	"sources": [],
} if {
	provided = {d.type | some d in input.documents}
	some doc_type in {"paystub", "w2", "form_1040"}
	not doc_type in provided
}

# --- Rule violations ---
rule_violations contains {
	"rule": "w2_1040_corroboration",
	"severity": "high",
	"claim_a": "w2.wages_tips_other_compensation",
	"claim_b": "form_1040.line1z_wages",
	"reason": "W-2 and Form 1040 wages must match",
} if {
	some w in input.claims
	w.semantic_type == "gross_income_taxable"
	some ws in w.sources
	some wd in input.documents
	ws.document_id == wd.id
	wd.type == "w2"

	some f in input.claims
	f.semantic_type == "gross_income_taxable"
	some fs in f.sources
	some fd in input.documents
	fs.document_id == fd.id
	fd.type == "form_1040"

	w.value != f.value
}

# --- Review triggers ---
review_triggers contains {
	"rule": "zero_denominator",
	"severity": "high",
	"reason": "W-2 wages are zero; variance percentage is undefined",
} if {
	some w in input.claims
	w.semantic_type == "gross_income_taxable"
	w.value == 0
}

review_triggers contains {
	"rule": "paystub_variance",
	"severity": "medium",
	"claim_a": "paystub.annualized_gross_ytd",
	"claim_b": "w2.wages_tips_other_compensation",
} if {
	some p in input.claims
	p.semantic_type == "gross_income_projected"
	some ps in p.sources
	some pd in input.documents
	ps.document_id == pd.id
	pd.type == "paystub"

	some w in input.claims
	w.semantic_type == "gross_income_taxable"
	some ws in w.sources
	some wd in input.documents
	ws.document_id == wd.id
	wd.type == "w2"

	w.value != 0
	p.value != w.value
	((abs(p.value - w.value)) / w.value) * 100 > 5
}

review_triggers contains {
	"rule": "low_confidence_source",
	"severity": "medium",
} if {
	some c in input.claims
	some s in c.sources
	s.confidence < 0.8
}

# --- Helpers ---
default get_bonus_value := 0
get_bonus_value := v if {
	some c in input.claims
	c.semantic_type == "bonus_compensation"
	v = c.value
}

abs(x) = x if x >= 0
abs(x) = (-1 * x) if x < 0
