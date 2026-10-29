// Command jaas-pm-mcp is a Model Context Protocol server (stdio) for JAAS Projects & Tasks. Any MCP client (Claude, other
// models, IDE agents) can use it. It acts as the signed-in user: credentials come from the config written by
// `jaas-pm login` (or the JAAS_PM_CONFIG file), so the API enforces that user's permissions on every tool call.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/jaas/jaas/pkg/pmclient"
	"github.com/jaas/jaas/pkg/pmmcp"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "jaas-pm-mcp:", err) // stdout carries the protocol, so diagnostics go to stderr
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	path, err := pmclient.DefaultConfigPath()
	if err != nil {
		return err
	}
	cfg, err := pmclient.LoadConfig(path)
	if err != nil {
		return err
	}
	client, err := pmclient.FromConfig(path, cfg)
	if err != nil {
		return err
	}
	return pmmcp.New(client, "jaas-pm", "1.0.0").Serve(ctx, os.Stdin, os.Stdout)
}
