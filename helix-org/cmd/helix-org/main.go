// Helix Org CLI: runs the HTTP server. The first start of `serve`
// against an empty database creates the initial owner Worker. Beyond
// that, every mutation goes through MCP — point an MCP client (or the
// `chat` subcommand) at /workers/{id}/mcp on the running server.
package main

import (
	"fmt"
	"os"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		printUsage()
		return fmt.Errorf("no subcommand given")
	}
	switch args[0] {
	case "serve":
		return runServe(args[1:])
	case "chat":
		return runChat(args[1:])
	case "config":
		return runConfig(args[1:])
	case "bootstrap":
		return runBootstrap(args[1:])
	case "help", "-h", "--help":
		printUsage()
		return nil
	default:
		printUsage()
		return fmt.Errorf("unknown subcommand %q", args[0])
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, `usage: helix-org <subcommand> [flags]

Subcommands:
  serve       Run the HTTP server. On first start against an empty
              database, creates the initial owner Worker. Exposes
              /workers/{id}/mcp (Streamable HTTP MCP transport) and
              the /ui/ HTML surface.
  chat        Open an interactive claude session pointed at a
              Worker's MCP endpoint (default: w-owner). Continues
              the most recent session in the current directory;
              pass --resume for the picker or --new for a fresh one.
  bootstrap   Provision external dependencies. Run
              'bootstrap helix-runtime [--project-id <id>]' to
              validate a Helix project, run a smoke test, and
              persist the project ID. See design/helix-integration.md.
  config      Read or write operational configuration (transport
              credentials, claude binary, model, public URL, etc.).
              CLI-only — never via MCP. See design/config.md.
              Subcommands: set, get, list, delete.
  help        Show this message.

Run 'helix-org <subcommand> --help' for per-subcommand flags.`)
}
