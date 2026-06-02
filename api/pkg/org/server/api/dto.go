// Package api exposes the org-graph state as JSON over the same
// /api/v1/org/ surface the MCP / webhook routes already live on. The
// React pages at /helix-org/* (Phase B of the UI migration) consume
// these endpoints. Phase C deleted the htmx SSR that used to live
// alongside this package; the JSON shape is now the sole UI surface.
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
type Chart struct {
	Roots []ChartNode `json:"roots"`
}

// PositionDTO is one row in GET /positions.
type PositionDTO struct {
	ID       string `json:"id"`
	RoleID   string `json:"role_id"`
	ParentID string `json:"parent_id,omitempty"`
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
	ID            string       `json:"id"`
	Name          string       `json:"name"`
	Description   string       `json:"description,omitempty"`
	Kind          string       `json:"kind"`
	CreatedBy     string       `json:"created_by"`
	CreatedAt     string       `json:"created_at"`
	Subscribers   []string     `json:"subscribers,omitempty"`
	CanPublish    bool         `json:"can_publish"`
	DisableReason string       `json:"disable_reason,omitempty"`
	RecentEvents  []EventCard  `json:"recent_events,omitempty"`
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
