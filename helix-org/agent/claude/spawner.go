// Package claude is the local-development Spawner runtime: it embodies
// each AI Worker activation by exec'ing the `claude` CLI in the
// Worker's Environment directory and streaming its stream-json output
// onto the Worker's activation Stream.
//
// This runtime is the dev-mode counterpart to agent/helix. The Helix
// runtime is the production target; claude is what you reach for when
// you want to drive the org graph end-to-end without standing up a
// Helix server.
package claude

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

	"github.com/helixml/helix/api/pkg/org/activation"
	"github.com/helixml/helix/api/pkg/org/broadcast"
	"github.com/helixml/helix/api/pkg/org/event"
	"github.com/helixml/helix/api/pkg/org/message"
	"github.com/helixml/helix/api/pkg/org/runtime"
	"github.com/helixml/helix/api/pkg/org/stream"
	"github.com/helixml/helix/api/pkg/org/worker"
	"github.com/helixml/helix/helix-org/agent"
	"github.com/helixml/helix/helix-org/domain"
	"github.com/helixml/helix/helix-org/store"
)

// SpawnerConfig configures the claude-backed Spawner.
type SpawnerConfig struct {
	// ClaudeBin is the path to the claude CLI (e.g. "claude").
	ClaudeBin string
	// PublicURL is the base URL the spawned agent uses to reach the
	// helix-org MCP endpoint. Each Worker's tools are exposed at
	// PublicURL + "/workers/{workerID}/mcp".
	PublicURL string
	// Model, if non-empty, is passed to claude as --model. Aliases like
	// "sonnet" or "opus" resolve to the latest model in that family.
	Model string
	// Effort, if non-empty, is passed to claude as --effort. Valid
	// values are low|medium|high|xhigh|max.
	Effort string
	// Logger receives spawn bookkeeping. Must be non-nil.
	Logger *slog.Logger

	// Store, Hub, Now and NewID are used to publish per-message
	// activation events to the Worker's activation Stream
	// (s-activations-<workerID>). Store and NewID and Now are required;
	// Hub is optional (long-poll observers won't wake without it).
	Store *store.Store
	Hub   *broadcast.Hub
	Now   func() time.Time
	NewID func() string
}

// mcpServerName is the key under which the helix MCP server is registered
// in each Worker's mcp.json. Tool names surface in Claude as
// mcp__<mcpServerName>__<toolName>.
const mcpServerName = "helix"

// Spawner returns an runtime.Spawner that runs `claude -p` in the new
// Worker's Environment directory and BLOCKS until claude exits. The
// dispatcher is responsible for serialising calls per Worker.
//
// State lives in the domain (DB), not on disk. Before exec'ing claude,
// the Spawner projects current state into the Environment as three
// markdown files:
//
//   - role.md     — the canonical Role.Content read from the store.
//   - identity.md — the Worker's IdentityContent read from the store.
//   - agent.md    — agent.Policy, the fixed org-wide policy on speaking
//     discipline, log.md hygiene, and AI-origin vs human-origin handling.
//
// This is the single seam that knows "how to make role/identity visible
// to a worker." Local envs write files (today). When envs eventually go
// remote (SSH targets, container exec, prompt-only), only this
// projection step swaps strategy — tools and bootstrap don't change.
//
// Tools are exposed to the agent over MCP. Per activation the Spawner
// writes <envPath>/mcp.json pointing at /workers/<id>/mcp on the helix
// server and passes --mcp-config + --strict-mcp-config so claude only
// sees the helix tools and not the user's machine-wide config.
//
// Claude is run with --output-format stream-json so every assistant
// message, tool call, and tool result flows through a parser in this
// process that publishes one Event per atomic message segment to the
// Worker's activation Stream. Observers (typically the hiring Worker,
// auto-subscribed at hire) watch via read_events on that Stream.
func Spawner(cfg SpawnerConfig) runtime.Spawner {
	return func(ctx context.Context, workerID worker.ID, envPath string, triggers []activation.Trigger) error {
		if len(triggers) == 0 {
			return fmt.Errorf("spawner invoked with no triggers")
		}
		if err := projectEnv(ctx, cfg.Store, workerID, envPath); err != nil {
			return fmt.Errorf("project env for %s: %w", workerID, err)
		}

		mcpConfigPath, err := writeMCPConfig(envPath, cfg.PublicURL, workerID)
		if err != nil {
			return fmt.Errorf("write mcp config: %w", err)
		}

		prompt := agent.BuildPrompt(workerID, agent.Policy, triggers)

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
		if cfg.Effort != "" {
			args = append(args, "--effort", cfg.Effort)
		}

		cmd := exec.CommandContext(ctx, cfg.ClaudeBin, args...) //nolint:gosec // spawning claude with generated prompt is this Spawner's purpose
		cmd.Dir = envPath
		cmd.Env = append(os.Environ(),
			"HELIX_WORKER_ID="+string(workerID),
		)

		streamID := agent.ActivationStreamID(workerID)
		publish := func(body string) {
			publishActivationEvent(ctx, cfg, workerID, streamID, body)
		}

		// Mark the start of this activation on the stream so consecutive
		// activations are easy to tell apart for an observer reading
		// events. The trigger description matches what callers see when
		// inspecting their hires.
		publish(fmt.Sprintf("=== activation: %s ===", agent.DescribeTriggers(triggers)))

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return fmt.Errorf("stdout pipe: %w", err)
		}
		// Claude's stderr is rare and usually a hard failure (bad flag,
		// missing binary). Fold it into the activation stream so it's
		// visible alongside the rest of the transcript.
		stderrR, err := cmd.StderrPipe()
		if err != nil {
			return fmt.Errorf("stderr pipe: %w", err)
		}
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

		if err := cmd.Start(); err != nil {
			return fmt.Errorf("start claude: %w", err)
		}
		pid := cmd.Process.Pid
		cfg.Logger.Info("spawned claude",
			"worker", workerID,
			"pid", pid,
			"env", envPath,
			"trigger", triggers[0].Kind,
			"triggers", len(triggers),
			"stream", streamID,
		)

		// Drain stderr in the background so the pipe doesn't block.
		stderrDone := make(chan struct{})
		go func() {
			defer close(stderrDone)
			scanner := bufio.NewScanner(stderrR)
			for scanner.Scan() {
				publish("stderr: " + agent.OneLine(scanner.Text(), 500))
			}
		}()

		// Parse stream-json synchronously (blocks until stdout closes).
		streamTranscript(stdout, publish)
		<-stderrDone

		err = cmd.Wait()
		publish(fmt.Sprintf("=== exit: %s ===", okOr(errString(err))))
		cfg.Logger.Info("claude exited",
			"worker", workerID,
			"pid", pid,
			"err", errString(err),
		)
		return err
	}
}

// projectEnv writes the current canonical state of a Worker — role,
// identity, and the fixed agent.md entry stub — into envPath. Called
// once per activation, just before claude is exec'd. Reads from the
// domain (DB); writes to disk (env). The DB is the source of truth;
// disk is a per-activation projection.
func projectEnv(ctx context.Context, s *store.Store, workerID worker.ID, envPath string) error {
	if s == nil {
		return fmt.Errorf("spawner has no store")
	}
	worker, err := s.Workers.Get(ctx, workerID)
	if err != nil {
		return fmt.Errorf("get worker: %w", err)
	}
	positions := worker.Positions()
	if len(positions) == 0 {
		return fmt.Errorf("worker %s has no positions", workerID)
	}
	pos, err := s.Positions.Get(ctx, positions[0])
	if err != nil {
		return fmt.Errorf("get position: %w", err)
	}
	role, err := s.Roles.Get(ctx, pos.RoleID)
	if err != nil {
		return fmt.Errorf("get role: %w", err)
	}
	if err := writeEnvFile(envPath, "role.md", role.Content); err != nil {
		return err
	}
	if err := writeEnvFile(envPath, "identity.md", worker.IdentityContent()); err != nil {
		return err
	}
	if err := writeEnvFile(envPath, "agent.md", agent.Policy); err != nil {
		return err
	}
	return nil
}

// writeEnvFile writes content to a file inside a Worker's Environment
// directory. The mode is 0o600 — these files describe behaviour and
// identity and shouldn't be world-readable.
func writeEnvFile(envPath, name, content string) error {
	full := filepath.Join(envPath, name)
	if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
		return fmt.Errorf("write %q: %w", full, err)
	}
	return nil
}

// publishActivationEvent appends one Event to the Worker's activation
// Stream and notifies long-poll observers. It does NOT go through the
// dispatcher: per-message events would otherwise re-trigger any
// subscribed AI Worker on every line, which would be unbounded. The
// Worker themselves is intentionally never subscribed to their own
// activation stream for the same reason.
//
// All errors are logged and swallowed; a transient SQLite hiccup must
// not abort the activation.
func publishActivationEvent(ctx context.Context, cfg SpawnerConfig, workerID worker.ID, streamID stream.ID, body string) {
	if cfg.Store == nil || cfg.NewID == nil || cfg.Now == nil || body == "" {
		return
	}
	event, err := domain.NewMessageEvent(
		event.ID("e-"+cfg.NewID()),
		streamID,
		workerID,
		message.Message{From: string(workerID), Body: body},
		cfg.Now(),
	)
	if err != nil {
		cfg.Logger.Warn("activation event: build", "worker", workerID, "err", err)
		return
	}
	if err := cfg.Store.Events.Append(ctx, event); err != nil {
		cfg.Logger.Warn("activation event: append", "worker", workerID, "err", err)
		return
	}
	if cfg.Hub != nil {
		cfg.Hub.Notify(streamID)
	}
}

// writeMCPConfig writes a per-worker mcp.json into envPath wiring claude
// to the worker's MCP endpoint. Returning the path keeps the caller
// honest about pointing --mcp-config at a real file.
func writeMCPConfig(envPath, publicURL string, workerID worker.ID) (string, error) {
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

func okOr(s string) string {
	if s == "" {
		return "ok"
	}
	return s
}

// streamTranscript reads newline-delimited JSON from r (claude's stdout)
// and calls publish once per atomic message segment — assistant text,
// tool call, tool result, system init, run result. Lines that don't
// parse as JSON are published verbatim so nothing is silently dropped.
func streamTranscript(r io.Reader, publish func(body string)) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		var ev streamEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			publish(agent.OneLine(string(line), 500))
			continue
		}
		for _, body := range renderEvent(ev) {
			publish(body)
		}
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
		publish(fmt.Sprintf("[stream] scanner error: %v", err))
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

// renderEvent turns one parsed stream-json line into zero or more
// transcript bodies — one per atomic segment. Each becomes its own
// Event on the Worker's activation Stream.
func renderEvent(ev streamEvent) []string {
	switch ev.Type {
	case "system":
		if ev.Subtype == "init" {
			return []string{"--- session start ---"}
		}
	case "result":
		tag := "result"
		if ev.IsError {
			tag = "result-error"
		}
		return []string{fmt.Sprintf("%s: %s", tag, agent.OneLine(ev.Result, 500))}
	case "assistant":
		var msg messagePayload
		if err := json.Unmarshal(ev.Message, &msg); err != nil {
			return nil
		}
		var out []string
		for _, seg := range msg.Content {
			switch seg.Type {
			case "text":
				if seg.Text != "" {
					out = append(out, fmt.Sprintf("assistant: %s", agent.OneLine(seg.Text, 500)))
				}
			case "tool_use":
				out = append(out, fmt.Sprintf("tool_use %s: %s", seg.Name, agent.OneLine(string(seg.Input), 500)))
			}
		}
		return out
	case "user":
		var msg messagePayload
		if err := json.Unmarshal(ev.Message, &msg); err != nil {
			return nil
		}
		var out []string
		for _, seg := range msg.Content {
			if seg.Type != "tool_result" {
				continue
			}
			tag := "tool_result"
			if seg.IsError {
				tag = "tool_result-error"
			}
			out = append(out, fmt.Sprintf("%s: %s", tag, agent.OneLine(string(seg.Content), 500)))
		}
		return out
	}
	return nil
}
