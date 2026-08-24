package service

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/doctrust/doctrust/internal/review"
)

// provisionShipmentRuleset creates a minimal promoted ruleset for the test.
func provisionShipmentRuleset(t *testing.T) string {
	t.Helper()
	rsDir := filepath.Join(t.TempDir(), "rulesets", "shipment_release")
	if err := os.MkdirAll(rsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	yaml := "id: shipment_release\nversion: \"1\"\nchecks:\n    - id: required_shipment_documents\n      version: \"1.0\"\n    - id: gross_weight_reconciliation\n      version: \"1.0\"\n"
	if err := os.WriteFile(filepath.Join(rsDir, "v1.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	return filepath.Dir(rsDir)
}

func writeSnapshotFile(t *testing.T, dir string) (string, string) {
	t.Helper()
	snap := `{"case_id":"shipment_test","documents":[{"id":"d1","filename":"a.pdf","hash":"h1","type":"commercial_invoice"},{"id":"d2","filename":"b.pdf","hash":"h2","type":"bill_of_lading"}],"claims":[],"relationships":[],"created_at":"2026-08-24T00:00:00Z"}`
	p := filepath.Join(dir, "evidence_snapshot.json")
	if err := os.WriteFile(p, []byte(snap), 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256Hex([]byte(snap))
	return p, sum
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func TestLoadAuthorizedReviews_FullFlow(t *testing.T) {
	rulesetsDir := provisionShipmentRuleset(t)
	root := t.TempDir()
	svc, err := NewDocTrustService("shipment_release", rulesetsDir)
	if err != nil {
		t.Fatalf("service: %v", err)
	}
	snapPath, snapSHA := writeSnapshotFile(t, root)
	ctx := context.Background()
	if err := svc.LoadCase(ctx, snapPath); err != nil {
		t.Fatal(err)
	}
	d, err := svc.Evaluate(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if d.Status != "REVIEW" {
		t.Fatalf("expected REVIEW on partial snapshot, got %s", d.Status)
	}
	badIdx := len(d.Results) // out of range

	pub, priv, err := review.GenerateReviewerKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	ring := map[string]ed25519.PublicKey{"reviewer-a": pub}

	rec := review.SignedReview{
		CaseID: svc.GetCaseID(), SnapshotSHA256: snapSHA,
		FindingIndex: badIdx, Action: review.ActionConfirm,
		ReviewerIdentity: "reviewer-a", Channel: review.ChannelHumanTTY,
		KeyID:   "reviewer-a",
		Ruleset: review.RuleBinding{ID: "shipment_release", Version: "1", Hash: svc.GetRulesetHash()},
	}
	if err := review.SignRecord(priv, &rec, time.Time{}); err != nil {
		t.Fatal(err)
	}

	// A4: out-of-range index rejected before signature even matters
	if err := svc.LoadAuthorizedReviews([]review.SignedReview{rec}, ring); err == nil {
		t.Fatal("out-of-range finding index must be rejected")
	} else if !strings.Contains(err.Error(), "out of range") {
		t.Fatalf("unexpected error: %v", err)
	}

	// valid record for an in-range finding
	rec.FindingIndex = 0
	rec.ResolvedAt = time.Time{}
	rec.Signature = nil
	if err := review.SignRecord(priv, &rec, time.Time{}); err != nil {
		t.Fatal(err)
	}
	// resolve BOTH findings via signed human consent (confirm-style)
	var allRecords []review.SignedReview
	allRecords = append(allRecords, rec)
	for idx := range d.Results {
		if idx == rec.FindingIndex {
			continue
		}
		extra := review.SignedReview{
			CaseID: svc.GetCaseID(), SnapshotSHA256: snapSHA,
			FindingIndex: idx, Action: review.ActionConfirm,
			ReviewerIdentity: "reviewer-a", Channel: review.ChannelHumanTTY,
			KeyID:   "reviewer-a",
			Ruleset: rec.Ruleset,
		}
		if err := review.SignRecord(priv, &extra, time.Time{}); err != nil {
			t.Fatal(err)
		}
		allRecords = append(allRecords, extra)
	}
	if err := svc.LoadAuthorizedReviews(allRecords, ring); err != nil {
		t.Fatalf("valid signed records must load: %v", err)
	}
	revs, _ := svc.GetReviews()
	if len(revs) != 2 {
		t.Fatalf("expected 2 merged reviews, got %d", len(revs))
	}
	for _, r := range revs {
		if r.ReviewerIdentity != "reviewer-a" || r.Channel != review.ChannelHumanTTY {
			t.Fatalf("merged review missing identity/channel: %+v", r)
		}
	}

	artifact, err := svc.BuildArtifact()
	if err != nil {
		t.Fatal(err)
	}
	if artifact.FinalDisposition != "PASS" {
		t.Fatalf("confirm-style resolution should seal PASS on this single-finding fixture; got %s", artifact.FinalDisposition)
	}
	if len(artifact.HumanReviews) != 2 || artifact.HumanReviews[0].ReviewerIdentity != "reviewer-a" {
		t.Fatalf("artifact must carry the human identities")
	}
	if artifact.Manifest.ArtifactHash != artifact.Hash() {
		t.Fatal("artifact hash integrity broken")
	}

	// A2/A3: tampered action and cross-case reuse must fail closed
	rec.Action = review.ActionOverride // differs from signed confirm
	if err := svc.LoadAuthorizedReviews([]review.SignedReview{rec}, ring); err == nil {
		t.Fatal("tampered record must be rejected")
	}
	foreign := rec
	foreign.CaseID = "different-case-graph"
	foreign.GraphCaseID = "different-case-graph"
	foreign.ResolvedAt = time.Time{}
	foreign.Signature = nil
	if err := review.SignRecord(priv, &foreign, time.Time{}); err != nil {
		t.Fatal(err)
	}
	if err := svc.LoadAuthorizedReviews([]review.SignedReview{foreign}, ring); err == nil {
		t.Fatal("record bound to a different case must be rejected")
	}
}
