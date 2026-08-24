package main

import (
	"crypto/ed25519"
	"fmt"
	"os"
	"path/filepath"

	"github.com/doctrust/doctrust/internal/review"
)

func runProvision(args []string) {
	fmt.Fprintln(os.Stderr, "DEBUG-MARKER provision entered")
	reviewer := ""
	keyDir := defaultKeyDir()
	pubOut := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--name":
			if i+1 < len(args) {
				i++
				reviewer = args[i]
			}
		case "--key-dir":
			if i+1 < len(args) {
				i++
				keyDir = args[i]
			}
		case "--publish-to":
			if i+1 < len(args) {
				i++
				pubOut = args[i]
			}
		default:
			fmt.Fprintf(os.Stderr, "unknown argument %q\n", args[i])
			os.Exit(2)
		}
	}

	if reviewer == "" {
		reviewer = osUser()
	}
	fmt.Printf("Provisioning reviewer %q.\n", reviewer)
	fmt.Println("Choose a reviewer passphrase. It unlocks your private signing key")
	fmt.Println("and is NEVER stored anywhere — not in config, logs, or artifacts.")
	pass1, err := readSecret("passphrase: ")
	exitOn(err)
	if len(pass1) < 8 {
		fmt.Fprintln(os.Stderr, "FATAL: passphrase must be at least 8 characters")
		os.Exit(1)
	}
	pass2, err := readSecret("confirm passphrase: ")
	exitOn(err)
	if pass1 != pass2 {
		fmt.Fprintln(os.Stderr, "FATAL: passphrases do not match")
		os.Exit(1)
	}

	if err := os.MkdirAll(keyDir, 0o700); err != nil {
		exitOn(err)
	}
	path := filepath.Join(keyDir, reviewer+".key.enc")

	pub, err := saveReviewerKey(path, reviewer, pass1)
	exitOn(err)

	fmt.Printf("\nprivate key written: %s (passphrase-encrypted)\n", path)
	fmt.Printf("public key:          %s\n", b64(pub))
	if pubOut != "" {
		if err := os.MkdirAll(pubOut, 0o755); err != nil {
			exitOn(err)
		}
		pubPath := filepath.Join(pubOut, reviewer+".pub")
		if err := os.WriteFile(pubPath, []byte(b64(pub)+"\n"), 0o644); err != nil {
			exitOn(err)
		}
		fmt.Printf("public key published to reviewers ring: %s\n", pubPath)
	} else {
		fmt.Println("(copy the public key line into the trusted reviewers ring manually)")
	}
	fmt.Println("\nGuard this passphrase: it IS your signing authority.")
}

// saveReviewerKey generates a fresh Ed25519 pair and writes the
// passphrase-encrypted container; returns the public key.
func saveReviewerKey(path, keyID, passphrase string) (ed25519.PublicKey, error) {
	_, priv, err := review.GenerateReviewerKeyPair()
	if err != nil {
		return ed25519.PublicKey{}, err
	}
	return review.SaveEncryptedPrivateKey(path, keyID, priv, passphrase)
}

// loadReviewerKey decrypts the reviewer's private signing key.
func loadReviewerKey(path, passphrase string) (string, ed25519.PublicKey,
	ed25519.PrivateKey, error) {
	return review.LoadEncryptedPrivateKey(path, passphrase)
}
