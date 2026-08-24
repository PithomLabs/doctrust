package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func resolveRoot(flagVal string) string {
	if flagVal != "" {
		return canonicalize(flagVal)
	}
	if env := os.Getenv("DOCTRUST_SNAPSHOT_ROOT"); env != "" {
		return canonicalize(env)
	}
	cwd, _ := os.Getwd()
	return cwd
}

func canonicalize(p string) string {
	abs, _ := filepath.Abs(p)
	clean := filepath.Clean(abs)
	real, err := filepath.EvalSymlinks(clean)
	if err != nil {
		return clean
	}
	return real
}

func validateSnapshotPath(snapshotPath, allowedRoot string) (string, error) {
	var abs string
	if filepath.IsAbs(snapshotPath) {
		abs = snapshotPath
	} else {
		abs = filepath.Join(allowedRoot, snapshotPath)
	}
	clean := filepath.Clean(abs)
	real, err := filepath.EvalSymlinks(clean)
	if err != nil {
		return "", fmt.Errorf("path not resolvable: %w", err)
	}
	if real != allowedRoot && !strings.HasPrefix(real, allowedRoot+string(os.PathSeparator)) {
		return "", fmt.Errorf("path outside allowed root")
	}
	if _, err := os.Stat(real); err != nil {
		return "", fmt.Errorf("file not found: %w", err)
	}
	return real, nil
}

func validateDocPath(docPath, allowedRoot string) (string, error) {
	real, err := validateSnapshotPath(docPath, allowedRoot)
	if err != nil {
		return "", err
	}
	if !strings.HasSuffix(strings.ToLower(real), ".pdf") {
		return "", fmt.Errorf("not a PDF: %s", real)
	}
	return real, nil
}

func validateDirPath(dirPath, allowedRoot string) (string, error) {
	abs := dirPath
	if !filepath.IsAbs(dirPath) {
		abs = filepath.Join(allowedRoot, dirPath)
	}
	real := canonicalize(abs)
	if real != allowedRoot && !strings.HasPrefix(real, allowedRoot+string(os.PathSeparator)) {
		return "", fmt.Errorf("directory outside allowed root")
	}
	return real, nil
}

func filepathDir(p string) string { return filepath.Dir(p) }

func filepathJoin(elem ...string) string { return filepath.Join(elem...) }
