package doctrust.policy

import rego.v1

# Pass: all docs present, variance under 5%
test_pass_within_threshold if {
	result == {"decision": "PASS", "findings": []} with input as {
		"documents": [
			{"id": "d1", "type": "paystub", "filename": "paystub.pdf"},
			{"id": "d2", "type": "w2", "filename": "w2.pdf"},
			{"id": "d3", "type": "form_1040", "filename": "1040.pdf"},
		],
		"claims": [
			{"id": "c1", "semantic_type": "gross_income_projected", "value": 105000, "sources": [{"document_id": "d1", "confidence": 0.95}]},
			{"id": "c2", "semantic_type": "gross_income_taxable", "value": 100000, "sources": [{"document_id": "d2", "confidence": 0.95}]},
			{"id": "c3", "semantic_type": "gross_income_taxable", "value": 100000, "sources": [{"document_id": "d3", "confidence": 0.95}]},
		],
	}
}

# Conflict: 15% variance triggers REVIEW
test_conflict_with_bonus if {
	result.decision == "REVIEW" with input as {
		"documents": [
			{"id": "d1", "type": "paystub", "filename": "paystub.pdf"},
			{"id": "d2", "type": "w2", "filename": "w2.pdf"},
			{"id": "d3", "type": "form_1040", "filename": "1040.pdf"},
		],
		"claims": [
			{"id": "c1", "semantic_type": "gross_income_projected", "value": 115000, "sources": [{"document_id": "d1", "confidence": 0.95}]},
			{"id": "c2", "semantic_type": "gross_income_taxable", "value": 100000, "sources": [{"document_id": "d2", "confidence": 0.95}]},
			{"id": "c3", "semantic_type": "gross_income_taxable", "value": 100000, "sources": [{"document_id": "d3", "confidence": 0.95}]},
		],
	}
}

# Missing evidence: W-2 absent
test_missing_w2 if {
	result.decision == "MISSING_EVIDENCE" with input as {
		"documents": [
			{"id": "d1", "type": "paystub", "filename": "paystub.pdf"},
			{"id": "d3", "type": "form_1040", "filename": "1040.pdf"},
		],
		"claims": [
			{"id": "c1", "semantic_type": "gross_income_projected", "value": 105000, "sources": [{"document_id": "d1", "confidence": 0.95}]},
			{"id": "c3", "semantic_type": "gross_income_taxable", "value": 100000, "sources": [{"document_id": "d3", "confidence": 0.95}]},
		],
	}
}

# Zero-wage: W-2 wages = 0 triggers REVIEW (undefined variance)
test_zero_denominator if {
	result.decision == "REVIEW" with input as {
		"documents": [
			{"id": "d1", "type": "paystub", "filename": "paystub.pdf"},
			{"id": "d2", "type": "w2", "filename": "w2.pdf"},
			{"id": "d3", "type": "form_1040", "filename": "1040.pdf"},
		],
		"claims": [
			{"id": "c1", "semantic_type": "gross_income_projected", "value": 50000, "sources": [{"document_id": "d1", "confidence": 0.95}]},
			{"id": "c2", "semantic_type": "gross_income_taxable", "value": 0, "sources": [{"document_id": "d2", "confidence": 0.95}]},
			{"id": "c3", "semantic_type": "gross_income_taxable", "value": 0, "sources": [{"document_id": "d3", "confidence": 0.95}]},
		],
	}
}

# Low confidence source triggers REVIEW
test_low_confidence if {
	result.decision == "REVIEW" with input as {
		"documents": [
			{"id": "d1", "type": "paystub", "filename": "paystub.pdf"},
			{"id": "d2", "type": "w2", "filename": "w2.pdf"},
			{"id": "d3", "type": "form_1040", "filename": "1040.pdf"},
		],
		"claims": [
			{"id": "c1", "semantic_type": "gross_income_projected", "value": 105000, "sources": [{"document_id": "d1", "confidence": 0.95}]},
			{"id": "c2", "semantic_type": "gross_income_taxable", "value": 100000, "sources": [{"document_id": "d2", "confidence": 0.70}]},
			{"id": "c3", "semantic_type": "gross_income_taxable", "value": 100000, "sources": [{"document_id": "d3", "confidence": 0.95}]},
		],
	}
}
