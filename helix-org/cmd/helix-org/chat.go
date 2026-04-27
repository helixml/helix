package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

// runChat exec's a `claude` session pointed at a Worker's MCP endpoint,
// so a human can drive the org from the perspective of that Worker.
//
// By default the session is interactive and continues the most recent
// conversation in the current directory (claude's per-cwd session store
// handles persistence). Pass `-p`/`--print` for one-shot use:
//
//	helix-org chat                          # interactive, restorable
//	helix-org chat --new                    # fresh interactive session
//	helix-org chat --resume                 # interactive picker
//	helix-org chat -p "publish 'hi' on s-x" # one-shot, prints + exits
//	echo "..." | helix-org chat -p          # one-shot from stdin
//
// The default Worker is `w-owner` — what bootstrap produced — but any
// Worker ID can be passed. The MCP config is built inline, so this
// command works regardless of which entries the user has registered
// with their local `claude` CLI.
func runChat(args []string) error {
	fs := flag.NewFlagSet("chat", flag.ContinueOnError)
	workerID := fs.String("worker", "w-owner", "Worker to act as; the chat connects to that Worker's MCP endpoint.")
	serverURL := fs.String("server-url", "http://localhost:8080", "Base URL of the running helix-org server.")
	claudeBin := fs.String("claude-bin", "claude", "Path to the claude CLI.")
	newSession := fs.Bool("new", false, "Start a fresh interactive session instead of continuing the most recent one.")
	resume := fs.Bool("resume", false, "Open claude's interactive session picker instead of continuing the most recent one.")
	model := fs.String("model", "", "Claude model to pass via --model (empty = let claude choose).")
	printMode := fs.Bool("p", false, "Non-interactive: send the positional prompt (or stdin) and exit. Mirrors `claude -p`.")
	fs.BoolVar(printMode, "print", false, "Alias for -p.")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *newSession && *resume {
		return fmt.Errorf("--new and --resume are mutually exclusive")
	}
	if *printMode && (*newSession || *resume) {
		return fmt.Errorf("-p is mutually exclusive with --new and --resume")
	}
	if strings.TrimSpace(*workerID) == "" {
		return fmt.Errorf("--worker must not be empty")
	}

	mcpURL := fmt.Sprintf("%s/workers/%s/mcp", strings.TrimRight(*serverURL, "/"), *workerID)
	mcpConfig, err := json.Marshal(map[string]any{
		"mcpServers": map[string]any{
			"helix": map[string]string{
				"type": "http",
				"url":  mcpURL,
			},
		},
	})
	if err != nil {
		return fmt.Errorf("marshal mcp config: %w", err)
	}

	// Build the claude invocation. In `-p` mode the prompt must come
	// before `--mcp-config`, otherwise claude's flag parser greedily
	// consumes the positional as another mcp config file.
	cmd := []string{*claudeBin}
	if *printMode {
		cmd = append(cmd, "-p")
		if positional := fs.Args(); len(positional) > 0 {
			cmd = append(cmd, strings.Join(positional, " "))
		}
		// If no positional was given, claude reads stdin until EOF.
	}
	cmd = append(cmd,
		"--permission-mode", "bypassPermissions",
		"--strict-mcp-config",
		"--mcp-config", string(mcpConfig),
	)
	if *model != "" {
		cmd = append(cmd, "--model", *model)
	}
	if !*printMode {
		cmd = append(cmd, "--name", "helix-org: "+*workerID)
		switch {
		case *resume:
			cmd = append(cmd, "--resume")
		case !*newSession:
			cmd = append(cmd, "--continue")
		}
	}

	binPath, err := exec.LookPath(*claudeBin)
	if err != nil {
		return fmt.Errorf("locate claude %q: %w", *claudeBin, err)
	}
	if err := syscall.Exec(binPath, cmd, os.Environ()); err != nil { //nolint:gosec // claudeBin is operator-supplied
		return fmt.Errorf("exec claude: %w", err)
	}
	return nil
}
