package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// runChat exec's a `claude` session pointed at a Worker's MCP endpoint,
// so a human can drive the org from the perspective of that Worker.
//
// By default the session is interactive and resumes the most recent
// conversation in the current directory. We do this by reading the
// per-cwd `.jsonl` files claude writes under
// `~/.claude/projects/<cwd-with-slashes-replaced>/`, picking the newest,
// and passing its session id to `--resume <sid>` — see
// `latestClaudeSessionID()` below. We deliberately do NOT use claude's
// own `--continue` flag because it refuses some resumable sessions in
// practice. Pass `-p`/`--print` for one-shot use:
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
			// Resume the latest session for this cwd by explicit ID. We
			// avoid `--continue` because its "most recent resumable session"
			// heuristic refuses sessions whose log ended on certain non-user
			// events ("No conversation found to continue"), even when the
			// session is fine to resume by ID. If there is no prior session,
			// we pass nothing and claude starts fresh.
			if sid := latestClaudeSessionID(); sid != "" {
				cmd = append(cmd, "--resume", sid)
			}
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

// latestClaudeSessionID returns the sessionId of the most recently
// modified .jsonl in claude's per-cwd session store, or "" if none.
// Claude stores sessions under ~/.claude/projects/<cwd-with-slashes-
// as-hyphens>/<uuid>.jsonl, with one JSON event per line (the first
// line carries the session's `sessionId`).
func latestClaudeSessionID() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	dir := filepath.Join(home, ".claude", "projects", strings.ReplaceAll(cwd, "/", "-"))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var (
		newestPath string
		newestTime time.Time
	)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(newestTime) {
			newestTime = info.ModTime()
			newestPath = filepath.Join(dir, e.Name())
		}
	}
	if newestPath == "" {
		return ""
	}
	f, err := os.Open(newestPath) //nolint:gosec // path is built from a known prefix and a directory entry name
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	if !scanner.Scan() {
		return ""
	}
	var record struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
		return ""
	}
	return record.SessionID
}
