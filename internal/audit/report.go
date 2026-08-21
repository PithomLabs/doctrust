package audit

import (
	"bytes"
	"fmt"

	"github.com/jung-kurt/gofpdf/v2"
)

func GenerateAuditReport(a *Artifact) ([]byte, error) {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.AddPage()

	pdf.SetFont("Helvetica", "B", 20)
	pdf.Cell(0, 12, "DocTrust Audit Report")
	pdf.Ln(16)

	pdf.SetFont("Helvetica", "", 11)
	pdf.SetTextColor(100, 100, 100)
	pdf.Cell(0, 7, fmt.Sprintf("Generated: %s", a.CreatedAt.Format("2006-01-02 15:04:05 UTC")))
	pdf.Ln(8)

	pdf.SetTextColor(0, 0, 0)
	pdf.SetFont("Helvetica", "B", 13)
	pdf.Cell(0, 8, "Case Information")
	pdf.Ln(10)

	pdf.SetFont("Helvetica", "", 11)
	info := []struct{ k, v string }{
		{"Policy ID", a.PolicyID},
		{"Policy Hash", a.PolicyHash},
		{"Ruleset ID", a.RulesetID},
		{"Ruleset Version", a.RulesetVersion},
		{"Ruleset Hash", a.RulesetHash},
		{"Version", a.Version},
		{"Documents", fmt.Sprintf("%d", len(a.Documents))},
		{"Decisions", fmt.Sprintf("%d", len(a.Decisions))},
		{"Human Reviews", fmt.Sprintf("%d", len(a.HumanReviews))},
		{"Final Disposition", a.FinalDisposition},
	}
	for _, i := range info {
		pdf.SetFont("Helvetica", "B", 10)
		pdf.Cell(50, 7, i.k)
		pdf.SetFont("Helvetica", "", 10)
		pdf.Cell(0, 7, i.v)
		pdf.Ln(7)
	}
	pdf.Ln(4)

	pdf.SetFont("Helvetica", "B", 13)
	pdf.Cell(0, 8, "Documents")
	pdf.Ln(10)

	pdf.SetFont("Helvetica", "", 10)
	for _, d := range a.Documents {
		pdf.Cell(60, 6, d.FileName)
		pdf.Cell(30, 6, d.DocType)
		hash := d.Hash
		if len(hash) > 16 {
			hash = hash[:16] + "..."
		}
		pdf.Cell(0, 6, hash)
		pdf.Ln(6)
	}
	pdf.Ln(4)

	if len(a.Decisions) > 0 {
		pdf.SetFont("Helvetica", "B", 13)
		pdf.Cell(0, 8, "Policy Decision")
		pdf.Ln(10)

		for _, d := range a.Decisions {
			pdf.SetFont("Helvetica", "B", 11)
			pdf.Cell(0, 7, fmt.Sprintf("State: %s", d.State))
			pdf.Ln(8)

			if len(d.Findings) > 0 {
				pdf.SetFont("Helvetica", "", 10)
				for _, f := range d.Findings {
					pdf.Cell(5, 5, "-")
					pdf.Cell(0, 5, fmt.Sprintf("%s (%s): %s vs %s", f.Rule, f.Severity, f.ClaimA, f.ClaimB))
					pdf.Ln(5)
				}
			}
		}
		pdf.Ln(4)
	}

	if len(a.HumanReviews) > 0 {
		pdf.SetFont("Helvetica", "B", 13)
		pdf.Cell(0, 8, "Human Reviews")
		pdf.Ln(10)

		pdf.SetFont("Helvetica", "", 10)
		for _, r := range a.HumanReviews {
			pdf.Cell(0, 6, fmt.Sprintf("Finding #%d: %s at %s", r.FindingIndex, r.Action, r.ResolvedAt.Format("15:04:05")))
			pdf.Ln(6)
			if r.Note != "" {
				pdf.SetFont("Helvetica", "I", 9)
				pdf.Cell(10, 5, "")
				pdf.Cell(0, 5, fmt.Sprintf("Note: %s", r.Note))
				pdf.Ln(5)
				pdf.SetFont("Helvetica", "", 10)
			}
		}
		pdf.Ln(4)
	}

	pdf.SetFont("Helvetica", "B", 13)
	pdf.Cell(0, 8, "Artifact Integrity")
	pdf.Ln(10)

	pdf.SetFont("Helvetica", "", 9)
	pdf.SetTextColor(100, 100, 100)
	pdf.MultiCell(0, 5, fmt.Sprintf("Artifact Hash (SHA-256): %s", a.Manifest.ArtifactHash), "", "", false)
	pdf.Ln(3)

	if a.CompletedAt != nil {
		pdf.Cell(0, 5, fmt.Sprintf("Completed: %s", a.CompletedAt.Format("2006-01-02 15:04:05 UTC")))
		pdf.Ln(5)
	}

	if len(a.Signatures) > 0 {
		pdf.Ln(3)
		pdf.SetFont("Helvetica", "B", 13)
		pdf.SetTextColor(0, 0, 0)
		pdf.Cell(0, 8, "Digital Signatures")
		pdf.Ln(10)

		pdf.SetFont("Helvetica", "", 10)
		for _, sig := range a.Signatures {
			pdf.Cell(0, 6, fmt.Sprintf("%s signed at %s", sig.Algorithm, sig.SignedAt.Format("15:04:05")))
			pdf.Ln(6)
			pdf.SetFont("Helvetica", "I", 8)
			pdf.Cell(0, 5, sig.Hash)
			pdf.Ln(5)
			pdf.SetFont("Helvetica", "", 10)
		}
	}

	var buf bytes.Buffer
	err := pdf.Output(&buf)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
