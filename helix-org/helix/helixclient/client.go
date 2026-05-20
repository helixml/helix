// Package helixclient is a thin REST + WebSocket client for the
// co-located Helix server.
//
// Scope (after the per-Worker-project refactor):
//   - Project lifecycle via the declarative apply endpoint.
//   - Project secrets — env-var injection into agent containers.
//   - Git contents — reading and writing job/* files on the helix-specs branch.
//   - Chat session lifecycle (start, get, stop, output, live updates).
//
// All shapes mirror Helix's `api/pkg/types` so the client posts exactly
// what Helix expects with no translation layer.
package helixclient

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// Default per-call timeout for REST calls. The WebSocket has no
// timeout — the caller controls its lifetime via context.
const defaultRESTTimeout = 30 * time.Second

// ErrNotFound wraps every 404 response so callers can detect a
// missing resource with errors.Is and react (e.g. clear stale
// persisted IDs and re-create).
var ErrNotFound = errors.New("helix: resource not found")

// Client is the surface helix-org depends on. Defining it as an
// interface lets tests inject a fake without HTTP.
type Client interface {
	// Connectivity probe. Returns the authenticated user.
	WhoAmI(ctx context.Context) (UserStatus, error)

	// ServerStatus returns the operator-facing /api/v1/config snapshot.
	// helix-org only consumes the desktop-quota fields today (see
	// CheckDesktopQuota); the rest are surfaced for forward
	// compatibility.
	ServerStatus(ctx context.Context) (ServerStatus, error)

	// ListProviders returns the slug list Helix exposes at
	// /api/v1/providers (e.g. ["openai","anthropic","helix",…]). Used
	// to validate `chat.provider` at startup so a typo doesn't surface
	// as a confusing 422 from /sessions/{id}/zed-config much later in
	// the request chain.
	ListProviders(ctx context.Context) ([]string, error)
	// ListModelsForProvider returns the list of model IDs the given
	// provider exposes. IDs are bare (no `provider/` prefix) — that's
	// the shape Helix uses everywhere except the OpenAI-aggregate
	// endpoint (which itself is unreliable for Anthropic since Helix
	// gates anthropic models behind the `anthropic-version` header).
	// Combined with ListProviders, callers can validate a
	// (provider, model) pair against this Helix instance before
	// applying any per-Worker project that references it.
	ListModelsForProvider(ctx context.Context, provider string) ([]Model, error)

	// Project lifecycle. helix-org applies one project per Worker.
	// ApplyProject is upsert-by-name within the operator's org.
	ApplyProject(ctx context.Context, req ProjectApplyRequest) (ProjectApplyResponse, error)
	GetProject(ctx context.Context, id string) (Project, error)
	DeleteProject(ctx context.Context, id string) error

	// Project secrets. Written via /projects/{id}/secrets; surface as
	// env vars inside the agent's container at session start.
	PutProjectSecret(ctx context.Context, projectID, name, value string) error

	// Git contents. helix-org writes job/role.md, job/identity.md,
	// job/agent.md to the project's primary repo at the helix-specs
	// branch. content (passed plain) is base64-encoded by PutFile.
	PutFile(ctx context.Context, repoID string, req PutFileRequest) error
	GetFile(ctx context.Context, repoID, path, branch string) (string, error)

	// Repository creation + attachment. Helix's project-apply does NOT
	// auto-create a default repository; the desktop's startup script
	// then refuses to launch Zed (`No repositories were cloned
	// successfully`). For our owner-chat / org-graph use case we don't
	// need a *real* code repo, just a Helix-internal one to satisfy
	// the workspace check. Two-step: CreateGitRepo → AttachRepo (with
	// primary=true).
	CreateGitRepo(ctx context.Context, req CreateGitRepoRequest) (GitRepo, error)
	AttachRepoToProject(ctx context.Context, projectID, repoID string, primary bool) error
	// CreateBranch makes a new branch from baseBranch on the repo. Used
	// by HelixProjectApplier to ensure `helix-specs` exists before
	// pushing role/identity files there — the desktop's startup script
	// only creates the helix-specs worktree if the branch is on the
	// remote.
	CreateBranch(ctx context.Context, repoID, branch, baseBranch string) error

	// App lifecycle. Used by the chat backend to provision a
	// helix_basic Assistant with MCPs — Helix's `/projects/apply`
	// only creates zed_external Agent Apps (`projectAgentRuntimeToTypes`
	// hard-codes that), so chat-only surfaces that need MCP tool
	// wiring without a sandbox runner take this separate path.
	CreateApp(ctx context.Context, req AppRequest) (App, error)
	GetApp(ctx context.Context, id string) (App, error)
	UpdateApp(ctx context.Context, id string, req AppRequest) (App, error)

	// Chat session lifecycle.
	//
	// StartChat opens a new session (Messages[0] becomes the first
	// turn). Use this only for *first* contact — once the session ID
	// is persisted, subsequent messages must go through
	// SendSessionMessage so they queue durably across cold starts.
	StartChat(ctx context.Context, req StartChatRequest) (Session, error)
	// StartChatWithStatus is the streaming-aware variant: same wire
	// call as StartChat, but additionally reports whether the SSE
	// stream surfaced a transient "no agent WS" error after the
	// session ID came through. Callers use the flag to decide whether
	// to immediately re-queue the same prompt via SendSessionMessage
	// (which queues durably and is delivered on agent reconnect).
	StartChatWithStatus(ctx context.Context, req StartChatRequest) (Session, bool, error)
	// SendSessionMessage POSTs a message to an existing session via
	// /api/v1/sessions/{id}/messages. Helix persists the interaction
	// and `pickupWaitingInteraction` delivers it once the agent's
	// WebSocket is reachable — no client-side warmup loop required.
	// Returns 200 even when no agent is connected yet.
	SendSessionMessage(ctx context.Context, sessionID, content string, opts SendMessageOptions) (SendMessageResponse, error)
	GetSession(ctx context.Context, id string) (Session, error)
	GetOutput(ctx context.Context, sessionID string) (Output, error)
	SubscribeUpdates(ctx context.Context, sessionID string) (<-chan SessionUpdate, error)
	StopExternalAgent(ctx context.Context, sessionID string) error
}

// SendMessageOptions are the optional knobs on SendSessionMessage.
// Interrupt mirrors the frontend RobustPromptInput's interrupt flag —
// set true to cancel any in-flight generation before queueing this
// message. NotifyUserID populates Helix's commenter mappings so
// response notifications route to a third party (used by the
// design-review path; helix-org leaves it empty).
type SendMessageOptions struct {
	Interrupt    bool
	NotifyUserID string
}

// SendMessageResponse mirrors Helix's POST /sessions/{id}/messages
// response body. Both IDs are returned so the caller can correlate
// notifications even if the message was queued (no WS) at the time
// of the call.
type SendMessageResponse struct {
	RequestID     string `json:"request_id"`
	InteractionID string `json:"interaction_id"`
}

// ServerStatus mirrors the slice of /api/v1/config helix-org reads.
// Today only the desktop-quota fields are consumed, surfaced as a
// pre-flight gate before opening a new zed_external session.
type ServerStatus struct {
	MaxConcurrentDesktops    int `json:"max_concurrent_desktops"`
	ActiveConcurrentDesktops int `json:"active_concurrent_desktops"`
}

// HasDesktopRoom reports whether at least one desktop slot is free
// against the operator-configured cap. Returns true if the server has
// no quota configured (Max == 0) — Helix uses 0 to mean "unlimited"
// at the server level.
func (s ServerStatus) HasDesktopRoom() bool {
	if s.MaxConcurrentDesktops <= 0 {
		return true
	}
	return s.ActiveConcurrentDesktops < s.MaxConcurrentDesktops
}

// Model is one entry from /v1/models. Helix ships an OpenAI-compatible
// model catalogue; only ID and Enabled are consumed today. ID is in the
// form `provider/model` (e.g. "anthropic/claude-opus-4-6"); Enabled is
// false for models the operator has hidden.
type Model struct {
	ID      string `json:"id"`
	Enabled bool   `json:"enabled"`
}

// UserStatus is the slim auth-probe response. Helix returns more
// fields; only `User` (the user ID) and `Slug` are consumed today,
// the latter for human-readable logs.
type UserStatus struct {
	Admin bool   `json:"admin"`
	User  string `json:"user"`
	Slug  string `json:"slug"`
}

// ProjectApplyRequest mirrors `types.ProjectApplyRequest`. The whole
// declarative project (repos, agent app, startup script, kanban
// settings, …) is described in the embedded `Spec`.
type ProjectApplyRequest struct {
	OrganizationID string      `json:"organization_id,omitempty"`
	Name           string      `json:"name"`
	Spec           ProjectSpec `json:"spec"`
}

// ProjectApplyResponse mirrors `types.ProjectApplyResponse`. We
// always read both IDs — `ProjectID` for chat sessions and git
// writes; `AgentAppID` for adding the org-graph MCP server.
type ProjectApplyResponse struct {
	ProjectID  string `json:"project_id"`
	AgentAppID string `json:"agent_app_id,omitempty"`
	Created    bool   `json:"created"`
}

// ProjectSpec mirrors `types.ProjectSpec`. helix-org populates only
// the subset relevant to the per-Worker-project model: name (set on
// the wrapping request), description, agent, startup, repositories.
type ProjectSpec struct {
	Description  string                  `json:"description,omitempty"`
	Technologies []string                `json:"technologies,omitempty"`
	Guidelines   string                  `json:"guidelines,omitempty"`
	Repositories []ProjectRepositorySpec `json:"repositories,omitempty"`
	Startup      *ProjectStartup         `json:"startup,omitempty"`
	Agent        *ProjectAgentSpec       `json:"agent,omitempty"`
}

// ProjectRepositorySpec describes a repository attachment.
type ProjectRepositorySpec struct {
	URL           string `json:"url"`
	DefaultBranch string `json:"default_branch,omitempty"`
	Primary       bool   `json:"primary,omitempty"`
}

// ProjectStartup is the script run on agent-container startup.
type ProjectStartup struct {
	Script string `json:"script,omitempty"`
}

// ProjectAgentSpec configures the auto-provisioned Agent App.
type ProjectAgentSpec struct {
	Name        string             `json:"name,omitempty"`
	Runtime     string             `json:"runtime,omitempty"` // "claude_code", "zed", …
	Model       string             `json:"model,omitempty"`
	Provider    string             `json:"provider,omitempty"`
	Credentials string             `json:"credentials,omitempty"`
	Tools       *ProjectAgentTools `json:"tools,omitempty"`
}

// ProjectAgentTools enables the simple built-in tools (web search,
// browser, calculator). MCP servers are added separately via the
// Agent App's `assistants[0].mcps[]` once Helix exposes a per-app
// MCP-write endpoint; today helix-org bundles the MCP wiring into
// the project apply where supported and otherwise treats this as a
// follow-up step.
type ProjectAgentTools struct {
	WebSearch  bool `json:"web_search,omitempty"`
	Browser    bool `json:"browser,omitempty"`
	Calculator bool `json:"calculator,omitempty"`
}

// Project mirrors the slice of `types.Project` helix-org reads.
type Project struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	UserID         string `json:"user_id"`
	OrganizationID string `json:"organization_id"`
	DefaultRepoID  string `json:"default_repo_id"`
}

// CreateGitRepoRequest is the helix-org → Helix payload for POST
// /api/v1/git/repositories. We only ever create Helix-internal repos
// (no external git URL), so most of `types.GitRepositoryCreateRequest`
// is irrelevant. Required: Name, OwnerID. OrganizationID for org-scoped
// projects.
type CreateGitRepoRequest struct {
	Name           string            `json:"name"`
	OwnerID        string            `json:"owner_id"`
	OrganizationID string            `json:"organization_id,omitempty"`
	RepoType       string            `json:"repo_type,omitempty"` // defaults to "code"
	DefaultBranch  string            `json:"default_branch,omitempty"`
	IsExternal     bool              `json:"is_external"`             // always false for helix-org
	InitialFiles   map[string]string `json:"initial_files,omitempty"` // seed the default branch so subsequent PutFile to other branches has something to fork from
}

// GitRepo is the slice of `types.GitRepository` helix-org reads —
// just the ID, which is the value we attach to the project.
type GitRepo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// AppRequest is the body sent to POST /apps and PUT /apps/{id}.
// Config is opaque JSON — callers either build it themselves
// (e.g. helix-org's project-apply step doesn't write Apps directly,
// it only updates the auto-provisioned one to attach MCPs) or use
// AttachMCPToApp, which round-trips the live config to avoid
// dropping unknown fields.
type AppRequest struct {
	OrganizationID string          `json:"organization_id,omitempty"`
	Global         bool            `json:"global,omitempty"`
	Config         json.RawMessage `json:"config,omitempty"`
}

// App mirrors the slice of `types.App` helix-org reads. Config is
// raw — callers parse only what they need.
type App struct {
	ID     string          `json:"id"`
	Owner  string          `json:"owner"`
	Config json.RawMessage `json:"config"`
}

// PutFileRequest mirrors `types.UpdateGitRepositoryFileContentsRequest`.
// `Content` is plain text; PutFile base64-encodes it for the operator.
type PutFileRequest struct {
	Path    string
	Branch  string
	Message string
	Author  string
	Email   string
	Content string
}

// StartChatRequest is the helix-org → Helix payload that opens a new
// chat session (or continues one when SessionID is set). Mirrors
// `types.SessionChatRequest`.
type StartChatRequest struct {
	ProjectID           string               `json:"project_id"`
	OrganizationID      string               `json:"organization_id,omitempty"`
	SessionID           string               `json:"session_id,omitempty"`
	SessionRole         string               `json:"session_role,omitempty"`
	AgentType           string               `json:"agent_type,omitempty"`
	AppID               string               `json:"app_id,omitempty"`
	AssistantID         string               `json:"assistant_id,omitempty"`
	Type                string               `json:"type,omitempty"`
	ExternalAgentConfig *ExternalAgentConfig `json:"external_agent_config,omitempty"`
	SystemPrompt        string               `json:"system,omitempty"`
	Messages            []SessionChatMessage `json:"messages"`
	Stream              bool                 `json:"stream,omitempty"`
	Provider            string               `json:"provider,omitempty"`
	Model               string               `json:"model,omitempty"`
	CallbackURL         string               `json:"callback_url,omitempty"`

	// OnSessionID, if set, is invoked as soon as the SSE stream emits
	// the session ID chunk — before the agent has produced a reply.
	// Callers use this to attach a WS subscriber early so they don't
	// miss the response while the SSE loop keeps reading (looking for
	// later error chunks). Not serialised.
	OnSessionID func(sessionID string) `json:"-"`
}

// ExternalAgentConfig must be sent as a non-nil object whenever
// AgentType=zed_external — Helix uses presence-of-object to wire up
// a runner.
type ExternalAgentConfig struct {
	Resolution    string `json:"resolution,omitempty"`
	DisplayWidth  int    `json:"display_width,omitempty"`
	DisplayHeight int    `json:"display_height,omitempty"`
	DesktopType   string `json:"desktop_type,omitempty"`
}

// SessionChatMessage is one entry in a SessionChatRequest.Messages
// array. Helix's Message struct is OpenAI-style multipart; we only
// ever send a single text part.
type SessionChatMessage struct {
	Role    string         `json:"role"`
	Content MessageContent `json:"content"`
}

// MessageContent is the multipart body. helix-org only ever sends a
// single text part. We omit content_type to match the wire shape the
// Helix UI sends ({"parts":[...]}); Helix infers text from the part
// type.
type MessageContent struct {
	Parts []any `json:"parts"`
}

// NewTextMessage builds a single user text message — the only shape
// helix-org ever sends.
func NewTextMessage(role, text string) SessionChatMessage {
	return SessionChatMessage{
		Role:    role,
		Content: MessageContent{Parts: []any{text}},
	}
}

// Output is the polling result for a session. Mirrors
// `types.SessionOutputResponse`. Status: "waiting" | "complete" | "error".
type Output struct {
	SessionID  string `json:"session_id"`
	Status     string `json:"status"`
	Output     string `json:"output"`
	DurationMs int64  `json:"duration_ms"`
}

// IsTerminal reports whether o.Status indicates the session is done.
func (o Output) IsTerminal() bool {
	return o.Status == "complete" || o.Status == "error"
}

// SessionUpdate is one frame from `/api/v1/ws/user`. Mirrors
// `types.WebsocketEvent`. The streaming payload helix-org consumes
// is `interaction_patch` carrying `EntryPatches[]` — the per-entry
// typed deltas Helix uses for response-entries streaming.
//
// session_update / interaction_update frames are still observed
// (final-state snapshots), but EntryPatches are the source of truth
// for assistant text + tool calls during a turn.
type SessionUpdate struct {
	Type          string       `json:"type"`
	SessionID     string       `json:"session_id"`
	InteractionID string       `json:"interaction_id"`
	Owner         string       `json:"owner"`
	Session       *Session     `json:"session,omitempty"`
	Interaction   *Interaction `json:"interaction,omitempty"`
	EntryCount    int          `json:"entry_count,omitempty"`
	EntryPatches  []EntryPatch `json:"entry_patches,omitempty"`
}

// EntryPatch is one per-entry delta. Mirrors `types.EntryPatch`.
//
//   - Index identifies the entry within the interaction.
//   - MessageID is the entry's identity — re-using a message ID
//     means "extend this entry"; a new ID means "this is a new
//     entry at the same Index" (e.g. a tool_call following text).
//   - Patch is the text delta to splice in at PatchOffset (UTF-16).
//   - Type is "text" or "tool_call".
//   - For tool_call entries, ToolName/ToolStatus carry metadata.
type EntryPatch struct {
	Index       int    `json:"index"`
	MessageID   string `json:"message_id"`
	Type        string `json:"type"`
	Patch       string `json:"patch,omitempty"`
	PatchOffset int    `json:"patch_offset,omitempty"`
	TotalLength int    `json:"total_length,omitempty"`
	ToolName    string `json:"tool_name,omitempty"`
	ToolStatus  string `json:"tool_status,omitempty"`
}

// Session is the subset of Helix's Session struct we read.
type Session struct {
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	ProjectID     string         `json:"project_id"`
	ParentApp     string         `json:"parent_app,omitempty"`
	DefaultRepoID string         `json:"default_repo_id,omitempty"`
	Interactions  []*Interaction `json:"interactions,omitempty"`
}

// Interaction collects what an assistant produced in one turn.
// PromptMessage carries the user input that opened the turn (Helix
// stores it on the same row as the response). The chat UI uses both
// fields to reconstruct history on refresh.
type Interaction struct {
	ID              string           `json:"id"`
	GenerationID    int              `json:"generation_id"`
	State           string           `json:"state"`
	Status          string           `json:"status"`
	Error           string           `json:"error"`
	PromptMessage   string           `json:"prompt_message,omitempty"`
	ResponseMessage string           `json:"response_message,omitempty"`
	ToolCalls       []OpenAIToolCall `json:"tool_calls,omitempty"`
	ResponseEntries json.RawMessage  `json:"response_entries,omitempty"`
}

// OpenAIToolCall mirrors the openai.ToolCall shape.
type OpenAIToolCall struct {
	ID       string             `json:"id,omitempty"`
	Type     string             `json:"type,omitempty"`
	Function OpenAIFunctionCall `json:"function"`
}

// OpenAIFunctionCall is the "function" payload of a ToolCall.
type OpenAIFunctionCall struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

// Config configures a real HTTP+WS Client.
type Config struct {
	BaseURL string
	APIKey  string
	HTTP    *http.Client
}

// New constructs a real Client backed by HTTP and gorilla/websocket.
func New(cfg Config) (Client, error) {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return nil, errors.New("helixclient: BaseURL is required")
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, errors.New("helixclient: APIKey is required")
	}
	hc := cfg.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: defaultRESTTimeout}
	}
	return &realClient{base: strings.TrimRight(cfg.BaseURL, "/"), apiKey: cfg.APIKey, http: hc}, nil
}

type realClient struct {
	base   string
	apiKey string
	http   *http.Client
}

// bearerTokenKey is the context.Context key under which an embedding
// host (e.g. helix-org running inside Helix) can stash a per-request
// bearer token, overriding the static apiKey the Client was built
// with. The handler chain reads the host's authenticated user, looks
// up that user's own api key, and injects it here — so calls hit
// Helix as the actual logged-in user rather than as a shared service
// account. Stays nil-safe: when not set, the client falls back to
// the constructor-supplied apiKey.
type bearerTokenKey struct{}

// WithBearerToken returns a context that carries the given token as
// the bearer realClient uses on its next request. Empty token is a
// no-op (returns the unchanged context). Intended for handler-side
// code that knows the per-request identity; bridge / spawner code
// elsewhere in helix-org keeps passing the raw context untouched.
func WithBearerToken(ctx context.Context, token string) context.Context {
	if token == "" {
		return ctx
	}
	return context.WithValue(ctx, bearerTokenKey{}, token)
}

// BearerFromContext returns the per-request bearer stashed by
// WithBearerToken, or "" if none. Lets callers outside this package
// (e.g. helix-org tools persisting the hiring user's identity onto
// a Worker's runtime state) discover the live bearer without
// dragging the bearerTokenKey type out.
func BearerFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(bearerTokenKey{}).(string); ok {
		return v
	}
	return ""
}

// userIDKey is the context key for the upstream caller's user
// identifier (typically a Helix user_id). Stashed alongside the
// bearer by the embedding host's HTTP handler so tools can persist
// the identity onto domain state without holding the api_key itself.
type userIDKey struct{}

// WithUserID returns a context carrying userID for later retrieval
// via UserIDFromContext. Empty userID is a no-op. The bearer and
// user ID are independent: the bearer authenticates THIS request,
// the user ID identifies who later activations should run as. A
// host may set one without the other.
func WithUserID(ctx context.Context, userID string) context.Context {
	if userID == "" {
		return ctx
	}
	return context.WithValue(ctx, userIDKey{}, userID)
}

// UserIDFromContext returns the user identifier stashed by
// WithUserID, or "" if none.
func UserIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(userIDKey{}).(string); ok {
		return v
	}
	return ""
}

// bearer returns the token realClient should send: the per-request
// override if one is in ctx, otherwise the static apiKey.
func (c *realClient) bearer(ctx context.Context) string {
	if v, ok := ctx.Value(bearerTokenKey{}).(string); ok && v != "" {
		return v
	}
	return c.apiKey
}

// do is the shared HTTP execution path. body may be nil. If out is
// non-nil and the response is 2xx, the body is JSON-decoded into out.
func (c *realClient) do(ctx context.Context, method, path string, body any, out any) error {
	var rdr io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal: %w", err)
		}
		rdr = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.bearer(ctx))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		base := fmt.Errorf("%s %s: %s: %s", method, path, resp.Status, strings.TrimSpace(string(raw)))
		if resp.StatusCode == http.StatusNotFound {
			return fmt.Errorf("%w: %w", ErrNotFound, base)
		}
		return base
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}

// ---- WhoAmI ----

func (c *realClient) WhoAmI(ctx context.Context) (UserStatus, error) {
	var us UserStatus
	if err := c.do(ctx, http.MethodGet, "/api/v1/status", nil, &us); err != nil {
		return UserStatus{}, err
	}
	return us, nil
}

// ServerStatus calls GET /api/v1/config.
func (c *realClient) ServerStatus(ctx context.Context) (ServerStatus, error) {
	var st ServerStatus
	if err := c.do(ctx, http.MethodGet, "/api/v1/config", nil, &st); err != nil {
		return ServerStatus{}, err
	}
	return st, nil
}

// CheckDesktopQuota refuses the call when the operator-configured
// `max_concurrent_desktops` would be exceeded by spinning up one more
// session. Returns nil on success; on a real network/API error
// transports the failure verbatim. Used as a pre-flight before any
// helix-org code path opens a *new* zed_external session — follow-ups
// reuse an already-running container and don't need this check.
//
// The error message names every active session helix-org knows about
// so the operator can decide which one to stop. We can't enumerate
// other users' sessions from here — they show up in the count, not
// the list.
func CheckDesktopQuota(ctx context.Context, c Client) error {
	st, err := c.ServerStatus(ctx)
	if err != nil {
		return fmt.Errorf("read server status: %w", err)
	}
	if st.HasDesktopRoom() {
		return nil
	}
	return fmt.Errorf("desktop quota reached on Helix (%d/%d active) — stop one of the existing sessions before opening a new one", st.ActiveConcurrentDesktops, st.MaxConcurrentDesktops)
}

// ListProviders calls GET /api/v1/providers. Returns the list of
// provider slugs the operator has configured on this Helix instance.
func (c *realClient) ListProviders(ctx context.Context) ([]string, error) {
	var providers []string
	if err := c.do(ctx, http.MethodGet, "/api/v1/providers", nil, &providers); err != nil {
		return nil, err
	}
	return providers, nil
}

// ListModelsForProvider calls GET /v1/models?provider=<provider>. Helix
// returns an OpenAI-compatible `{"data":[…]}` envelope; we unwrap it
// and surface the slim Model shape. IDs are bare (no `provider/` prefix).
func (c *realClient) ListModelsForProvider(ctx context.Context, provider string) ([]Model, error) {
	var resp struct {
		Data []Model `json:"data"`
	}
	path := "/v1/models?provider=" + url.QueryEscape(provider)
	if err := c.do(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// ValidateProviderModel checks that `provider` exists in
// /api/v1/providers and that `provider/model` exists (and is enabled)
// in /v1/models. Returns a descriptive error pointing at the first
// missing piece — designed to be surfaced verbatim to operators at
// startup so a typo in `chat.provider` / `chat.model` doesn't get
// papered over and surface as a confusing 422 from /zed-config much
// later in the request chain.
func ValidateProviderModel(ctx context.Context, c Client, provider, model string) error {
	if strings.TrimSpace(provider) == "" || strings.TrimSpace(model) == "" {
		return fmt.Errorf("validate provider/model: both provider and model are required (got provider=%q model=%q)", provider, model)
	}
	providers, err := c.ListProviders(ctx)
	if err != nil {
		return fmt.Errorf("list providers: %w", err)
	}
	known := false
	for _, p := range providers {
		if p == provider {
			known = true
			break
		}
	}
	if !known {
		return fmt.Errorf("provider %q not configured on Helix (got %v) — set chat.provider to one of these", provider, providers)
	}
	models, err := c.ListModelsForProvider(ctx, provider)
	if err != nil {
		return fmt.Errorf("list models for %q: %w", provider, err)
	}
	for _, m := range models {
		if m.ID == model {
			if !m.Enabled {
				return fmt.Errorf("model %q on provider %q exists but is disabled on Helix — pick a different chat.model or have the operator re-enable it", model, provider)
			}
			return nil
		}
	}
	available := make([]string, 0, len(models))
	for _, m := range models {
		if m.Enabled {
			available = append(available, m.ID)
		}
	}
	return fmt.Errorf("model %q not found on provider %q — available: %v", model, provider, available)
}

// ---- Project lifecycle ----

func (c *realClient) ApplyProject(ctx context.Context, req ProjectApplyRequest) (ProjectApplyResponse, error) {
	var resp ProjectApplyResponse
	if err := c.do(ctx, http.MethodPut, "/api/v1/projects/apply", req, &resp); err != nil {
		return ProjectApplyResponse{}, err
	}
	if resp.ProjectID == "" {
		return ProjectApplyResponse{}, errors.New("apply project: empty project_id in response")
	}
	return resp, nil
}

func (c *realClient) GetProject(ctx context.Context, id string) (Project, error) {
	var p Project
	if err := c.do(ctx, http.MethodGet, "/api/v1/projects/"+url.PathEscape(id), nil, &p); err != nil {
		return Project{}, err
	}
	return p, nil
}

func (c *realClient) DeleteProject(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/projects/"+url.PathEscape(id), nil, nil)
}

// ---- Git repository lifecycle ----

func (c *realClient) CreateGitRepo(ctx context.Context, req CreateGitRepoRequest) (GitRepo, error) {
	if req.RepoType == "" {
		req.RepoType = "code"
	}
	if req.DefaultBranch == "" {
		req.DefaultBranch = "main"
	}
	var resp GitRepo
	if err := c.do(ctx, http.MethodPost, "/api/v1/git/repositories", req, &resp); err != nil {
		return GitRepo{}, err
	}
	if resp.ID == "" {
		return GitRepo{}, errors.New("create git repo: empty id in response")
	}
	return resp, nil
}

func (c *realClient) CreateBranch(ctx context.Context, repoID, branch, baseBranch string) error {
	body := struct {
		BranchName string `json:"branch_name"`
		BaseBranch string `json:"base_branch,omitempty"`
	}{BranchName: branch, BaseBranch: baseBranch}
	if err := c.do(ctx, http.MethodPost, "/api/v1/git/repositories/"+url.PathEscape(repoID)+"/branches", body, nil); err != nil {
		return fmt.Errorf("create branch %s on %s: %w", branch, repoID, err)
	}
	return nil
}

func (c *realClient) AttachRepoToProject(ctx context.Context, projectID, repoID string, primary bool) error {
	if err := c.do(ctx, http.MethodPut, "/api/v1/projects/"+url.PathEscape(projectID)+"/repositories/"+url.PathEscape(repoID)+"/attach", nil, nil); err != nil {
		return fmt.Errorf("attach repo: %w", err)
	}
	if primary {
		if err := c.do(ctx, http.MethodPut, "/api/v1/projects/"+url.PathEscape(projectID)+"/repositories/"+url.PathEscape(repoID)+"/primary", nil, nil); err != nil {
			return fmt.Errorf("set primary repo: %w", err)
		}
	}
	return nil
}

// ---- App lifecycle ----

func (c *realClient) CreateApp(ctx context.Context, req AppRequest) (App, error) {
	var resp App
	if err := c.do(ctx, http.MethodPost, "/api/v1/apps", req, &resp); err != nil {
		return App{}, err
	}
	if resp.ID == "" {
		return App{}, errors.New("create app: empty id in response")
	}
	return resp, nil
}

func (c *realClient) GetApp(ctx context.Context, id string) (App, error) {
	var resp App
	if err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+url.PathEscape(id), nil, &resp); err != nil {
		return App{}, err
	}
	return resp, nil
}

// UpdateApp puts to /api/v1/apps/{id}. Helix's handler reads the
// app ID from the request *body* (not the URL path), so the `id`
// field is added to the JSON body alongside the request fields.
func (c *realClient) UpdateApp(ctx context.Context, id string, req AppRequest) (App, error) {
	body := struct {
		ID             string          `json:"id"`
		OrganizationID string          `json:"organization_id,omitempty"`
		Global         bool            `json:"global,omitempty"`
		Config         json.RawMessage `json:"config,omitempty"`
	}{
		ID:             id,
		OrganizationID: req.OrganizationID,
		Global:         req.Global,
		Config:         req.Config,
	}
	var resp App
	if err := c.do(ctx, http.MethodPut, "/api/v1/apps/"+url.PathEscape(id), body, &resp); err != nil {
		return App{}, err
	}
	return resp, nil
}

// AttachMCPToApp adds (or updates) a single HTTP MCP server entry
// on an App's first Assistant. Idempotent — replaces any existing
// entry whose `name` matches.
//
// Implemented as a get-mutate-put round-trip with the raw config as
// `map[string]any` so unknown fields (everything Helix's
// AssistantConfig has that helix-org doesn't model) survive.
//
// Used by helix-org's per-Worker project apply: project-apply
// auto-provisions a `zed_external` Agent App but doesn't accept
// MCPs in its spec, so we attach them in this second step.
func AttachMCPToApp(ctx context.Context, c Client, appID, name, transport, mcpURL string) error {
	return AttachMCPToAppWithHeaders(ctx, c, appID, name, transport, mcpURL, nil)
}

// AttachMCPToAppWithHeaders is AttachMCPToApp plus an optional headers
// map that becomes the MCP entry's `headers` field. Used when the MCP
// endpoint sits behind Helix auth and the calling agent needs to carry
// an explicit Authorization header (or any other static header).
func AttachMCPToAppWithHeaders(ctx context.Context, c Client, appID, name, transport, mcpURL string, headers map[string]string) error {
	if appID == "" {
		return errors.New("AttachMCPToApp: appID is empty")
	}
	app, err := c.GetApp(ctx, appID)
	if err != nil {
		return fmt.Errorf("get app: %w", err)
	}
	var raw map[string]any
	if len(app.Config) == 0 {
		raw = map[string]any{}
	} else if err := json.Unmarshal(app.Config, &raw); err != nil {
		return fmt.Errorf("decode config: %w", err)
	}
	helix, _ := raw["helix"].(map[string]any)
	if helix == nil {
		helix = map[string]any{}
		raw["helix"] = helix
	}
	asstsAny, _ := helix["assistants"].([]any)
	if len(asstsAny) == 0 {
		return errors.New("AttachMCPToApp: app has no assistants")
	}
	asst, _ := asstsAny[0].(map[string]any)
	if asst == nil {
		return errors.New("AttachMCPToApp: assistant is not an object")
	}
	mcpsAny, _ := asst["mcps"].([]any)
	mcps := make([]any, 0, len(mcpsAny)+1)
	replaced := false
	for _, mAny := range mcpsAny {
		m, ok := mAny.(map[string]any)
		if !ok {
			mcps = append(mcps, mAny)
			continue
		}
		if m["name"] == name {
			m["transport"] = transport
			m["url"] = mcpURL
			if len(headers) > 0 {
				m["headers"] = headers
			} else {
				delete(m, "headers")
			}
			replaced = true
		}
		mcps = append(mcps, m)
	}
	if !replaced {
		entry := map[string]any{
			"name":      name,
			"transport": transport,
			"url":       mcpURL,
		}
		if len(headers) > 0 {
			entry["headers"] = headers
		}
		mcps = append(mcps, entry)
	}
	asst["mcps"] = mcps
	asstsAny[0] = asst
	helix["assistants"] = asstsAny
	raw["helix"] = helix
	body, err := json.Marshal(raw)
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	if _, err := c.UpdateApp(ctx, appID, AppRequest{Config: body}); err != nil {
		return fmt.Errorf("update app: %w", err)
	}
	return nil
}

// ---- Project secrets ----

func (c *realClient) PutProjectSecret(ctx context.Context, projectID, name, value string) error {
	body := map[string]string{"name": name, "value": value}
	return c.do(ctx, http.MethodPost, "/api/v1/projects/"+url.PathEscape(projectID)+"/secrets", body, nil)
}

// ---- Git contents ----

func (c *realClient) PutFile(ctx context.Context, repoID string, req PutFileRequest) error {
	body := map[string]string{
		"path":    req.Path,
		"branch":  req.Branch,
		"message": req.Message,
		"author":  req.Author,
		"email":   req.Email,
		"content": base64.StdEncoding.EncodeToString([]byte(req.Content)),
	}
	return c.do(ctx, http.MethodPut, "/api/v1/git/repositories/"+url.PathEscape(repoID)+"/contents", body, nil)
}

func (c *realClient) GetFile(ctx context.Context, repoID, path, branch string) (string, error) {
	q := url.Values{"path": {path}, "branch": {branch}}
	var resp struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/v1/git/repositories/"+url.PathEscape(repoID)+"/contents?"+q.Encode(), nil, &resp); err != nil {
		return "", err
	}
	// Helix's GET /contents returns raw plain text in the `content`
	// field for small files — not base64 like PutFile expects on the
	// way in. Try a base64 decode first; fall through to raw on
	// failure to handle both shapes.
	if decoded, err := base64.StdEncoding.DecodeString(resp.Content); err == nil {
		return string(decoded), nil
	}
	return resp.Content, nil
}

// ---- Chat session lifecycle ----

// SendToSession posts `prompt` to an existing Helix session via the
// /sessions/chat continuation path. Returns nil iff Helix accepted the
// message AND no error chunk appeared on the SSE stream. Either
// failure mode means the persisted session is dead on the server side
// (in-memory external-agent state evicted after restart, session row
// deleted, etc.) and the caller should treat the persisted SessionID
// as stale and open a fresh one.
//
// This is the single source of truth for "is this persisted session
// still usable?" — both the owner-chat bridge and the Spawner share
// it so worker activations and chat followups behave identically when
// the operator's api restarts overnight.
func SendToSession(ctx context.Context, client Client, req StartChatRequest) (Session, error) {
	if req.SessionID == "" {
		return Session{}, errors.New("SendToSession: SessionID required")
	}
	session, streamHadErr, err := client.StartChatWithStatus(ctx, req)
	if err != nil {
		return Session{}, err
	}
	if streamHadErr {
		return Session{}, errors.New("session no longer running on the server")
	}
	return session, nil
}

func (c *realClient) StartChat(ctx context.Context, req StartChatRequest) (Session, error) {
	s, _, err := c.startChat(ctx, req)
	return s, err
}

func (c *realClient) StartChatWithStatus(ctx context.Context, req StartChatRequest) (Session, bool, error) {
	return c.startChat(ctx, req)
}

// SendSessionMessage POSTs to /api/v1/sessions/{id}/messages. The
// endpoint persists a Waiting interaction and returns 200 even when
// the agent's WebSocket is not connected — pickupWaitingInteraction
// delivers the message on reconnect. This is the durable replacement
// for the client-side warmup loop helix-org used to run during cold
// starts.
func (c *realClient) SendSessionMessage(ctx context.Context, sessionID, content string, opts SendMessageOptions) (SendMessageResponse, error) {
	if strings.TrimSpace(sessionID) == "" {
		return SendMessageResponse{}, errors.New("SendSessionMessage: sessionID is empty")
	}
	body := struct {
		Content      string `json:"content"`
		Interrupt    bool   `json:"interrupt,omitempty"`
		NotifyUserID string `json:"notify_user_id,omitempty"`
	}{Content: content, Interrupt: opts.Interrupt, NotifyUserID: opts.NotifyUserID}
	var resp SendMessageResponse
	if err := c.do(ctx, http.MethodPost, "/api/v1/sessions/"+url.PathEscape(sessionID)+"/messages", body, &resp); err != nil {
		return SendMessageResponse{}, err
	}
	return resp, nil
}

func (c *realClient) startChat(ctx context.Context, req StartChatRequest) (Session, bool, error) {
	if req.Type == "" {
		req.Type = "text"
	}
	if len(req.Messages) == 0 {
		return Session{}, false, errors.New("StartChat: req.Messages must contain at least one message")
	}
	if req.AgentType == "zed_external" && req.ExternalAgentConfig == nil {
		req.ExternalAgentConfig = &ExternalAgentConfig{}
	}
	if req.AgentType == "zed_external" {
		req.Stream = true
		return c.startChatStreaming(ctx, req)
	}
	// helix_basic / helix_agent path: Helix runs the agent (LLM +
	// tools) synchronously inside this HTTP call, so a 30s default
	// REST timeout is too tight — multi-step reasoning with MCP tool
	// calls routinely takes a couple of minutes. Use a dedicated
	// long-timeout client for just this request rather than bumping
	// the shared default and slowing failure detection on every other
	// endpoint.
	buf, err := json.Marshal(req)
	if err != nil {
		return Session{}, false, fmt.Errorf("marshal: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/api/v1/sessions/chat", bytes.NewReader(buf))
	if err != nil {
		return Session{}, false, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.bearer(ctx))
	httpReq.Header.Set("Content-Type", "application/json")
	longClient := &http.Client{Timeout: 10 * time.Minute}
	resp, err := longClient.Do(httpReq)
	if err != nil {
		return Session{}, false, fmt.Errorf("POST /api/v1/sessions/chat: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return Session{}, false, fmt.Errorf("POST /api/v1/sessions/chat: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return Session{}, false, fmt.Errorf("read /api/v1/sessions/chat: %w", err)
	}
	s, err := parseStartChatResponse(raw)
	return s, false, err
}

// startChatStreaming POSTs to /sessions/chat with stream=true and
// reads SSE chunks. We extract the session ID from the first chunk
// (Helix writes it before any LLM/agent call happens) and return —
// any in-stream error from the agent dispatch is non-fatal because
// the interaction is already persisted server-side and will be
// picked up when the agent connects. The actual response is
// delivered via SubscribeUpdates.
//
// We deliberately detach the upstream HTTP request from the caller's
// ctx. Helix's handler holds the connection open during
// waitForExternalAgentReady (up to 5 minutes for a cold container)
// and runs the agent dispatch synchronously. If the caller's ctx
// cancels (which happens immediately when our /ui/chat/send handler
// returns 200 to the browser after we've grabbed the session ID),
// the upstream conn drops, Helix's request ctx cancels, and the wait
// fails with "0 attempts". Detaching keeps Helix's handler running
// long enough to complete startup; the body-drain goroutine reads to
// EOF and closes the connection cleanly.
func (c *realClient) startChatStreaming(ctx context.Context, req StartChatRequest) (Session, bool, error) {
	buf, err := json.Marshal(req)
	if err != nil {
		return Session{}, false, fmt.Errorf("marshal: %w", err)
	}
	upstreamCtx, upstreamCancel := context.WithTimeout(context.Background(), 10*time.Minute)
	httpReq, err := http.NewRequestWithContext(upstreamCtx, http.MethodPost, c.base+"/api/v1/sessions/chat", bytes.NewReader(buf))
	if err != nil {
		upstreamCancel()
		return Session{}, false, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.bearer(ctx))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	resp, err := c.http.Do(httpReq) //nolint:bodyclose // body is closed inside the drain goroutine below or on early-return paths; the lint can't follow it across the closure
	if err != nil {
		upstreamCancel()
		return Session{}, false, fmt.Errorf("POST /api/v1/sessions/chat: %w", err)
	}
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
		upstreamCancel()
		return Session{}, false, fmt.Errorf("POST /api/v1/sessions/chat: %s: %s", resp.Status, strings.TrimSpace(string(raw)))
	}
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var sessionID string
	var streamErrMsg string
	hadWSError := false
	for scanner.Scan() {
		line := scanner.Text()
		payload := strings.TrimSpace(line)
		if payload == "" {
			continue
		}
		payload = strings.TrimPrefix(payload, "data:")
		payload = strings.TrimSpace(payload)
		if payload == "" || payload == "[DONE]" {
			continue
		}
		var chunk struct {
			ID    string `json:"id"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error,omitempty"`
		}
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}
		if chunk.ID != "" && sessionID == "" {
			sessionID = chunk.ID
			if req.OnSessionID != nil {
				req.OnSessionID(sessionID)
			}
		}
		// Any error chunk on the stream means the dispatch hit a snag —
		// could be the cold-start WS race, could be a stale in-memory
		// session after an api restart, could be something else. We
		// don't try to classify here; we set the flag and let the
		// caller decide (first-turn does a warmup retry, followup
		// restarts the session). Capturing the message keeps the
		// failure visible in the returned error for callers that care.
		if chunk.Error != nil {
			hadWSError = true
			streamErrMsg = chunk.Error.Message
			break
		}
	}
	// Drain anything remaining so Helix can finish its handler under
	// upstreamCtx — the body-close below would otherwise drop FIN
	// mid-write and the server-side log fills with broken-pipe noise.
	go func() {
		defer upstreamCancel()
		defer func() { _ = resp.Body.Close() }()
		for scanner.Scan() {
		}
	}()
	if sessionID != "" {
		// Session ID was emitted, so the row exists in Helix. Any
		// streamed error is surfaced via the bool so the caller can
		// decide whether to warmup-retry (first turn) or restart the
		// session (followup). We don't return it via err — callers
		// that didn't need recovery before shouldn't start now.
		_ = streamErrMsg
		return Session{ID: sessionID}, hadWSError, nil
	}
	if err := scanner.Err(); err != nil {
		return Session{}, false, fmt.Errorf("read SSE: %w", err)
	}
	if streamErrMsg != "" {
		return Session{}, false, fmt.Errorf("start chat streaming: %s", streamErrMsg)
	}
	return Session{}, false, errors.New("start chat streaming: no session id in stream")
}

// parseStartChatResponse normalises the two response shapes Helix
// returns from /sessions/chat. zed_external returns the full Session
// JSON; helix_basic / openai-style returns an OpenAI chat-completion
// shape with `id` (session ID) and `choices[0].message.content`.
func parseStartChatResponse(raw json.RawMessage) (Session, error) {
	var s Session
	_ = json.Unmarshal(raw, &s)
	if len(s.Interactions) > 0 {
		if s.ID == "" {
			return Session{}, errors.New("start chat: session has no id")
		}
		return s, nil
	}
	var oai struct {
		ID      string `json:"id"`
		Choices []struct {
			Index   int `json:"index"`
			Message struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &oai); err != nil {
		return Session{}, fmt.Errorf("decode start-chat response: %w", err)
	}
	if oai.ID == "" {
		return Session{}, errors.New("start chat: empty session id")
	}
	out := Session{ID: oai.ID}
	if len(oai.Choices) > 0 && oai.Choices[0].Message.Content != "" {
		out.Interactions = []*Interaction{{
			ID:              oai.ID + ":synth",
			State:           "complete",
			ResponseMessage: oai.Choices[0].Message.Content,
		}}
	}
	return out, nil
}

func (c *realClient) GetSession(ctx context.Context, id string) (Session, error) {
	var s Session
	if err := c.do(ctx, http.MethodGet, "/api/v1/sessions/"+url.PathEscape(id), nil, &s); err != nil {
		return Session{}, err
	}
	return s, nil
}

func (c *realClient) GetOutput(ctx context.Context, sessionID string) (Output, error) {
	var out Output
	if err := c.do(ctx, http.MethodGet, "/api/v1/sessions/"+url.PathEscape(sessionID)+"/output", nil, &out); err != nil {
		return Output{}, err
	}
	return out, nil
}

func (c *realClient) StopExternalAgent(ctx context.Context, sessionID string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/sessions/"+url.PathEscape(sessionID)+"/stop-external-agent", nil, nil)
}

// ---- Live updates ----

func (c *realClient) SubscribeUpdates(ctx context.Context, sessionID string) (<-chan SessionUpdate, error) {
	wsURL, err := wsURLFromBase(c.base, sessionID)
	if err != nil {
		return nil, err
	}
	header := http.Header{}
	header.Set("Authorization", "Bearer "+c.bearer(ctx))
	conn, resp, err := websocket.DefaultDialer.DialContext(ctx, wsURL, header)
	if resp != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		return nil, fmt.Errorf("ws dial: %w", err)
	}
	ch := make(chan SessionUpdate, 16)
	go func() {
		defer close(ch)
		defer func() { _ = conn.Close() }()
		go func() {
			<-ctx.Done()
			_ = conn.Close()
		}()
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var u SessionUpdate
			if err := json.Unmarshal(data, &u); err != nil {
				continue
			}
			select {
			case ch <- u:
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch, nil
}

func wsURLFromBase(base, sessionID string) (string, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("parse base url: %w", err)
	}
	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	default:
		return "", fmt.Errorf("unsupported scheme %q", u.Scheme)
	}
	u.Path = "/api/v1/ws/user"
	q := u.Query()
	q.Set("session_id", sessionID)
	u.RawQuery = q.Encode()
	return u.String(), nil
}
