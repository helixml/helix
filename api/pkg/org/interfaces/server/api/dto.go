// Package api exposes the org-graph state as JSON under
// /api/v1/orgs/{org}/, alongside the MCP and webhook handlers in the
// sibling server package. The React pages at /orgs/:org_id/helix-org/*
// consume these endpoints.
//
// DTOs carry only the data — predicates the React client derives
// client-side.
package api

import (
	"encoding/json"

	"github.com/helixml/helix/api/pkg/types"
)

// BotBadge is a compact reference to a Bot on the org overview.
type BotBadge struct {
	ID string `json:"id"`
}

// OrgOverview is the body of GET /overview — a flat list of every Bot
// in the org. The React Overview page renders the reporting graph from
// the bots + their parent_ids (fetched via GET /bots).
type OrgOverview struct {
	Nodes []BotBadge `json:"bots"`
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
// IS its own job description: Content is the canonical instruction markdown,
// Tools is its live MCP surface. ParentIDs are the Nodes this one reports
// to (empty for the org root). Reporting is many-to-many — a Bot may
// report to several managers. A Bot's attachments are not on the bot —
// they live as (worker, source) rows.
type BotDTO struct {
	ID            string `json:"id"`
	AgentID       string `json:"agent_id,omitempty"`
	LegacyAgentID string `json:"agent_app_id,omitempty"`
	// Name is the human-readable display label; empty means the UI falls
	// back to ID. Distinct from ID, which is the immutable handle.
	Name           string   `json:"name,omitempty"`
	Content        string   `json:"content"`
	Tools          []string `json:"tools,omitempty"`
	ProjectIDs     []string `json:"project_ids,omitempty"`
	ParentIDs      []string `json:"parent_ids,omitempty"`
	OrganizationID string   `json:"organization_id,omitempty"`
	// PreserveContext, when true, stops the runtime from wiping this
	// Bot's chat session before each re-activation, so it accumulates
	// context across triggers (e.g. Slack). Defaults to false.
	PreserveContext bool `json:"preserve_context"`
	// Kind is "" (agent) or "human". A human node is a person placeholder,
	// never activated; Identity holds their cross-system handles and
	// HelixUserID optionally links them to a Helix org member. Identity is
	// omitted for agent bots.
	Kind        string            `json:"kind,omitempty"`
	HelixUserID string            `json:"helix_user_id,omitempty"`
	Identity    map[string]string `json:"identity,omitempty"`
	// AgentStatus is "running" when the bot's desktop sandbox is online,
	// "stopped" otherwise (no session, paused, never activated). Drives
	// the green/grey presence dot on the org chart.
	AgentStatus string `json:"agent_status,omitempty"`
	// RestartRequired is true when the sandbox is running but still holds
	// the tool list and instructions from before the last save. Drives the
	// restart banner on the bot page and the org chat panel.
	RestartRequired bool `json:"restart_required,omitempty"`
	// ProjectID is the bot's own Helix project — the one whose exploratory
	// session is the bot's chat. SessionID is that session, when the bot
	// has been activated. Both come from runtime state and let the chat
	// sidebar list bots as top-level entries instead of surfacing their
	// project like an ordinary one.
	ProjectID               string                        `json:"project_id,omitempty"`
	SessionID               string                        `json:"session_id,omitempty"`
	AgentRuntime            string                        `json:"agent_runtime,omitempty"`
	AgentModel              string                        `json:"agent_model,omitempty"`
	CodeAgentRuntime        types.CodeAgentRuntime        `json:"code_agent_runtime,omitempty"`
	CodeAgentCredentialType types.CodeAgentCredentialType `json:"code_agent_credential_type,omitempty"`
	Provider                string                        `json:"provider,omitempty"`
	Model                   string                        `json:"model,omitempty"`
	ReasoningEffort         string                        `json:"reasoning_effort,omitempty"`
	CreatedAt               string                        `json:"created_at,omitempty"`
	UpdatedAt               string                        `json:"updated_at,omitempty"`
	// DefaultInstructions is the built-in seed prompt for this node, when
	// one exists (currently only the Chief of Staff every org is seeded
	// with). It lets the UI offer "reset instructions" and hide that
	// affordance for operator-created nodes, which have no default to
	// reset to. Detail-only: GET /bots/{id} populates it, the list does
	// not (it would repeat kilobytes of prompt per row).
	DefaultInstructions string `json:"default_instructions,omitempty"`
}

func (d BotDTO) MarshalJSON() ([]byte, error) {
	type botDTO BotDTO
	return json.Marshal(botDTO(canonicalBotDTO(d)))
}

func (d *BotDTO) UnmarshalJSON(data []byte) error {
	type botDTO BotDTO
	var decoded botDTO
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*d = canonicalBotDTO(BotDTO(decoded))
	return nil
}

func canonicalBotDTO(d BotDTO) BotDTO {
	if d.AgentID == "" {
		d.AgentID = d.LegacyAgentID
	}
	d.LegacyAgentID = d.AgentID
	return d
}

// BotChatDTO is the POST /bots/{id}/chat response. AgentID is the
// per-Bot Helix agent app id and ProjectID is the Helix project that
// owns it — the chart UI prefers ProjectID for the "chat via Human
// Desktop" deep-link (/orgs/<org>/projects/<id>/desktop/<session>),
// falling back to /agent/<agent_app_id> only when the project's
// exploratory session can't be reached.
type BotChatDTO struct {
	AgentID       string `json:"agent_id"`
	LegacyAgentID string `json:"agent_app_id"`
	ProjectID     string `json:"project_id,omitempty"`
}

// BotActivateDTO is the POST /bots/{id}/activate response.
type BotActivateDTO struct {
	ActivationID  string `json:"activation_id,omitempty"`
	ProjectID     string `json:"project_id,omitempty"`
	AgentID       string `json:"agent_id,omitempty"`
	LegacyAgentID string `json:"agent_app_id,omitempty"`
	SessionID     string `json:"session_id,omitempty"`
}

// BotDetailDTO is the full GET /bots/{id} response — the Bot plus the
// surrounding runtime context the UI's detail pane needs.
type BotDetailDTO struct {
	Bot BotDTO `json:"bot"`
	// AgentID + ProjectID — see BotChatDTO comments.
	AgentID       string `json:"agent_id,omitempty"`
	LegacyAgentID string `json:"agent_app_id,omitempty"`
	ProjectID     string `json:"project_id,omitempty"`
}

type AgentDetailDTO struct {
	BotDTO
	ProjectID string `json:"project_id,omitempty"`
}

func (d AgentDetailDTO) MarshalJSON() ([]byte, error) {
	type botDTO BotDTO
	type agentDetailDTO struct {
		botDTO
		ProjectID string `json:"project_id,omitempty"`
	}
	return json.Marshal(agentDetailDTO{
		botDTO:    botDTO(canonicalBotDTO(d.BotDTO)),
		ProjectID: d.ProjectID,
	})
}

func (d *AgentDetailDTO) UnmarshalJSON(data []byte) error {
	type botDTO BotDTO
	type agentDetailDTO struct {
		botDTO
		ProjectID string `json:"project_id,omitempty"`
	}
	var decoded agentDetailDTO
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	d.BotDTO = canonicalBotDTO(BotDTO(decoded.botDTO))
	d.ProjectID = decoded.ProjectID
	return nil
}

// CreateBotRequest is the body of POST /bots. Mirrors the MCP
// create_bot tool's args. ID is optional (a fresh handle is minted when
// empty). ParentID is the manager the new Bot reports to. Triggers are
// the Triggers the new Bot is attached to at creation (they must already
// exist).
type CreateBotRequest struct {
	ID string `json:"id,omitempty"`
	// Name is the human-readable display label (e.g. "Chief of Staff").
	// Optional; the ID stays the immutable handle.
	Name                    string                        `json:"name,omitempty"`
	Content                 string                        `json:"content"`
	Tools                   []string                      `json:"tools,omitempty"`
	Triggers                []string                      `json:"triggers,omitempty"`
	ParentID                string                        `json:"parent_id,omitempty"`
	PreserveContext         bool                          `json:"preserve_context,omitempty"`
	CodeAgentRuntime        types.CodeAgentRuntime        `json:"code_agent_runtime,omitempty"`
	CodeAgentCredentialType types.CodeAgentCredentialType `json:"code_agent_credential_type,omitempty"`
	Provider                string                        `json:"provider,omitempty"`
	Model                   string                        `json:"model,omitempty"`
	ReasoningEffort         string                        `json:"reasoning_effort,omitempty"`
	// Owner makes this a manager Bot: it receives the canonical owner
	// tool set (every org-graph mutation - create_bot, delete_bot,
	// set_bot_content, subscribe, ... - plus the read baseline) so it can
	// hire and manage other Nodes. When true, Tools is ignored in favour
	// of that set. Used to seed a starter/root Bot for a new org.
	Owner bool `json:"owner,omitempty"`
}

// CreateBotResponse is the body of POST /bots on success.
type CreateBotResponse struct {
	ID           string `json:"id"`
	ActivationID string `json:"activation_id,omitempty"`
}

// UpdateBotRequest is the body of PATCH /bots/{id}. A nil field is left
// unchanged (content-only edit preserves Tools). Attachments are not
// part of the bot row — change them via subscribe/unsubscribe.
// PreserveContext is a pointer for the same reason: nil leaves the current
// setting alone.
type UpdateBotRequest struct {
	Name            *string  `json:"name,omitempty"`
	Content         *string  `json:"content,omitempty"`
	Tools           []string `json:"tools,omitempty"`
	ProjectIDs      []string `json:"project_ids,omitempty"`
	PreserveContext *bool    `json:"preserve_context,omitempty"`
	// Identity is the per-channel handle map for a human node (slack/github/
	// email/…). When present it replaces the stored map; absent leaves it
	// unchanged. Only meaningful for kind=human bots.
	Identity                map[string]string              `json:"identity,omitempty"`
	CodeAgentRuntime        *types.CodeAgentRuntime        `json:"code_agent_runtime,omitempty"`
	CodeAgentCredentialType *types.CodeAgentCredentialType `json:"code_agent_credential_type,omitempty"`
	Provider                *string                        `json:"provider,omitempty"`
	Model                   *string                        `json:"model,omitempty"`
	ReasoningEffort         *string                        `json:"reasoning_effort,omitempty"`
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

// ErrorResponse is the envelope for non-2xx responses.
type ErrorResponse struct {
	Error string `json:"error"`
}
