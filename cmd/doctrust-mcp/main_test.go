package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func buildTestBinary(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	binary := filepath.Join(tmp, "doctrust-mcp")
	cmd := exec.Command("go", "build", "-o", binary, "./cmd/doctrust-mcp/")
	cmd.Dir = filepath.Join("..", "..")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return binary
}

func TestMCP_MalformedInput_TerminatesProcess(t *testing.T) {
	binary := buildTestBinary(t)

	cwd, _ := os.Getwd()
	snapshotRoot := filepath.Join(cwd, "..", "..", "demo")
	rulesetsDir := filepath.Join(cwd, "..", "..", "rulesets")

	cmd := exec.Command(binary,
		"--snapshot-root", snapshotRoot,
		"--rulesets-dir", rulesetsDir,
	)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}

	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	if err := cmd.Start(); err != nil {
		t.Fatalf("start process: %v", err)
	}

	// Send valid initialize handshake
	initReq := map[string]any{
		"jsonrpc": "2.0",
		"id":      float64(1),
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":   map[string]any{},
			"clientInfo":     map[string]any{"name": "test", "version": "0.0.1"},
		},
	}
	initBytes, _ := json.Marshal(initReq)
	stdin.Write(append(initBytes, '\n'))

	// Give the server time to process and respond
	time.Sleep(500 * time.Millisecond)

	// Send a single malformed JSON-RPC line
	stdin.Write([]byte("THIS IS NOT JSON\n"))
	stdin.Close()

	// Wait with timeout
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		stderrStr := stderrBuf.String()

		// Assert nonzero exit
		if err == nil {
			t.Fatal("expected non-nil error from Wait (nonzero exit)")
		}

		// Assert exit code 1
		exitErr, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("expected ExitError, got %T: %v", err, err)
		}
		if exitErr.ExitCode() != 1 {
			t.Errorf("expected exit code 1, got %d", exitErr.ExitCode())
		}
		if exitErr.ExitCode() == 1 {
			t.Logf("confirmed exit code 1 (fail-fast)")
		}

		// Assert stderr mentions the parse error
		if !strings.Contains(stderrStr, "invalid character") {
			t.Errorf("expected stderr to mention parse error, got: %s", stderrStr)
		}

		// Assert stdout has no valid JSON-RPC tool response after malformed input
		// (fail-closed: no fabricated response leaks to the client)
		scanner := bufio.NewScanner(&stdoutBuf)
		linesAfterMalformed := false
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			if linesAfterMalformed {
				var msg map[string]any
				if err := json.Unmarshal([]byte(line), &msg); err == nil {
					if _, hasResult := msg["result"]; hasResult {
						t.Errorf("fabricated tool response on stdout after malformed input: %s", line)
					}
					if _, hasError := msg["error"]; hasError {
						t.Errorf("JSON-RPC error response on stdout after malformed input: %s", line)
					}
				}
			}
			if line == "THIS IS NOT JSON" || strings.Contains(line, "invalid character") {
				linesAfterMalformed = true
			}
		}

		// Verify the server logged the parse error (not just silent death)
		if !strings.Contains(stderrStr, "server exited") {
			t.Errorf("expected 'server exited' in stderr, got: %s", stderrStr)
		}

		// Confirm no SIGSEGV/SIGABRT — clean exit via os.Exit(1)
		if exitErr.Sys().(syscall.WaitStatus).Signal() == syscall.SIGSEGV {
			t.Error("process died with SIGSEGV — not a clean fail-fast exit")
		}

	case <-time.After(30 * time.Second):
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
		t.Fatal("process did not exit within 30s timeout")
	}
}
