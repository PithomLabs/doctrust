package main

// doctrust-review is the HUMAN-ONLY authority channel (plans12 P6-1/P6-3a).
// It refuses non-interactive execution and is the sole writer of
// HumanReviewRecords. The agent-facing MCP surface has no review capability.

import (
	"os"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--provision" {
		runProvision(os.Args[2:])
		return
	}
	runReview()
}
