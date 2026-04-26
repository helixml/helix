package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/helixml/helix-org/domain"
)

// TriggerKind discriminates why a Spawner is being invoked.
type TriggerKind string

const (
	// TriggerHire fires once when a Worker is first created.
	TriggerHire TriggerKind = "hire"
	// TriggerEvent fires whenever a Worker receives an event on a Stream
	// they subscribe to.
	TriggerEvent TriggerKind = "event"
)

// Trigger is the per-activation context the Spawner gives to the agent.
// The mandate (entry-point file contents) is the static role; Trigger is
// what just happened that woke this Worker up.
type Trigger struct {
	Kind TriggerKind

	// Event fields, set when Kind == TriggerEvent.
	EventID   domain.EventID
	StreamID  domain.StreamID
	Source    domain.WorkerID
	Body      string
	CreatedAt time.Time
}

// ClaudeSpawnerConfig configures the claude-backed Spawner.
type ClaudeSpawnerConfig struct {
	// ClaudeBin is the path to the claude CLI (e.g. "claude").
	ClaudeBin string
	// PublicURL is the base URL the spawned agent uses to reach the
	// helix-org MCP endpoint. Each Worker's tools are exposed at
	// PublicURL + "/workers/{workerID}/mcp".
	PublicURL string
	// Model, if non-empty, is passed to claude as --model.
	Model string
	// Logger receives spawn bookkeeping. Must be non-nil.
	Logger *slog.Logger
}

// mcpServerName is the key under which the helix MCP server is registered
// in each Worker's mcp.json. Tool names surface in Claude as
// mcp__<mcpServerName>__<toolName>.
const mcpServerName = "helix"

// ClaudeSpawner returns a Spawner that runs `claude -p` in the new
// Worker's Environment directory and BLOCKS until claude exits. The
// dispatcher is responsible for serialising calls per Worker.
//
// The Worker's identity, role, and activation flow live as three
// markdown files in envPath, written by hire_worker:
//
//   - role.md     — the Role's canonical content (job description,
//     channels, duties). Updated by the owner via update_role.
//   - identity.md — the Worker's per-hire identity (name, voice,
//     stance). Set at hire, immutable thereafter.
//   - agent.md    — a fixed entry-point stub instructing the agent
//     to read role.md and identity.md and act on the trigger.
//
// The Spawner reads agent.md and embeds it in the prompt; Claude reads
// role.md and identity.md from cwd as its first action.
//
// Tools are exposed to the agent over MCP. Per activation the Spawner
// writes <envPath>/mcp.json pointing at /workers/<id>/mcp on the helix
// server and passes --mcp-config + --strict-mcp-config so claude only
// sees the helix tools and not the user's machine-wide config.
//
// Claude is run with --output-format stream-json so every assistant
// message, tool call, and tool result flows through a parser in this
// process that writes a human-readable transcript to
// <envPath>/activation.log, alongside the raw JSONL in
// <envPath>/activation.jsonl for anyone who wants to dig in.
func ClaudeSpawner(cfg ClaudeSpawnerConfig) Spawner {
	return func(ctx context.Context, workerID domain.WorkerID, envPath string, trigger Trigger) error {
		entryFile := filepath.Join(envPath, "agent.md")
		mandate, err := os.ReadFile(entryFile) //nolint:gosec // path sourced from trusted server state
		if err != nil {
			return fmt.Errorf("read entry-point %q: %w", entryFile, err)
		}

		mcpConfigPath, err := writeMCPConfig(envPath, cfg.PublicURL, workerID)
		if err != nil {
			return fmt.Errorf("write mcp config: %w", err)
		}

		prompt := buildPrompt(workerID, string(mandate), trigger)

		args := []string{
			"-p", prompt,
			"--permission-mode", "bypassPermissions",
			"--output-format", "stream-json",
			"--verbose",
			"--mcp-config", mcpConfigPath,
			"--strict-mcp-config",
		}
		if cfg.Model != "" {
			args = append(args, "--model", cfg.Model)
		}

		cmd := exec.CommandContext(ctx, cfg.ClaudeBin, args...) //nolint:gosec // spawning claude with generated prompt is this Spawner's purpose
		cmd.Dir = envPath
		cmd.Env = append(os.Environ(),
			"HELIX_WORKER_ID="+string(workerID),
		)

		// Pretty transcript + raw JSONL, side by side.
		prettyPath := filepath.Join(envPath, "activation.log")
		rawPath := filepath.Join(envPath, "activation.jsonl")
		pretty, err := os.OpenFile(prettyPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600) //nolint:gosec // path under trusted env dir
		if err != nil {
			return fmt.Errorf("open %q: %w", prettyPath, err)
		}
		defer func() { _ = pretty.Close() }()
		raw, err := os.OpenFile(rawPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600) //nolint:gosec // path under trusted env dir
		if err != nil {
			return fmt.Errorf("open %q: %w", rawPath, err)
		}
		defer func() { _ = raw.Close() }()

		// Stamp a separator into the pretty log so consecutive activations
		// are easy to tell apart.
		_, _ = fmt.Fprintf(pretty, "\n[%s] === activation: %s ===\n",
			time.Now().Format("15:04:05"), describeTrigger(trigger))

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return fmt.Errorf("stdout pipe: %w", err)
		}
		cmd.Stderr = pretty
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

		if err := cmd.Start(); err != nil {
			return fmt.Errorf("start claude: %w", err)
		}
		pid := cmd.Process.Pid
		cfg.Logger.Info("spawned claude",
			"worker", workerID,
			"pid", pid,
			"env", envPath,
			"trigger", trigger.Kind,
			"log", prettyPath,
		)

		// Parse stream-json synchronously (blocks until stdout closes).
		streamTranscript(stdout, raw, pretty)

		err = cmd.Wait()
		cfg.Logger.Info("claude exited",
			"worker", workerID,
			"pid", pid,
			"err", errString(err),
		)
		return err
	}
}

// writeMCPConfig writes a per-worker mcp.json into envPath wiring claude
// to the worker's MCP endpoint. Returning the path keeps the caller
// honest about pointing --mcp-config at a real file.
func writeMCPConfig(envPath, publicURL string, workerID domain.WorkerID) (string, error) {
	cfg := struct {
		MCPServers map[string]mcpServerEntry `json:"mcpServers"`
	}{
		MCPServers: map[string]mcpServerEntry{
			mcpServerName: {
				Type: "http",
				URL:  fmt.Sprintf("%s/workers/%s/mcp", strings.TrimRight(publicURL, "/"), workerID),
			},
		},
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal mcp config: %w", err)
	}
	path := filepath.Join(envPath, "mcp.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", fmt.Errorf("write %q: %w", path, err)
	}
	return path, nil
}

type mcpServerEntry struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// buildPrompt assembles the per-activation prompt: identity hint +
// agent.md contents + the specific trigger that woke the Worker up.
// Tools are exposed natively via MCP under the "helix" server (tool
// names appear as mcp__helix__<name>); Claude figures the rest out
// from tools/list.
func buildPrompt(workerID domain.WorkerID, mandate string, trigger Trigger) string {
	var ctx strings.Builder
	switch trigger.Kind {
	case TriggerHire:
		ctx.WriteString("You have just been hired. This is your first activation. Complete any one-time setup your role describes, then exit. The runtime will re-activate you when an event arrives on a Stream you subscribe to.\n")
	case TriggerEvent:
		fmt.Fprintf(&ctx, "A new event arrived on a Stream you subscribe to.\n\n  stream: %s\n  source: %s\n  time:   %s\n  body:\n%s\n",
			trigger.StreamID, trigger.Source, trigger.CreatedAt.Format(time.RFC3339), indentBlock(trigger.Body, "    "))
	default:
		fmt.Fprintf(&ctx, "Activation kind: %q.\n", trigger.Kind)
	}

	return fmt.Sprintf(`You are Worker %s, running inside helix-org. Your environment is
the current working directory. Each activation is a single turn — do
the work and exit.

%s

=== Trigger ===
%s=== end trigger ===

Act now. No preamble.
`, workerID, mandate, ctx.String())
}

// indentBlock prefixes every line of s with prefix. Used so multi-line
// event bodies render readably inside the prompt.
func indentBlock(s, prefix string) string {
	if s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}

func describeTrigger(t Trigger) string {
	switch t.Kind {
	case TriggerHire:
		return "hire"
	case TriggerEvent:
		return fmt.Sprintf("event %s on %s from %s", t.EventID, t.StreamID, t.Source)
	default:
		return string(t.Kind)
	}
}

// streamTranscript reads newline-delimited JSON from r (claude's stdout)
// and writes both the raw JSONL to rawOut and a human-readable transcript
// to prettyOut. Any lines that fail to parse as JSON are echoed verbatim
// to prettyOut so nothing ever gets silently dropped.
func streamTranscript(r io.Reader, rawOut, prettyOut io.Writer) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		_, _ = rawOut.Write(line)
		_, _ = rawOut.Write([]byte("\n"))

		var ev streamEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			_, _ = fmt.Fprintf(prettyOut, "%s\n", line)
			continue
		}
		renderPretty(prettyOut, ev)
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
		_, _ = fmt.Fprintf(prettyOut, "[stream] scanner error: %v\n", err)
	}
}

// streamEvent captures the parts of claude's stream-json format we care
// about for the transcript.
type streamEvent struct {
	Type    string          `json:"type"`
	Subtype string          `json:"subtype,omitempty"`
	Message json.RawMessage `json:"message,omitempty"`
	Result  string          `json:"result,omitempty"`
	IsError bool            `json:"is_error,omitempty"`
}

type messagePayload struct {
	Role    string           `json:"role"`
	Content []contentSegment `json:"content"`
}

type contentSegment struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
}

func renderPretty(w io.Writer, ev streamEvent) {
	ts := time.Now().Format("15:04:05")
	switch ev.Type {
	case "system":
		if ev.Subtype == "init" {
			_, _ = fmt.Fprintf(w, "[%s] --- session start ---\n", ts)
		}
	case "result":
		tag := "result"
		if ev.IsError {
			tag = "result-error"
		}
		_, _ = fmt.Fprintf(w, "[%s] %s: %s\n", ts, tag, oneLine(ev.Result, 500))
	case "assistant":
		var msg messagePayload
		if err := json.Unmarshal(ev.Message, &msg); err != nil {
			return
		}
		for _, seg := range msg.Content {
			switch seg.Type {
			case "text":
				if seg.Text != "" {
					_, _ = fmt.Fprintf(w, "[%s] assistant: %s\n", ts, oneLine(seg.Text, 500))
				}
			case "tool_use":
				_, _ = fmt.Fprintf(w, "[%s] tool_use %s: %s\n", ts, seg.Name, oneLine(string(seg.Input), 500))
			}
		}
	case "user":
		var msg messagePayload
		if err := json.Unmarshal(ev.Message, &msg); err != nil {
			return
		}
		for _, seg := range msg.Content {
			if seg.Type != "tool_result" {
				continue
			}
			tag := "tool_result"
			if seg.IsError {
				tag = "tool_result-error"
			}
			_, _ = fmt.Fprintf(w, "[%s] %s: %s\n", ts, tag, oneLine(string(seg.Content), 500))
		}
	}
}

// oneLine collapses whitespace and clips to max runes for readability.
func oneLine(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if max > 0 && len(s) > max {
		return s[:max] + "…"
	}
	return s
}
