// Package api exposes the org-graph state as JSON under
// /api/v1/orgs/{org}/, alongside the MCP and webhook handlers in the
// sibling server package. The React pages at /orgs/:org_id/helix-org/*
// consume these endpoints.
//
// DTOs carry only the data — predicates the React client derives
// client-side.
package api

// ChartNode is one node in the org-chart tree returned by GET /chart.
// Positions form the tree; Workers attach as leaves under their
// Position.
type ChartNode struct {
	PositionID string        `json:"position_id"`
	RoleID     string        `json:"role_id"`
	ParentID   string        `json:"parent_id,omitempty"`
	Workers    []WorkerBadge `json:"workers,omitempty"`
	Children   []ChartNode   `json:"children,omitempty"`
}

// WorkerBadge is a compact reference to a Worker as it appears inside
// a Position's chart node — rendered as a badge across the bottom of
// each Position rect in the React chart view.
type WorkerBadge struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
}

// Chart is the response body for GET /chart. Roots is the set of
// top-level positions (ParentID empty); the tree hangs off each.
// Roles is the full set of Role IDs in the org — surfaced so the
// React chart can render Role-as-group containers, including empty
// roles that have no Positions yet.
type Chart struct {
	Roots []ChartNode `json:"roots"`
	Roles []RoleBadge `json:"roles,omitempty"`
}

// RoleBadge is a compact reference to a Role for the chart payload.
type RoleBadge struct {
	ID string `json:"id"`
}

// PositionDTO is one row in GET /positions.
type PositionDTO struct {
	ID       string `json:"id"`
	RoleID   string `json:"role_id"`
	ParentID string `json:"parent_id,omitempty"`
}

// ToolDTO is one entry in GET /tools — the catalogue of every grant
// the org can hand to a Role. Powers the chart-UI role editor's
// multi-select. Description is the human-readable one-liner the
// underlying tool surfaces to LLM callers via MCP.
type ToolDTO struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// RoleDTO is one row in GET /roles. Content is the canonical
// role.md markdown.
type RoleDTO struct {
	ID        string   `json:"id"`
	Content   string   `json:"content"`
	Tools     []string `json:"tools,omitempty"`
	Streams   []string `json:"streams,omitempty"`
	CreatedAt string   `json:"created_at,omitempty"`
	UpdatedAt string   `json:"updated_at,omitempty"`
}

// WorkerDTO is one row in GET /workers and the body of GET
// /workers/{id}. Position is the slot the Worker currently fills.
// IdentityContent is the per-Worker persona markdown (the Spawner
// projects it into identity.md at activation time).
type WorkerDTO struct {
	ID              string   `json:"id"`
	Kind            string   `json:"kind"`
	PositionID      string   `json:"position_id,omitempty"`
	IdentityContent string   `json:"identity_content"`
	OrganizationID  string   `json:"organization_id,omitempty"`
	Tools           []string `json:"tools,omitempty"`
}

// WorkerChatDTO is the POST /workers/{id}/chat response. AgentAppID
// is the per-Worker Helix agent app id and ProjectID is the Helix
// project that owns it — the chart UI prefers ProjectID for the
// "chat via Human Desktop" deep-link (/orgs/<org>/projects/<id>/desktop/<session>),
// falling back to /agent/<agent_app_id> only when the project's
// exploratory session can't be reached.
type WorkerChatDTO struct {
	AgentAppID string `json:"agent_app_id"`
	ProjectID  string `json:"project_id,omitempty"`
}

// WorkerActivateDTO is the POST /workers/{id}/activate response. The
// handler runs ensureProject synchronously (so ProjectID and
// AgentAppID are always populated on a 202) then enqueues the manual
// activation. SessionID is the persisted per-Worker chat session id
// from WorkerRuntimeState; empty until the first activation has run
// at least once. ActivationID is the pre-allocated audit-row id so
// the UI can poll activation progress without a follow-up listing
// round-trip.
type WorkerActivateDTO struct {
	ActivationID string `json:"activation_id,omitempty"`
	ProjectID    string `json:"project_id,omitempty"`
	AgentAppID   string `json:"agent_app_id,omitempty"`
	SessionID    string `json:"session_id,omitempty"`
}

// WorkerDetailDTO is the full GET /workers/{id} response — Worker
// fields plus the surrounding context the UI's detail pane needs
// (the role markdown of the Position the Worker fills, sibling worker
// IDs at that position).
type WorkerDetailDTO struct {
	Worker WorkerDTO `json:"worker"`
	// Role of the Position this Worker fills (nil if the Worker is
	// unassigned or the position/role is gone).
	Role *RoleDTO `json:"role,omitempty"`
	// Position the Worker fills (nil if unassigned).
	Position *PositionDTO `json:"position,omitempty"`
	// AgentAppID is the Helix agent app this Worker chats through.
	// Empty until the Worker has been activated at least once. The
	// chart UI deep-links a "chat with this worker" button to
	// /orgs/<org>/agent/<agent_app_id> when set.
	AgentAppID string `json:"agent_app_id,omitempty"`
	// ProjectID is the Helix project the per-Worker agent app lives
	// in. The chart UI uses this to open the Human Desktop session
	// (/orgs/<org>/projects/<project_id>/desktop/<session_id>) rather
	// than the bare agent app, so the chat happens in the same
	// Zed/desktop context as a regular project.
	ProjectID string `json:"project_id,omitempty"`
}

// HireWorkerRequest is the body of POST /workers. Mirrors the MCP
// hire_worker tool's args so the same persona / position / grant
// shape works from the React chart. `id` is optional — the server
// falls back to `w-<uuid>` when empty, but the React UI is
// expected to pass a human-readable handle (`w-mark`, `w-priya`).
type HireWorkerRequest struct {
	ID              string           `json:"id,omitempty"`
	PositionID      string           `json:"position_id"`
	Kind            string           `json:"kind"`
	IdentityContent string           `json:"identity_content"`
	Grants          []HireGrantInput `json:"grants,omitempty"`
}

// HireGrantInput is one tool-grant bundled with a hire.
type HireGrantInput struct {
	ToolName string `json:"tool_name"`
}

// HireWorkerResponse is the body of POST /workers on success. The
// new Worker's ID is always set; ActivationID is set only for AI
// kind (humans don't dispatch a hire activation).
type HireWorkerResponse struct {
	ID           string `json:"id"`
	ActivationID string `json:"activation_id,omitempty"`
}

// UpdateWorkerIdentityRequest is the body of POST
// /workers/{id}/identity.
type UpdateWorkerIdentityRequest struct {
	Identity string `json:"identity"`
}

// UpdateWorkerRoleRequest is the body of POST /workers/{id}/role.
// Content replaces the role.md of the Role the Worker's Position
// references — keyed by Worker for ergonomic React routing.
type UpdateWorkerRoleRequest struct {
	Content string `json:"content"`
}

// SettingsSpecDTO is one row in GET /settings. Configured is true
// when an explicit row exists in the configs table (vs. falling back
// to the spec default).
type SettingsSpecDTO struct {
	Key         string `json:"key"`
	Type        string `json:"type"`
	Required    bool   `json:"required"`
	Description string `json:"description"`
	Configured  bool   `json:"configured"`
	// Value is the current REDACTED value — secrets in object configs
	// are masked per the registry's redaction rules. Frontend MUST
	// treat this as display-only.
	Value string `json:"value"`
}

// SettingsResponse is the body of GET /settings.
type SettingsResponse struct {
	Owner     string            `json:"owner"`
	PublicURL string            `json:"public_url,omitempty"`
	DBPath    string            `json:"db_path,omitempty"`
	EnvsDir   string            `json:"envs_dir,omitempty"`
	Specs     []SettingsSpecDTO `json:"specs"`
}

// SetSettingRequest is the body of PUT /settings/{key}. Value is the
// raw JSON (already encoded per the spec's Type — string-typed specs
// expect a JSON-encoded string, etc.) — matches the registry's Set
// contract.
type SetSettingRequest struct {
	Value string `json:"value"`
}

// StreamDTO is one row in GET /streams plus the recent-events feed
// the UI uses to render each stream's card.
type StreamDTO struct {
	ID            string      `json:"id"`
	Name          string      `json:"name"`
	Description   string      `json:"description,omitempty"`
	Kind          string      `json:"kind"`
	CreatedBy     string      `json:"created_by"`
	CreatedAt     string      `json:"created_at"`
	Subscribers   []string    `json:"subscribers,omitempty"`
	CanPublish    bool        `json:"can_publish"`
	DisableReason string      `json:"disable_reason,omitempty"`
	RecentEvents  []EventCard `json:"recent_events,omitempty"`
	// Config is the parsed transport-specific configuration so the
	// detail page can render and edit it without round-tripping
	// through the raw JSON column. Shape depends on Kind:
	//   - github   → {"repo": "owner/name", "events": ["…"]}
	//   - webhook  → {"inbound_path": "…", "outbound_url": "…"}
	//   - postmark → {"inbound_address": "…"}
	//   - local    → omitted (no config)
	Config map[string]interface{} `json:"config,omitempty"`

	// EffectivePublicURL is the resolved base URL the github
	// transport uses for webhook payload URLs — i.e.
	// `streams.public_url` (org config) when set, falling back to
	// SERVER_URL (env). Returned for github streams only; lets
	// the detail page evaluate whether the operator's "is my
	// webhook reachable?" check passes WITHOUT needing to know
	// about the org-config override itself. Empty when neither
	// source has a value.
	EffectivePublicURL string `json:"effective_public_url,omitempty"`
}

// CreateStreamRequest is the body of POST /streams. Mirrors the
// MCP create_stream tool's args so the same Stream + Transport
// shape works from the React UI. `id` is optional — the server
// falls back to s-<uuid> when empty.
type CreateStreamRequest struct {
	ID          string                 `json:"id,omitempty"`
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Transport   *TransportRequestField `json:"transport,omitempty"`
}

// TransportRequestField mirrors the MCP create_stream tool's
// `transport` sub-object: a Kind string ("local", "webhook",
// "github", "postmark") and a kind-specific Config map shipped
// through as raw JSON for the domain validator to typecheck.
type TransportRequestField struct {
	Kind   string                 `json:"kind"`
	Config map[string]interface{} `json:"config,omitempty"`
}

// UpdateStreamRequest is the body of PUT /streams/{id}. Mutable
// fields only — `id`, `created_by`, `created_at` and the owning
// `org_id` are pinned at creation and ignored on update. The
// transport block is replaced wholesale: omit it to keep the
// current transport untouched, pass it (kind + config) to swap.
// When `transport` is supplied without a `kind` the existing
// transport kind is preserved and only its `config` is rewritten —
// the common path for editing the GitHub repo / events whitelist.
type UpdateStreamRequest struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Transport   *TransportRequestField `json:"transport,omitempty"`
}

// StreamsResponse is the body of GET /streams.
type StreamsResponse struct {
	Streams []StreamDTO `json:"streams"`
	// Recent is the unified firehose across every Stream rendered as
	// the "All streams" landing view. Capped at 50 newest-first.
	Recent []EventCard `json:"recent,omitempty"`
}

// EventCard is one entry in a stream's event feed. HasMessage is
// true when the raw Body parsed as canonical Message; the From / To /
// Subject / MessageBody fields are populated only in that case.
type EventCard struct {
	ID          string `json:"id"`
	StreamID    string `json:"stream_id"`
	Source      string `json:"source,omitempty"`
	CreatedAt   string `json:"created_at"`
	Body        string `json:"body"`
	HasMessage  bool   `json:"has_message"`
	From        string `json:"from,omitempty"`
	To          string `json:"to,omitempty"`
	Subject     string `json:"subject,omitempty"`
	MessageBody string `json:"message_body,omitempty"`
}

// PositionSubscriptionDTO is one row in a position's subscription
// list. created_at is RFC3339 for parsing convenience.
type PositionSubscriptionDTO struct {
	StreamID  string `json:"stream_id"`
	CreatedAt string `json:"created_at"`
}

// PositionSubscriptionsResponse is the GET /positions/{id}/subscriptions
// response body. The position id is echoed back so the frontend can
// match the response to the position it requested (single-fetch
// concurrency safety).
type PositionSubscriptionsResponse struct {
	PositionID    string                    `json:"position_id"`
	Subscriptions []PositionSubscriptionDTO `json:"subscriptions"`
}

// SubscribePositionRequest is the POST /positions/{id}/subscriptions
// body. stream_id is the only field — the position is in the URL,
// metadata (created_at) is server-stamped.
type SubscribePositionRequest struct {
	StreamID string `json:"stream_id"`
}

// PublishRequest is the body of POST /streams/{id}/publish. Body is
// required; subject and to are optional.
type PublishRequest struct {
	Body    string   `json:"body"`
	Subject string   `json:"subject,omitempty"`
	To      []string `json:"to,omitempty"`
}

// PublishResponse is the body of POST /streams/{id}/publish on
// success — the new Event's ID so the client can attribute its own
// publish in the feed.
type PublishResponse struct {
	EventID string `json:"event_id"`
}

// ErrorResponse is the envelope for non-2xx responses.
type ErrorResponse struct {
	Error string `json:"error"`
}

// CreateRoleRequest is the body of POST /roles. Tools and Streams are
// optional declarative scopes; an empty slice means the role does not
// hold the corresponding scope. ID is required and must be a stable
// readable handle (`r-engineer`, `r-pm`) — the org-graph chart relies
// on these to address roles directly.
type CreateRoleRequest struct {
	ID      string   `json:"id"`
	Content string   `json:"content"`
	Tools   []string `json:"tools,omitempty"`
	Streams []string `json:"streams,omitempty"`
}

// UpdateRoleRequest is the body of PUT /roles/{id}. Content replaces
// the role.md; Tools and Streams replace the declarative scopes
// wholesale (pass an empty slice to clear, omit to leave untouched).
type UpdateRoleRequest struct {
	Content *string  `json:"content,omitempty"`
	Tools   []string `json:"tools,omitempty"`
	Streams []string `json:"streams,omitempty"`
}

// CreatePositionRequest is the body of POST /positions. ParentID may
// be empty for a root position (the canonical `p-root` for the owner).
type CreatePositionRequest struct {
	ID       string `json:"id"`
	RoleID   string `json:"role_id"`
	ParentID string `json:"parent_id,omitempty"`
}

// UpdatePositionRequest is the body of PUT /positions/{id}. Currently
// only the parent and role can be re-pointed.
type UpdatePositionRequest struct {
	RoleID   *string `json:"role_id,omitempty"`
	ParentID *string `json:"parent_id,omitempty"`
}
