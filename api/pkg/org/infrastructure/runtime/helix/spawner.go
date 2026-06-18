package helix

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/transcript"
	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/briefing"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/infrastructure/runtime"
	"github.com/helixml/helix/api/pkg/org/infrastructure/wakebus"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/types"
)

// SpawnerConfig wires the helix-backed Spawner. The Client is
// injectable so tests can run without HTTP. Defaults are applied for
// any unset duration / capacity.
//
// In the per-Worker-project model, the spawner does not hold a
// ProjectID of its own — every AI Worker gets its own Helix project,
// applied at hire time and persisted in the WorkerRuntimeState
// sidecar under the "helix" backend.
// DefaultMaxInflight bounds concurrent activations when a SpawnerConfig
// doesn't set MaxInflight. The host also uses it to size the shared
// semaphore it injects via SpawnerConfig.Sem.
const DefaultMaxInflight = 8

type SpawnerConfig struct {
	Client         SpawnerClient
	ProjectService ProjectService
	Workspace      *Workspace
	// PubSub + Snapshotter back SubscribeSessionUpdates, which streams
	// per-session WebsocketEvent frames to in-process subscribers.
	PubSub      pubsub.PubSub
	Snapshotter SessionPreamble
	// Mirror is the transcript writer; the spawner Ensure()s it per
	// activation. nil disables mirroring (tests / app-only wirings).
	Mirror      *Mirror
	HelixOrgURL string // forwarded to project secrets so the in-sandbox agent can reach helix-org's MCP server
	// Runtime overrides the default `zed_agent` runtime. Empty falls
	// back to helix.Runtime. See WorkerProject.Runtime for the
	// embedded SaaS use case (`claude_code` + subscription credentials).
	Runtime string
	// Provider/Model drive the project's Agent App config when Runtime
	// routes inference through Helix. Ignored when Runtime is
	// `claude_code` and Credentials is `"subscription"` — the sandbox
	// authenticates Anthropic directly via the operator's OAuth.
	Provider string
	Model    string
	// Credentials forwards to WorkerProject.Credentials. See there.
	Credentials string
	// MCPAuthBearer is the fallback bearer the spawner passes to
	// AttachHelixOrgMCP when no per-activation user bearer is on ctx.
	// It ends up as the `Authorization: Bearer <value>` header on the
	// helix-org MCP entry attached to each Worker's agent app. Used
	// when HelixOrgURL routes through an auth-gated proxy (embedded
	// SaaS alpha). Empty in standalone mode.
	MCPAuthBearer string
	// SpecsMandate is the activation-prompt directive that tells the
	// agent how to find role.md / identity.md / agent.md on the
	// helix-specs branch. Surfaced as an operator-editable config
	// (helixSpecsMandate) so changes to the file layout or git-pull
	// recipe don't require a deploy. Empty falls back to
	// DefaultHelixSpecsMandate.
	SpecsMandate string
	// BearerForUser, when non-nil, is called by the Spawner (and
	// WorkerProject) on every activation to resolve the api_key that
	// requests should be issued under. Passed the userID persisted on
	// the Worker's runtime state at hire time (see
	// state.go::HiringUserID). Letting the host mint or look up the
	// bearer on-demand avoids stashing tokens at rest. A nil callback
	// or empty return means "use the static Client's api_key" — the
	// service-account fallback.
	BearerForUser func(ctx context.Context, userID string) (string, error)
	// OrgID is the Helix organisation each per-Worker project lives
	// under. Empty for personal accounts.
	OrgID string
	// SessionStartupTimeout bounds how long ensureSession is allowed to
	// take — creating / picking the session row, opening the Helix
	// chat session, attaching the live transcript WebSocket. Five
	// minutes covers a cold sandbox boot with margin; if startup
	// genuinely hangs past it, returning the deadline error is the
	// right outcome. Defaults to 5 * time.Minute when zero.
	//
	// Previously called ActivationTimeout — that name conflated startup
	// with the whole activation lifetime and was applied to
	// pollUntilDone as well, which caused the spawner to release its
	// per-Worker serialisation lane on a stale 5-minute timer even when
	// the underlying session was still actively producing work.
	// pollUntilDone now uses ActivationRunawayGuard instead.
	SessionStartupTimeout time.Duration
	// ActivationRunawayGuard is the hard upper bound on how long
	// pollUntilDone is allowed to run before giving up. It is NOT a
	// liveness threshold and not operator-tunable — it exists only to
	// prevent a session that never reports terminal status from
	// pinning a Queue lane forever. Defaults to 24 * time.Hour when
	// zero. Real "is the session stuck?" detection lives at the
	// session layer (see api/pkg/server/auto_wake_stuck_interactions.go)
	// — the org layer's job is to serialise per-Worker and trust the
	// session API.
	ActivationRunawayGuard time.Duration
	MaxInflight            int
	// Sem, when non-nil, is the inflight semaphore the Spawner acquires
	// a slot from instead of minting its own from MaxInflight. The host
	// builds one fresh SpawnerConfig per activation (so OrgID /
	// HelixOrgURL stay scoped to the activating org — never frozen to
	// whichever org activated first), and shares a single Sem across all
	// of them to keep one process-wide inflight cap. Nil falls back to a
	// per-config semaphore of size MaxInflight.
	Sem         chan struct{}
	PollInitial time.Duration // default 250ms
	PollMax     time.Duration // default 30s
	Logger      *slog.Logger
	Store       *store.Store
	Hub         *wakebus.Bus
	Now         func() time.Time
	NewID       func() string
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
// dedup map and republished onto s-transcript-<workerID> in the
// canonical transcript line shape (assistant: …, tool_use foo: …,
// tool_result: …) so observers see one format regardless of which
// Worker fired.
func Spawner(cfg SpawnerConfig) runtime.Spawner {
	if cfg.PollInitial == 0 {
		cfg.PollInitial = 250 * time.Millisecond
	}
	if cfg.PollMax == 0 {
		cfg.PollMax = 30 * time.Second
	}
	if cfg.SessionStartupTimeout == 0 {
		cfg.SessionStartupTimeout = 5 * time.Minute
	}
	if cfg.ActivationRunawayGuard == 0 {
		cfg.ActivationRunawayGuard = 24 * time.Hour
	}
	if cfg.MaxInflight <= 0 {
		cfg.MaxInflight = DefaultMaxInflight
	}
	// Prefer a host-supplied shared semaphore so multiple per-org
	// SpawnerConfigs enforce one global inflight cap; otherwise mint one
	// sized to this config's MaxInflight.
	sem := cfg.Sem
	if sem == nil {
		sem = make(chan struct{}, cfg.MaxInflight)
	}
	return func(ctx context.Context, orgID string, workerID orgchart.WorkerID, triggers []activation.Trigger) (retErr error) {
		if len(triggers) == 0 {
			return errors.New("spawner invoked with no triggers")
		}
		if cfg.Client == nil {
			return errors.New("helix spawner: client is nil")
		}
		if cfg.Store == nil {
			return errors.New("helix spawner: store is nil")
		}
		mandate := cfg.SpecsMandate
		if mandate == "" {
			mandate = DefaultHelixSpecsMandate
		}
		prompt := briefing.BuildPrompt(workerID, mandate, triggers)

		// Acquire global slot. The dispatcher serialises per-Worker, so
		// blocking here only delays one Worker behind the rest of the
		// org under burst load.
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			return ctx.Err()
		}
		defer func() { <-sem }()

		// Record the audit row for this activation. Failures here are
		// best-effort during the B5 transition — the transcript stream
		// (next block) is still the primary record until callers depend
		// on the row. Once B5.7 lands worker_log's activation_id filter,
		// a Create failure becomes a hard error.
		act := newActivationRecord(cfg, orgID, workerID, triggers)
		if act != nil {
			// Skip Create when hire_worker pre-allocated the row (B5.8) —
			// the row exists from the caller's request; this path just
			// Completes it.
			preallocated := triggers[0].ActivationID != ""
			if !preallocated {
				if err := cfg.Store.Activations.Create(ctx, act); err != nil {
					if cfg.Logger != nil {
						cfg.Logger.Warn("helix spawner: persist activation row", "worker", workerID, "activation", act.ID, "err", err)
					}
				}
			}
			defer func() {
				if completeErr := cfg.Store.Activations.Complete(ctx, orgID, act.ID, activation.OutcomeFromError(retErr), cfg.Now()); completeErr != nil && cfg.Logger != nil {
					cfg.Logger.Warn("helix spawner: complete activation row", "worker", workerID, "activation", act.ID, "err", completeErr)
				}
			}()
		}

		streamID := activation.TranscriptID(workerID)
		publish := func(body string) {
			if body == "" {
				return
			}
			recordTranscript(ctx, cfg, orgID, workerID, streamID, body)
		}
		publish(fmt.Sprintf("=== activation: %s ===", briefing.DescribeTriggers(triggers)))

		// Two deadlines, two phases. Startup work (project apply, MCP
		// re-attach, secret injection, session creation, transcript WS
		// dial) is bounded by SessionStartupTimeout — five minutes is
		// generous and a hang here is a real failure. The poll loop
		// that waits for the session to report terminal status is
		// bounded only by ActivationRunawayGuard (24h default) — it
		// must not be torpedoed by an arbitrary mid-activation deadline,
		// because doing so releases the per-Worker Queue lane and
		// causes a decoy interaction to be spawned on top of a healthy
		// long-running session.
		//
		// Both contexts derive from a shared `parentCtx` so the bearer
		// token attached during the bearer-lookup phase below is
		// inherited by both phases without re-resolving.
		parentCtx := ctx

		// Resolve the hiring user's bearer for THIS activation, if a
		// host-provided callback is wired. The bearer is stashed on
		// parentCtx via the BearerToken context key; every subsequent
		// client call inside this activation (project apply, session
		// open, MCP attach, transcript subscribe) picks it up so the
		// Worker's footprint in Helix is attributed to the hiring
		// user, not the service account. Empty / nil / error all
		// degrade to "use the static client api_key".
		if cfg.BearerForUser != nil {
			bearerCtx, bearerCancel := context.WithTimeout(parentCtx, cfg.SessionStartupTimeout)
			if state, err := LoadState(bearerCtx, cfg.Store, orgID, workerID); err == nil && state.HiringUserID != "" {
				if bearer, berr := cfg.BearerForUser(bearerCtx, state.HiringUserID); berr == nil && bearer != "" {
					parentCtx = WithBearerToken(parentCtx, bearer)
				} else if berr != nil && cfg.Logger != nil {
					cfg.Logger.Warn("helix spawner: BearerForUser failed; falling back to service key", "worker", workerID, "user_id", state.HiringUserID, "err", berr.Error())
				}
			}
			bearerCancel()
		}

		startupCtx, startupCancel := context.WithTimeout(parentCtx, cfg.SessionStartupTimeout)
		defer startupCancel()

		// Make sure the Worker has a Helix project. First activation
		// (activation.TriggerHire, or a activation.TriggerEvent before hire fully ran)
		// applies one and persists the IDs.
		if err := cfg.ensureProject(startupCtx, orgID, workerID); err != nil {
			publish(activation.OutcomeFromError(err).Marker())
			return err
		}

		// Re-attach the helix-org MCP entry. ensureProject (and the
		// dynamic applier that may have run before us) calls helix
		// project-apply, which wholesale-replaces Config.Helix on
		// update and wipes the MCP list. This is the last write to the
		// agent app's MCPs before the desktop boots its Zed runtime.
		cfg.ensureHelixOrgMCP(startupCtx, orgID, workerID)

		// Register the worker with the transcript mirror (idempotent;
		// the tracker persists across activations and follows the
		// session as it churns). The spawner no longer owns a bridge.
		if cfg.Mirror != nil {
			cfg.Mirror.Ensure(orgID, workerID)
		}

		sessionID, err := cfg.ensureSession(startupCtx, orgID, workerID, prompt, publish)
		if err != nil {
			publish(activation.OutcomeFromError(err).Marker())
			return err
		}

		// Poll phase. Long-running but healthy sessions are normal — a
		// docs-writing activation, a slow `git push`, a `npm install`
		// can each easily exceed the old 5-minute ActivationTimeout.
		// The session API reports terminal status when the work is
		// genuinely done; until then the Queue lane stays held for
		// this Worker, which is the correct serialisation behaviour.
		// ActivationRunawayGuard is the resource-safety backstop only.
		pollCtx, pollCancel := context.WithTimeout(parentCtx, cfg.ActivationRunawayGuard)
		defer pollCancel()
		err = cfg.pollUntilDone(pollCtx, sessionID, publish)
		publish(activation.OutcomeFromError(err).Marker())
		return err
	}
}

// newActivationRecord builds a fresh activation.Activation for one
// Spawner invocation. Returns nil when the caller hasn't wired
// NewID / Now / the Activations repo — the legacy code path still
// runs (transcript stream only) so older tests and dev wirings
// keep working through the B5 transition. Once every caller wires
// these, the nil branch becomes a hard error.
//
// When the lead trigger carries a pre-allocated ActivationID (set by
// hire_worker in B5.8), the returned struct adopts that ID and the
// caller (Spawner) skips Create — the row already exists in the
// store. The Complete path still runs at end-of-activation to set
// EndedAt/Outcome on the pre-existing row.
func newActivationRecord(cfg SpawnerConfig, orgID string, workerID orgchart.WorkerID, triggers []activation.Trigger) *activation.Activation {
	if cfg.NewID == nil || cfg.Now == nil || cfg.Store == nil || cfg.Store.Activations == nil {
		return nil
	}
	id := triggers[0].ActivationID
	if id == "" {
		id = activation.ID("a-" + cfg.NewID())
	}
	act, err := activation.New(id, workerID, triggers, cfg.Now(), orgID)
	if err != nil {
		if cfg.Logger != nil {
			cfg.Logger.Warn("helix spawner: build activation record", "worker", workerID, "err", err)
		}
		return nil
	}
	return act
}

// DefaultHelixSpecsMandate points the agent at its role + identity
// files, which the project applier pushes to the per-Worker repo's
// `helix-specs` branch. Surfaced through SpawnerConfig.SpecsMandate
// — operators override via the `worker.specs_mandate` config key
// when the file layout or pull recipe changes (no deploy required).
//
// Helix's workspace-setup script creates a worktree for the branch
// at ~/work/helix-specs/ on every boot — but only when the branch
// exists on the remote at boot time, so if the worktree is missing
// the agent must materialise it itself.
const DefaultHelixSpecsMandate = `Your org-wide policy, role, and identity files live on the
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

// ensureProject is a thin wrapper around WorkerProject
// so the activation flow reads naturally. The Service / Git fields
// must be wired by the embedding host (api/pkg/server/helix_org.go).
func (c SpawnerConfig) ensureProject(ctx context.Context, orgID string, workerID orgchart.WorkerID) error {
	a := &WorkerProject{
		Service:     c.ProjectService,
		Workspace:   c.Workspace,
		Store:       c.Store,
		HelixOrgURL: c.HelixOrgURL,
		OrgID:       c.OrgID,
		Runtime:     c.Runtime,
		Provider:    c.Provider,
		Model:       c.Model,
		Credentials: c.Credentials,
		Logger:      c.Logger,
	}
	_, _, _, err := a.Ensure(ctx, orgID, workerID)
	return err
}

// ensureHelixOrgMCP re-attaches the helix-org MCP entry to the
// Worker's agent app on every activation. Best-effort: a failure here
// surfaces in the desktop as "no helix-org tools", which is a
// degraded but bootable state — failing the activation would be
// worse. The attach is idempotent (upsert by name), so re-running on
// every activation is safe.
//
// Why it runs here and not inside WorkerProject.Ensure: the project-
// apply path on the helix side wholesale-replaces Config.Helix when
// the agent app already exists, blowing away whatever MCPs were
// attached on the previous activation. Re-attaching after Ensure
// returns keeps the MCP present.
func (c SpawnerConfig) ensureHelixOrgMCP(ctx context.Context, orgID string, workerID orgchart.WorkerID) {
	if c.ProjectService == nil || c.HelixOrgURL == "" {
		return
	}
	state, err := LoadState(ctx, c.Store, orgID, workerID)
	if err != nil {
		if c.Logger != nil {
			c.Logger.Warn("helix spawner: load state for MCP attach", "worker", workerID, "err", err)
		}
		return
	}
	if state.AgentAppID == "" {
		return
	}
	if err := AttachHelixOrgMCP(ctx, c.ProjectService, state.AgentAppID, c.HelixOrgURL, workerID, c.MCPAuthBearer); err != nil && c.Logger != nil {
		c.Logger.Warn("helix spawner: attach helix-org MCP", "worker", workerID, "app", state.AgentAppID, "err", err)
	}
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
func (c SpawnerConfig) ensureSession(ctx context.Context, orgID string, workerID orgchart.WorkerID, prompt string, _ func(string)) (string, error) {
	state, err := LoadState(ctx, c.Store, orgID, workerID)
	if err != nil {
		return "", err
	}
	if state.ProjectID == "" {
		return "", fmt.Errorf("worker %s has no helix project — ensureProject must run first", workerID)
	}

	// Re-activation of an existing session: clear the prior conversation
	// before sending the new prompt so each activation starts on a fresh
	// context window. Re-using one long-lived session across many
	// activations grows the transcript until it hits the model's context
	// limit and triggers expensive, lossy compaction. A Worker persists
	// its durable learnings to markdown in the git workspace — not to the
	// chat history — so wiping the conversation discards no real state.
	// For Zed/ACP worker sessions (AgentType == "zed_external") the clear
	// also resets the Zed thread, so the EnsureAndSend follow-up below
	// lands on a brand-new thread. First activation (no persisted session
	// yet) has nothing to clear and falls straight through to a fresh
	// StartSession.
	if state.SessionID != "" {
		if err := c.Client.ClearSession(ctx, state.SessionID); err != nil {
			return "", fmt.Errorf("clear session %s before re-activation: %w", state.SessionID, err)
		}
		if c.Logger != nil {
			c.Logger.Info("spawner: cleared session before re-activation",
				"worker", workerID, "session", state.SessionID)
		}
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
	sid, fresh, err := EnsureAndSend(ctx, c.Client, SendPromptParams{
		SessionID: state.SessionID,
		ProjectID: state.ProjectID,
		// OrganizationID tags the session row with the Worker's org so
		// authorizeUserToSession can grant access to org members (e.g. the
		// operator viewing the inline transcript). Without it the session
		// is owned only by the org-service user and every other org admin
		// gets a 403 loading the worker's chat. The owner-chat bridge path
		// sets this via EnsureAndSend too; the activation path must match.
		OrganizationID: orgID,
		AppID:          state.AgentAppID,
		AgentType:      AgentType,
		Prompt:         prompt,
	})
	if err != nil {
		return "", fmt.Errorf("ensure session: %w", err)
	}
	if fresh {
		if c.Logger != nil && state.SessionID != "" {
			c.Logger.Info("spawner: persisted session unusable, opened fresh",
				"worker", workerID, "stale_sid", state.SessionID, "new_sid", sid)
		}
		if err := SaveSession(ctx, c.Store, orgID, workerID, sid); err != nil {
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
		} else if IsTerminalOutput(out) {
			if out.Status == "error" {
				return fmt.Errorf("session error: %s", briefing.OneLine(out.Output, 500))
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
	publish     func(body string)
	stream      *EntryStream
	seenPrompts map[string]bool // interaction IDs whose user prompt we've emitted (dedup)
}

func newBridge(publish func(body string)) *bridge {
	b := &bridge{publish: publish, seenPrompts: map[string]bool{}}
	b.stream = NewEntryStream(b.onEvent)
	return b
}

// apply renders one session frame: it emits the user's prompt (once per
// interaction) then feeds the agent's entry patches through EntryStream,
// so the transcript is two-sided. Prompts come from the current
// interaction only (not u.Session history), so a restart doesn't re-emit
// past prompts.
func (b *bridge) apply(u types.WebsocketEvent) {
	if in := u.Interaction; in != nil && in.ID != "" && !b.seenPrompts[in.ID] {
		if body := in.PromptMessage; body != "" {
			b.seenPrompts[in.ID] = true
			b.publish(activation.TranscriptSegment{Kind: activation.SegmentUser, Body: body}.Marker())
		}
	}
	b.stream.Apply(u)
}

// onEvent renders one settled EntryStream event into the canonical
// activation-transcript line shape. The owner-chat bridge in
// server/chat uses the same TranscriptBody helper to publish
// identical lines to s-transcript-w-owner.
func (b *bridge) onEvent(e Event) {
	if body := TranscriptBody(e); body != "" {
		b.publish(body)
	}
}

// TranscriptBody renders one Helix WS event into the line shape every
// Worker's activation transcript uses (assistant: …, tool_use foo:
// …, tool_result: …). Exported so the owner-chat bridge in
// server/chat can produce identical lines for s-transcript-w-owner
// without duplicating the rendering. Empty string for kinds that
// shouldn't appear on the transcript.
//
// The wire shape is owned by activation.TranscriptSegment.Marker();
// this function is just the Helix-Event → typed-segment adapter.
func TranscriptBody(e Event) string {
	seg, ok := transcriptSegmentFromEvent(e)
	if !ok {
		return ""
	}
	return seg.Marker()
}

// transcriptSegmentFromEvent maps the EntryStream Event variants to
// the canonical TranscriptSegment kinds. Kinds with no transcript
// representation (any future Event added without a Segment kind)
// return (_, false).
func transcriptSegmentFromEvent(e Event) (activation.TranscriptSegment, bool) {
	switch e.Kind {
	case EventAssistant:
		return activation.TranscriptSegment{Kind: activation.SegmentAssistant, Body: e.Text}, true
	case EventToolUse:
		return activation.TranscriptSegment{Kind: activation.SegmentToolUse, ToolName: e.ToolName, Body: e.Text}, true
	case EventToolResult:
		return activation.TranscriptSegment{Kind: activation.SegmentToolResult, Body: e.Text}, true
	case EventToolResultError:
		return activation.TranscriptSegment{Kind: activation.SegmentToolResultError, Body: e.Text}, true
	case EventError:
		return activation.TranscriptSegment{Kind: activation.SegmentError, Body: e.Text}, true
	}
	return activation.TranscriptSegment{}, false
}

// recordTranscript records one turn onto the Worker's transcript via the
// shared transcript.Recorder so the helix spawner's call sites stay
// terse. The owner-chat bridge records through the same recorder — both
// paths produce identical event shapes on s-transcript-<workerID>.
func recordTranscript(ctx context.Context, cfg SpawnerConfig, orgID string, workerID orgchart.WorkerID, _ streaming.StreamID, body string) {
	_, _ = newTranscriptRecorder(cfg.Store, cfg.Hub, cfg.NewID, cfg.Now, cfg.Logger).Record(ctx, orgID, workerID, body)
}

// newTranscriptRecorder builds a transcript.Recorder from the loose
// store/hub/clock collaborators the runtime configs carry. A nil store
// yields a no-op recorder; a nil hub means no live wake (the typed-nil
// guard mirrors publishing's Hub handling).
func newTranscriptRecorder(st *store.Store, hub *wakebus.Bus, newID func() string, now func() time.Time, logger *slog.Logger) *transcript.Recorder {
	d := transcript.Deps{NewID: newID, Now: now, Logger: logger}
	if st != nil {
		d.Events = st.Events
	}
	if hub != nil {
		d.Notifier = hub
	}
	return transcript.New(d)
}
