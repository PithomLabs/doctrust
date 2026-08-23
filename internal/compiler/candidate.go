package compiler

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// CheckCandidate holds the LLM-generated check and its metadata.
type CheckCandidate struct {
	CheckID         string         `json:"check_id" yaml:"id"`
	Version         string         `json:"version" yaml:"version"`
	Description     string         `json:"description" yaml:"description"`
	GoSource        string         `json:"go_source"`
	Parameters      map[string]any `json:"parameters" yaml:"parameters"`
	Scenarios       []ScenarioDef  `json:"scenarios"`
	AdversarialHint string         `json:"adversarial_hint"`
}

// ScenarioDef is a single scenario definition from the LLM output.
type ScenarioDef struct {
	Name     string         `json:"name" yaml:"name"`
	Origin   string         `json:"origin" yaml:"origin"`
	Input    ScenarioInput  `json:"input" yaml:"input"`
	Params   map[string]any `json:"params" yaml:"params"`
	Expected ScenarioExpect `json:"expected" yaml:"expected"`
}

// ScenarioInput holds the facts input for a scenario.
type ScenarioInput struct {
	Facts []FactDef `json:"facts" yaml:"facts"`
}

// FactDef is a single fact definition in a scenario.
type FactDef struct {
	SemanticType string  `json:"semantic_type" yaml:"semantic_type"`
	SourceDoc    string  `json:"source_doc" yaml:"source_doc"`
	Field        string  `json:"field" yaml:"field"`
	Value        any     `json:"value" yaml:"value"`
	SourceSpan   string  `json:"source_span" yaml:"source_span"`
	Confidence   float64 `json:"confidence" yaml:"confidence"`
}

// ScenarioExpect holds the expected result for a scenario.
type ScenarioExpect struct {
	CheckID  string        `json:"check_id" yaml:"check_id"`
	Status   string        `json:"status" yaml:"status"`
	Severity string        `json:"severity" yaml:"severity"`
	Reason   string        `json:"reason" yaml:"reason"`
	Evidence []EvidenceDef `json:"evidence" yaml:"evidence"`
}

// EvidenceDef is an expected evidence entry.
type EvidenceDef struct {
	Field      string  `json:"field" yaml:"field"`
	SourceDoc  string  `json:"source_doc" yaml:"source_doc"`
	SourceSpan string  `json:"source_span" yaml:"source_span"`
	Confidence float64 `json:"confidence" yaml:"confidence"`
}

// CandidateState represents the lifecycle state of a candidate.
type CandidateState string

const (
	StateDraft       CandidateState = "DRAFT"
	StateHumanReview CandidateState = "HUMAN_REVIEW"
	StateApproved    CandidateState = "APPROVED"
	StateRejected    CandidateState = "REJECTED"
	StateTransformed CandidateState = "TRANSFORMED"
	StateBuilt       CandidateState = "BUILT"
	StateVerified    CandidateState = "VERIFIED"
	StatePromoted    CandidateState = "PROMOTED"
	StateArchived    CandidateState = "ARCHIVED"
)

// ApprovalData holds content hashes at approval time.
type ApprovalData struct {
	CheckID         string `json:"check_id"`
	Version         string `json:"version"`
	CheckSourceHash string `json:"check_source_hash"`
	ScenariosHash   string `json:"scenarios_hash"`
	MetadataHash    string `json:"metadata_hash"`
	AdversarialHash string `json:"adversarial_hash"`
	ApprovedAt      string `json:"approved_at"`
	ReviewerID      string `json:"reviewer_id"`
}

// CandidateSnapshot holds the exact bytes of a validated candidate.
// Once created, it is immutable for the duration of the promotion pipeline.
// The snapshot is the single read point — all downstream operations use these bytes.
type CandidateSnapshot struct {
	CheckID     string
	Version     string
	Parameters  map[string]any
	GoSource    []byte            // check.go content
	Metadata    []byte            // metadata.yaml content
	Scenarios   []byte            // scenarios.yaml content
	Adversarial []byte            // adversarial.yaml content (may be nil)
	State       []byte            // state file content
	Dir         string            // original candidate directory
	Hashes      map[string]string // SHA-256 of each file's bytes
}

// SnapshotCandidate reads all candidate files ONCE into memory and computes
// hashes from the in-memory bytes. This is the single read point — no further
// filesystem reads of the candidate directory occur after this call.
func SnapshotCandidate(candidateDir string) (*CandidateSnapshot, error) {
	snap := &CandidateSnapshot{
		Dir:    candidateDir,
		Hashes: make(map[string]string),
	}

	// Read check.go
	data, err := os.ReadFile(filepath.Join(candidateDir, "check.go"))
	if err != nil {
		return nil, fmt.Errorf("read check.go: %w", err)
	}
	snap.GoSource = data
	h := sha256.Sum256(data)
	snap.Hashes["check.go"] = fmt.Sprintf("%x", h)

	// Read metadata.yaml
	data, err = os.ReadFile(filepath.Join(candidateDir, "metadata.yaml"))
	if err != nil {
		return nil, fmt.Errorf("read metadata.yaml: %w", err)
	}
	snap.Metadata = data
	h = sha256.Sum256(data)
	snap.Hashes["metadata.yaml"] = fmt.Sprintf("%x", h)

	// Parse metadata for identity
	var meta struct {
		ID         string         `yaml:"id"`
		Version    string         `yaml:"version"`
		Parameters map[string]any `yaml:"parameters"`
	}
	if err := yaml.Unmarshal(snap.Metadata, &meta); err != nil {
		return nil, fmt.Errorf("parse metadata.yaml: %w", err)
	}
	snap.CheckID = meta.ID
	snap.Version = meta.Version
	snap.Parameters = meta.Parameters

	// Read scenarios.yaml
	data, err = os.ReadFile(filepath.Join(candidateDir, "scenarios.yaml"))
	if err != nil {
		return nil, fmt.Errorf("read scenarios.yaml: %w", err)
	}
	snap.Scenarios = data
	h = sha256.Sum256(data)
	snap.Hashes["scenarios.yaml"] = fmt.Sprintf("%x", h)

	// Read adversarial.yaml (may not exist)
	data, err = os.ReadFile(filepath.Join(candidateDir, "adversarial.yaml"))
	if err == nil {
		snap.Adversarial = data
		h = sha256.Sum256(data)
		snap.Hashes["adversarial.yaml"] = fmt.Sprintf("%x", h)
	} else {
		snap.Hashes["adversarial.yaml"] = ""
	}

	// Read state
	data, err = os.ReadFile(filepath.Join(candidateDir, "state"))
	if err != nil {
		return nil, fmt.Errorf("read state: %w", err)
	}
	snap.State = data

	return snap, nil
}

// VerifyApprovalAgainstSnapshot verifies that approval.json hashes match the
// snapshot's in-memory hashes. The approval is loaded from candidateDir but
// compared against snapshot bytes (NOT re-read from filesystem).
func VerifyApprovalAgainstSnapshot(candidateDir string, snapshot *CandidateSnapshot) error {
	existing, err := LoadApproval(candidateDir)
	if err != nil {
		return err
	}

	// Verify identity binding
	if existing.CheckID != snapshot.CheckID {
		return fmt.Errorf("approval bound to check_id %q, snapshot is %q", existing.CheckID, snapshot.CheckID)
	}
	if existing.Version != snapshot.Version {
		return fmt.Errorf("approval bound to version %q, snapshot is %q", existing.Version, snapshot.Version)
	}

	// Verify content hashes against snapshot bytes (NOT filesystem)
	if existing.CheckSourceHash != snapshot.Hashes["check.go"] {
		return fmt.Errorf("check.go hash mismatch: approval=%s snapshot=%s", existing.CheckSourceHash, snapshot.Hashes["check.go"])
	}
	if existing.ScenariosHash != snapshot.Hashes["scenarios.yaml"] {
		return fmt.Errorf("scenarios.yaml hash mismatch: approval=%s snapshot=%s", existing.ScenariosHash, snapshot.Hashes["scenarios.yaml"])
	}
	if existing.MetadataHash != snapshot.Hashes["metadata.yaml"] {
		return fmt.Errorf("metadata.yaml hash mismatch: approval=%s snapshot=%s", existing.MetadataHash, snapshot.Hashes["metadata.yaml"])
	}
	if existing.AdversarialHash != snapshot.Hashes["adversarial.yaml"] {
		return fmt.Errorf("adversarial.yaml hash mismatch: approval=%s snapshot=%s", existing.AdversarialHash, snapshot.Hashes["adversarial.yaml"])
	}
	return nil
}

// HasAdversarialInSnapshot checks if the snapshot contains a human-authored adversarial scenario.
func HasAdversarialInSnapshot(snapshot *CandidateSnapshot) (bool, string) {
	if len(snapshot.Adversarial) == 0 {
		return false, ""
	}

	var scenarioData struct {
		Scenarios []ScenarioDef `yaml:"scenarios"`
	}
	if err := yaml.Unmarshal(snapshot.Adversarial, &scenarioData); err != nil {
		return false, ""
	}

	for _, s := range scenarioData.Scenarios {
		if s.Origin == "human_adversarial" {
			return true, s.Name
		}
	}
	return false, ""
}

// checkIDRe is the strict regex for valid check IDs.
var checkIDRe = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// ValidateCheckID validates a check ID against the strict regex and ensures
// the resolved path stays within the intended base directory.
func ValidateCheckID(baseDir, checkID string) error {
	if !checkIDRe.MatchString(checkID) {
		return fmt.Errorf("invalid check_id %q: must match ^[a-z][a-z0-9_]*$", checkID)
	}
	resolved := filepath.Join(baseDir, "active", checkID)
	absBase, err := filepath.Abs(filepath.Join(baseDir, "active"))
	if err != nil {
		return fmt.Errorf("resolve base dir: %w", err)
	}
	absResolved, err := filepath.Abs(resolved)
	if err != nil {
		return fmt.Errorf("resolve candidate dir: %w", err)
	}
	if !strings.HasPrefix(absResolved, absBase+string(filepath.Separator)) && absResolved != absBase {
		return fmt.Errorf("check_id %q resolves outside candidates/active/", checkID)
	}
	return nil
}

// ValidateCandidatePath resolves symlinks and verifies a nested containment chain:
//
//	EvalSymlinks(baseDir)      ← resolved root
//	EvalSymlinks(activeDir)    ← must be contained by resolved baseDir
//	EvalSymlinks(candidateDir) ← must be contained by resolved activeDir
//
// Must be called AFTER os.MkdirAll on candidateDir so the path exists for symlink resolution.
// This protects against pre-existing symlinks and path traversal.
// Note: this does NOT protect against concurrent symlink-swap attacks during the write window.
func ValidateCandidatePath(baseDir, checkID string) error {
	activeDir := filepath.Join(baseDir, "active")
	candidateDir := filepath.Join(activeDir, checkID)

	// Resolve root
	resolvedBase, err := filepath.EvalSymlinks(baseDir)
	if err != nil {
		return fmt.Errorf("resolve base dir: %w", err)
	}

	// Resolve active dir — must be contained by resolved base
	resolvedActive, err := filepath.EvalSymlinks(activeDir)
	if err != nil {
		return fmt.Errorf("resolve active dir: %w", err)
	}
	if !isUnder(resolvedBase, resolvedActive) {
		return fmt.Errorf("active directory %q resolves outside base %q", activeDir, baseDir)
	}

	// Resolve candidate dir — must be contained by resolved active
	resolvedCandidate, err := filepath.EvalSymlinks(candidateDir)
	if err != nil {
		return fmt.Errorf("resolve candidate dir: %w", err)
	}
	if !isUnder(resolvedActive, resolvedCandidate) {
		return fmt.Errorf("candidate directory %q resolves outside active %q", candidateDir, activeDir)
	}

	return nil
}

// ValidateArchivePath ensures the archive parent directory is safe.
// Since the archive child may not exist yet, we validate the resolved
// existing parent (candidates/archive/) and then construct the child path.
func ValidateArchivePath(baseDir, checkID string) (string, error) {
	archiveParent := filepath.Join(baseDir, "archive")

	// Resolve root
	resolvedBase, err := filepath.EvalSymlinks(baseDir)
	if err != nil {
		return "", fmt.Errorf("resolve base dir: %w", err)
	}

	// Resolve archive parent — must be contained by resolved base
	resolvedArchiveParent, err := filepath.EvalSymlinks(archiveParent)
	if err != nil {
		// If archive parent doesn't exist yet, try resolving the base first
		// and verify the parent would be under it after creation
		resolvedBase2, err2 := filepath.EvalSymlinks(baseDir)
		if err2 != nil {
			return "", fmt.Errorf("resolve base dir for archive: %w", err2)
		}
		// Construct expected path — will be validated after MkdirAll
		candidatePath := filepath.Join(resolvedBase2, "archive", checkID)
		return candidatePath, nil
	}

	if !isUnder(resolvedBase, resolvedArchiveParent) {
		return "", fmt.Errorf("archive parent %q resolves outside base %q", archiveParent, baseDir)
	}

	return filepath.Join(resolvedArchiveParent, checkID), nil
}

// isUnder reports whether child is contained within parent.
// Both paths should be absolute and resolved.
func isUnder(parent, child string) bool {
	parent = filepath.Clean(parent) + string(filepath.Separator)
	return strings.HasPrefix(child, parent) || filepath.Clean(child) == filepath.Clean(parent[:len(parent)-1])
}

// CandidateDir returns the path to a candidate's directory.
func CandidateDir(baseDir, checkID string) string {
	return filepath.Join(baseDir, "active", checkID)
}

// ArchiveDir returns the path to an archived candidate.
func ArchiveDir(baseDir, checkID string) string {
	return filepath.Join(baseDir, "archive", checkID)
}

// StageCandidate writes a candidate to the staging directory.
func StageCandidate(candidate *CheckCandidate, baseDir string) (string, error) {
	// Validate check ID strictly
	if err := ValidateCheckID(baseDir, candidate.CheckID); err != nil {
		return "", err
	}

	dir := CandidateDir(baseDir, candidate.CheckID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create candidate dir: %w", err)
	}

	// Validate the created directory is not a symlink escape
	if err := ValidateCandidatePath(baseDir, candidate.CheckID); err != nil {
		os.RemoveAll(dir) // clean up the potentially-dangerous dir
		return "", err
	}

	// Write check.go
	if err := os.WriteFile(filepath.Join(dir, "check.go"), []byte(candidate.GoSource), 0644); err != nil {
		return "", fmt.Errorf("write check.go: %w", err)
	}

	// Write metadata.yaml
	meta := map[string]any{
		"id":          candidate.CheckID,
		"version":     candidate.Version,
		"description": candidate.Description,
		"parameters":  candidate.Parameters,
	}
	metaBytes, err := yaml.Marshal(meta)
	if err != nil {
		return "", fmt.Errorf("marshal metadata: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "metadata.yaml"), metaBytes, 0644); err != nil {
		return "", fmt.Errorf("write metadata.yaml: %w", err)
	}

	// Write scenarios.yaml
	scenarioData := map[string]any{"scenarios": candidate.Scenarios}
	scenarioBytes, err := yaml.Marshal(scenarioData)
	if err != nil {
		return "", fmt.Errorf("marshal scenarios: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "scenarios.yaml"), scenarioBytes, 0644); err != nil {
		return "", fmt.Errorf("write scenarios.yaml: %w", err)
	}

	// Write state
	if err := os.WriteFile(filepath.Join(dir, "state"), []byte(string(StateDraft)), 0644); err != nil {
		return "", fmt.Errorf("write state: %w", err)
	}

	return dir, nil
}

// GetCurrentUser returns the current OS username, or "unknown" if unavailable.
func GetCurrentUser() string {
	u, err := user.Current()
	if err != nil {
		return "unknown"
	}
	return u.Username
}

// WriteApproval writes approval.json with content hashes and candidate identity.
func WriteApproval(candidateDir string, checkID, version, reviewerID string) error {
	hashes, err := ComputeCandidateHashes(candidateDir)
	if err != nil {
		return err
	}
	approval := ApprovalData{
		CheckID:         checkID,
		Version:         version,
		CheckSourceHash: hashes["check.go"],
		ScenariosHash:   hashes["scenarios.yaml"],
		MetadataHash:    hashes["metadata.yaml"],
		AdversarialHash: hashes["adversarial.yaml"],
		ApprovedAt:      time.Now().UTC().Format(time.RFC3339),
		ReviewerID:      reviewerID,
	}
	data, err := json.MarshalIndent(approval, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal approval: %w", err)
	}
	return os.WriteFile(filepath.Join(candidateDir, "approval.json"), data, 0644)
}

// VerifyApproval re-hashes candidate artifacts and compares to approval.json.
// Also verifies the candidate identity (CheckID, Version) matches the approval.
func VerifyApproval(candidateDir string, expectedCheckID, expectedVersion string) error {
	existing, err := LoadApproval(candidateDir)
	if err != nil {
		return err
	}

	// Verify identity binding
	if existing.CheckID != expectedCheckID {
		return fmt.Errorf("approval bound to check_id %q, candidate is %q", existing.CheckID, expectedCheckID)
	}
	if existing.Version != expectedVersion {
		return fmt.Errorf("approval bound to version %q, candidate is %q", existing.Version, expectedVersion)
	}

	current, err := ComputeCandidateHashes(candidateDir)
	if err != nil {
		return err
	}
	if existing.CheckSourceHash != current["check.go"] {
		return fmt.Errorf("check.go changed after approval")
	}
	if existing.ScenariosHash != current["scenarios.yaml"] {
		return fmt.Errorf("scenarios.yaml changed after approval")
	}
	if existing.MetadataHash != current["metadata.yaml"] {
		return fmt.Errorf("metadata.yaml changed after approval")
	}
	if existing.AdversarialHash != current["adversarial.yaml"] {
		return fmt.Errorf("adversarial.yaml changed after approval")
	}
	return nil
}

// LoadApproval reads approval.json.
func LoadApproval(candidateDir string) (*ApprovalData, error) {
	data, err := os.ReadFile(filepath.Join(candidateDir, "approval.json"))
	if err != nil {
		return nil, fmt.Errorf("no approval.json: %w", err)
	}
	var approval ApprovalData
	if err := json.Unmarshal(data, &approval); err != nil {
		return nil, fmt.Errorf("parse approval.json: %w", err)
	}
	return &approval, nil
}

// ComputeCandidateHashes computes SHA-256 hashes of candidate artifacts.
func ComputeCandidateHashes(candidateDir string) (map[string]string, error) {
	hashes := make(map[string]string)
	for _, name := range []string{"check.go", "scenarios.yaml", "metadata.yaml", "adversarial.yaml"} {
		path := filepath.Join(candidateDir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				hashes[name] = ""
				continue
			}
			return nil, fmt.Errorf("read %s: %w", name, err)
		}
		h := sha256.Sum256(data)
		hashes[name] = fmt.Sprintf("%x", h)
	}
	return hashes, nil
}

// LoadCandidate reads a candidate from disk.
func LoadCandidate(candidateDir string) (*CheckCandidate, error) {
	// Read state
	stateBytes, err := os.ReadFile(filepath.Join(candidateDir, "state"))
	if err != nil {
		return nil, fmt.Errorf("read state: %w", err)
	}
	state := CandidateState(strings.TrimSpace(string(stateBytes)))

	// Read check.go
	goSource, err := os.ReadFile(filepath.Join(candidateDir, "check.go"))
	if err != nil {
		return nil, fmt.Errorf("read check.go: %w", err)
	}

	// Read metadata.yaml
	metaBytes, err := os.ReadFile(filepath.Join(candidateDir, "metadata.yaml"))
	if err != nil {
		return nil, fmt.Errorf("read metadata.yaml: %w", err)
	}
	var meta struct {
		ID          string         `yaml:"id"`
		Version     string         `yaml:"version"`
		Description string         `yaml:"description"`
		Parameters  map[string]any `yaml:"parameters"`
	}
	if err := yaml.Unmarshal(metaBytes, &meta); err != nil {
		return nil, fmt.Errorf("parse metadata.yaml: %w", err)
	}

	// Read scenarios.yaml
	scenarioBytes, err := os.ReadFile(filepath.Join(candidateDir, "scenarios.yaml"))
	if err != nil {
		return nil, fmt.Errorf("read scenarios.yaml: %w", err)
	}
	var scenarioData struct {
		Scenarios []ScenarioDef `yaml:"scenarios"`
	}
	if err := yaml.Unmarshal(scenarioBytes, &scenarioData); err != nil {
		return nil, fmt.Errorf("parse scenarios.yaml: %w", err)
	}

	candidate := &CheckCandidate{
		CheckID:     meta.ID,
		Version:     meta.Version,
		Description: meta.Description,
		GoSource:    string(goSource),
		Parameters:  meta.Parameters,
		Scenarios:   scenarioData.Scenarios,
	}

	// Check adversarial
	advPath := filepath.Join(candidateDir, "adversarial.yaml")
	if _, err := os.Stat(advPath); err == nil {
		advBytes, err := os.ReadFile(advPath)
		if err == nil {
			_ = advBytes // loaded for validation
		}
	}

	_ = state // state is available for caller inspection

	return candidate, nil
}

// HasAdversarial checks if the candidate has a human-authored adversarial scenario.
func HasAdversarial(candidateDir string) (bool, string) {
	advPath := filepath.Join(candidateDir, "adversarial.yaml")
	data, err := os.ReadFile(advPath)
	if err != nil {
		return false, ""
	}

	var scenarioData struct {
		Scenarios []ScenarioDef `yaml:"scenarios"`
	}
	if err := yaml.Unmarshal(data, &scenarioData); err != nil {
		return false, ""
	}

	for _, s := range scenarioData.Scenarios {
		if s.Origin == "human_adversarial" {
			return true, s.Name
		}
	}
	return false, ""
}

// SetState updates the candidate's state file.
func SetState(candidateDir string, state CandidateState) error {
	return os.WriteFile(filepath.Join(candidateDir, "state"), []byte(string(state)), 0644)
}

// GetState reads the candidate's current state.
func GetState(candidateDir string) (CandidateState, error) {
	data, err := os.ReadFile(filepath.Join(candidateDir, "state"))
	if err != nil {
		return "", err
	}
	return CandidateState(strings.TrimSpace(string(data))), nil
}
