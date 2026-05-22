package helix

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/helixml/helix/api/pkg/org/activation"
	"github.com/helixml/helix/api/pkg/org/broadcast"
	"github.com/helixml/helix/api/pkg/org/runtime"
	runtimehelix "github.com/helixml/helix/api/pkg/org/runtime/helix"
	"github.com/helixml/helix/api/pkg/org/stream"
	"github.com/helixml/helix/api/pkg/org/worker"
	"github.com/helixml/helix/helix-org/agent"
	"github.com/helixml/helix/helix-org/helix/helixclient"
	"github.com/helixml/helix/helix-org/store"
)

// SpawnerConfig wires the helix-backed Spawner. The Client is
// injectable so tests can run without HTTP. Defaults are applied for
// any unset duration / capacity.
//
// In the per-Worker-project model, the spawner does not hold a
// ProjectID of its own — every AI Worker gets its own Helix project,
// applied at hire time and persisted in the WorkerRuntimeState
// sidecar under the "helix" backend.
type SpawnerConfig struct {
	Client         helixclient.Client
	ProjectService runtimehelix.ProjectService
	ProjectGit     runtimehelix.ProjectGitWriter
	HelixOrgURL    string // forwarded to project secrets so the in-sandbox agent can reach helix-org's MCP server
	// Runtime overrides the default `zed_agent` runtime. Empty falls
	// back to helix.Runtime. See ProjectApplier.Runtime for the
	// embedded SaaS use case (`claude_code` + subscription credentials).
	Runtime string
	// Provider/Model drive the project's Agent App config when Runtime
	// routes inference through Helix. Ignored when Runtime is
	// `claude_code` and Credentials is `"subscription"` — the sandbox
	// authenticates Anthropic directly via the operator's OAuth.
	Provider string
	Model    string
	// Credentials forwards to ProjectApplier.Credentials. See there.
	Credentials string
	// AgentMD is the org-wide agent.md policy text pushed to
	// `.context/agent.md` on each per-Worker project's helix-specs
	// branch. The spawner's activation prompt tells every Worker to
	// read it first. Embedded by main.go from agent/policy.md.
	AgentMD string
	// MCPAuthBearer is forwarded to ProjectApplier so the helix-org
	// MCP entry on each Worker's agent app carries an Authorization
	// header. Used when HelixOrgURL routes through an auth-gated
	// proxy (embedded SaaS alpha). Empty in standalone mode.
	MCPAuthBearer string
	// BearerForUser, when non-nil, is called by the Spawner (and
	// ProjectApplier) on every activation to resolve the api_key that
	// requests should be issued under. Passed the userID persisted on
	// the Worker's runtime state at hire time (see
	// state.go::HiringUserID). Letting the host mint or look up the
	// bearer on-demand avoids stashing tokens at rest. A nil callback
	// or empty return means "use the static Client's api_key" — the
	// service-account fallback.
	BearerForUser func(ctx context.Context, userID string) (string, error)
	// OrgID is the Helix organisation each per-Worker project lives
	// under. Empty for personal accounts.
	OrgID             string
	ActivationTimeout time.Duration
	MaxInflight       int
	PollInitial       time.Duration // default 250ms
	PollMax           time.Duration // default 30s
	Logger            *slog.Logger
	Store             *store.Store
	Hub               *broadcast.Hub
	Now               func() time.Time
	NewID             func() string
}

// Spawner returns an runtime.Spawner that runs each activation as a
// long-lived Helix chat session. Either a fresh one (first activation
// or stale pointer) or a follow-up message on the Worker's already-
// open session.
//
// The Spawner does five things, in order: build the prompt, take a
// global semaphore slot, ensure a live session exists, open the live
// transcript WebSocket, then poll for completion. New transcript
// segments arriving on the WebSocket are diffed against a per-call
// dedup map and republished onto s-activations-<workerID> in the
// same shape claude.Spawner emits, so observers see one transcript
// format regardless of backend.
func Spawner(cfg SpawnerConfig) runtime.Spawner {
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
	return func(ctx context.Context, workerID worker.ID, _ string, triggers []activation.Trigger) error {
		if len(triggers) == 0 {
			return errors.New("spawner invoked with no triggers")
		}
		if cfg.Client == nil {
			return errors.New("helix spawner: client is nil")
		}
		if cfg.Store == nil {
			return errors.New("helix spawner: store is nil")
		}
		prompt := agent.BuildPrompt(workerID, helixSpecsMandate, triggers)

		// Acquire global slot. The dispatcher serialises per-Worker, so
		// blocking here only delays one Worker behind the rest of the
		// org under burst load.
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			return ctx.Err()
		}
		defer func() { <-sem }()

		streamID := agent.ActivationStreamID(workerID)
		publish := func(body string) {
			if body == "" {
				return
			}
			publishActivationEvent(ctx, cfg, workerID, streamID, body)
		}
		publish(fmt.Sprintf("=== activation: %s ===", agent.DescribeTriggers(triggers)))

		actCtx, cancel := context.WithTimeout(ctx, cfg.ActivationTimeout)
		defer cancel()

		// Resolve the hiring user's bearer for THIS activation, if a
		// host-provided callback is wired. The bearer is stashed on
		// actCtx via the helixclient context key; every subsequent
		// client call inside this activation (project apply, session
		// open, MCP attach, transcript subscribe) picks it up so the
		// Worker's footprint in Helix is attributed to the hiring
		// user, not the service account. Empty / nil / error all
		// degrade to "use the static client api_key".
		if cfg.BearerForUser != nil {
			if state, err := runtimehelix.LoadState(actCtx, cfg.Store, workerID); err == nil && state.HiringUserID != "" {
				if bearer, berr := cfg.BearerForUser(actCtx, state.HiringUserID); berr == nil && bearer != "" {
					actCtx = runtimehelix.WithBearerToken(actCtx, bearer)
				} else if berr != nil && cfg.Logger != nil {
					cfg.Logger.Warn("helix spawner: BearerForUser failed; falling back to service key", "worker", workerID, "user_id", state.HiringUserID, "err", berr.Error())
				}
			}
		}

		// Make sure the Worker has a Helix project. First activation
		// (activation.TriggerHire, or a activation.TriggerEvent before hire fully ran)
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
		bridge := newBridge(publish)
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
// which the project applier pushes to the per-Worker repo's
// `helix-specs` branch. Helix's workspace-setup script creates a
// worktree for that branch at ~/work/helix-specs/ on every boot —
// but the worktree is only created when the branch exists on the
// remote at boot time, so if the worktree is missing the agent must
// materialise it itself.
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

// ensureProject is a thin wrapper around runtimehelix.ProjectApplier
// so the activation flow reads naturally. The Service / Git fields
// must be wired by the embedding host (api/pkg/server/helix_org.go).
func (c SpawnerConfig) ensureProject(ctx context.Context, workerID worker.ID) error {
	// Fallback: tests build SpawnerConfig with a fakeHelixClient and no
	// explicit ProjectService — derive one from the Client via the
	// helixclient adapter. Production wiring sets ProjectService /
	// ProjectGit directly.
	svc := c.ProjectService
	if svc == nil && c.Client != nil {
		svc = helixclient.AsProjectService(c.Client)
	}
	a := &runtimehelix.ProjectApplier{
		Service:       svc,
		Git:           c.ProjectGit,
		Store:         c.Store,
		HelixOrgURL:   c.HelixOrgURL,
		OrgID:         c.OrgID,
		Runtime:       c.Runtime,
		Provider:      c.Provider,
		Model:         c.Model,
		Credentials:   c.Credentials,
		AgentMD:       c.AgentMD,
		MCPAuthBearer: c.MCPAuthBearer,
		Logger:        c.Logger,
	}
	_, _, _, err := a.Ensure(ctx, workerID)
	return err
}

// ensureSession dispatches the activation prompt to the Worker's
// long-lived chat session, reusing the persisted session ID when one
// exists. We DON'T open a fresh session per activation: each fresh
// session spawns a fresh desktop container in Helix (~3 min cold
// start), which makes routine DM-driven activity painfully slow.
// Reusing the session keeps the container warm and lets follow-ups
// land in seconds.
//
// Cross-activation context: the agent has its prior chat history in
// the same Helix session AND the helix-specs branch in its workspace.
// Either is a sufficient reminder of "what came before"; carrying
// both is intentional belt-and-braces.
//
// Two paths:
//   - **Follow-up** (state.SessionID exists): POST
//     /api/v1/sessions/{id}/messages. Helix queues the message and
//     pickupWaitingInteraction delivers it on agent reconnect — no
//     warmup loop, no cold-start handling on our side.
//   - **First activation** (no session yet): POST /sessions/chat to
//     create the session. The dispatch may race the desktop's WS
//     connect; if it does (hadWSError) we immediately re-queue the
//     same prompt via the durable /messages endpoint so it lands as
//     soon as the agent dials home.
func (c SpawnerConfig) ensureSession(ctx context.Context, workerID worker.ID, prompt string, _ func(string)) (string, error) {
	state, err := runtimehelix.LoadState(ctx, c.Store, workerID)
	if err != nil {
		return "", err
	}
	if state.ProjectID == "" {
		return "", fmt.Errorf("worker %s has no helix project — ensureProject must run first", workerID)
	}

	// Follow-up: the persisted session ID is the durable target. We
	// continue the session via /sessions/chat with SessionID set
	// rather than the /sessions/{id}/messages queue endpoint —
	// embedded Helix doesn't expose the latter; this path is what
	// standard Helix supports for both new and continued sessions.
	// If the session is gone (404 / not found) we fall through and
	// open a fresh one.
	// EnsureAndSend is the single primitive shared with the owner-chat
	// bridge: it resumes the persisted session if possible, falls
	// through to a fresh one on any failure (HTTP, SSE-error chunk,
	// stale in-memory state after api restart), and posts the
	// activation prompt — all with session_role="exploratory" so the
	// new session is discoverable from Helix's per-project UI. Without
	// one shared primitive the two paths drift apart and develop
	// independent stale-session bugs.
	sid, fresh, err := helixclient.EnsureAndSend(ctx, c.Client, helixclient.SendPromptParams{
		SessionID: state.SessionID,
		ProjectID: state.ProjectID,
		AppID:     state.AgentAppID,
		AgentType: runtimehelix.AgentType,
		Prompt:    prompt,
	})
	if err != nil {
		return "", fmt.Errorf("ensure session: %w", err)
	}
	if fresh {
		if c.Logger != nil && state.SessionID != "" {
			c.Logger.Info("spawner: persisted session unusable, opened fresh",
				"worker", workerID, "stale_sid", state.SessionID, "new_sid", sid)
		}
		if err := runtimehelix.SaveSession(ctx, c.Store, workerID, sid); err != nil {
			return "", fmt.Errorf("persist session id: %w", err)
		}
	}
	return sid, nil
}

// pollUntilDone polls GetOutput with exponential backoff until a
// terminal status is reported or ctx fires.
func (c SpawnerConfig) pollUntilDone(ctx context.Context, sessionID string, publish func(string)) error {
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
				return fmt.Errorf("session error: %s", agent.OneLine(out.Output, 500))
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

// bridge consumes WebSocket frames and publishes one transcript
// event per *settled* response entry. It owns a single `EntryStream`
// for the lifetime of the activation; the EntryStream's dedup state
// (per Index/MessageID) keeps snapshot replay safe across reconnects.
type bridge struct {
	publish func(body string)
	stream  *helixclient.EntryStream
}

func newBridge(publish func(body string)) *bridge {
	b := &bridge{publish: publish}
	b.stream = helixclient.NewEntryStream(b.onEvent)
	return b
}

// onEvent renders one settled EntryStream event into the line shape
// claude.Spawner has emitted historically. Both backends emit the
// same shape so observers don't have to discriminate. The owner-chat
// bridge in server/chat uses the same TranscriptBody helper to
// publish identical lines to s-activations-w-owner.
func (b *bridge) onEvent(e helixclient.Event) {
	if body := TranscriptBody(e); body != "" {
		b.publish(body)
	}
}

// TranscriptBody renders one Helix WS event into the line shape every
// Worker's activation transcript uses (assistant: …, tool_use foo:
// …, tool_result: …). Exported so the owner-chat bridge in
// server/chat can produce identical lines for s-activations-w-owner
// without duplicating the rendering. Empty string for kinds that
// shouldn't appear on the transcript.
func TranscriptBody(e helixclient.Event) string {
	switch e.Kind {
	case helixclient.EventAssistant:
		return "assistant: " + agent.OneLine(e.Text, 500)
	case helixclient.EventToolUse:
		return fmt.Sprintf("tool_use %s: %s", e.ToolName, agent.OneLine(e.Text, 500))
	case helixclient.EventToolResult:
		return "tool_result: " + agent.OneLine(e.Text, 500)
	case helixclient.EventToolResultError:
		return "tool_result-error: " + agent.OneLine(e.Text, 500)
	case helixclient.EventError:
		return "error: " + agent.OneLine(e.Text, 500)
	}
	return ""
}

func (b *bridge) run(ctx context.Context, cfg SpawnerConfig, sessionID string) {
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

// publishActivationEvent is a thin wrapper around the shared
// agent.PublishActivationEvent so the helix spawner's call sites
// stay terse. The owner-chat bridge uses the same shared helper
// directly — both paths produce identical event shapes on
// s-activations-<workerID>.
func publishActivationEvent(ctx context.Context, cfg SpawnerConfig, workerID worker.ID, _ stream.ID, body string) {
	_, _ = agent.PublishActivationEvent(ctx, cfg.Store, cfg.Hub, cfg.NewID, cfg.Now, cfg.Logger, workerID, body)
}
