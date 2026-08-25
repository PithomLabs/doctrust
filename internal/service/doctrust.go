package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"github.com/PithomLabs/doctrust/internal/audit"
	"github.com/PithomLabs/doctrust/internal/eval"
	"github.com/PithomLabs/doctrust/internal/evidence"
	"github.com/PithomLabs/doctrust/internal/facts"
	"github.com/PithomLabs/doctrust/internal/review"
	"github.com/PithomLabs/doctrust/internal/types"
)

type caseState struct {
	id            string
	snapshotPath  string
	snapshotBytes []byte
	snapshot      *evidence.EvidenceGraph
	facts         facts.Facts
	ruleset       eval.Ruleset
	rulesetHash   string
	policyHash    string
	decision      *eval.Decision
	reviewStore   *review.ReviewStore
}

type RulesetInfo struct {
	ID      string          `json:"id"`
	Version string          `json:"version"`
	Checks  []eval.CheckRef `json:"checks"`
}

type DocTrustService struct {
	mu          sync.Mutex
	domain      string
	runner      *eval.Runner
	ruleset     eval.Ruleset
	rulesetHash string
	case_       *caseState
}

func NewDocTrustService(domain, rulesetsDir string) (*DocTrustService, error) {
	registry := eval.NewRegistry(rulesetsDir)
	rs, err := registry.LoadPromoted(domain)
	if err != nil {
		return nil, fmt.Errorf("no promoted ruleset for domain %s: %w", domain, err)
	}
	rHash, err := rs.ComputeHash()
	if err != nil {
		return nil, fmt.Errorf("compute ruleset hash: %w", err)
	}
	runner := eval.NewRunner(eval.DefaultRegistry().All())
	return &DocTrustService{
		domain:      domain,
		runner:      runner,
		ruleset:     rs,
		rulesetHash: rHash,
	}, nil
}

func (s *DocTrustService) LoadCase(ctx context.Context, snapshotPath string) error {
	snapshotBytes, err := os.ReadFile(snapshotPath)
	if err != nil {
		return fmt.Errorf("read snapshot: %w", err)
	}
	var snapshot evidence.EvidenceGraph
	if err := json.Unmarshal(snapshotBytes, &snapshot); err != nil {
		return fmt.Errorf("invalid snapshot: %w", err)
	}
	snapshotHash := fmt.Sprintf("%x", sha256.Sum256(snapshotBytes))
	newCase := &caseState{
		id:            snapshotHash[:16],
		snapshotPath:  snapshotPath,
		snapshotBytes: snapshotBytes,
		snapshot:      &snapshot,
		reviewStore:   review.NewReviewStore(),
		policyHash:    snapshotHash,
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.case_ = newCase
	return nil
}

func (s *DocTrustService) Evaluate(ctx context.Context) (*eval.Decision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.case_ == nil {
		return nil, fmt.Errorf("no case loaded")
	}
	f, err := BuildFactsFromSnapshot(s.case_.snapshot)
	if err != nil {
		return nil, fmt.Errorf("build facts: %w", err)
	}
	s.case_.facts = f
	s.case_.ruleset = s.ruleset
	s.case_.rulesetHash = s.rulesetHash
	decision, err := s.runner.Evaluate(ctx, s.ruleset, f)
	if err != nil {
		return nil, fmt.Errorf("evaluate: %w", err)
	}
	s.case_.decision = &decision
	return &decision, nil
}

func (s *DocTrustService) GetDecision() *eval.Decision {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.case_ == nil {
		return nil
	}
	return s.case_.decision
}

func (s *DocTrustService) GetCaseID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.case_ == nil {
		return ""
	}
	return s.case_.id
}

func (s *DocTrustService) GetFinding(index int) (*eval.Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.case_ == nil {
		return nil, fmt.Errorf("no case loaded")
	}
	if s.case_.decision == nil {
		return nil, fmt.Errorf("case not evaluated")
	}
	if index < 0 || index >= len(s.case_.decision.Results) {
		return nil, fmt.Errorf("finding_index %d out of range (0-%d)", index, len(s.case_.decision.Results)-1)
	}
	return &s.case_.decision.Results[index], nil
}

func (s *DocTrustService) GetEvidence(findingIndex int) ([]types.EvidenceRef, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.case_ == nil {
		return nil, fmt.Errorf("no case loaded")
	}
	if s.case_.decision == nil {
		return nil, fmt.Errorf("case not evaluated")
	}
	if findingIndex < 0 || findingIndex >= len(s.case_.decision.Results) {
		return nil, fmt.Errorf("finding_index %d out of range (0-%d)", findingIndex, len(s.case_.decision.Results)-1)
	}
	return s.case_.decision.Results[findingIndex].Evidence, nil
}

func (s *DocTrustService) GetRuleset() RulesetInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	return RulesetInfo{
		ID:      s.ruleset.ID,
		Version: s.ruleset.Version,
		Checks:  s.ruleset.Checks,
	}
}

func (s *DocTrustService) GetReviews() ([]*review.HumanReview, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.case_ == nil {
		return nil, fmt.Errorf("no case loaded")
	}
	return s.case_.reviewStore.GetAll(), nil
}

func (s *DocTrustService) RequestHumanReview(findingIndex int, action review.FindingAction, note string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.case_ == nil {
		return "", fmt.Errorf("no case loaded")
	}
	if s.case_.decision == nil {
		return "", fmt.Errorf("case not evaluated")
	}
	if findingIndex < 0 || findingIndex >= len(s.case_.decision.Results) {
		return "", fmt.Errorf("finding_index %d out of range (0-%d)", findingIndex, len(s.case_.decision.Results)-1)
	}
	s.case_.reviewStore.AddReview(&review.HumanReview{
		FindingIndex: findingIndex,
		Action:       action,
		Note:         note,
	})
	return fmt.Sprintf("review_%d", findingIndex), nil
}

func (s *DocTrustService) BuildArtifact() (*audit.Artifact, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.case_ == nil {
		return nil, fmt.Errorf("no case loaded")
	}
	if s.case_.decision == nil {
		return nil, fmt.Errorf("case not evaluated")
	}

	artifact := audit.NewArtifact(s.domain, s.case_.policyHash) // D7: configured domain (was literal "income_verification")
	artifact.SetRuleset(s.case_.ruleset.ID, s.case_.ruleset.Version, s.case_.rulesetHash)

	for _, doc := range s.case_.snapshot.Documents {
		artifact.AddDocument(audit.DocumentRecord{
			FileName: doc.Filename,
			DocType:  string(doc.Type),
			Hash:     doc.Hash,
		})
	}

	var findings []review.Finding
	for _, res := range s.case_.decision.Results {
		findings = append(findings, review.Finding{
			Rule:     res.CheckID,
			Severity: string(res.Severity),
			ClaimA:   res.CheckID,
			ClaimB:   "",
		})
	}

	artifact.AddDecision(audit.Decision{
		CaseID:   s.case_.id,
		State:    string(s.case_.decision.Status),
		Findings: convertFindings(s.case_.decision.Results),
	})

	reviewsMap := make(map[int]*review.HumanReview)
	for _, r := range s.case_.reviewStore.GetAll() {
		reviewsMap[r.FindingIndex] = r
		artifact.AddHumanReview(audit.HumanReviewRecord{
			FindingIndex:     r.FindingIndex,
			Action:           string(r.Action),
			Note:             r.Note,
			ResolvedAt:       r.ResolvedAt,
			ReviewerIdentity: r.ReviewerIdentity, // Phase 6
			Channel:          r.Channel,          // Phase 6
		})
	}

	final := review.ComputeFinalDisposition(string(s.case_.decision.Status), findings, reviewsMap)
	artifact.SetFinalDisposition(final)
	artifact.Finalize()

	return artifact, nil
}

func convertFindings(results []eval.Result) []audit.Finding {
	var out []audit.Finding
	for _, r := range results {
		out = append(out, audit.Finding{
			Rule:     r.CheckID,
			Severity: string(r.Severity),
			ClaimA:   r.CheckID,
			ClaimB:   "",
		})
	}
	return out
}
