// Package api exposes the org-graph state as JSON under
// /api/v1/orgs/{org}/, alongside the MCP and webhook handlers in the
// sibling server package. The React pages at /orgs/:org_id/helix-org/*
// consume these endpoints.
//
// DTOs carry only the data — predicates the React client derives
// client-side.
package api

// BotBadge is a compact reference to a Bot on the org overview.
type BotBadge struct {
	ID string `json:"id"`
}

// OrgOverview is the body of GET /overview — a flat list of every Bot
// in the org. The React Overview page renders the reporting graph from
// the bots + their parent_ids (fetched via GET /bots).
type OrgOverview struct {
	Bots []BotBadge `json:"bots"`
}

// ToolDTO is one entry in GET /tools — the catalogue of every tool
// that can be listed on a Bot. Powers the chart-UI bot editor's
// multi-select. Description is the human-readable one-liner the
// underlying tool surfaces to LLM callers via MCP.
type ToolDTO struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// BotDTO is one row in GET /bots and the body of GET /bots/{id}. A Bot
// IS its own job description: Content is the canonical role.md markdown,
// Tools is its live MCP surface. ParentIDs are the Bots this one reports
// to (empty for the org root). Reporting is many-to-many — a Bot may
// report to several managers. A Bot's subscriptions are not on the bot —
// they live as (bot, topic) rows.
type BotDTO struct {
	ID             string   `json:"id"`
	Content        string   `json:"content"`
	Tools          []string `json:"tools,omitempty"`
	ParentIDs      []string `json:"parent_ids,omitempty"`
	OrganizationID string   `json:"organization_id,omitempty"`
	// PreserveContext, when true, stops the runtime from wiping this
	// Bot's chat session before each re-activation, so it accumulates
	// context across triggers (e.g. Slack). Defaults to false.
	PreserveContext bool   `json:"preserve_context"`
	CreatedAt       string `json:"created_at,omitempty"`
	UpdatedAt       string `json:"updated_at,omitempty"`
}

// BotChatDTO is the POST /bots/{id}/chat response. AgentAppID is the
// per-Bot Helix agent app id and ProjectID is the Helix project that
// owns it — the chart UI prefers ProjectID for the "chat via Human
// Desktop" deep-link (/orgs/<org>/projects/<id>/desktop/<session>),
// falling back to /agent/<agent_app_id> only when the project's
// exploratory session can't be reached.
type BotChatDTO struct {
	AgentAppID string `json:"agent_app_id"`
	ProjectID  string `json:"project_id,omitempty"`
}

// BotActivateDTO is the POST /bots/{id}/activate response.
type BotActivateDTO struct {
	ActivationID string `json:"activation_id,omitempty"`
	ProjectID    string `json:"project_id,omitempty"`
	AgentAppID   string `json:"agent_app_id,omitempty"`
	SessionID    string `json:"session_id,omitempty"`
}

// BotDetailDTO is the full GET /bots/{id} response — the Bot plus the
// surrounding runtime context the UI's detail pane needs.
type BotDetailDTO struct {
	Bot BotDTO `json:"bot"`
	// AgentAppID + ProjectID — see BotChatDTO comments.
	AgentAppID string `json:"agent_app_id,omitempty"`
	ProjectID  string `json:"project_id,omitempty"`
}

// CreateBotRequest is the body of POST /bots. Mirrors the MCP
// create_bot tool's args. ID is optional (a fresh handle is minted when
// empty). ParentID is the manager the new Bot reports to. Topics are the
// topics the new Bot is subscribed to at creation (they must already
// exist).
type CreateBotRequest struct {
	ID              string   `json:"id,omitempty"`
	Content         string   `json:"content"`
	Tools           []string `json:"tools,omitempty"`
	Topics          []string `json:"topics,omitempty"`
	ParentID        string   `json:"parent_id,omitempty"`
	PreserveContext bool     `json:"preserve_context,omitempty"`
	// Owner makes this a manager Bot: it receives the canonical owner
	// tool set (every org-graph mutation - create_bot, delete_bot,
	// set_bot_content, subscribe, ... - plus the read baseline) so it can
	// hire and manage other Bots. When true, Tools is ignored in favour
	// of that set. Used to seed a starter/root Bot for a new org.
	Owner bool `json:"owner,omitempty"`
}

// CreateBotResponse is the body of POST /bots on success.
type CreateBotResponse struct {
	ID           string `json:"id"`
	ActivationID string `json:"activation_id,omitempty"`
}

// UpdateBotRequest is the body of PATCH /bots/{id}. A nil field is left
// unchanged (content-only edit preserves Tools). Subscriptions are not
// part of the bot row — change them via subscribe/unsubscribe.
// PreserveContext is a pointer for the same reason: nil leaves the current
// setting alone.
type UpdateBotRequest struct {
	Content         *string  `json:"content,omitempty"`
	Tools           []string `json:"tools,omitempty"`
	PreserveContext *bool    `json:"preserve_context,omitempty"`
}

// AddBotParentRequest is the body of POST /bots/{id}/parents. ParentID
// is a manager the Bot should now report to. Reporting is many-to-many,
// so this ADDS a line rather than replacing — the chart UI posts it
// when an accountability edge is drawn; deleting an edge hits DELETE
// /bots/{id}/parents/{parent_id}.
type AddBotParentRequest struct {
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
	PublicURL string            `json:"public_url,omitempty"`
	DBPath    string            `json:"db_path,omitempty"`
	Specs     []SettingsSpecDTO `json:"specs"`
}

// SetSettingRequest is the body of PUT /settings/{key}.
type SetSettingRequest struct {
	Value string `json:"value"`
}

// TopicDTO is one row in GET /topics plus the recent-events feed
// the UI uses to render each topic's card.
type TopicDTO struct {
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

// CreateTopicRequest is the body of POST /topics.
type CreateTopicRequest struct {
	ID          string `json:"id,omitempty"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	// As is the Bot that creates the topic — the bot whose chat
	// the human is in. Empty leaves the topic unattributed (CreatedBy is
	// cosmetic: it only anchors the node on the chart).
	As        string                 `json:"as,omitempty"`
	Transport *TransportRequestField `json:"transport,omitempty"`
}

// TransportRequestField mirrors the MCP create_topic tool's transport sub-object.
type TransportRequestField struct {
	Kind   string                 `json:"kind"`
	Config map[string]interface{} `json:"config,omitempty"`
}

// UpdateTopicRequest is the body of PUT /topics/{id}.
type UpdateTopicRequest struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Transport   *TransportRequestField `json:"transport,omitempty"`
}

// TopicsResponse is the body of GET /topics.
type TopicsResponse struct {
	Topics []TopicDTO  `json:"topics"`
	Recent []EventCard `json:"recent,omitempty"`
}

// EventCard is one entry in a topic's event feed.
type EventCard struct {
	ID          string `json:"id"`
	TopicID     string `json:"topic_id"`
	Source      string `json:"source,omitempty"`
	CreatedAt   string `json:"created_at"`
	Body        string `json:"body"`
	HasMessage  bool   `json:"has_message"`
	From        string `json:"from,omitempty"`
	To          string `json:"to,omitempty"`
	Subject     string `json:"subject,omitempty"`
	MessageBody string `json:"message_body,omitempty"`
}

// MessageAttributes is the JSON:API `attributes` object for a
// `messages` resource: the decoded Message envelope plus the event
// coordinates. Body is the visible text — the parsed Message.Body when
// the event carries a Message, otherwise the raw stored body.
type MessageAttributes struct {
	TopicID    string   `json:"topic_id"`
	Source     string   `json:"source,omitempty"`
	CreatedAt  string   `json:"created_at"`
	From       string   `json:"from,omitempty"`
	To         []string `json:"to,omitempty"`
	Subject    string   `json:"subject,omitempty"`
	Body       string   `json:"body"`
	HasMessage bool     `json:"has_message"`
	// Raw is the canonical Message envelope JSON exactly as stored — the
	// same shape a processor's `.Message` template/filter context sees
	// ({"from":…,"subject":…,"body":…,"thread_id":…,…}). Lets the UI show
	// operators which fields are available.
	Raw string `json:"raw,omitempty"`
}

// MessageResource is one JSON:API resource object in the messages list.
type MessageResource struct {
	Type       string            `json:"type"`
	ID         string            `json:"id"`
	Attributes MessageAttributes `json:"attributes"`
}

// MessagesMeta is the top-level `meta` of the messages document:
// total item count plus the pagination state.
type MessagesMeta struct {
	Total      int `json:"total"`
	Page       int `json:"page"`
	Size       int `json:"size"`
	TotalPages int `json:"total_pages"`
}

// MessagesDocument is the JSON:API document returned by
// GET /topics/{id}/messages. It documents the concrete shape the
// jsonapi composition helpers emit so the generated OpenAPI client has
// a typed response. Links are page-relative references
// (self/first/prev/next/last).
type MessagesDocument struct {
	Data  []MessageResource `json:"data"`
	Meta  MessagesMeta      `json:"meta"`
	Links map[string]string `json:"links,omitempty"`
}

// BotSubscriptionDTO is one row in a bot's subscription list.
type BotSubscriptionDTO struct {
	TopicID   string `json:"topic_id"`
	CreatedAt string `json:"created_at"`
}

// BotSubscriptionsResponse is the GET /bots/{id}/subscriptions
// response body.
type BotSubscriptionsResponse struct {
	BotID         string               `json:"bot_id"`
	Subscriptions []BotSubscriptionDTO `json:"subscriptions"`
}

// SubscribeBotRequest is the POST /bots/{id}/subscriptions body.
type SubscribeBotRequest struct {
	TopicID string `json:"topic_id"`
}

// PublishRequest is the body of POST /topics/{id}/publish.
type PublishRequest struct {
	Body    string   `json:"body"`
	Subject string   `json:"subject,omitempty"`
	To      []string `json:"to,omitempty"`
	// As is the Bot the message is sent as — the bot whose chat the
	// human is in. Empty means human/system-origin (the dispatcher treats
	// it as such). There is no global "owner" sender any more.
	As string `json:"as,omitempty"`
}

// PublishResponse is the body of POST /topics/{id}/publish on success.
type PublishResponse struct {
	EventID string `json:"event_id"`
}

// ErrorResponse is the envelope for non-2xx responses.
type ErrorResponse struct {
	Error string `json:"error"`
}
