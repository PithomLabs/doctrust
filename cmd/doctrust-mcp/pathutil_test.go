package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveSnapshotRoot_FlagOverride(t *testing.T) {
	root := resolveSnapshotRoot("/tmp/test-root")
	abs, _ := filepath.Abs("/tmp/test-root")
	if root != abs {
		t.Errorf("expected %q, got %q", abs, root)
	}
}

func TestResolveSnapshotRoot_EnvFallback(t *testing.T) {
	os.Setenv("DOCTRUST_SNAPSHOT_ROOT", "/tmp/env-root")
	defer os.Unsetenv("DOCTRUST_SNAPSHOT_ROOT")

	root := resolveSnapshotRoot("")
	abs, _ := filepath.Abs("/tmp/env-root")
	if root != abs {
		t.Errorf("expected %q, got %q", abs, root)
	}
}

func TestResolveSnapshotRoot_CWDFallback(t *testing.T) {
	os.Unsetenv("DOCTRUST_SNAPSHOT_ROOT")
	cwd, _ := os.Getwd()
	root := resolveSnapshotRoot("")
	if root != cwd {
		t.Errorf("expected %q, got %q", cwd, root)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	cwd, _ := os.Getwd()
	return filepath.Join(cwd, "..", "..")
}

func TestValidateSnapshotPath_RelativeToRoot(t *testing.T) {
	root := filepath.Join(repoRoot(t), "demo")

	resolved, err := validateSnapshotPath("income_verification/evidence_snapshot.json", root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := filepath.Join(root, "income_verification/evidence_snapshot.json")
	if resolved != expected {
		t.Errorf("expected %q, got %q", expected, resolved)
	}
}

func TestValidateSnapshotPath_AbsolutePath(t *testing.T) {
	root := filepath.Join(repoRoot(t), "demo")
	abs := filepath.Join(root, "income_verification/evidence_snapshot.json")

	resolved, err := validateSnapshotPath(abs, root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved != abs {
		t.Errorf("expected %q, got %q", abs, resolved)
	}
}

func TestValidateSnapshotPath_TraversalRejected(t *testing.T) {
	root := filepath.Join(repoRoot(t), "demo")

	_, err := validateSnapshotPath("../../../etc/passwd", root)
	if err == nil {
		t.Fatal("expected error for traversal path")
	}
}

func TestValidateSnapshotPath_OutsideRootRejected(t *testing.T) {
	root := filepath.Join(repoRoot(t), "demo")

	_, err := validateSnapshotPath("/tmp/evil.json", root)
	if err == nil {
		t.Fatal("expected error for absolute path outside root")
	}
}

func TestValidateSnapshotPath_SymlinkRejected(t *testing.T) {
	root := filepath.Join(repoRoot(t), "demo")
	tmpDir := t.TempDir()

	symlinkPath := filepath.Join(tmpDir, "link.json")
	os.Symlink("/tmp/evil.json", symlinkPath)

	_, err := validateSnapshotPath(symlinkPath, root)
	if err == nil {
		t.Fatal("expected error for symlink outside root")
	}
}

func TestValidateSnapshotPath_NonexistentRejected(t *testing.T) {
	root := filepath.Join(repoRoot(t), "demo")

	_, err := validateSnapshotPath("nonexistent.json", root)
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestValidateSnapshotPath_FileInsideRoot(t *testing.T) {
	root := filepath.Join(repoRoot(t), "demo")

	resolved, err := validateSnapshotPath("income_verification/evidence_snapshot.json", root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(resolved); err != nil {
		t.Errorf("resolved path does not exist: %v", err)
	}
}

func TestValidateSnapshotPath_ResolvableSymlinkEscapingRoot(t *testing.T) {
	tmpDir := t.TempDir()
	root := filepath.Join(tmpDir, "root")
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatalf("mkdir root: %v", err)
	}

	outsideDir := filepath.Join(tmpDir, "outside")
	if err := os.MkdirAll(outsideDir, 0755); err != nil {
		t.Fatalf("mkdir outside: %v", err)
	}
	outsideFile := filepath.Join(outsideDir, "target.json")
	if err := os.WriteFile(outsideFile, []byte(`{"evil": true}`), 0644); err != nil {
		t.Fatalf("write outside file: %v", err)
	}

	symlinkPath := filepath.Join(root, "link.json")
	relTarget, err := filepath.Rel(root, outsideFile)
	if err != nil {
		t.Fatalf("filepath.Rel: %v", err)
	}
	if err := os.Symlink(relTarget, symlinkPath); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	_, err = validateSnapshotPath("link.json", root)
	if err == nil {
		t.Fatal("expected error for resolvable symlink escaping root")
	}
	t.Logf("correctly rejected resolvable symlink escape: %v", err)
}

func TestValidateSnapshotPath_SiblingPrefixRejected(t *testing.T) {
	tmpDir := t.TempDir()
	root := filepath.Join(tmpDir, "root")
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatalf("mkdir root: %v", err)
	}

	siblingDir := filepath.Join(tmpDir, "root-evil")
	if err := os.MkdirAll(siblingDir, 0755); err != nil {
		t.Fatalf("mkdir sibling: %v", err)
	}
	siblingFile := filepath.Join(siblingDir, "file.json")
	if err := os.WriteFile(siblingFile, []byte(`{"evil": true}`), 0644); err != nil {
		t.Fatalf("write sibling file: %v", err)
	}

	_, err := validateSnapshotPath("../root-evil/file.json", root)
	if err == nil {
		t.Fatal("expected error for sibling-prefix directory escape")
	}
	t.Logf("correctly rejected sibling-prefix escape: %v", err)
}
