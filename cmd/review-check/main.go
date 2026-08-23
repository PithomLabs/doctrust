package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/doctrust/doctrust/internal/compiler"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: bin/review-check <candidate-dir>\n")
		os.Exit(1)
	}
	candidateDir := os.Args[1]

	// Load candidate
	candidate, err := compiler.LoadCandidate(candidateDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	state, _ := compiler.GetState(candidateDir)
	hasAdv, advName := compiler.HasAdversarial(candidateDir)

	// Display
	fmt.Println("═══════════════════════════════════════════")
	fmt.Printf("CANDIDATE CHECK: %s v%s\n", candidate.CheckID, candidate.Version)
	fmt.Println("═══════════════════════════════════════════")
	fmt.Println()
	fmt.Printf("DESCRIPTION:\n  %s\n\n", candidate.Description)

	fmt.Println("PARAMETERS:")
	for k, v := range candidate.Parameters {
		fmt.Printf("  %s: %v\n", k, v)
	}
	fmt.Println()

	fmt.Println("GO SOURCE:")
	fmt.Println("───────────────────────────────────────────")
	fmt.Println(candidate.GoSource)
	fmt.Println("───────────────────────────────────────────")
	fmt.Println()

	fmt.Printf("GENERATED SCENARIOS (%d):\n", len(candidate.Scenarios))
	for i, s := range candidate.Scenarios {
		status := "PASS"
		if s.Expected.Status != "" {
			status = s.Expected.Status
		}
		fmt.Printf("  %d. %s → %s\n", i+1, s.Name, status)
	}
	fmt.Println()

	fmt.Println("ADVERSARIAL SCENARIO:")
	if hasAdv {
		fmt.Printf("  ✓ Authored: %s\n", advName)
		// Display adversarial scenario content
		advPath := candidateDir + "/adversarial.yaml"
		if data, err := os.ReadFile(advPath); err == nil {
			fmt.Println("  ┌───────────────────────────────────────")
			for _, line := range strings.Split(string(data), "\n") {
				fmt.Printf("  │ %s\n", line)
			}
			fmt.Println("  └───────────────────────────────────────")
		}
	} else {
		fmt.Println("  ⚠ None authored yet")
		if candidate.AdversarialHint != "" {
			fmt.Printf("  Hint from LLM: %q\n", candidate.AdversarialHint)
		}
		fmt.Println("  You must author at least 1 adversarial scenario before approval.")
	}
	fmt.Println()

	fmt.Printf("STATE: %s\n", state)
	fmt.Println("═══════════════════════════════════════════")
	fmt.Println()

	// Pre-approval scenario execution
	canApprove := hasAdv
	if state == compiler.StateDraft || state == compiler.StateHumanReview {
		if hasAdv {
			fmt.Println("Executing scenarios for pre-approval verification...")
			snapshot, snapErr := compiler.SnapshotCandidate(candidateDir)
			if snapErr != nil {
				fmt.Fprintf(os.Stderr, "  Snapshot failed: %v\n", snapErr)
			} else {
				execResult, execErr := compiler.ExecuteCandidateScenarios(snapshot)
				if execErr != nil {
					fmt.Fprintf(os.Stderr, "  Scenario execution error: %v\n", execErr)
					if execResult != nil {
						for _, r := range execResult.Results {
							status := "PASS"
							if !r.Match {
								status = "FAIL"
							}
							fmt.Fprintf(os.Stderr, "    %s: expected=%s/%s actual=%s/%s [%s]\n",
								r.Name, r.Expected.Status, r.Expected.Severity,
								r.Actual.Status, r.Actual.Severity, status)
						}
					}
					canApprove = false
				} else if execResult != nil {
					fmt.Printf("  Scenarios: %d/%d passed\n", execResult.Passed, execResult.Total)
					for _, r := range execResult.Results {
						status := "PASS"
						if !r.Match {
							status = "FAIL"
						}
						fmt.Printf("    %s: expected=%s/%s actual=%s/%s [%s]\n",
							r.Name, r.Expected.Status, r.Expected.Severity,
							r.Actual.Status, r.Actual.Severity, status)
					}
					if execResult.Failed > 0 {
						fmt.Fprintf(os.Stderr, "  ✗ %d scenario(s) failed — approval blocked\n", execResult.Failed)
						canApprove = false
					} else {
						fmt.Println("  ✓ All scenarios pass")
					}
				}
			}
			fmt.Println()
		}
	}

	// Actions
	if state == compiler.StateDraft || state == compiler.StateHumanReview {
		if !hasAdv {
			fmt.Println("Actions:")
			fmt.Println("  [e] Edit       — open files for manual editing")
			fmt.Println("  [r] Reject     — mark as REJECTED, discard candidate")
			fmt.Println("  [q] Quit       — exit without changes")
			fmt.Println()

			reader := bufio.NewReader(os.Stdin)
			fmt.Print("> ")
			input, _ := reader.ReadString('\n')
			input = strings.TrimSpace(input)

			switch input {
			case "e":
				editor := os.Getenv("EDITOR")
				if editor == "" {
					editor = "vi"
				}
				fmt.Printf("Opening %s in %s...\n", candidateDir, editor)
				fmt.Println("(Editor integration not implemented in MVP — edit files manually)")
			case "r":
				compiler.SetState(candidateDir, compiler.StateRejected)
				fmt.Println("Candidate rejected.")
			case "q":
				fmt.Println("Exiting without changes.")
			default:
				fmt.Println("Invalid option.")
			}
		} else if canApprove {
			fmt.Println("Actions:")
			fmt.Println("  [a] Approve    — mark as APPROVED, proceed to validation")
			fmt.Println("  [e] Edit       — open files for manual editing")
			fmt.Println("  [r] Reject     — mark as REJECTED, discard candidate")
			fmt.Println("  [q] Quit       — exit without changes")
			fmt.Println()

			reader := bufio.NewReader(os.Stdin)
			fmt.Print("> ")
			input, _ := reader.ReadString('\n')
			input = strings.TrimSpace(input)

			switch input {
			case "a":
				// P1-B: explicit human adversarial confirmation.
				// Re-display the FULL adversarial scenario and require an
				// affirmative y/Y before any approval state is written.
				advData, advReadErr := os.ReadFile(filepath.Join(candidateDir, "adversarial.yaml"))
				fmt.Println()
				fmt.Println("───────────────────────────────────────────")
				fmt.Println("ADVERSARIAL SCENARIO (full content):")
				fmt.Println("───────────────────────────────────────────")
				if advReadErr != nil {
					fmt.Println("  (unreadable:", advReadErr, ")")
					fmt.Println("Approval cancelled.")
					return
				}
				fmt.Println(string(advData))
				fmt.Println("───────────────────────────────────────────")
				fmt.Println()
				fmt.Print("Confirm adversarial expectations? [y/N] > ")
				confirmInput, _ := reader.ReadString('\n')
				confirm := strings.TrimSpace(confirmInput)
				if confirm != "y" && confirm != "Y" {
					fmt.Printf("Approval cancelled (received %q). No approval was written.\n", confirm)
					return
				}

				reviewerID := os.Getenv("DOCTRUST_REVIEWER")
				if reviewerID == "" {
					reviewerID = compiler.GetCurrentUser()
				}
				compiler.SetState(candidateDir, compiler.StateHumanReview)
				if err := compiler.WriteApproval(candidateDir, candidate.CheckID, candidate.Version, reviewerID); err != nil {
					fmt.Fprintf(os.Stderr, "Error writing approval: %v\n", err)
					os.Exit(1)
				}
				compiler.SetState(candidateDir, compiler.StateApproved)
				fmt.Printf("Candidate approved by %s. Run bin/promote-check to validate and promote.\n", reviewerID)
			case "e":
				editor := os.Getenv("EDITOR")
				if editor == "" {
					editor = "vi"
				}
				fmt.Printf("Opening %s in %s...\n", candidateDir, editor)
				fmt.Println("(Editor integration not implemented in MVP — edit files manually)")
			case "r":
				compiler.SetState(candidateDir, compiler.StateRejected)
				fmt.Println("Candidate rejected.")
			case "q":
				fmt.Println("Exiting without changes.")
			default:
				fmt.Println("Invalid option.")
			}
		} else {
			fmt.Println("Approval blocked: scenarios failed or adversarial scenarios missing.")
			fmt.Println("Actions:")
			fmt.Println("  [e] Edit       — open files for manual editing")
			fmt.Println("  [r] Reject     — mark as REJECTED, discard candidate")
			fmt.Println("  [q] Quit       — exit without changes")
			fmt.Println()

			reader := bufio.NewReader(os.Stdin)
			fmt.Print("> ")
			input, _ := reader.ReadString('\n')
			input = strings.TrimSpace(input)

			switch input {
			case "e":
				editor := os.Getenv("EDITOR")
				if editor == "" {
					editor = "vi"
				}
				fmt.Printf("Opening %s in %s...\n", candidateDir, editor)
				fmt.Println("(Editor integration not implemented in MVP — edit files manually)")
			case "r":
				compiler.SetState(candidateDir, compiler.StateRejected)
				fmt.Println("Candidate rejected.")
			case "q":
				fmt.Println("Exiting without changes.")
			default:
				fmt.Println("Invalid option.")
			}
		}
	} else {
		fmt.Printf("Candidate is in state %s — no actions available.\n", state)
	}
}
