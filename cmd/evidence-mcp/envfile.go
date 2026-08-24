package main

// Runtime credential loading (plan11 P5-7): the Nutrient API key must never
// be materialized inside ~/.hermes/config.yaml. evidence-mcp receives only a
// PATH to the repository's existing .env file and parses it at process
// startup; values are held in memory and never logged.

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

var envFileValues = map[string]string{}

// loadEnvFile parses a .env file (KEY=VALUE lines) into an in-memory fallback
// consulted by extractionKey(). Missing or malformed files are non-fatal:
// plain environment variables still take precedence.
func loadEnvFile(path string) {
	if path == "" {
		return
	}
	fh, err := os.Open(path)
	if err != nil {
		return
	}
	defer fh.Close()
	sc := bufio.NewScanner(fh)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
		if key != "" {
			envFileValues[key] = val
		}
	}
}

// lookupCred resolves a credential: process environment first, then the
// parsed --env-file values.
func lookupCred(key string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return envFileValues[key]
}

// defaultEnvFileNearExecutable discovers <repo>/doctrust/.env when the binary
// runs from <repo>/doctrust/bin/, so registrations can omit even the path.
func defaultEnvFileNearExecutable() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	dir := filepath.Dir(exe)
	candidate := filepath.Join(filepath.Dir(dir), ".env") // <repo>/doctrust/.env
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}
	return ""
}
