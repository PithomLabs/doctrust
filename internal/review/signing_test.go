package review

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"testing"
	"time"
)

func testRecord(t *testing.T, priv ed25519.PrivateKey, mutate func(*SignedReview)) SignedReview {
	t.Helper()
	_, pub, err := GenerateReviewerKeyPair()
	_ = pub
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	rec := SignedReview{
		CaseID:           "abc123",
		SnapshotSHA256:   "deadbeef",
		FindingIndex:     1,
		Action:           ActionReject,
		Note:             "HOLD",
		ReviewerIdentity: "reviewer-a",
		Channel:          ChannelHumanTTY,
		KeyID:            "reviewer-a",
		Ruleset:          RuleBinding{ID: "shipment_release", Version: "1", Hash: "d3f1a945"},
	}
	if mutate != nil {
		mutate(&rec)
	}
	if err := SignRecord(priv, &rec, time.Time{}); mutate == nil && err != nil {
		t.Fatalf("sign: %v", err)
	} else if mutate != nil {
		// re-sign after mutation so only verification-time detection matters
		rec2 := rec
		rec2.Signature = nil
		if err := SignRecord(priv, &rec2, time.Time{}); err != nil {
			t.Fatalf("resign: %v", err)
		}
		rec.Signature = rec2.Signature
	}
	return rec
}

func TestSignVerify_Valid(t *testing.T) {
	pub, priv, err := GenerateReviewerKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	rec := SignedReview{CaseID: "c", SnapshotSHA256: "s", FindingIndex: 1,
		Action: ActionReject, ReviewerIdentity: "r", KeyID: "k"}
	if err := SignRecord(priv, &rec, time.Time{}); err != nil {
		t.Fatal(err)
	}
	if rec.Alg != AlgEd25519 || rec.Channel != ChannelHumanTTY || len(rec.Signature) == 0 {
		t.Fatalf("defaults not applied: %+v", rec)
	}
	if err := VerifyRecord(pub, rec); err != nil {
		t.Fatalf("valid record must verify: %v", err)
	}
}

func TestSignVerify_Matrix(t *testing.T) {
	_, goodPriv, _ := GenerateReviewerKeyPair()
	wrongPub, _, _ := GenerateReviewerKeyPair()

	cases := []struct {
		name    string
		mutate  func(*SignedReview)
		wantErr bool
	}{
		{"modified action", func(r *SignedReview) { r.Action = ActionConfirm }, true},
		{"modified identity", func(r *SignedReview) { r.ReviewerIdentity = "someone-else" }, true},
		{"modified note", func(r *SignedReview) { r.Note = "tampered" }, true},
		{"modified timestamp", func(r *SignedReview) { r.ResolvedAt = r.ResolvedAt.Add(time.Minute) }, true},
		{"modified finding index", func(r *SignedReview) { r.FindingIndex = 0 }, true},
		{"modified case id", func(r *SignedReview) { r.CaseID = "other-case" }, true},
		{"modified snapshot hash", func(r *SignedReview) { r.SnapshotSHA256 = "ff00" }, true},
		{"modified ruleset hash", func(r *SignedReview) { r.Ruleset.Hash = "0000" }, true},
	}
	for _, tc := range cases {
		rec := SignedReview{CaseID: "c", SnapshotSHA256: "s", FindingIndex: 1,
			Action: ActionReject, ReviewerIdentity: "r", KeyID: "k"}
		if err := SignRecord(goodPriv, &rec, time.Time{}); err != nil {
			t.Fatalf("%s: sign: %v", tc.name, err)
		}
		tc.mutate(&rec)
		err := VerifyRecord(wrongPub, rec)
		// wrongPub also fails; distinguish by testing against goodPub for
		// content mutations.
		if err := VerifyRecord(pubOf(goodPriv), rec); (err != nil) != tc.wantErr {
			t.Fatalf("%s: wantErr=%v got=%v", tc.name, tc.wantErr, err)
		}
		_ = err
	}
}

func pubOf(priv ed25519.PrivateKey) ed25519.PublicKey {
	return priv.Public().(ed25519.PublicKey)
}

func TestSignVerify_ForgedAndWrongKey(t *testing.T) {
	_, goodPriv, _ := GenerateReviewerKeyPair()
	otherPub, _, _ := GenerateReviewerKeyPair()

	rec := SignedReview{CaseID: "c", SnapshotSHA256: "s", FindingIndex: 1,
		Action: ActionReject, ReviewerIdentity: "r", KeyID: "k"}
	if err := SignRecord(goodPriv, &rec, time.Time{}); err != nil {
		t.Fatal(err)
	}

	// forged signature with random bytes
	forged := rec
	forged.Signature = make([]byte, ed25519.SignatureSize)
	rand.Read(forged.Signature)
	if err := VerifyRecord(pubOf(goodPriv), forged); err == nil {
		t.Fatal("forged signature must fail")
	}

	// valid signature but verified against the WRONG public key
	if err := VerifyRecord(otherPub, rec); err == nil {
		t.Fatal("wrong-key verification must fail")
	}
}

func TestSignVerify_MissingSignature(t *testing.T) {
	pub, priv, _ := GenerateReviewerKeyPair()
	rec := SignedReview{CaseID: "c", SnapshotSHA256: "s", FindingIndex: 1,
		Action: ActionReject, ReviewerIdentity: "r"}
	if err := SignRecord(priv, &rec, time.Time{}); err != nil {
		t.Fatal(err)
	}
	rec.Signature = nil
	if err := VerifyRecord(pub, rec); err == nil {
		t.Fatal("missing signature must fail")
	}
}

func TestCanonicalPayload_Deterministic(t *testing.T) {
	r := SignedReview{CaseID: "c", FindingIndex: 2, Action: ActionOverride}
	b1, _ := r.canonicalPayload()
	b2, _ := r.canonicalPayload()
	if string(b1) != string(b2) {
		t.Fatal("canonical payload must be deterministic")
	}
	var m map[string]any
	if err := json.Unmarshal(b1, &m); err != nil {
		t.Fatal(err)
	}
	if _, has := m["signature"]; has {
		t.Fatal("canonical payload must exclude signature")
	}
}
