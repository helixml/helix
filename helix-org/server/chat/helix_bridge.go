package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"log/slog"

	agenthelix "github.com/helixml/helix-org/agent/helix"
	"github.com/helixml/helix-org/domain"
	"github.com/helixml/helix-org/helix/helixclient"
	"github.com/helixml/helix-org/prompts"
)

// HelixBridge drives the owner chat surface against a Helix chat
// session instead of a local `claude` subprocess. Each Bridge owns
// **one** Helix session at a time (the "current" session); New chat
// or Switch reset the pointer and the next Send creates / resumes the
// chosen session.
//
// Why one session per Bridge today: there is exactly one Owner chat
// surface and the existing `*Bridge` shares its single subprocess
// across every browser tab. Mirroring that keeps the UI's mental
// model unchanged. When per-Worker chat surfaces arrive, swap the
// "current session" field for a per-Worker map.
//
// SSE listeners are fanned out the same way the claude bridge does
// it: one channel per subscriber, broadcast publishes drop on slow
// listeners. Frame translation lives in renderHelixFrames below — it
// converts Helix's WebsocketEvent payloads into the same HTML
// fragment shape `chat.go::renderFragments` produces, so the UI
// renders both backends identically.
type HelixBridge struct {
	client      helixclient.Client
	ensure      ProjectEnsurer // resolves the owner Worker's per-Worker project; nil in app-only mode
	appID       string         // app-only mode: when set, skip project lifecycle and chat against this existing Helix app
	appIDFunc   func(context.Context) (string, error) // app-only mode: dynamic lookup (re-read per send so config changes take effect without restart). Takes precedence over appID.
	ownerID     domain.WorkerID
	sessionRole string
	provider    string
	model       string
	cwd         string
	logger      *slog.Logger
	prompts     *prompts.Registry

	mu           sync.Mutex // guards sessionID, listeners, ws, freshFromBlank
	sessionID    string     // current Helix session ID; "" means "next Send creates one"
	listeners    map[chan string]struct{}
	wsCancel     context.CancelFunc // closes the active WS goroutine when we switch sessions
	wsWG         sync.WaitGroup
	freshFromNew bool                // true while the user just clicked New chat and no Helix session exists yet
	seen         map[string]struct{} // dedup keys for translated frames; cleared on session switch

	// orgIDByProject caches project_id → organization_id so we don't
	// re-fetch the project on every send. Populated lazily on first
	// send for a project. We MUST send organization_id on /sessions/chat
	// because Helix's handler doesn't auto-populate it from project_id,
	// and without it desktop quota falls back to the user's personal
	// org (limit 2 by default).
	orgIDByProject map[string]string
}

// ProjectEnsurer resolves a Worker's Helix project IDs. The chat
// bridge calls Ensure(ctx, ownerID) per send so the owner Worker's
// project (and its auto-provisioned Agent App with MCP wiring) is
// always the target. The interface keeps the chat package free of a
// hard import on tools/.
type ProjectEnsurer interface {
	Ensure(ctx context.Context, workerID domain.WorkerID) (projectID, agentAppID, repoID string, err error)
}

// HelixConfig wires a HelixBridge. The bridge holds no global
// project ID — each chat session opens against the owner Worker's
// per-Worker project, looked up via Ensure on every send.
//
// agent_type is fixed at agenthelix.AgentType ("zed_external") — see
// the constant for why. There is no `chat.agent_type` knob.
type HelixConfig struct {
	Client      helixclient.Client
	Ensure      ProjectEnsurer
	OwnerID     domain.WorkerID // typically "w-owner"
	SessionRole string          // chat.session_role, e.g. "owner-chat"
	Provider    string          // chat.provider (ignored in app-only mode)
	Model       string          // chat.model (ignored in app-only mode)
	CWD         string          // server cwd, only used as a stable label
	Logger      *slog.Logger

	// AppID enables "app-only" mode: instead of helix-org provisioning
	// its own per-Worker project, the bridge opens chat sessions
	// against this existing Helix app. agent_type, provider, model,
	// and organization_id are derived server-side from the app. Ensure
	// is not called in this mode. Mutually exclusive with Ensure.
	AppID string

	// AppIDFunc is the dynamic variant of AppID: when set, the bridge
	// re-reads the chosen app on each send, so an operator can change
	// the picked agent (e.g. via /ui/settings or the alpha picker UI)
	// without a process restart. Mutually exclusive with AppID and
	// Ensure. The function may return ("", nil) if no agent has been
	// picked yet — in that case the bridge returns a clear error to
	// the caller instead of starting a session.
	AppIDFunc func(context.Context) (string, error)
}

// NewHelix returns a HelixBridge bound to the supplied Helix client.
// Either Ensure (project-applier mode) or AppID (app-only mode) must
// be set; they're mutually exclusive.
func NewHelix(cfg HelixConfig) (*HelixBridge, error) {
	if cfg.Client == nil {
		return nil, fmt.Errorf("chat helix bridge: Client is required")
	}
	configured := 0
	if cfg.Ensure != nil {
		configured++
	}
	if cfg.AppID != "" {
		configured++
	}
	if cfg.AppIDFunc != nil {
		configured++
	}
	if configured == 0 {
		return nil, fmt.Errorf("chat helix bridge: one of Ensure, AppID, AppIDFunc must be set")
	}
	if configured > 1 {
		return nil, fmt.Errorf("chat helix bridge: Ensure, AppID, AppIDFunc are mutually exclusive")
	}
	if cfg.OwnerID == "" {
		return nil, fmt.Errorf("chat helix bridge: OwnerID is required")
	}
	if cfg.SessionRole == "" {
		return nil, fmt.Errorf("chat helix bridge: SessionRole is required (set chat.session_role)")
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &HelixBridge{
		client:         cfg.Client,
		ensure:         cfg.Ensure,
		appID:          cfg.AppID,
		appIDFunc:      cfg.AppIDFunc,
		ownerID:        cfg.OwnerID,
		sessionRole:    cfg.SessionRole,
		provider:       cfg.Provider,
		model:          cfg.Model,
		cwd:            cfg.CWD,
		logger:         logger,
		listeners:      make(map[chan string]struct{}),
		seen:           make(map[string]struct{}),
		orgIDByProject: make(map[string]string),
	}, nil
}

// WithPrompts attaches the slash-command registry so SendHandler can
// expand `/<name>` inputs server-side before posting to Helix. Same
// shape as Bridge.WithPrompts; returns Backend so it composes with the
// interface at the wiring layer.
func (b *HelixBridge) WithPrompts(reg *prompts.Registry) Backend {
	b.prompts = reg
	return b
}

// CWD returns the server's working directory. Used by the UI as the
// stable label under which Helix-backed Recents are grouped — there
// is only one helix-org instance per cwd.
func (b *HelixBridge) CWD() string { return b.cwd }

// Label satisfies chat.Backend. Renders as "helix · <model>" so the
// chat UI footer truthfully reports which LLM stack is active.
func (b *HelixBridge) Label() string {
	if b.model == "" {
		return "helix"
	}
	return "helix · " + b.model
}

// HistoryStartsFresh reports whether the chat page should suppress
// rendered history because the user just clicked New and no Helix
// session has been created for this fresh chat yet.
func (b *HelixBridge) HistoryStartsFresh() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.freshFromNew && b.sessionID == ""
}

// subscribe / unsubscribe / broadcast follow the same shape the
// claude bridge uses, so SSE plumbing in StreamHandler is identical.
func (b *HelixBridge) subscribe() chan string {
	ch := make(chan string, 64)
	b.mu.Lock()
	b.listeners[ch] = struct{}{}
	b.mu.Unlock()
	return ch
}

func (b *HelixBridge) unsubscribe(ch chan string) {
	b.mu.Lock()
	delete(b.listeners, ch)
	b.mu.Unlock()
}

func (b *HelixBridge) broadcast(frag string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.listeners {
		select {
		case ch <- frag:
		default:
			// drop on slow listener
		}
	}
}

// StreamHandler serves /ui/chat/stream as SSE. It is identical to
// the claude bridge's handler in shape — listeners are subscribed
// here, the background WS goroutine started by Send broadcasts
// frames, and the connection lives until the browser closes it.
func (b *HelixBridge) StreamHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		ch := b.subscribe()
		defer b.unsubscribe(ch)

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		ping := time.NewTicker(15 * time.Second)
		defer ping.Stop()

		for {
			select {
			case frag := <-ch:
				_, _ = fmt.Fprint(w, "event: message\n")
				for _, line := range strings.Split(frag, "\n") {
					_, _ = fmt.Fprintf(w, "data: %s\n", line)
				}
				_, _ = fmt.Fprint(w, "\n")
				flusher.Flush()
			case <-ping.C:
				_, _ = fmt.Fprint(w, ": keepalive\n\n")
				flusher.Flush()
			case <-r.Context().Done():
				return
			}
		}
	})
}

// SendHandler accepts a user message at /ui/chat/send. Synchronous:
// it calls Helix, waits for the response, then writes the user bubble
// back. This means the textarea sits frozen for the generation time —
// not ideal UX, but the async-with-mutex variant we tried first dropped
// follow-up responses on the floor and wasn't worth fixing this round.
// Pick this back up if/when the chat surface is busy enough to care.
func (b *HelixBridge) SendHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		msg := strings.TrimSpace(r.PostFormValue("message"))
		if msg == "" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		bubble := msg
		if expanded, ok := b.expandSlashCommand(r.Context(), msg); ok {
			msg = expanded
		}
		// The /sessions/chat call for the helix_agent (helix_basic) path
		// blocks the upstream HTTP request until the agent finishes
		// reasoning + every tool call. With 29 MCP tools and multi-step
		// reasoning that easily exceeds htmx's request timeout, and the
		// browser cancels — we then 500 even though the agent ran fine.
		// Detach the ctx so the bridge keeps running after the response
		// returns; the WS subscriber attached by attachSession (kicked
		// off inside b.send once the session ID is known) pushes
		// interactions to the SSE stream regardless.
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()
			if err := b.send(ctx, msg); err != nil {
				b.logger.Error("chat send (detached)", "err", err)
				b.broadcast(renderTurnError(err.Error()))
			}
		}()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, renderUserBubble(bubble))
	})
}

// send dispatches one user message to the owner Worker's chat
// session. Resolves the per-Worker project (and its auto-provisioned
// Agent App carrying our MCP wiring) via Ensure on every call —
// idempotent, so the cost is one DB lookup once the Worker has a
// project.
//
// Two paths:
//   - **Follow-up** (sessionID already attached): POST
//     /api/v1/sessions/{id}/messages — Helix queues the message and
//     pickupWaitingInteraction delivers it on agent reconnect.
//   - **First turn** (no session): POST /sessions/chat to create the
//     session. If the desktop's WS hasn't connected yet (hadWSError)
//     we immediately re-queue the same prompt via the /messages
//     endpoint so it lands once the agent dials home.
func (b *HelixBridge) send(ctx context.Context, msg string) error {
	var projectID, agentAppID string
	var err error
	appOnly := b.appID != "" || b.appIDFunc != nil
	if appOnly {
		// App-only mode: skip ProjectApplier. The session is opened
		// against an existing Helix app; agent_type, provider, model,
		// and organization_id are derived server-side from the app
		// (see session_handlers.go:383). This is the path for the
		// embedded SaaS alpha where helix-org reuses agents the
		// operator already configured under /orgs/<org>/agents.
		if b.appIDFunc != nil {
			agentAppID, err = b.appIDFunc(ctx)
			if err != nil {
				return fmt.Errorf("look up chat.app_id: %w", err)
			}
			if agentAppID == "" {
				return fmt.Errorf("no Helix agent picked yet — set chat.app_id under /ui/settings or via the alpha agent picker")
			}
		} else {
			agentAppID = b.appID
		}
	} else {
		projectID, agentAppID, _, err = b.ensure.Ensure(ctx, b.ownerID)
		if err != nil {
			return fmt.Errorf("ensure owner project: %w", err)
		}
	}

	b.mu.Lock()
	sid := b.sessionID
	b.mu.Unlock()

	// Follow-up: continue the existing Helix session by re-posting
	// /sessions/chat with SessionID set. The /sessions/{id}/messages
	// queue endpoint helix-org's standalone build targets does not
	// exist in this embedded Helix yet; falling back to the
	// well-supported continuation path keeps follow-ups working
	// without a server-side change.
	if sid != "" {
		req := helixclient.StartChatRequest{
			SessionID:   sid,
			AppID:       agentAppID,
			SessionRole: b.sessionRole,
			Type:        "text",
			Messages:    []helixclient.SessionChatMessage{helixclient.NewTextMessage("user", msg)},
		}
		session, _, err := b.client.StartChatWithStatus(ctx, req)
		if err != nil {
			return fmt.Errorf("helix followup: %w", err)
		}
		b.logger.Info("chat helix followup", "sid", sid, "project", projectID, "app", agentAppID)
		b.broadcastInteractions(session.Interactions)
		return nil
	}

	// First turn: build the StartChat request. In app-only mode we
	// rely on Helix to derive agent_type/provider/model/org from the
	// app — we leave them empty. In project-applier mode we explicitly
	// set zed_external + the configured provider/model, and pre-flight
	// the desktop quota since project-applier sessions always spin up
	// a Zed sandbox.
	var req helixclient.StartChatRequest
	if appOnly {
		req = helixclient.StartChatRequest{
			AppID:       agentAppID,
			SessionRole: b.sessionRole,
			Type:        "text",
			Messages:    []helixclient.SessionChatMessage{helixclient.NewTextMessage("user", msg)},
		}
	} else {
		if err := helixclient.CheckDesktopQuota(ctx, b.client); err != nil {
			return err
		}
		orgID, err := b.resolveProjectOrg(ctx, projectID)
		if err != nil {
			return fmt.Errorf("resolve project org: %w", err)
		}
		// AppID MUST be set — it becomes session.ParentApp, and Helix's
		// external MCP proxy at /api/v1/mcp/external/{name} bails with
		// "session has no associated agent" if ParentApp is empty
		// (mcp_backend_external.go:272).
		req = helixclient.StartChatRequest{
			ProjectID:           projectID,
			OrganizationID:      orgID,
			AppID:               agentAppID,
			SessionRole:         b.sessionRole,
			AgentType:           agenthelix.AgentType,
			Type:                "text",
			Provider:            b.provider,
			Model:               b.model,
			ExternalAgentConfig: &helixclient.ExternalAgentConfig{},
			Messages:            []helixclient.SessionChatMessage{helixclient.NewTextMessage("user", msg)},
		}
	}
	session, hadWSError, err := b.client.StartChatWithStatus(ctx, req)
	if err != nil {
		return fmt.Errorf("start helix chat: %w", err)
	}
	b.attachSession(session.ID)
	b.logger.Info("chat helix session opened", "sid", session.ID, "project", projectID, "app", agentAppID)

	// Synchronous (helix_basic) sessions return the assistant reply
	// inline; render it immediately. Streaming sessions populate
	// Interactions later via the WS bridge.
	b.broadcastInteractions(session.Interactions)

	// Cold-start race: Helix's first /sessions/chat raced the desktop's
	// WS connect, so the prompt is sitting in state=error. Re-queue the
	// same message via the durable /messages endpoint — it'll be
	// delivered on reconnect.
	if hadWSError {
		b.broadcast(renderAssistantText("_Warming up the Zed desktop. This usually takes a minute or two on a cold session..._"))
		if _, err := b.client.SendSessionMessage(ctx, session.ID, msg, helixclient.SendMessageOptions{}); err != nil {
			b.logger.Warn("chat helix queue cold-start retry", "sid", session.ID, "err", err)
		}
	}
	return nil
}

// broadcastInteractions handles the *synchronous* response shape
// (helix_basic chat completions, where the assistant reply is on the
// returned `Session.Interactions[*].ResponseMessage` rather than
// arriving over the WebSocket as EntryPatches). Each unique reply
// becomes one HTML fragment broadcast to SSE listeners. The
// EntryStream's dedup state covers the streamed path — this method
// only fires on the OpenAI-shape path where there are no patches.
func (b *HelixBridge) broadcastInteractions(ixs []*helixclient.Interaction) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, ix := range ixs {
		if ix == nil {
			continue
		}
		key := fmt.Sprintf("sync:%s:%d", ix.ID, ix.GenerationID)
		if _, dup := b.seen[key]; dup {
			continue
		}
		b.seen[key] = struct{}{}
		if ix.ResponseMessage != "" {
			b.broadcastLocked(renderAssistantText(ix.ResponseMessage))
		}
		if ix.State == "error" && ix.Error != "" && !strings.Contains(ix.Error, "no external agent WebSocket connection") {
			b.broadcastLocked(renderTurnError(ix.Error))
		}
	}
}

// broadcastLocked publishes one fragment without re-acquiring b.mu.
// Caller already holds it.
func (b *HelixBridge) broadcastLocked(frag string) {
	for ch := range b.listeners {
		select {
		case ch <- frag:
		default:
		}
	}
}

// attachSession records sid as the current session and starts a new
// WS reader goroutine. Any prior reader is cancelled first. The dedup
// map is reset because interaction IDs only need to be unique within
// one session.
func (b *HelixBridge) attachSession(sid string) {
	b.mu.Lock()
	if b.wsCancel != nil {
		b.wsCancel()
	}
	b.wsWG.Wait()
	b.sessionID = sid
	b.freshFromNew = false
	b.seen = make(map[string]struct{})
	ctx, cancel := context.WithCancel(context.Background())
	b.wsCancel = cancel
	b.mu.Unlock()
	b.wsWG.Add(1)
	go b.runWebsocket(ctx, sid)
}

// runWebsocket subscribes to /api/v1/ws/user for sid, applies each
// frame to a per-session EntryStream, and broadcasts settled events
// as HTML fragments to SSE listeners. Reconnects with capped
// exponential backoff for the life of ctx.
//
// EntryStream's per-Index/MessageID dedup covers the WS path; the
// synchronous OpenAI-shape path (broadcastInteractions) carries its
// own dedup keyed on interaction ID. The two paths are mutually
// exclusive in practice — a chat completion either streams patches
// or returns inline.
func (b *HelixBridge) runWebsocket(ctx context.Context, sid string) {
	defer b.wsWG.Done()
	stream := helixclient.NewEntryStream(func(e helixclient.Event) {
		b.broadcast(b.renderEvent(e))
	})
	delay := time.Second
	for {
		ch, err := b.client.SubscribeUpdates(ctx, sid)
		if err != nil {
			b.logger.Warn("chat helix ws subscribe", "sid", sid, "err", err)
		} else {
			for u := range ch {
				stream.Apply(u)
			}
		}
		select {
		case <-ctx.Done():
			stream.Flush()
			return
		case <-time.After(delay):
		}
		if delay < 30*time.Second {
			delay *= 2
		}
	}
}

// renderEvent maps one EntryStream event to the HTML fragment the
// chat SSE bridge serves. Same render functions the legacy claude
// bridge uses, so both backends are visually indistinguishable.
func (b *HelixBridge) renderEvent(e helixclient.Event) string {
	switch e.Kind {
	case helixclient.EventAssistant:
		return renderAssistantText(e.Text)
	case helixclient.EventToolUse:
		return renderToolUse(e.ToolName, e.Text)
	case helixclient.EventToolResult:
		return renderToolResult(e.Text, false)
	case helixclient.EventToolResultError:
		return renderToolResult(e.Text, true)
	case helixclient.EventError:
		// Suppress the warmup-race error chip — it only fires while
		// the desktop's Zed agent is still booting, and warmupAndRetry
		// re-sends the prompt automatically. Showing it would leak a
		// confusing scary message every few seconds during the cold
		// start.
		if strings.Contains(e.Text, "no external agent WebSocket connection") {
			return ""
		}
		return renderTurnError(e.Text)
	}
	return ""
}

// NewHandler wipes the current session pointer at /ui/chat/new. The
// next Send opens a fresh Helix session. SSE listeners stay
// connected; the broadcaster keeps publishing once the new WS reader
// starts.
func (b *HelixBridge) NewHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b.mu.Lock()
		if b.wsCancel != nil {
			b.wsCancel()
			b.wsCancel = nil
		}
		b.sessionID = ""
		b.freshFromNew = true
		b.seen = make(map[string]struct{})
		b.mu.Unlock()
		b.wsWG.Wait()
		b.logger.Info("chat helix session reset by user")
		w.Header().Set("HX-Redirect", "/ui/")
		w.WriteHeader(http.StatusOK)
	})
}

// SwitchHandler attaches the bridge to an existing Helix session at
// /ui/chat/switch. The form field "sid" carries the target ID; the
// next SSE listener picks up the new session's stream.
func (b *HelixBridge) SwitchHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		sid := strings.TrimSpace(r.PostFormValue("sid"))
		if sid == "" {
			http.Error(w, "sid required", http.StatusBadRequest)
			return
		}
		b.attachSession(sid)
		w.Header().Set("HX-Redirect", "/ui/?sid="+sid)
		w.WriteHeader(http.StatusOK)
	})
}

// CommandsHandler renders the slash-command typeahead at
// /ui/chat/commands. Identical to the claude bridge's behaviour;
// reusing renderSlashSuggestion keeps both backends visually
// indistinguishable.
func (b *HelixBridge) CommandsHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if b.prompts == nil {
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
		if err := r.ParseForm(); err != nil {
			return
		}
		msg := r.PostFormValue("message")
		if !strings.HasPrefix(msg, "/") {
			return
		}
		token, _, _ := strings.Cut(msg[1:], " ")
		prefix := strings.ToLower(token)

		all := b.prompts.All()
		matches := make([]prompts.Prompt, 0, len(all))
		for _, p := range all {
			if strings.HasPrefix(strings.ToLower(string(p.Name())), prefix) {
				matches = append(matches, p)
			}
		}
		sort.Slice(matches, func(i, j int) bool { return matches[i].Name() < matches[j].Name() })
		for _, p := range matches {
			_, _ = fmt.Fprint(w, renderSlashSuggestion(p))
		}
	})
}

// expandSlashCommand mirrors the claude bridge's behaviour. Slash
// commands are resolved server-side by the prompt registry; the
// rendered text replaces the user input before posting to Helix.
func (b *HelixBridge) expandSlashCommand(ctx context.Context, msg string) (string, bool) {
	if b.prompts == nil || !strings.HasPrefix(msg, "/") {
		return "", false
	}
	name, rest, _ := strings.Cut(msg[1:], " ")
	if name == "" {
		return "", false
	}
	p, err := b.prompts.Get(prompts.Name(name))
	if err != nil {
		return "", false
	}
	args := map[string]string{}
	rest = strings.TrimSpace(rest)
	if rest != "" {
		if a := p.Arguments(); len(a) > 0 {
			args[a[0].Name] = rest
		}
	}
	rendered, err := p.Render(ctx, args)
	if err != nil {
		b.logger.Info("chat slash command render failed", "name", name, "err", err)
		return "", false
	}
	parts := make([]string, 0, len(rendered))
	for _, m := range rendered {
		parts = append(parts, m.Text)
	}
	return strings.Join(parts, "\n\n"), true
}

// resolveProjectOrg returns the project's organization_id, caching
// the result so we make at most one GetProject call per project per
// process. We MUST send organization_id on /sessions/chat — Helix's
// handler doesn't auto-populate it from project_id, and the desktop
// quota check defaults to the user's personal org (limit 2) when
// missing.
func (b *HelixBridge) resolveProjectOrg(ctx context.Context, projectID string) (string, error) {
	b.mu.Lock()
	if orgID, ok := b.orgIDByProject[projectID]; ok {
		b.mu.Unlock()
		return orgID, nil
	}
	b.mu.Unlock()
	proj, err := b.client.GetProject(ctx, projectID)
	if err != nil {
		return "", err
	}
	b.mu.Lock()
	b.orgIDByProject[projectID] = proj.OrganizationID
	b.mu.Unlock()
	return proj.OrganizationID, nil
}

// jsonField is a tiny helper used by render translation when peeking
// at structured Helix payloads we don't fully model.
func jsonField(raw json.RawMessage, key string) string {
	if len(raw) == 0 {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return ""
	}
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// keep compiler happy if jsonField becomes unused as we evolve renderHelixFrames
var _ = jsonField
