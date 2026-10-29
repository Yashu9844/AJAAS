// Command jaas-pm is the command-line client for JAAS Projects & Tasks. It signs in with the user credentials (the same
// RBAC as the web console: the API decides what the user may do) and prints compact tables, or JSON with --json.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/jaas/jaas/pkg/pmclient"
)

const usage = `jaas-pm - JAAS Projects & Tasks from the terminal

  jaas-pm login --url URL --tenant SLUG --email EMAIL   (the password is read from JAAS_PM_PASSWORD or asked on stdin)
  jaas-pm projects [--query TEXT]
  jaas-pm tasks PROJECT [--column NAME] [--limit N]      PROJECT is a key (ENG) or an id
  jaas-pm task KEY-12
  jaas-pm create PROJECT "Title" [--priority P] [--description TEXT]
  jaas-pm move KEY-12 "Column name"
  jaas-pm comment KEY-12 "Text"
  jaas-pm time KEY-12 MINUTES [--note TEXT]
  jaas-pm mywork
  jaas-pm search TEXT

Add --json before the command to print JSON.`

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, argv []string, out *os.File) error {
	asJSON := false
	if len(argv) > 0 && argv[0] == "--json" {
		asJSON, argv = true, argv[1:]
	}
	if len(argv) == 0 || argv[0] == "help" || argv[0] == "-h" || argv[0] == "--help" {
		fmt.Fprintln(out, usage)
		return nil
	}
	path, err := pmclient.DefaultConfigPath()
	if err != nil {
		return err
	}
	if argv[0] == "login" {
		return login(ctx, path, argv[1:], out)
	}
	cfg, err := pmclient.LoadConfig(path)
	if err != nil {
		return err
	}
	c, err := pmclient.FromConfig(path, cfg)
	if err != nil {
		return err
	}
	return dispatch(ctx, c, argv, printer{out: out, json: asJSON})
}

func login(ctx context.Context, path string, argv []string, out *os.File) error {
	fs := flag.NewFlagSet("login", flag.ContinueOnError)
	url := fs.String("url", "http://localhost:8080", "API base URL")
	tenant := fs.String("tenant", "", "tenant slug")
	email := fs.String("email", "", "e-mail address")
	if err := fs.Parse(argv); err != nil {
		return err
	}
	if *tenant == "" || *email == "" {
		return fmt.Errorf("--tenant and --email are required")
	}
	pw := os.Getenv("JAAS_PM_PASSWORD")
	if pw == "" {
		fmt.Fprint(os.Stderr, "password: ")
		_, _ = fmt.Fscanln(os.Stdin, &pw)
	}
	c := pmclient.New(*url, *tenant)
	if err := c.Login(ctx, *email, strings.TrimSpace(pw)); err != nil {
		return err
	}
	if err := pmclient.SaveConfig(path, pmclient.Config{URL: *url, Tenant: *tenant, Tokens: c.Tokens()}); err != nil {
		return err
	}
	fmt.Fprintf(out, "signed in to %s as %s (credentials saved to %s)\n", *tenant, *email, path)
	return nil
}
