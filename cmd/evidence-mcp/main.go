// evidence-mcp is a thin MCP stdio adapter over the trusted DocTrust ingest
// provider path (plan10 D6). It carries transport/orchestration only: no
// normalization logic, no policy semantics. It may import internal/ingest and
// internal/nutrient because it sits on the provider side of the R13 boundary
// (unlike internal/service and cmd/doctrust-mcp).
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	snapshotRoot := flag.String("snapshot-root", "", "allowed root for snapshot and document paths (env: DOCTRUST_SNAPSHOT_ROOT, default: cwd)")
	envFile := flag.String("env-file", "", "optional .env file parsed at runtime for provider credentials (values never logged; keeps secrets out of MCP registration config)")
	flag.Parse()

	root := resolveRoot(*snapshotRoot)
	envPath := *envFile
	if envPath == "" {
		envPath = defaultEnvFileNearExecutable()
	}
	loadEnvFile(envPath)

	server := mcp.NewServer(
		&mcp.Implementation{Name: "evidence-mcp", Version: "0.1.0"},
		&mcp.ServerOptions{Logger: slog.Default()},
	)

	registerTools(server, root)

	ctx := context.Background()
	if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil {
		slog.Error("server exited", "error", err)
		os.Exit(1)
	}
}
