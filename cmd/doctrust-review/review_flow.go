package main

// Core review flow with injected I/O (unit-testable); TTY gating lives in
// runReview(). The command can ONLY author signed records — finalization
// belongs exclusively to DocTrust (plans12 P6-7).

import (
	"context"
	"crypto/ed25519"

	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	osStdlib "os"
	"path/filepath"
	"strings"
	"time"

	"github.com/doctrust/doctrust/internal/review"
	"github.com/doctrust/doctrust/internal/service"
)

type reviewIO struct {
	in       io.Reader
	out      io.Writer
	secretFn func(prompt string) (string, error) // no-echo read
}

type reviewOptions struct {
	SnapshotPath string // evidence_snapshot.json inside the trusted root
	Domain       string
	RulesetsDir  string
	Reviewer     string
	KeyDir       string
}

func runReview() {
	fs := flag.NewFlagSet("doctrust-review", flag.ExitOnError)
	snapshot := fs.String("snapshot", "", "path to evidence_snapshot.json (its .decision.json sidecar is consumed)")
	domain := fs.String("domain", "shipment_release", "compliance domain")
	rulesetsDir := fs.String("rulesets-dir", defaultRulesetsDir(), "promoted rulesets directory")
	reviewer := fs.String("reviewer", "", "reviewer name / key id (defaults to OS user)")
	keyDir := fs.String("key-dir", defaultKeyDir(), "directory holding the passphrase-encrypted private key")
	requireTTY := fs.Bool("tty", true, "require an interactive terminal (never disable in production)")
	fs.Parse(osStdlib.Args[1:])

	if *requireTTY && !isInteractive() {
		fmt.Fprintln(osStdlib.Stderr,
			"FATAL: human review requires an interactive terminal (TTY).")
		fmt.Fprintln(osStdlib.Stderr,
			"Refusing to proceed: this channel is reserved for the human authority.")
		osStdlib.Exit(1)
	}

	reviewerName := *reviewer
	if reviewerName == "" {
		reviewerName = osUser()
	}

	ioh := &reviewIO{in: osStdlib.Stdin, out: osStdlib.Stdout,
		secretFn: func(p string) (string, error) { return readSecret(p) }}

	err := runReviewFlow(ioh, reviewOptions{
		SnapshotPath: *snapshot,
		Domain:       *domain,
		RulesetsDir:  *rulesetsDir,
		Reviewer:     reviewerName,
		KeyDir:       *keyDir,
	})
	exitOn(err)
}

// runReviewFlow is the testable core.
func runReviewFlow(ioh *reviewIO, opts reviewOptions) error {
	if opts.SnapshotPath == "" {
		return fmt.Errorf("--snapshot is required")
	}
	decisionPath := opts.SnapshotPath + ".decision.json"
	raw, err := osStdlib.ReadFile(decisionPath)
	if err != nil {
		return fmt.Errorf("read decision sidecar %s: %w (evaluate_case first)", decisionPath, err)
	}
	var sidecar service.DecisionSidecar
	if err := json.Unmarshal(raw, &sidecar); err != nil {
		return fmt.Errorf("parse decision sidecar: %w", err)
	}
	if len(sidecar.Findings) == 0 {
		return fmt.Errorf("decision sidecar carries no findings")
	}

	fmt.Fprintf(ioh.out, "\nHuman authority required — case %s\n", sidecar.LoadcaseID)
	fmt.Fprintf(ioh.out, "Ruleset: %s v%s (hash %.12s…)\n",
		sidecar.Ruleset.ID, sidecar.Ruleset.Version, sidecar.Ruleset.Hash)
	fmt.Fprintf(ioh.out, "Snapshot sha256: %.16s…\n", sidecar.SnapshotSHA256)
	for _, f := range sidecar.Findings {
		fmt.Fprintf(ioh.out, "  [%d] %-32s %s / %s\n", f.Index, f.CheckID, f.Status, f.Severity)
		fmt.Fprintf(ioh.out, "      %s\n", oneLine(f.Reason))
		for _, ev := range f.Evidence {
			fmt.Fprintf(ioh.out, "      · %s @ %s (conf %.2f)\n",
				ev.Field, ev.SourceSpan, ev.Confidence)
		}
	}
	fmt.Fprintln(ioh.out)

	pass, err := ioh.secretFn("reviewer passphrase (unlocks signing key): ")
	if err != nil {
		return err
	}
	keyID, pub, priv, err := loadReviewerKey(
		filepath.Join(opts.KeyDir, opts.Reviewer+".key.enc"), pass)
	if err != nil {
		return fmt.Errorf("signing key unavailable: %w", err)
	}
	fmt.Fprintf(ioh.out, "signing key unlocked: key_id=%q\n", keyID)

	lr := lineReader{r: ioh.in}
	var records []review.SignedReview
	for {
		fmt.Fprintf(ioh.out, "\nfinding index to resolve (or 'done'): ")
		line, _ := lr.ReadString('\n')
		line = strings.TrimSpace(line)
		if line == "" || strings.EqualFold(line, "done") {
			break
		}
		var idx int
		if _, err := fmt.Sscanf(line, "%d", &idx); err != nil {
			fmt.Fprintln(ioh.out, "  not a number; try again")
			continue
		}
		var found *service.SidecarFinding
		for i := range sidecar.Findings {
			if sidecar.Findings[i].Index == idx {
				found = &sidecar.Findings[i]
			}
		}
		if found == nil {
			fmt.Fprintln(ioh.out, "  unknown finding index")
			continue
		}
		fmt.Fprintf(ioh.out, "action for [%d] %s — confirm/reject/override (or 'skip'): ",
			idx, found.CheckID)
		actLine, _ := lr.ReadString('\n')
		action := strings.TrimSpace(strings.ToLower(actLine))
		switch action {
		case "confirm", "reject", "override":
		case "skip":
			continue
		default:
			fmt.Fprintln(ioh.out, "  invalid action; try again")
			continue
		}
		fmt.Fprintf(ioh.out, "note (optional): ")
		noteLine, _ := lr.ReadString('\n')

		// Explicit typed consent: re-enter the finding index.
		fmt.Fprintf(ioh.out, "type the finding index %d to sign this decision: ", idx)
		consent, _ := lr.ReadString('\n')
		if strings.TrimSpace(consent) != line {
			fmt.Fprintln(ioh.out, "  consent index mismatch; finding skipped")
			continue
		}

		rec := review.SignedReview{
			CaseID:           sidecar.LoadcaseID,
			GraphCaseID:      sidecar.GraphCaseID,
			SnapshotSHA256:   sidecar.SnapshotSHA256,
			FindingIndex:     idx,
			Action:           review.FindingAction(action),
			Note:             strings.TrimSpace(noteLine),
			ReviewerIdentity: opts.Reviewer,
			Channel:          review.ChannelHumanTTY,
			KeyID:            keyID,
			Ruleset:          sidecar.Ruleset,
		}
		if err := review.SignRecord(priv, &rec, time.Now().UTC()); err != nil {
			return fmt.Errorf("sign: %w", err)
		}
		records = append(records, rec)
		fmt.Fprintf(ioh.out, "  signed (%d record(s) pending write)\n", len(records))
	}

	if len(records) == 0 {
		return errors.New("no records signed — nothing written")
	}

	sidecarReviews := opts.SnapshotPath + ".doctrust_reviews.json"
	for _, rec := range records {
		if err := review.AppendSignedRecord(sidecarReviews, sidecar.LoadcaseID, rec); err != nil {
			return fmt.Errorf("persist signed record: %w", err)
		}
	}
	fmt.Fprintf(ioh.out, "\n%d signed record(s) written to:\n  %s\n", len(records), sidecarReviews)

	// Finalization (P6-7): this command IS part of DocTrust — it deterministically
	// replays the evaluation, loads the signed records through the same
	// fail-closed verification used everywhere else, and seals the audit
	// artifact. The agent-facing MCP surface has no such capability.
	svc, err := service.NewDocTrustService(opts.Domain, opts.RulesetsDir)
	if err != nil {
		return fmt.Errorf("service: %w", err)
	}
	ctx := context.Background()
	if err := svc.LoadCase(ctx, opts.SnapshotPath); err != nil {
		return fmt.Errorf("load case: %w", err)
	}
	d, err := svc.Evaluate(ctx)
	if err != nil {
		return fmt.Errorf("re-evaluate: %w", err)
	}
	ring := map[string]ed25519.PublicKey{keyID: pub}
	if err := svc.LoadAuthorizedReviews(records, ring); err != nil {
		return fmt.Errorf("authorize records: %w", err)
	}
	artifact, err := svc.BuildArtifact()
	if err != nil {
		return fmt.Errorf("build artifact: %w", err)
	}
	data, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		return err
	}
	auditPath := opts.SnapshotPath + ".audit.json"
	if err := osStdlib.WriteFile(auditPath, data, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(ioh.out, "\nFINAL DISPOSITION: %s\n", artifact.FinalDisposition)
	fmt.Fprintf(ioh.out, "artifact: %s\nartifact_hash: %s\n", auditPath, artifact.Manifest.ArtifactHash)
	_ = d
	return nil
}

func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > 160 {
		s = s[:157] + "..."
	}
	return s
}

// lineReader wraps an io.Reader with byte-at-a-time line assembly so no
// read-ahead ever steals a later prompt's input.
type lineReader struct{ r io.Reader }

func (l lineReader) ReadString(delim byte) (string, error) {
	var sb strings.Builder
	b := make([]byte, 1)
	for {
		n, err := l.r.Read(b)
		if n > 0 {
			sb.WriteByte(b[0])
			if b[0] == delim {
				return sb.String(), nil
			}
		}
		if err != nil {
			return sb.String(), err
		}
	}
}
