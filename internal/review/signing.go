package review

// Ed25519-signed human-review records (plans12 P6-3).
//
// Authority split: the human TTY channel holds the private signing key
// (passphrase-encrypted at rest); DocTrust holds only public keys from the
// provisioned reviewers ring. Signatures cover a canonical payload that binds
// case, snapshot, finding, action, identity, note, timestamp, and ruleset —
// copying a record to another case/finding or changing the Ruleset breaks
// verification. Verification happens whenever records are loaded/used;
// missing, forged, wrong-key, or content-mismatched signatures fail closed.

import (
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"
)

const (
	AlgEd25519      = "ed25519"
	ChannelHumanTTY = "human-tty"
)

// RuleBinding pins the Ruleset identity a review was recorded against.
type RuleBinding struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	Hash    string `json:"hash"`
}

// SignedReview is one authoritative human decision, bound to the exact case,
// snapshot, and Ruleset it resolved.
type SignedReview struct {
	CaseID           string        `json:"case_id"` // LoadCase id (sha256 of raw snapshot bytes, first 16 hex)
	GraphCaseID      string        `json:"graph_case_id,omitempty"`
	SnapshotSHA256   string        `json:"snapshot_sha256"`
	FindingIndex     int           `json:"finding_index"`
	Action           FindingAction `json:"action"`
	Note             string        `json:"note"`
	ReviewerIdentity string        `json:"reviewer_identity"`
	Channel          string        `json:"channel"`
	KeyID            string        `json:"key_id"`
	Alg              string        `json:"alg"`
	Ruleset          RuleBinding   `json:"ruleset"`
	ResolvedAt       time.Time     `json:"resolved_at"`
	Signature        []byte        `json:"signature"`
}

// canonicalPayload is the deterministic serialization covered by the
// signature. Field order follows the struct definition; the Signature field
// is excluded.
type canonicalPayload struct {
	CaseID           string        `json:"case_id"`
	GraphCaseID      string        `json:"graph_case_id,omitempty"`
	SnapshotSHA256   string        `json:"snapshot_sha256"`
	FindingIndex     int           `json:"finding_index"`
	Action           FindingAction `json:"action"`
	Note             string        `json:"note"`
	ReviewerIdentity string        `json:"reviewer_identity"`
	Channel          string        `json:"channel"`
	KeyID            string        `json:"key_id"`
	Alg              string        `json:"alg"`
	Ruleset          RuleBinding   `json:"ruleset"`
	ResolvedAt       time.Time     `json:"resolved_at"`
}

func (r SignedReview) canonicalPayload() ([]byte, error) {
	return json.Marshal(canonicalPayload{
		CaseID:           r.CaseID,
		GraphCaseID:      r.GraphCaseID,
		SnapshotSHA256:   r.SnapshotSHA256,
		FindingIndex:     r.FindingIndex,
		Action:           r.Action,
		Note:             r.Note,
		ReviewerIdentity: r.ReviewerIdentity,
		Channel:          r.Channel,
		KeyID:            r.KeyID,
		Alg:              r.Alg,
		Ruleset:          r.Ruleset,
		ResolvedAt:       r.ResolvedAt,
	})
}

// SignRecord finalizes Alg/ResolvedAt defaults and signs the canonical
// payload in place.
func SignRecord(priv ed25519.PrivateKey, r *SignedReview, resolvedAt time.Time) error {
	if len(priv) != ed25519.PrivateKeySize {
		return fmt.Errorf("invalid ed25519 private key size %d", len(priv))
	}
	if r.Alg == "" {
		r.Alg = AlgEd25519
	}
	if r.Alg != AlgEd25519 {
		return fmt.Errorf("unsupported signature alg %q", r.Alg)
	}
	if r.Channel == "" {
		r.Channel = ChannelHumanTTY
	}
	if resolvedAt.IsZero() {
		resolvedAt = time.Now().UTC()
	}
	r.ResolvedAt = resolvedAt.UTC()
	r.Signature = nil
	b, err := r.canonicalPayload()
	if err != nil {
		return fmt.Errorf("canonical payload: %w", err)
	}
	r.Signature = ed25519.Sign(priv, b)
	return nil
}

// VerifyRecord checks the signature against the public key registered for the
// record's key_id.
func VerifyRecord(pub ed25519.PublicKey, r SignedReview) error {
	if len(pub) != ed25519.PublicKeySize {
		return fmt.Errorf("invalid ed25519 public key size %d", len(pub))
	}
	if r.Alg != AlgEd25519 {
		return fmt.Errorf("unsupported signature alg %q", r.Alg)
	}
	sig := r.Signature
	cp := r
	cp.Signature = nil
	b, err := cp.canonicalPayload()
	if err != nil {
		return fmt.Errorf("canonical payload: %w", err)
	}
	if !ed25519.Verify(pub, b, sig) {
		return fmt.Errorf("signature verification failed for finding %d (%s)",
			r.FindingIndex, r.Action)
	}
	return nil
}

// ReviewsSidecar is the persisted append-only collection of signed records
// for one case, stored next to the snapshot.
type ReviewsSidecar struct {
	CaseID  string         `json:"case_id"`
	Records []SignedReview `json:"records"`
}

// SortRecords orders records deterministically before writing.
func SortRecords(recs []SignedReview) {
	sort.Slice(recs, func(i, j int) bool {
		if recs[i].FindingIndex != recs[j].FindingIndex {
			return recs[i].FindingIndex < recs[j].FindingIndex
		}
		if !recs[i].ResolvedAt.Equal(recs[j].ResolvedAt) {
			return recs[i].ResolvedAt.Before(recs[j].ResolvedAt)
		}
		return recs[i].ReviewerIdentity < recs[j].ReviewerIdentity
	})
}

// LoadReviewsSidecar reads a reviews sidecar from disk.
func LoadReviewsSidecar(path string) (*ReviewsSidecar, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var sc ReviewsSidecar
	if err := json.Unmarshal(data, &sc); err != nil {
		return nil, fmt.Errorf("parse reviews sidecar %s: %w", path, err)
	}
	return &sc, nil
}

// AppendSignedRecord loads (or creates) the sidecar, appends the record,
// re-sorts deterministically, and writes it back. Returns an error if the
// same finding_index already carries a signed record by the same key_id
// (exactly-one authoritative consent per finding per reviewer).
func AppendSignedRecord(path, caseID string, rec SignedReview) error {
	sc := &ReviewsSidecar{CaseID: caseID}
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, sc); err != nil {
			return fmt.Errorf("parse existing sidecar: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	for _, existing := range sc.Records {
		if existing.FindingIndex == rec.FindingIndex && existing.KeyID == rec.KeyID {
			return fmt.Errorf("finding %d already reviewed by %s", rec.FindingIndex, rec.KeyID)
		}
	}
	sc.CaseID = caseID
	sc.Records = append(sc.Records, rec)
	SortRecords(sc.Records)
	data, err := json.MarshalIndent(sc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// VerifyAgainstRing verifies one record against the provisioned reviewers
// ring (key_id -> public key). Unknown key ids fail closed.
func VerifyAgainstRing(rec SignedReview, ring map[string]ed25519.PublicKey) error {
	pub, ok := ring[rec.KeyID]
	if !ok {
		return fmt.Errorf("unknown review key_id %q", rec.KeyID)
	}
	return VerifyRecord(pub, rec)
}
