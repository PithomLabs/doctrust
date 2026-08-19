package compiler

import (
	"crypto/sha256"
	"fmt"
	"time"
)

type PolicyVersion struct {
	PolicyMDHash     string    `json:"policy_md_hash"`
	PromptHash       string    `json:"prompt_hash"`
	RegoHash         string    `json:"rego_hash"`
	FixtureSetHash   string    `json:"fixture_set_hash"`
	ExtractionHash   string    `json:"extraction_hash"`
	CompilerModel    string    `json:"compiler_model"`
	CompilerVersion  string    `json:"compiler_version"`
	GeneratedAt      time.Time `json:"generated_at"`
	AttemptCount     int       `json:"attempt_count"`
	ValidationPassed bool      `json:"validation_passed"`
}

func ComputeHash(data []byte) string {
	h := sha256.Sum256(data)
	return fmt.Sprintf("%x", h)
}

func NewPolicyVersion(policyMDHash, promptHash, regoHash, fixtureSetHash, extractionHash, model string, attempts int, passed bool) *PolicyVersion {
	return &PolicyVersion{
		PolicyMDHash:     policyMDHash,
		PromptHash:       promptHash,
		RegoHash:         regoHash,
		FixtureSetHash:   fixtureSetHash,
		ExtractionHash:   extractionHash,
		CompilerModel:    model,
		CompilerVersion:  "phase3-v1",
		GeneratedAt:      time.Now(),
		AttemptCount:     attempts,
		ValidationPassed: passed,
	}
}
