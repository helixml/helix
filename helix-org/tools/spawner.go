package tools

import (
	"bufio"
	"context"
	_ "embed"
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

	"github.com/helixml/helix-org/broadcast"
	"github.com/helixml/helix-org/domain"
	"github.com/helixml/helix-org/store"
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
	EventID  domain.EventID
	StreamID domain.StreamID
	Source   domain.WorkerID
	// SourceKind is the WorkerKind ("human" / "ai") of Source — looked
	// up by the dispatcher at fan-out time and rendered into the
	// activation prompt so the recipient can apply the org-wide policy
	// (agent.md) of de-prioritising AI-origin events. Empty when the
	// event has no internal Source (system-emitted, or inbound from an
	// external transport with no resolved Worker).
	SourceKind domain.WorkerKind
	// Message is the canonical envelope parsed from the event body.
	// Every populated field (From, Subject, ThreadID, MessageID,
	// Extra, …) is rendered into the activation prompt so the
	// Worker can branch on transport-shaped metadata directly,
	// without a separate read_events round-trip.
	Message   domain.Message
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
	// Model, if non-empty, is passed to claude as --model. Aliases like
	// "sonnet" or "opus" resolve to the latest model in that family.
	Model string
	// Effort, if non-empty, is passed to claude as --effort. Valid
	// values are low|medium|high|xhigh|max. Defaults to "low" upstream
	// because every Worker activation runs claude headlessly and a
	// run-away effort budget is what blew up the first multi-agent
	// demo — keeping the floor low contains cost without preventing
	// operators from cranking it up per deployment.
	Effort string
	// Logger receives spawn bookkeeping. Must be non-nil.
	Logger *slog.Logger

	// Store, Broadcaster, Now and NewID are used to publish per-message
	// activation events to the Worker's activation Stream
	// (s-activations-<workerID>). Store and NewID and Now are required;
	// Broadcaster is optional (long-poll observers won't wake without it).
	Store       *store.Store
	Broadcaster *broadcast.Broadcaster
	Now         Clock
	NewID       IDGen
}

// mcpServerName is the key under which the helix MCP server is registered
// in each Worker's mcp.json. Tool names surface in Claude as
// mcp__<mcpServerName>__<toolName>.
const mcpServerName = "helix"

// activationStreamID returns the deterministic Stream ID where a Worker's
// activation transcript is published — assistant text, tool calls, tool
// results, lifecycle markers. One Stream per Worker; created at hire
// time by hire_worker, written to by the Spawner, read by anyone with a
// subscription (typically the hiring Worker).
func activationStreamID(workerID domain.WorkerID) domain.StreamID {
	return domain.StreamID("s-activations-" + string(workerID))
}

// agentMDContent is the fixed entry-point text projected into every
// Worker's Environment at activation time. The Spawner writes it to
// `agent.md`; Claude reads it. Same for every Worker — not per-hire
// state, not editable by Roles. It's the org-wide policy on how to
// be an agent: speaking discipline, log.md hygiene, AI-origin vs
// human-origin handling. Lives in a template file so it can be
// edited like prose without touching code.
//
//go:embed templates/agent.md
var agentMDContent string

// ClaudeSpawner returns a Spawner that runs `claude -p` in the new
// Worker's Environment directory and BLOCKS until claude exits. The
// dispatcher is responsible for serialising calls per Worker.
//
// State lives in the domain (DB), not on disk. Before exec'ing claude,
// the Spawner projects current state into the Environment as three
// markdown files:
//
//   - role.md     — the canonical Role.Content read from the store.
//     `update_role` rewrites the DB row; the next activation picks it
//     up here.
//   - identity.md — the Worker's IdentityContent read from the store.
//     `update_identity` rewrites the DB row; same lazy projection.
//   - agent.md    — agentMDContent (templates/agent.md), the fixed
//     org-wide policy on speaking discipline, log.md hygiene, and
//     AI-origin vs human-origin handling. Same for every Worker.
//
// This is the single seam that knows "how to make role/identity visible
// to a worker." Local envs write files (today). When envs eventually
// go remote (SSH targets, container exec, prompt-only), only this
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
// Worker's activation Stream (s-activations-<workerID>). Observers
// (typically the hiring Worker, auto-subscribed at hire) watch via
// read_events on that Stream — the same primitive every other read
// flows through. There is no on-disk transcript.
func ClaudeSpawner(cfg ClaudeSpawnerConfig) Spawner {
	return func(ctx context.Context, workerID domain.WorkerID, envPath string, trigger Trigger) error {
		if err := projectEnv(ctx, cfg.Store, workerID, envPath); err != nil {
			return fmt.Errorf("project env for %s: %w", workerID, err)
		}

		mcpConfigPath, err := writeMCPConfig(envPath, cfg.PublicURL, workerID)
		if err != nil {
			return fmt.Errorf("write mcp config: %w", err)
		}

		prompt := buildPrompt(workerID, agentMDContent, trigger)

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

		streamID := activationStreamID(workerID)
		publish := func(body string) {
			publishActivationEvent(ctx, cfg, workerID, streamID, body)
		}

		// Mark the start of this activation on the stream so consecutive
		// activations are easy to tell apart for an observer reading
		// events. The trigger description matches what callers see when
		// inspecting their hires.
		publish(fmt.Sprintf("=== activation: %s ===", describeTrigger(trigger)))

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
			"trigger", trigger.Kind,
			"stream", streamID,
		)

		// Drain stderr in the background so the pipe doesn't block.
		stderrDone := make(chan struct{})
		go func() {
			defer close(stderrDone)
			scanner := bufio.NewScanner(stderrR)
			for scanner.Scan() {
				publish("stderr: " + oneLine(scanner.Text(), 500))
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
//
// A Worker may hold multiple positions; we project the role of the
// first listed. In practice today every Worker has exactly one
// position (hire_worker only ever assigns one). If multi-position
// Workers become real, this is the seam to revisit.
func projectEnv(ctx context.Context, s *store.Store, workerID domain.WorkerID, envPath string) error {
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
	if err := writeEnvFile(envPath, "agent.md", agentMDContent); err != nil {
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
func publishActivationEvent(ctx context.Context, cfg ClaudeSpawnerConfig, workerID domain.WorkerID, streamID domain.StreamID, body string) {
	if cfg.Store == nil || cfg.NewID == nil || cfg.Now == nil || body == "" {
		return
	}
	event, err := domain.NewMessageEvent(
		domain.EventID("e-"+cfg.NewID()),
		streamID,
		workerID,
		domain.Message{From: string(workerID), Body: body},
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
	if cfg.Broadcaster != nil {
		cfg.Broadcaster.Notify(streamID)
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

func okOr(s string) string {
	if s == "" {
		return "ok"
	}
	return s
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
		ctx.WriteString(renderTrigger(trigger))
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

// renderTrigger formats an event-kind Trigger for the activation
// prompt. Every populated field of the canonical Message envelope is
// rendered so the Worker can branch on Subject, From, ThreadID, Extra,
// etc. directly — no separate read_events round-trip needed for the
// trigger event itself. Empty fields are omitted to keep the prompt
// tight.
//
// Header keys are aligned for legibility but the parser the Worker is
// going to apply (Claude reading the prompt) is robust to spacing, so
// "neat" is for humans tailing the prompt.
func renderTrigger(t Trigger) string {
	var b strings.Builder
	b.WriteString("A new event arrived on a Stream you subscribe to.\n\n")
	fmt.Fprintf(&b, "  stream:      %s\n", t.StreamID)
	fmt.Fprintf(&b, "  event:       %s\n", t.EventID)
	fmt.Fprintf(&b, "  time:        %s\n", t.CreatedAt.Format(time.RFC3339))
	if t.Source != "" {
		fmt.Fprintf(&b, "  source:      %s\n", t.Source)
	}
	// source_kind drives the agent.md priority rule: AI-origin events
	// are low-priority by default. Always emit when known (even when
	// Source itself is empty — a future inbound transport that can
	// classify origin without resolving a Worker still needs to flag
	// AI vs human here).
	if t.SourceKind != "" {
		fmt.Fprintf(&b, "  source_kind: %s\n", t.SourceKind)
	}
	m := t.Message
	if m.From != "" {
		fmt.Fprintf(&b, "  from:        %s\n", m.From)
	}
	if len(m.To) > 0 {
		fmt.Fprintf(&b, "  to:          %s\n", strings.Join(m.To, ", "))
	}
	if m.Subject != "" {
		fmt.Fprintf(&b, "  subject:     %s\n", m.Subject)
	}
	if m.ThreadID != "" {
		fmt.Fprintf(&b, "  thread_id:   %s\n", m.ThreadID)
	}
	if m.InReplyTo != "" {
		fmt.Fprintf(&b, "  in_reply_to: %s\n", m.InReplyTo)
	}
	if m.MessageID != "" {
		fmt.Fprintf(&b, "  message_id:  %s\n", m.MessageID)
	}
	if m.Body != "" {
		b.WriteString("  body:\n")
		b.WriteString(indentBlock(m.Body, "    "))
		b.WriteByte('\n')
	}
	if len(m.Extra) > 0 {
		b.WriteString("  extra:\n")
		b.WriteString(indentBlock(string(m.Extra), "    "))
		b.WriteByte('\n')
	}
	return b.String()
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
			publish(oneLine(string(line), 500))
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
		return []string{fmt.Sprintf("%s: %s", tag, oneLine(ev.Result, 500))}
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
					out = append(out, fmt.Sprintf("assistant: %s", oneLine(seg.Text, 500)))
				}
			case "tool_use":
				out = append(out, fmt.Sprintf("tool_use %s: %s", seg.Name, oneLine(string(seg.Input), 500)))
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
			out = append(out, fmt.Sprintf("%s: %s", tag, oneLine(string(seg.Content), 500)))
		}
		return out
	}
	return nil
}

// oneLine collapses whitespace and clips to max runes for readability.
func oneLine(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if max > 0 && len(s) > max {
		return s[:max] + "…"
	}
	return s
}
