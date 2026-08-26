// resign-publication creates publication-verification artifacts from historical
// Phase-6 execution runs. It re-signs review records using an operator-supplied
// keypair and produces a verification artifact set under
// demo/shipment_release/publication-verification/.
//
// The original historical Phase-6 runs are NEVER modified.
// The private signing key is NEVER committed to the repository.
package main

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/PithomLabs/doctrust/internal/review"
	"github.com/PithomLabs/doctrust/internal/service"
)

func main() {
	var (
		privateKeyPath string
		passphrase     string
		publicKeyOut   string
		domain         string
		forceReplace   bool
		dryRun         bool
	)
	flag.StringVar(&privateKeyPath, "private-key", "", "path to operator-supplied Ed25519 private key file (raw 64-byte or base64)")
	flag.StringVar(&passphrase, "passphrase", "", "passphrase for encrypted key file (or set DOCTRUST_REVIEW_PASSPHRASE)")
	flag.StringVar(&publicKeyOut, "public-key-out", "demo/shipment_release/reviewers/owner.pub", "path to write the public key")
	flag.StringVar(&domain, "domain", "shipment_release", "target domain")
	flag.BoolVar(&forceReplace, "force-replace", false, "replace existing owner.pub without confirmation")
	flag.BoolVar(&dryRun, "dry-run", false, "verify without writing files")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: resign-publication [flags]\n")
		fmt.Fprintf(os.Stderr, "\nCreates publication-verification artifacts by re-signing historical\n")
		fmt.Fprintf(os.Stderr, "Phase-6 review records with an operator-supplied keypair.\n")
		fmt.Fprintf(os.Stderr, "\nThe private key is NEVER committed to the repository.\n")
		fmt.Fprintf(os.Stderr, "\nEnvironment:\n")
		fmt.Fprintf(os.Stderr, "  DOCTRUST_REVIEW_PASSPHRASE  passphrase for encrypted key file\n")
		fmt.Fprintf(os.Stderr, "\nFlags:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	// Resolve passphrase from env if not provided
	if passphrase == "" {
		passphrase = os.Getenv("DOCTRUST_REVIEW_PASSPHRASE")
	}

	// Load private key
	priv, pub, keyID, err := loadPrivateKey(privateKeyPath, passphrase)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: load private key: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Loaded private key: key_id=%s\n", keyID)

	// Owner.pub provenance check
	if err := checkOwnerPubProvenance(publicKeyOut, pub, forceReplace); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: owner.pub provenance check failed: %v\n", err)
		os.Exit(1)
	}

	// Find P6 runs
	runsDir := filepath.Join("demo", domain, "runs")
	entries, err := os.ReadDir(runsDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: read runs directory: %v\n", err)
		os.Exit(1)
	}

	var p6Runs []os.DirEntry
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), "p6-") {
			p6Runs = append(p6Runs, e)
		}
	}
	if len(p6Runs) == 0 {
		fmt.Fprintf(os.Stderr, "ERROR: no p6-* runs found in %s\n", runsDir)
		os.Exit(1)
	}
	fmt.Printf("Found %d P6 runs\n", len(p6Runs))

	// Output directory
	outBase := filepath.Join("demo", domain, "publication-verification")
	totalRecords := 0

	for _, run := range p6Runs {
		runID := run.Name()
		availDir := filepath.Join(runsDir, runID, "available")
		snapshotPath := filepath.Join(availDir, "evidence_snapshot.json")
		reviewsPath := snapshotPath + ".doctrust_reviews.json"
		decisionPath := snapshotPath + ".decision.json"

		// Check required files exist
		hasRequired := true
		for _, p := range []string{snapshotPath, reviewsPath, decisionPath} {
			if _, err := os.Stat(p); os.IsNotExist(err) {
				fmt.Printf("  SKIP %s: missing %s\n", runID, filepath.Base(p))
				hasRequired = false
				break
			}
		}
		if !hasRequired {
			continue
		}

		fmt.Printf("\n=== Processing %s ===\n", runID)

		// Create output directory
		outDir := filepath.Join(outBase, runID, "available")
		if !dryRun {
			if err := os.MkdirAll(outDir, 0o755); err != nil {
				fmt.Fprintf(os.Stderr, "ERROR: create output dir: %v\n", err)
				os.Exit(1)
			}
		}

		// Copy all files from the historical run
		runDir := filepath.Join(runsDir, runID)
		if err := copyRun(runDir, filepath.Join(outBase, runID), outDir, dryRun); err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: copy run %s: %v\n", runID, err)
			os.Exit(1)
		}

		// Load original review records
		sidecar, err := review.LoadReviewsSidecar(reviewsPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: load reviews sidecar %s: %v\n", reviewsPath, err)
			os.Exit(1)
		}

		// Re-sign each record
		newRecords := make([]review.SignedReview, len(sidecar.Records))
		for i, rec := range sidecar.Records {
			newRec := review.SignedReview{
				CaseID:           rec.CaseID,
				GraphCaseID:      rec.GraphCaseID,
				SnapshotSHA256:   rec.SnapshotSHA256,
				FindingIndex:     rec.FindingIndex,
				Action:           rec.Action,
				Note:             rec.Note,
				ReviewerIdentity: rec.ReviewerIdentity,
				Channel:          rec.Channel,
				KeyID:            rec.KeyID,
				Alg:              rec.Alg,
				Ruleset:          rec.Ruleset,
				ResolvedAt:       rec.ResolvedAt,
			}
			if err := review.SignRecord(priv, &newRec, newRec.ResolvedAt); err != nil {
				fmt.Fprintf(os.Stderr, "ERROR: sign record %d: %v\n", i, err)
				os.Exit(1)
			}
			newRecords[i] = newRec
		}

		// Verify each new signature
		for i, rec := range newRecords {
			if err := review.VerifyRecord(pub, rec); err != nil {
				fmt.Fprintf(os.Stderr, "ERROR: verify new signature %d failed: %v\n", i, err)
				os.Exit(1)
			}
		}

		// Write re-signed sidecar
		newSidecar := &review.ReviewsSidecar{
			CaseID:  sidecar.CaseID,
			Records: newRecords,
		}
		if !dryRun {
			outReviewsPath := filepath.Join(outDir, "evidence_snapshot.json.doctrust_reviews.json")
			data, err := json.MarshalIndent(newSidecar, "", "  ")
			if err != nil {
				fmt.Fprintf(os.Stderr, "ERROR: marshal reviews: %v\n", err)
				os.Exit(1)
			}
			if err := os.WriteFile(outReviewsPath, data, 0o644); err != nil {
				fmt.Fprintf(os.Stderr, "ERROR: write reviews: %v\n", err)
				os.Exit(1)
			}

			// Rebuild audit artifact with re-signed reviews
			if err := rebuildAuditArtifact(outDir, domain, pub); err != nil {
				fmt.Fprintf(os.Stderr, "WARNING: rebuild audit for %s: %v (skipping)\n", runID, err)
			}
		}

		totalRecords += len(newRecords)
		fmt.Printf("  Re-signed %d records\n", len(newRecords))
		for _, rec := range newRecords {
			fmt.Printf("    finding[%d] %s by %s (key=%s)\n", rec.FindingIndex, rec.Action, rec.ReviewerIdentity, rec.KeyID)
		}
	}

	// Write public key
	if !dryRun {
		if err := os.MkdirAll(filepath.Dir(publicKeyOut), 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: create public key directory: %v\n", err)
			os.Exit(1)
		}
		pubData := base64.StdEncoding.EncodeToString(pub)
		if err := os.WriteFile(publicKeyOut, []byte(pubData+"\n"), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: write public key: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("\nWrote public key: %s\n", publicKeyOut)
	}

	fmt.Printf("\n=== Summary ===\n")
	fmt.Printf("Runs processed: %d\n", len(p6Runs))
	fmt.Printf("Records re-signed: %d\n", totalRecords)
	fmt.Printf("Output directory: %s\n", outBase)
	if dryRun {
		fmt.Printf("Mode: DRY RUN (no files written)\n")
	}
	fmt.Println("\nPublication-verification artifacts created successfully.")
}

// rebuildAuditArtifact rebuilds the audit artifact for a publication-verification run.
func rebuildAuditArtifact(availDir, domain string, pub ed25519.PublicKey) error {
	snapshotPath := filepath.Join(availDir, "evidence_snapshot.json")
	reviewsPath := snapshotPath + ".doctrust_reviews.json"
	auditPath := snapshotPath + ".audit.json"

	// Create service and load case
	svc, err := service.NewDocTrustService(domain, "rulesets")
	if err != nil {
		return fmt.Errorf("create service: %w", err)
	}
	ctx := context.Background()
	if err := svc.LoadCase(ctx, snapshotPath); err != nil {
		return fmt.Errorf("load case: %w", err)
	}
	if _, err := svc.Evaluate(ctx); err != nil {
		return fmt.Errorf("evaluate: %w", err)
	}

	// Load re-signed reviews
	sidecar, err := review.LoadReviewsSidecar(reviewsPath)
	if err != nil {
		return fmt.Errorf("load reviews: %w", err)
	}
	ring := map[string]ed25519.PublicKey{"owner": pub}
	if err := svc.LoadAuthorizedReviews(sidecar.Records, ring); err != nil {
		return fmt.Errorf("load authorized reviews: %w", err)
	}

	// Build and write artifact
	artifact, err := svc.BuildArtifact()
	if err != nil {
		return fmt.Errorf("build artifact: %w", err)
	}
	data, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal artifact: %w", err)
	}
	return os.WriteFile(auditPath, data, 0o644)
}

// loadPrivateKey loads an Ed25519 private key from a file.
// Supports both raw 64-byte files and base64-encoded files.
func loadPrivateKey(path, passphrase string) (ed25519.PrivateKey, ed25519.PublicKey, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, "", fmt.Errorf("read key file: %w", err)
	}

	// Try as encrypted key file first (JSON container)
	var kf struct {
		KeyID string `json:"key_id"`
	}
	if json.Unmarshal(data, &kf) == nil && kf.KeyID != "" {
		// It's an encrypted key file
		if passphrase == "" {
			return nil, nil, "", fmt.Errorf("passphrase required for encrypted key file")
		}
		keyID, pub, priv, err := review.LoadEncryptedPrivateKey(path, passphrase)
		if err != nil {
			return nil, nil, "", fmt.Errorf("decrypt key: %w", err)
		}
		return priv, pub, keyID, nil
	}

	// Try as base64-encoded raw key
	trimmed := strings.TrimSpace(string(data))
	if decoded, err := base64.StdEncoding.DecodeString(trimmed); err == nil && len(decoded) == ed25519.PrivateKeySize {
		priv := ed25519.PrivateKey(decoded)
		pub := priv.Public().(ed25519.PublicKey)
		return priv, pub, "operator", nil
	}

	// Try as raw bytes
	if len(data) == ed25519.PrivateKeySize {
		priv := ed25519.PrivateKey(data)
		pub := priv.Public().(ed25519.PublicKey)
		return priv, pub, "operator", nil
	}

	return nil, nil, "", fmt.Errorf("unsupported key format (expected Ed25519 private key, 64 bytes)")
}

// checkOwnerPubProvenance verifies the derived public key matches the existing owner.pub.
func checkOwnerPubProvenance(pubKeyPath string, derivedPub ed25519.PublicKey, force bool) error {
	existing, err := os.ReadFile(pubKeyPath)
	if os.IsNotExist(err) {
		fmt.Printf("No existing %s — will create\n", pubKeyPath)
		return nil
	}
	if err != nil {
		return fmt.Errorf("read existing public key: %w", err)
	}

	existingDecoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(existing)))
	if err != nil {
		return fmt.Errorf("decode existing public key: %w", err)
	}

	existingPub := ed25519.PublicKey(existingDecoded)
	if string(existingPub) == string(derivedPub) {
		fmt.Printf("Owner.pub matches derived public key — no replacement needed\n")
		return nil
	}

	if !force {
		return fmt.Errorf("owner.pub exists but differs from derived key; use --force-replace to override")
	}

	fmt.Printf("WARNING: replacing owner.pub (operator requested)\n")
	return nil
}

// copyRun copies all files from a historical run to the publication-verification directory.
func copyRun(srcDir, dstDir, availDir string, dryRun bool) error {
	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		dst := filepath.Join(dstDir, rel)
		if dryRun {
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, data, info.Mode())
	})
}
