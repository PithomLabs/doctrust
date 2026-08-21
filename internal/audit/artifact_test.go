package audit

import (
	"strings"
	"testing"
)

func TestSetRuleset(t *testing.T) {
	a := NewArtifact("income_verification", "abc123")
	if a.RulesetID != "" {
		t.Fatalf("expected empty RulesetID, got %q", a.RulesetID)
	}

	hashBefore := a.Hash()
	a.SetRuleset("income_verification", "2", "def456")
	if a.RulesetID != "income_verification" {
		t.Errorf("expected RulesetID=income_verification, got %q", a.RulesetID)
	}
	if a.RulesetVersion != "2" {
		t.Errorf("expected RulesetVersion=2, got %q", a.RulesetVersion)
	}
	if a.RulesetHash != "def456" {
		t.Errorf("expected RulesetHash=def456, got %q", a.RulesetHash)
	}

	hashAfter := a.Hash()
	if hashBefore == hashAfter {
		t.Error("expected hash to change after SetRuleset")
	}
}

func TestSetRulesetJSON(t *testing.T) {
	a := NewArtifact("income_verification", "abc123")
	a.SetRuleset("income_verification", "3", "hash789")

	data, err := a.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON: %v", err)
	}

	json := string(data)
	for _, want := range []string{`"ruleset_id"`, `"income_verification"`, `"ruleset_version"`, `"3"`, `"ruleset_hash"`, `"hash789"`} {
		if !strings.Contains(json, want) {
			t.Errorf("JSON missing %s", want)
		}
	}
}

func TestArtifact_Finalize_HashIntegrity(t *testing.T) {
	a := NewArtifact("income_verification", "policy123")
	a.SetRuleset("income_verification", "2", "rulesethash456")
	a.AddDocument(DocumentRecord{FileName: "paystub.pdf", DocType: "paystub", Hash: "abc"})
	a.AddDecision(Decision{
		CaseID: "case1",
		State:  "PASS",
		Findings: []Finding{
			{Rule: "gross_income_consistency", Severity: "INFO"},
		},
	})

	a.Finalize()

	// ArtifactHash from manifest must equal Hash()
	if a.Manifest.ArtifactHash != a.Hash() {
		t.Errorf("ArtifactHash mismatch: manifest=%s, Hash()=%s", a.Manifest.ArtifactHash, a.Hash())
	}

	// Hash must be stable across calls
	h1 := a.Hash()
	h2 := a.Hash()
	if h1 != h2 {
		t.Errorf("Hash() not stable: %s != %s", h1, h2)
	}

	// Hash must change if artifact changes
	a2 := NewArtifact("income_verification", "policy123")
	a2.SetRuleset("income_verification", "2", "rulesethash456")
	a2.AddDocument(DocumentRecord{FileName: "paystub.pdf", DocType: "paystub", Hash: "abc"})
	a2.AddDecision(Decision{
		CaseID: "case1",
		State:  "FAIL",
		Findings: []Finding{
			{Rule: "required_documents", Severity: "BLOCKING"},
		},
	})
	a2.Finalize()
	if a.Hash() == a2.Hash() {
		t.Error("expected different hash for different artifact content")
	}
}
