package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// runPrompt is the human Owner's primary interface to a running
// helix-org server. It spawns a Claude Code instance pointed at a
// Worker's MCP endpoint and hands it the user's prompt; Claude figures
// out which helix tools to call.
//
// Usage:
//
//	helix-org prompt "create the roles defined in ./roles/"
//	echo "publish a hello message to c-general" | helix-org prompt
//
// Defaults the acting Worker to w-owner. Use --as to act as another
// Worker (whose grants will then constrain what Claude can do).
func runPrompt(args []string) error {
	fs := flag.NewFlagSet("prompt", flag.ContinueOnError)
	url := fs.String("url", "http://localhost:8080", "Server base URL")
	as := fs.String("as", "w-owner", "Worker ID to act as. Claude only sees the tools this Worker holds grants for.")
	claudeBin := fs.String("claude-bin", "claude", "Path to the claude CLI")
	model := fs.String("model", "", "Claude model to pass via --model (empty = let claude choose)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	prompt, err := readPromptInput(fs.Args())
	if err != nil {
		return err
	}

	mcpConfig, err := mcpConfigJSON(*url, *as)
	if err != nil {
		return err
	}

	cmdArgs := []string{
		"-p", prompt,
		"--mcp-config", mcpConfig,
		"--strict-mcp-config",
		"--permission-mode", "bypassPermissions",
	}
	if *model != "" {
		cmdArgs = append(cmdArgs, "--model", *model)
	}

	cmd := exec.Command(*claudeBin, cmdArgs...) //nolint:gosec // user-driven CLI; the prompt is the user's own input
	// claude warns when -p mode gets no stdin within a few seconds. We
	// already have the prompt, so close stdin to make the silence
	// intentional rather than waiting for nothing.
	cmd.Stdin = nil
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// readPromptInput pulls the prompt from positional args (joined with a
// space) or, if none, from stdin. Empty input is an error — without a
// prompt there's nothing for Claude to do.
func readPromptInput(args []string) (string, error) {
	if len(args) > 0 {
		return strings.Join(args, " "), nil
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", fmt.Errorf("read stdin: %w", err)
	}
	prompt := strings.TrimSpace(string(data))
	if prompt == "" {
		return "", fmt.Errorf("no prompt provided (give it as a positional arg or pipe it on stdin)")
	}
	return prompt, nil
}

// mcpConfigJSON renders the inline --mcp-config payload pointing claude
// at the given Worker's MCP endpoint. Claude's --mcp-config flag
// accepts either a path or a JSON string; inline keeps us out of the
// temp-file business.
func mcpConfigJSON(serverURL, workerID string) (string, error) {
	doc := struct {
		MCPServers map[string]struct {
			Type string `json:"type"`
			URL  string `json:"url"`
		} `json:"mcpServers"`
	}{
		MCPServers: map[string]struct {
			Type string `json:"type"`
			URL  string `json:"url"`
		}{
			"helix": {
				Type: "http",
				URL:  fmt.Sprintf("%s/workers/%s/mcp", strings.TrimRight(serverURL, "/"), workerID),
			},
		},
	}
	data, err := json.Marshal(doc)
	if err != nil {
		return "", fmt.Errorf("marshal mcp config: %w", err)
	}
	return string(data), nil
}
