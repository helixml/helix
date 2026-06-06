// Package api exposes the org-graph state as JSON under
// /api/v1/orgs/{org}/, alongside the MCP and webhook handlers in the
// sibling server package. The React pages at /orgs/:org_id/helix-org/*
// consume these endpoints.
//
// DTOs carry only the data — predicates the React client derives
// client-side.
package api

// WorkerBadge is a compact reference to a Worker on the org overview.
type WorkerBadge struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
}

// RoleBadge is a compact reference to a Role for the overview payload.
type RoleBadge struct {
	ID string `json:"id"`
}

// RoleGroup is one Role + the Workers currently holding it. The
// overview page renders these as cards: the Role description (loaded
// separately) plus a list of Workers under it.
type RoleGroup struct {
	RoleID  string        `json:"role_id"`
	Workers []WorkerBadge `json:"workers,omitempty"`
}

// OrgOverview is the body of GET /overview. Roles is the full set of
// Role IDs in the org (so the React overview can render empty
// groups). Groups lists every Role with its current Workers.
type OrgOverview struct {
	Roles  []RoleBadge `json:"roles,omitempty"`
	Groups []RoleGroup `json:"groups"`
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
// /workers/{id}. RoleID is the live capability binding; ParentID is
// the Worker this one reports to (empty for the owner).
// IdentityContent is the per-Worker persona markdown (the Spawner
// projects it into identity.md at activation time).
type WorkerDTO struct {
	ID              string   `json:"id"`
	Kind            string   `json:"kind"`
	RoleID          string   `json:"role_id,omitempty"`
	ParentID        string   `json:"parent_id,omitempty"`
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

// WorkerActivateDTO is the POST /workers/{id}/activate response.
type WorkerActivateDTO struct {
	ActivationID string `json:"activation_id,omitempty"`
	ProjectID    string `json:"project_id,omitempty"`
	AgentAppID   string `json:"agent_app_id,omitempty"`
	SessionID    string `json:"session_id,omitempty"`
}

// WorkerDetailDTO is the full GET /workers/{id} response — Worker
// fields plus the surrounding context the UI's detail pane needs.
type WorkerDetailDTO struct {
	Worker WorkerDTO `json:"worker"`
	// Role this Worker holds (nil if the role row is gone).
	Role *RoleDTO `json:"role,omitempty"`
	// AgentAppID + ProjectID — see WorkerChatDTO comments.
	AgentAppID string `json:"agent_app_id,omitempty"`
	ProjectID  string `json:"project_id,omitempty"`
}

// HireWorkerRequest is the body of POST /workers. Mirrors the MCP
// hire_worker tool's args.
type HireWorkerRequest struct {
	ID              string `json:"id,omitempty"`
	RoleID          string `json:"role_id"`
	ParentID        string `json:"parent_id,omitempty"`
	Kind            string `json:"kind"`
	IdentityContent string `json:"identity_content"`
}

// HireWorkerResponse is the body of POST /workers on success.
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
// Content replaces the role.md of the Role the Worker holds.
type UpdateWorkerRoleRequest struct {
	Content string `json:"content"`
}

// UpdateWorkerParentRequest is the body of POST /workers/{id}/parent.
// ParentID is the Worker this one now reports to; an empty string
// clears the manager (the Worker becomes top-level). The chart UI
// posts this when an accountability edge is drawn or deleted.
type UpdateWorkerParentRequest struct {
	ParentID string `json:"parent_id"`
}

// SettingsSpecDTO is one row in GET /settings.
type SettingsSpecDTO struct {
	Key         string `json:"key"`
	Type        string `json:"type"`
	Required    bool   `json:"required"`
	Description string `json:"description"`
	Configured  bool   `json:"configured"`
	Value       string `json:"value"`
}

// SettingsResponse is the body of GET /settings.
type SettingsResponse struct {
	Owner     string            `json:"owner"`
	PublicURL string            `json:"public_url,omitempty"`
	DBPath    string            `json:"db_path,omitempty"`
	EnvsDir   string            `json:"envs_dir,omitempty"`
	Specs     []SettingsSpecDTO `json:"specs"`
}

// SetSettingRequest is the body of PUT /settings/{key}.
type SetSettingRequest struct {
	Value string `json:"value"`
}

// StreamDTO is one row in GET /streams plus the recent-events feed
// the UI uses to render each stream's card.
type StreamDTO struct {
	ID                 string                 `json:"id"`
	Name               string                 `json:"name"`
	Description        string                 `json:"description,omitempty"`
	Kind               string                 `json:"kind"`
	CreatedBy          string                 `json:"created_by"`
	CreatedAt          string                 `json:"created_at"`
	Subscribers        []string               `json:"subscribers,omitempty"`
	CanPublish         bool                   `json:"can_publish"`
	DisableReason      string                 `json:"disable_reason,omitempty"`
	RecentEvents       []EventCard            `json:"recent_events,omitempty"`
	Config             map[string]interface{} `json:"config,omitempty"`
	EffectivePublicURL string                 `json:"effective_public_url,omitempty"`
}

// CreateStreamRequest is the body of POST /streams.
type CreateStreamRequest struct {
	ID          string                 `json:"id,omitempty"`
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Transport   *TransportRequestField `json:"transport,omitempty"`
}

// TransportRequestField mirrors the MCP create_stream tool's transport sub-object.
type TransportRequestField struct {
	Kind   string                 `json:"kind"`
	Config map[string]interface{} `json:"config,omitempty"`
}

// UpdateStreamRequest is the body of PUT /streams/{id}.
type UpdateStreamRequest struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Transport   *TransportRequestField `json:"transport,omitempty"`
}

// StreamsResponse is the body of GET /streams.
type StreamsResponse struct {
	Streams []StreamDTO `json:"streams"`
	Recent  []EventCard `json:"recent,omitempty"`
}

// EventCard is one entry in a stream's event feed.
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

// WorkerSubscriptionDTO is one row in a worker's subscription list.
type WorkerSubscriptionDTO struct {
	StreamID  string `json:"stream_id"`
	CreatedAt string `json:"created_at"`
}

// WorkerSubscriptionsResponse is the GET /workers/{id}/subscriptions
// response body.
type WorkerSubscriptionsResponse struct {
	WorkerID      string                  `json:"worker_id"`
	Subscriptions []WorkerSubscriptionDTO `json:"subscriptions"`
}

// SubscribeWorkerRequest is the POST /workers/{id}/subscriptions body.
type SubscribeWorkerRequest struct {
	StreamID string `json:"stream_id"`
}

// PublishRequest is the body of POST /streams/{id}/publish.
type PublishRequest struct {
	Body    string   `json:"body"`
	Subject string   `json:"subject,omitempty"`
	To      []string `json:"to,omitempty"`
}

// PublishResponse is the body of POST /streams/{id}/publish on success.
type PublishResponse struct {
	EventID string `json:"event_id"`
}

// ErrorResponse is the envelope for non-2xx responses.
type ErrorResponse struct {
	Error string `json:"error"`
}

// CreateRoleRequest is the body of POST /roles.
type CreateRoleRequest struct {
	ID      string   `json:"id"`
	Content string   `json:"content"`
	Tools   []string `json:"tools,omitempty"`
	Streams []string `json:"streams,omitempty"`
}

// UpdateRoleRequest is the body of PUT /roles/{id}.
type UpdateRoleRequest struct {
	Content *string  `json:"content,omitempty"`
	Tools   []string `json:"tools,omitempty"`
	Streams []string `json:"streams,omitempty"`
}
