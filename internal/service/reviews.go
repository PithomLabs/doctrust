package service

// Phase-6 human-authority support (plans12 P6-3/P6-7): decision sidecar
// construction and fail-closed merging of Ed25519-signed human review
// records produced exclusively by the human TTY channel.

import (
	"crypto/ed25519"
	"fmt"

	"github.com/PithomLabs/doctrust/internal/review"
	"github.com/PithomLabs/doctrust/internal/types"
)

// DecisionSidecar is the machine-readable decision context the human review
// channel consumes. Written by doctrust-mcp after evaluate_case.
type DecisionSidecar struct {
	LoadcaseID     string             `json:"loadcase_id"`
	GraphCaseID    string             `json:"graph_case_id,omitempty"`
	SnapshotSHA256 string             `json:"snapshot_sha256"`
	Status         string             `json:"status"`
	Findings       []SidecarFinding   `json:"findings"`
	Ruleset        review.RuleBinding `json:"ruleset"`
}

type SidecarFinding struct {
	Index    int                    `json:"index"`
	CheckID  string                 `json:"check_id"`
	Status   string                 `json:"status"`
	Severity string                 `json:"severity"`
	Reason   string                 `json:"reason"`
	Metrics  map[string]interface{} `json:"metrics,omitempty"`
	Evidence []types.EvidenceRef    `json:"evidence,omitempty"`
}

func (s *DocTrustService) GetRulesetHash() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rulesetHash
}

func (s *DocTrustService) GetSnapshotSHA256() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.case_ == nil {
		return ""
	}
	return s.case_.policyHash // sha256 of raw snapshot bytes (full hex)
}

func (s *DocTrustService) GetSnapshotPath() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.case_ == nil {
		return ""
	}
	return s.case_.snapshotPath
}

func (s *DocTrustService) GetGraphCaseID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.case_ == nil {
		return ""
	}
	return s.case_.snapshot.CaseID
}

// BuildDecisionSidecar constructs the decision sidecar from the pinned,
// evaluated case.
func (s *DocTrustService) BuildDecisionSidecar() (*DecisionSidecar, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.case_ == nil {
		return nil, fmt.Errorf("no case loaded")
	}
	if s.case_.decision == nil {
		return nil, fmt.Errorf("case not evaluated")
	}
	sc := &DecisionSidecar{
		LoadcaseID:     s.case_.id,
		GraphCaseID:    s.case_.snapshot.CaseID,
		SnapshotSHA256: s.case_.policyHash,
		Status:         string(s.case_.decision.Status),
		Ruleset: review.RuleBinding{
			ID:      s.ruleset.ID,
			Version: s.ruleset.Version,
			Hash:    s.rulesetHash,
		},
	}
	for i, r := range s.case_.decision.Results {
		sc.Findings = append(sc.Findings, SidecarFinding{
			Index:    i,
			CheckID:  r.CheckID,
			Status:   string(r.Status),
			Severity: string(r.Severity),
			Reason:   r.Reason,
			Metrics:  r.Metrics,
			Evidence: r.Evidence,
		})
	}
	return sc, nil
}

// LoadAuthorizedReviews merges externally produced (human TTY channel)
// SIGNED review records into the pinned case's review store after verifying:
//   - the records bind THIS case (loadcase id + snapshot hash),
//   - the Ruleset binding matches the pinned promoted Ruleset hash,
//   - finding indexes are valid against the pinned decision,
//   - every signature verifies against the supplied reviewers ring.
//
// Any failure is an error and leaves the review store untouched (fail-closed).
// This is the ONLY path by which HumanReviewRecords enter a case outside the
// legacy in-process request path.
func (s *DocTrustService) LoadAuthorizedReviews(
	records []review.SignedReview,
	ring map[string]ed25519.PublicKey,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.case_ == nil {
		return fmt.Errorf("no case loaded")
	}
	if s.case_.decision == nil {
		return fmt.Errorf("case not evaluated")
	}
	snapshotSHA := s.case_.policyHash
	expectedBinding := review.RuleBinding{
		ID:      s.ruleset.ID,
		Version: s.ruleset.Version,
		Hash:    s.rulesetHash,
	}

	for _, rec := range records {
		if rec.CaseID != s.case_.id && rec.GraphCaseID != s.case_.snapshot.CaseID {
			return fmt.Errorf("review record bound to different case (%s/%s)",
				rec.CaseID, rec.GraphCaseID)
		}
		if rec.SnapshotSHA256 != "" && rec.SnapshotSHA256 != snapshotSHA {
			return fmt.Errorf("review record snapshot hash mismatch")
		}
		if rec.Ruleset != expectedBinding {
			return fmt.Errorf("review record ruleset binding mismatch (%s v%s %s)",
				rec.Ruleset.ID, rec.Ruleset.Version, rec.Ruleset.Hash)
		}
		if rec.ReviewerIdentity != rec.KeyID {
			return fmt.Errorf("reviewer identity %q does not match signing key %q",
				rec.ReviewerIdentity, rec.KeyID)
		}
		if rec.FindingIndex < 0 || rec.FindingIndex >= len(s.case_.decision.Results) {
			return fmt.Errorf("review record finding_index %d out of range",
				rec.FindingIndex)
		}
		pub, ok := ring[rec.KeyID]
		if !ok {
			return fmt.Errorf("unknown review key_id %q", rec.KeyID)
		}
		if err := review.VerifyRecord(pub, rec); err != nil {
			return fmt.Errorf("signature verification failed: %w", err)
		}
	}

	for _, rec := range records {
		s.case_.reviewStore.AddReview(&review.HumanReview{
			FindingIndex:     rec.FindingIndex,
			Action:           rec.Action,
			Note:             rec.Note,
			ReviewerIdentity: rec.ReviewerIdentity,
			Channel:          rec.Channel,
			KeyID:            rec.KeyID,
		})
	}
	return nil
}
