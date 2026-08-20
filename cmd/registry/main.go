package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/doctrust/doctrust/internal/eval"
)

func main() {
	domain := flag.String("domain", "", "show details for a specific ruleset domain")
	rulesetsDir := flag.String("dir", "rulesets", "rulesets directory")
	flag.Parse()

	registry := eval.NewRegistry(*rulesetsDir)

	ids, err := registry.ListRulesets()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing rulesets: %v\n", err)
		os.Exit(1)
	}

	if *domain != "" {
		// Show details for one domain
		showDomain(registry, *rulesetsDir, *domain)
		return
	}

	// List all domains
	if len(ids) == 0 {
		fmt.Println("No rulesets found.")
		return
	}

	fmt.Println("RULESETS")
	fmt.Println("========")
	for _, id := range ids {
		showDomain(registry, *rulesetsDir, id)
		fmt.Println()
	}
}

func showDomain(registry *eval.Registry, dir, id string) {
	// Show promoted versions
	versionedFiles, _ := filepath.Glob(filepath.Join(dir, id, "v*.yaml"))
	var versions []string
	for _, f := range versionedFiles {
		base := filepath.Base(f)
		ver := strings.TrimSuffix(base, ".yaml")
		versions = append(versions, ver)
	}

	// Check for working draft
	workingPath := filepath.Join(dir, id, "working.yaml")
	hasWorking := false
	if _, err := os.Stat(workingPath); err == nil {
		hasWorking = true
	}

	// Show latest promoted
	latest, err := registry.LoadPromoted(id)
	latestVer := "?"
	latestChecks := 0
	if err == nil {
		latestVer = latest.Version
		latestChecks = len(latest.Checks)
	}

	fmt.Printf("  %s\n", id)
	if hasWorking {
		fmt.Printf("    [draft]  (has working changes)\n")
	}
	if len(versions) > 0 {
		fmt.Printf("    versions: %s\n", strings.Join(versions, ", "))
		fmt.Printf("    latest:   v%s (%d checks)\n", latestVer, latestChecks)
	} else {
		fmt.Printf("    (no promoted versions)\n")
	}

	// Show manifest details for latest
	manifestPath := filepath.Join(dir, id, fmt.Sprintf("%s.manifest.json", latestVer))
	if _, err := os.Stat(manifestPath); err == nil {
		m, err := eval.LoadManifest(manifestPath)
		if err == nil {
			fmt.Printf("    hash:     %s\n", m.Hash[:16]+"...")
			fmt.Printf("    promoted: %s\n", m.PromotedAt.Format("2006-01-02 15:04:05"))
		}
	}
}
