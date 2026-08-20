package compiler

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateCheckID(t *testing.T) {
	baseDir := t.TempDir()

	tests := []struct {
		name    string
		checkID string
		wantErr bool
	}{
		{"valid simple", "my_check", false},
		{"valid with numbers", "check_v2", false},
		{"valid single char", "a", false},
		{"invalid uppercase", "MyCheck", true},
		{"invalid hyphen", "my-check", true},
		{"invalid space", "my check", true},
		{"invalid path traversal", "../../etc/passwd", true},
		{"invalid dot-dot", "check/../evil", true},
		{"invalid leading dot", ".hidden", true},
		{"invalid empty", "", true},
		{"invalid with slash", "foo/bar", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCheckID(baseDir, tt.checkID)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateCheckID() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCandidateDir_Containment(t *testing.T) {
	baseDir := t.TempDir()

	// Even if somehow a traversal gets through, CandidateDir should be safe
	dir := CandidateDir(baseDir, "safe_check")
	if !filepath.IsAbs(dir) {
		// Make it absolute for comparison
		dir, _ = filepath.Abs(dir)
	}
	absBase, _ := filepath.Abs(filepath.Join(baseDir, "active"))
	if !filepath.HasPrefix(dir, absBase) {
		t.Errorf("CandidateDir resolved outside base: %s", dir)
	}
}

func TestWriteApproval_SetsTimestamp(t *testing.T) {
	dir := t.TempDir()
	// Create minimal candidate files
	os.WriteFile(filepath.Join(dir, "check.go"), []byte("package candidate"), 0644)
	os.WriteFile(filepath.Join(dir, "scenarios.yaml"), []byte("scenarios: []"), 0644)
	os.WriteFile(filepath.Join(dir, "metadata.yaml"), []byte("id: test"), 0644)
	os.WriteFile(filepath.Join(dir, "adversarial.yaml"), []byte("scenarios: []"), 0644)

	err := WriteApproval(dir, "test_check", "1.0")
	if err != nil {
		t.Fatalf("WriteApproval: %v", err)
	}

	approval, err := LoadApproval(dir)
	if err != nil {
		t.Fatalf("LoadApproval: %v", err)
	}

	if approval.CheckID != "test_check" {
		t.Errorf("CheckID = %q, want test_check", approval.CheckID)
	}
	if approval.Version != "1.0" {
		t.Errorf("Version = %q, want 1.0", approval.Version)
	}
	if approval.ApprovedAt == "" {
		t.Error("ApprovedAt should be set, got empty string")
	}
}

func TestVerifyApproval_IdentityMismatch(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "check.go"), []byte("package candidate"), 0644)
	os.WriteFile(filepath.Join(dir, "scenarios.yaml"), []byte("scenarios: []"), 0644)
	os.WriteFile(filepath.Join(dir, "metadata.yaml"), []byte("id: test"), 0644)
	os.WriteFile(filepath.Join(dir, "adversarial.yaml"), []byte("scenarios: []"), 0644)

	// Approve as "check_a" v1.0
	WriteApproval(dir, "check_a", "1.0")

	// Verify as "check_b" — should fail
	err := VerifyApproval(dir, "check_b", "1.0")
	if err == nil {
		t.Error("VerifyApproval should fail with identity mismatch")
	}

	// Verify with wrong version — should fail
	err = VerifyApproval(dir, "check_a", "2.0")
	if err == nil {
		t.Error("VerifyApproval should fail with version mismatch")
	}

	// Verify with correct identity — should pass
	err = VerifyApproval(dir, "check_a", "1.0")
	if err != nil {
		t.Errorf("VerifyApproval with correct identity should pass: %v", err)
	}
}

func TestVerifyApproval_ContentChanged(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "check.go"), []byte("package candidate"), 0644)
	os.WriteFile(filepath.Join(dir, "scenarios.yaml"), []byte("scenarios: []"), 0644)
	os.WriteFile(filepath.Join(dir, "metadata.yaml"), []byte("id: test"), 0644)
	os.WriteFile(filepath.Join(dir, "adversarial.yaml"), []byte("scenarios: []"), 0644)

	WriteApproval(dir, "test_check", "1.0")

	// Modify check.go after approval
	os.WriteFile(filepath.Join(dir, "check.go"), []byte("package candidate\n// modified"), 0644)

	err := VerifyApproval(dir, "test_check", "1.0")
	if err == nil {
		t.Error("VerifyApproval should fail when content changed after approval")
	}
}

func TestHasAdversarial(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		want     bool
		wantName string
	}{
		{
			name:    "has human_adversarial",
			content: "scenarios:\n  - name: edge_case\n    origin: human_adversarial\n",
			want:    true,
		},
		{
			name:    "only ai scenarios",
			content: "scenarios:\n  - name: test\n    origin: ai\n",
			want:    false,
		},
		{
			name:    "empty file",
			content: "",
			want:    false,
		},
		{
			name:    "missing file",
			content: "",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if tt.name != "missing file" {
				os.WriteFile(filepath.Join(dir, "adversarial.yaml"), []byte(tt.content), 0644)
			}
			got, _ := HasAdversarial(dir)
			if got != tt.want {
				t.Errorf("HasAdversarial() = %v, want %v", got, tt.want)
			}
		})
	}
}
