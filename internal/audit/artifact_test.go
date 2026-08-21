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
