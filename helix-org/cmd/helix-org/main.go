// Helix Org CLI: runs the HTTP server and bootstraps the initial owner
// Worker. Beyond bootstrap, mutations are made via MCP — point an MCP
// client at /workers/{id}/mcp on the running server.
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
  serve       Run the HTTP server. Exposes one endpoint:
              /workers/{id}/mcp (Streamable HTTP MCP transport).
  bootstrap   Create the initial owner Worker by writing directly to
              the SQLite store. Run before 'serve'. Pass
              --install-claude-mcp to register the owner's MCP
              endpoint with the local claude CLI so plain 'claude'
              sessions can drive the org.
  help        Show this message.

Run 'helix-org <subcommand> --help' for per-subcommand flags.`)
}
