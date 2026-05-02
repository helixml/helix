package tools

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/helixml/helix-org/broadcast"
	"github.com/helixml/helix-org/domain"
	"github.com/helixml/helix-org/store"
	"github.com/helixml/helix-org/tools/helixclient"
)

// HelixSpawnerConfig wires the helix-backed Spawner. The Client is
// injectable so tests can run without HTTP. Defaults are applied for
// any unset duration / capacity.
//
// In the per-Worker-project model, the spawner does not hold a
// `ProjectID` of its own — every AI Worker gets its own Helix
// project, applied at hire time and persisted on the Worker domain
// row (`Worker.HelixProjectID`).
type HelixSpawnerConfig struct {
	Client      helixclient.Client
	HelixOrgURL string // forwarded to project secrets so the in-sandbox agent can reach helix-org's MCP server
	// Provider/Model/Runtime drive the project's Agent App config.
	// Each Worker's project is applied with these values; a future
	// Role-overrides system can pass them per-Worker.
	Provider string
	Model    string
	Runtime  string // "claude_code" by default
	// AgentMD is the org-wide agent.md policy text pushed to
	// `.context/agent.md` on each per-Worker project's helix-specs
	// branch. The spawner's activation prompt tells every Worker to
	// read it first. Embedded by main.go from bootstrap_agent.md.
	AgentMD string
	// OrgID is the Helix organisation each per-Worker project lives
	// under. Empty for personal accounts.
	OrgID             string
	ActivationTimeout time.Duration
	MaxInflight       int
	PollInitial       time.Duration // default 250ms
	PollMax           time.Duration // default 30s
	Logger            *slog.Logger
	Store             *store.Store
	Broadcaster       *broadcast.Broadcaster
	Now               Clock
	NewID             IDGen
}

// HelixSpawner returns a Spawner that runs each activation as a
// long-lived Helix chat session. Either a fresh one (first activation
// or stale pointer) or a follow-up message on the Worker's already-
// open session.
//
// The Spawner does five things, in order: build the prompt, take a
// global semaphore slot, ensure a live session exists, open the live
// transcript WebSocket, then poll for completion. New transcript
// segments arriving on the WebSocket are diffed against a per-call
// dedup map and republished onto s-activations-<workerID> in the
// same shape ClaudeSpawner emits, so observers see one transcript
// format regardless of backend.
func HelixSpawner(cfg HelixSpawnerConfig) Spawner {
	if cfg.PollInitial == 0 {
		cfg.PollInitial = 250 * time.Millisecond
	}
	if cfg.PollMax == 0 {
		cfg.PollMax = 30 * time.Second
	}
	if cfg.ActivationTimeout == 0 {
		cfg.ActivationTimeout = 5 * time.Minute
	}
	if cfg.MaxInflight <= 0 {
		cfg.MaxInflight = 8
	}
	sem := make(chan struct{}, cfg.MaxInflight)
	return func(ctx context.Context, workerID domain.WorkerID, _ string, triggers []Trigger) error {
		if len(triggers) == 0 {
			return errors.New("spawner invoked with no triggers")
		}
		if cfg.Client == nil {
			return errors.New("helix spawner: client is nil")
		}
		if cfg.Store == nil {
			return errors.New("helix spawner: store is nil")
		}
		prompt := buildPrompt(workerID, helixSpecsMandate, triggers)

		// Acquire global slot. The dispatcher serialises per-Worker, so
		// blocking here only delays one Worker behind the rest of the
		// org under burst load.
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			return ctx.Err()
		}
		defer func() { <-sem }()

		streamID := activationStreamID(workerID)
		publish := func(body string) {
			if body == "" {
				return
			}
			publishHelixActivationEvent(ctx, cfg, workerID, streamID, body)
		}
		publish(fmt.Sprintf("=== activation: %s ===", describeTriggers(triggers)))

		actCtx, cancel := context.WithTimeout(ctx, cfg.ActivationTimeout)
		defer cancel()

		// Make sure the Worker has a Helix project. First activation
		// (TriggerHire, or a TriggerEvent before hire fully ran)
		// applies one and persists the IDs.
		if err := cfg.ensureProject(actCtx, workerID); err != nil {
			publish(fmt.Sprintf("=== exit: error: %v ===", err))
			return err
		}

		sessionID, err := cfg.ensureSession(actCtx, workerID, prompt, publish)
		if err != nil {
			publish(fmt.Sprintf("=== exit: error: %v ===", err))
			return err
		}

		// Live transcript bridge. On disconnect the spawner reconnects
		// for the lifetime of the activation; the dedup map prevents
		// republishing on snapshot replay.
		bridge := newHelixBridge(publish)
		bridgeCtx, bridgeCancel := context.WithCancel(actCtx)
		defer bridgeCancel()
		go bridge.run(bridgeCtx, cfg, sessionID)

		err = cfg.pollUntilDone(actCtx, sessionID, publish)
		bridgeCancel()
		if err != nil {
			publish(fmt.Sprintf("=== exit: error: %v ===", err))
			return err
		}
		publish("=== exit: ok ===")
		return nil
	}
}

// helixSpecsMandate points the agent at its role + identity files,
// which helix-org's HelixProjectApplier pushes to the per-Worker
// repo's `helix-specs` branch. Helix's workspace-setup script
// creates a worktree for that branch at ~/work/helix-specs/ on every
// boot — but the worktree is only created when the branch exists on
// the remote at boot time, so if the worktree is missing the agent
// must materialise it itself.
const helixSpecsMandate = `Your org-wide policy, role, and identity files live on the
**helix-specs** branch of your per-Worker repo. helix-org pushes them
on hire and re-pushes them on every activation, so the remote always
has the current owner-edited version. Path inside the branch:
  .context/agent.md                                    (org-wide policy)
  workers/${HELIX_WORKER_ID}/.context/role.md
  workers/${HELIX_WORKER_ID}/.context/identity.md

ALWAYS pull before reading — your local worktree is stale from prior
activations and won't reflect owner edits made since:

  if [ ! -d ~/work/helix-specs ]; then
    cd ~/work/$(ls ~/work | grep -v helix-specs | head -1)
    git fetch origin helix-specs
    git worktree add ../helix-specs helix-specs
  else
    cd ~/work/helix-specs && git pull --ff-only origin helix-specs
  fi

Then read in this order — agent.md FIRST (it's the entrypoint that
tells you how to be an agent at all), then role.md, then identity.md:
  cat ~/work/helix-specs/.context/agent.md
  cat ~/work/helix-specs/workers/${HELIX_WORKER_ID}/.context/role.md
  cat ~/work/helix-specs/workers/${HELIX_WORKER_ID}/.context/identity.md

After meaningful work, persist state on helix-specs:
  cd ~/work/helix-specs && git add -A && git commit -m 'checkpoint: <what>' && git push origin helix-specs`

// ensureProject is a thin wrapper around HelixProjectApplier so the
// activation flow reads naturally. The actual apply logic is shared
// with the chat bridge — see helixproject.go.
func (c HelixSpawnerConfig) ensureProject(ctx context.Context, workerID domain.WorkerID) error {
	a := &HelixProjectApplier{
		Client:      c.Client,
		Store:       c.Store,
		HelixOrgURL: c.HelixOrgURL,
		OrgID:       c.OrgID,
		Provider:    c.Provider,
		Model:       c.Model,
		Runtime:     c.Runtime,
		AgentMD:     c.AgentMD,
		Logger:      c.Logger,
	}
	_, _, _, err := a.Ensure(ctx, workerID)
	return err
}

// ensureSession dispatches the activation prompt to the Worker's
// long-lived chat session, reusing the persisted helix_session_id
// when one exists. We DON'T open a fresh session per activation:
// each fresh session spawns a fresh desktop container in Helix
// (~3 min cold start), which makes routine DM-driven activity
// painfully slow. Reusing the session keeps the container warm and
// lets follow-ups land in seconds.
//
// Cross-activation context: the agent has its prior chat history in
// the same Helix session AND the helix-specs branch in its workspace.
// Either is a sufficient reminder of "what came before"; carrying
// both is intentional belt-and-braces.
//
// Cold start race: when this is the first activation against a fresh
// Worker project, the desktop's container is still booting when we
// POST /sessions/chat. Helix's session_handlers.go dispatches the
// command synchronously and fails fast with `no external agent
// WebSocket connection` (a global readiness check passes the moment
// any other user has a desktop up; the per-session sendCommand then
// fails because OUR container hasn't WS-connected yet). Helix marks
// the interaction state=error, and its auto-wake retry won't pick it
// up because that path only revives state=waiting interactions.
//
// Workaround: detect the hadWSError signal from StartChatWithStatus
// and re-POST the SAME prompt with the same SessionID every 8–20s
// until one lands. Each retry leaks an extra error interaction in
// the DB; the eventual successful one drives the actual run, and
// pollUntilDone reads its status. Same pattern the chat bridge uses
// — see chat/helix_bridge.go::warmupAndRetry.
func (c HelixSpawnerConfig) ensureSession(ctx context.Context, workerID domain.WorkerID, prompt string, publish func(string)) (string, error) {
	worker, err := c.Store.Workers.Get(ctx, workerID)
	if err != nil {
		return "", fmt.Errorf("get worker: %w", err)
	}
	projectID := worker.HelixProjectID()
	if projectID == "" {
		return "", fmt.Errorf("worker %s has no helix project — ensureProject must run first", workerID)
	}
	req := helixclient.StartChatRequest{
		ProjectID:           projectID,
		AppID:               worker.HelixAgentAppID(),
		SessionID:           worker.HelixSessionID(), // empty on first activation, reused thereafter
		SessionRole:         "job",
		AgentType:           "zed_external",
		Type:                "text",
		ExternalAgentConfig: &helixclient.ExternalAgentConfig{},
		Messages:            []helixclient.SessionChatMessage{helixclient.NewTextMessage("user", prompt)},
	}
	session, hadWSError, err := c.Client.StartChatWithStatus(ctx, req)
	if err != nil {
		// If the persisted session_id is gone (Helix reaped it, user
		// stopped the desktop, etc.), retry once without it so we
		// open a fresh session instead of failing the activation.
		if req.SessionID != "" {
			if c.Logger != nil {
				c.Logger.Info("spawner: persisted session unusable, opening fresh", "worker", workerID, "stale_sid", req.SessionID, "err", err)
			}
			req.SessionID = ""
			session, hadWSError, err = c.Client.StartChatWithStatus(ctx, req)
		}
		if err != nil {
			return "", fmt.Errorf("start chat: %w", err)
		}
	}
	// Persist whatever ID we ended up using — first-time hires get a
	// freshly-minted ID; later activations reuse the same one.
	if session.ID != worker.HelixSessionID() {
		if err := c.Store.Workers.SetHelixSessionID(ctx, workerID, session.ID); err != nil {
			return "", fmt.Errorf("persist session id: %w", err)
		}
	}
	if hadWSError {
		if publish != nil {
			publish("=== warming up Zed desktop (cold start, ~1–2 min)... ===")
		}
		if err := c.warmupSession(ctx, session.ID, req); err != nil {
			return "", fmt.Errorf("warmup session: %w", err)
		}
	}
	return session.ID, nil
}

// warmupSession re-POSTs the same activation prompt with the same
// session_id until the SSE stream stops surfacing the WS-not-ready
// error, which means the desktop's Zed agent has connected and the
// dispatch landed. Caps retries at 5 minutes — desktops that haven't
// booted by then aren't going to.
func (c HelixSpawnerConfig) warmupSession(ctx context.Context, sessionID string, req helixclient.StartChatRequest) error {
	req.SessionID = sessionID
	delay := 8 * time.Second
	for attempt := 1; attempt <= 30; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
		_, hadWSError, err := c.Client.StartChatWithStatus(ctx, req)
		if err != nil {
			if c.Logger != nil {
				c.Logger.Warn("spawner warmup retry: HTTP error",
					"attempt", attempt,
					"sid", sessionID,
					"err", err,
				)
			}
			continue
		}
		if !hadWSError {
			if c.Logger != nil {
				c.Logger.Info("spawner warmup succeeded", "attempt", attempt, "sid", sessionID)
			}
			return nil
		}
		if c.Logger != nil {
			c.Logger.Info("spawner warmup retry: still no WS",
				"attempt", attempt,
				"sid", sessionID,
			)
		}
		if delay < 20*time.Second {
			delay = delay * 5 / 4
		}
	}
	return fmt.Errorf("desktop didn't come up after 30 warmup attempts")
}

// pollUntilDone polls GetOutput with exponential backoff until a
// terminal status is reported or ctx fires.
func (c HelixSpawnerConfig) pollUntilDone(ctx context.Context, sessionID string, publish func(string)) error {
	delay := c.PollInitial
	for {
		out, err := c.Client.GetOutput(ctx, sessionID)
		if err != nil {
			// Don't fail the activation on a transient poll error; just
			// back off and retry until the timeout fires.
			if c.Logger != nil {
				c.Logger.Warn("helix poll", "session", sessionID, "err", err)
			}
		} else if out.IsTerminal() {
			if out.Status == "error" {
				return fmt.Errorf("session error: %s", oneLine(out.Output, 500))
			}
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
		delay *= 2
		if delay > c.PollMax {
			delay = c.PollMax
		}
	}
}

// helixBridge consumes WebSocket frames and publishes one transcript
// event per *settled* response entry. It owns a single `EntryStream`
// for the lifetime of the activation; the EntryStream's dedup state
// (per Index/MessageID) keeps snapshot replay safe across reconnects.
type helixBridge struct {
	publish func(body string)
	stream  *helixclient.EntryStream
}

func newHelixBridge(publish func(body string)) *helixBridge {
	b := &helixBridge{publish: publish}
	b.stream = helixclient.NewEntryStream(b.onEvent)
	return b
}

// onEvent renders one settled EntryStream event into the line shape
// claudeSpawner has emitted historically (`assistant: …`,
// `tool_use <name>: <args>`, `tool_result: …`, `tool_result-error:
// …`, `error: …`). Both backends emit the same shape so observers
// don't have to discriminate.
func (b *helixBridge) onEvent(e helixclient.Event) {
	switch e.Kind {
	case helixclient.EventAssistant:
		b.publish("assistant: " + oneLine(e.Text, 500))
	case helixclient.EventToolUse:
		b.publish(fmt.Sprintf("tool_use %s: %s", e.ToolName, oneLine(e.Text, 500)))
	case helixclient.EventToolResult:
		b.publish("tool_result: " + oneLine(e.Text, 500))
	case helixclient.EventToolResultError:
		b.publish("tool_result-error: " + oneLine(e.Text, 500))
	case helixclient.EventError:
		b.publish("error: " + oneLine(e.Text, 500))
	}
}

func (b *helixBridge) run(ctx context.Context, cfg HelixSpawnerConfig, sessionID string) {
	delay := time.Second
	for {
		ch, err := cfg.Client.SubscribeUpdates(ctx, sessionID)
		if err != nil {
			if cfg.Logger != nil {
				cfg.Logger.Warn("helix subscribe", "session", sessionID, "err", err)
			}
		} else {
			for u := range ch {
				b.stream.Apply(u)
			}
		}
		// Reconnect with capped exponential backoff while the
		// activation context is still live.
		select {
		case <-ctx.Done():
			b.stream.Flush()
			return
		case <-time.After(delay):
		}
		if delay < 30*time.Second {
			delay *= 2
		}
	}
}

func publishHelixActivationEvent(ctx context.Context, cfg HelixSpawnerConfig, workerID domain.WorkerID, streamID domain.StreamID, body string) {
	if cfg.Store == nil || cfg.NewID == nil || cfg.Now == nil || strings.TrimSpace(body) == "" {
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
		if cfg.Logger != nil {
			cfg.Logger.Warn("helix activation event: build", "worker", workerID, "err", err)
		}
		return
	}
	if err := cfg.Store.Events.Append(ctx, event); err != nil {
		if cfg.Logger != nil {
			cfg.Logger.Warn("helix activation event: append", "worker", workerID, "err", err)
		}
		return
	}
	if cfg.Broadcaster != nil {
		cfg.Broadcaster.Notify(streamID)
	}
}
