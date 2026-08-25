package main

import (
	"context"
	"flag"
	"log/slog"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/PithomLabs/doctrust/internal/service"
)

func main() {
	snapshotRoot := flag.String("snapshot-root", "", "allowed root for snapshot paths (env: DOCTRUST_SNAPSHOT_ROOT, default: cwd)")
	domain := flag.String("domain", "income_verification", "compliance domain")
	rulesetsDir := flag.String("rulesets-dir", "rulesets", "path to promoted rulesets directory")
	flag.Parse()

	root := resolveSnapshotRoot(*snapshotRoot)

	svc, err := service.NewDocTrustService(*domain, *rulesetsDir)
	if err != nil {
		slog.Error("failed to initialize service", "error", err)
		os.Exit(1)
	}

	server := mcp.NewServer(
		&mcp.Implementation{Name: "doctrust-mcp", Version: "0.1.0"},
		&mcp.ServerOptions{
			Logger: slog.Default(),
		},
	)

	registerTools(server, svc, root)

	ctx := context.Background()
	// Fail-fast: malformed JSON-RPC input terminates the process.
	// The MCP client is expected to restart the server process.
	if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil {
		slog.Error("server exited", "error", err)
		os.Exit(1)
	}
}
