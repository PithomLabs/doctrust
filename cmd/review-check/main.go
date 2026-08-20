package main

import (
	"bufio"
	"fmt"
	"os"
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
				// In a real implementation, this would open the editor
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
				compiler.SetState(candidateDir, compiler.StateHumanReview)
				if err := compiler.WriteApproval(candidateDir, candidate.CheckID, candidate.Version); err != nil {
					fmt.Fprintf(os.Stderr, "Error writing approval: %v\n", err)
					os.Exit(1)
				}
				compiler.SetState(candidateDir, compiler.StateApproved)
				fmt.Println("Candidate approved. Run bin/promote-check to validate and promote.")
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
