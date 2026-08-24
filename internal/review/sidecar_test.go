package review

import (
	"crypto/ed25519"
	"path/filepath"
	"testing"
	"time"
)

func TestEncryptedKeyFile_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "reviewer-a.key.enc")
	pub, priv, err := GenerateReviewerKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	gotPub, err := SaveEncryptedPrivateKey(path, "reviewer-a", priv, "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if string(gotPub) != string(pub) {
		t.Fatal("published public key mismatch")
	}

	keyID, ringPub, ringPriv, err := LoadEncryptedPrivateKey(path, "correct horse battery staple")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if keyID != "reviewer-a" || string(ringPub) != string(pub) || string(ringPriv) != string(priv) {
		t.Fatal("round trip mismatch")
	}

	if _, _, _, err := LoadEncryptedPrivateKey(path, "wrong-passphrase"); err == nil {
		t.Fatal("wrong passphrase must fail closed")
	}
}

func TestSidecar_AppendLoadVerify(t *testing.T) {
	dir := t.TempDir()
	sidecar := filepath.Join(dir, "case.doctrust_reviews.json")

	_, privA, _ := GenerateReviewerKeyPair()
	pubB, privB, _ := GenerateReviewerKeyPair()
	ring := map[string]ed25519.PublicKey{
		"reviewer-a": pubOf(privA),
		"reviewer-b": pubB,
	}
	_ = privB

	mk := func(t *testing.T, keyID string, priv ed25519.PrivateKey, idx int,
		action FindingAction) SignedReview {
		rec := SignedReview{CaseID: "case1", SnapshotSHA256: "s1", FindingIndex: idx,
			Action: action, ReviewerIdentity: keyID, KeyID: keyID}
		if err := SignRecord(priv, &rec, time.Time{}); err != nil {
			t.Fatal(err)
		}
		return rec
	}

	r1 := mk(t, "reviewer-a", privA, 1, ActionReject)
	if err := AppendSignedRecord(sidecar, "case1", r1); err != nil {
		t.Fatal(err)
	}
	r2 := mk(t, "reviewer-b", privB, 0, ActionOverride)
	if err := AppendSignedRecord(sidecar, "case1", r2); err != nil {
		t.Fatal(err)
	}

	sc, err := LoadReviewsSidecar(sidecar)
	if err != nil {
		t.Fatal(err)
	}
	if len(sc.Records) != 2 || sc.CaseID != "case1" {
		t.Fatalf("unexpected sidecar: %+v", sc)
	}
	// deterministic order: finding index ascending
	if sc.Records[0].FindingIndex != 0 {
		t.Fatalf("records not sorted: %d before %d", sc.Records[0].FindingIndex, sc.Records[1].FindingIndex)
	}
	for _, rec := range sc.Records {
		if err := VerifyAgainstRing(rec, ring); err != nil {
			t.Fatalf("record %d failed verification: %v", rec.FindingIndex, err)
		}
	}

	// tampered record fails against the ring
	bad := sc.Records[0]
	bad.Action = ActionConfirm
	if err := VerifyAgainstRing(bad, ring); err == nil {
		t.Fatal("tampered record must fail verification")
	}
	// unknown key id fails
	unknown := sc.Records[0]
	unknown.KeyID = "ghost"
	if err := VerifyAgainstRing(unknown, ring); err == nil {
		t.Fatal("unknown key_id must fail")
	}
}
