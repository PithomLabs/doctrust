package review

import (
	"sync"
	"time"
)

type FindingAction string

const (
	ActionConfirm  FindingAction = "confirm"
	ActionReject   FindingAction = "reject"
	ActionOverride FindingAction = "override"
)

type HumanReview struct {
	FindingIndex int           `json:"finding_index"`
	Action       FindingAction `json:"action"`
	Note         string        `json:"note"`
	ResolvedAt   time.Time     `json:"resolved_at"`
}

type Finding struct {
	Rule     string  `json:"rule"`
	Severity string  `json:"severity"`
	ClaimA   string  `json:"claim_a"`
	ClaimB   string  `json:"claim_b"`
	ValueA   float64 `json:"value_a"`
	ValueB   float64 `json:"value_b"`
	Variance float64 `json:"variance,omitempty"`
}

type ReviewStore struct {
	mu      sync.RWMutex
	reviews map[int]*HumanReview
}

func NewReviewStore() *ReviewStore {
	return &ReviewStore{
		reviews: make(map[int]*HumanReview),
	}
}

func (s *ReviewStore) AddReview(r *HumanReview) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r.ResolvedAt = time.Now().UTC()
	s.reviews[r.FindingIndex] = r
}

func (s *ReviewStore) GetReview(findingIndex int) *HumanReview {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.reviews[findingIndex]
}

func (s *ReviewStore) GetAll() []*HumanReview {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*HumanReview, 0, len(s.reviews))
	for _, r := range s.reviews {
		result = append(result, r)
	}
	return result
}

func (s *ReviewStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.reviews)
}

func (s *ReviewStore) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reviews = make(map[int]*HumanReview)
}

func ComputeFinalDisposition(opaState string, findings []Finding, reviews map[int]*HumanReview) string {
	if opaState == "PASS" && len(findings) == 0 {
		return "PASS"
	}

	if len(reviews) < len(findings) {
		return "REVIEW"
	}

	for i := range findings {
		review, ok := reviews[i]
		if !ok {
			return "REVIEW"
		}
		switch review.Action {
		case ActionReject:
			return "FAIL"
		case ActionOverride:
			continue
		case ActionConfirm:
			// Confirm means the human agrees this finding is correct.
			// Continue checking remaining findings.
			continue
		}
	}

	return "PASS"
}
