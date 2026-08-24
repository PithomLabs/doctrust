package main

// TTY gating + terminal helpers (plans12 P6-1): human review requires an
// interactive terminal. Passphrase entry suppresses echo where the OS allows.

import (
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

func defaultKeyDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".doctrust-reviewer"
	}
	return filepath.Join(home, ".doctrust-reviewer")
}

func defaultRulesetsDir() string {
	if wd, err := os.Getwd(); err == nil {
		if _, err := os.Stat(filepath.Join(wd, "rulesets")); err == nil {
			return filepath.Join(wd, "rulesets")
		}
	}
	return "rulesets"
}

func osUser() string {
	if u, err := user.Current(); err == nil && u.Username != "" {
		return u.Username
	}
	return "unknown"
}

// isInteractive reports whether stdin and stdout are character devices
// (i.e., an interactive terminal rather than a pipe/file).
func isInteractive() bool {
	for _, f := range []*os.File{os.Stdin, os.Stdout} {
		fi, err := f.Stat()
		if err != nil {
			return false
		}
		if fi.Mode()&os.ModeCharDevice == 0 {
			return false
		}
	}
	return true
}

func exitOn(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "FATAL:", err)
		os.Exit(1)
	}
}

func b64(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

// readLineUnbuffered reads one \n-terminated line WITHOUT read-ahead, so
// scripted/pty feeds are consumed exactly line-by-line.
func readLineUnbuffered(r io.Reader) (string, error) {
	var sb strings.Builder
	b := make([]byte, 1)
	for {
		n, err := r.Read(b)
		if n > 0 {
			sb.WriteByte(b[0])
			if b[0] == '\n' {
				return sb.String(), nil
			}
		}
		if err != nil {
			return sb.String(), err
		}
	}
}

// readSecret reads one line while suppressing terminal echo (best effort via
// termios). Uses unbuffered reads so scripted/pty feeds are never stolen.
func readSecret(prompt string) (string, error) {
	fmt.Fprint(os.Stdout, prompt)
	fd := int(os.Stdin.Fd())
	saved, termErr := unix.IoctlGetTermios(fd, unix.TCGETS)
	echoOff := false
	if termErr == nil {
		raw := *saved
		raw.Lflag &^= unix.ECHO
		if err := unix.IoctlSetTermios(fd, unix.TCSETS, &raw); err == nil {
			echoOff = true
			defer func() {
				_ = unix.IoctlSetTermios(fd, unix.TCSETS, saved)
				fmt.Fprintln(os.Stdout)
			}()
		}
	}
	line, err := readLineUnbuffered(os.Stdin)
	if !echoOff {
		fmt.Fprintln(os.Stdout, "(warning: input was visible)")
	}
	return strings.TrimRight(line, "\r\n"), err
}
