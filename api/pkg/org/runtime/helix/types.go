package helix

import "encoding/json"

// Helix session / streaming types — same wire shape Helix's
// /sessions/chat and /ws/user endpoints emit. Lifted out of
// helix-org/helix/helixclient during H1.3a so EntryStream + EnsureAndSend
// live in their canonical home (api/pkg/org/runtime/helix) without
// importing the legacy helixclient package.
//
// helixclient transitionally re-exports these via type aliases so
// existing call sites compile during the H1.3 sequence; H1.4 deletes
// the package outright.

// StartChatRequest is the body of POST /sessions/chat.
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

	// OnSessionID, if set, is invoked the moment Helix emits the
	// session ID — before the agent has produced a reply. Callers
	// attach a WS subscriber early via this hook.
	OnSessionID func(sessionID string) `json:"-"`
}

// ExternalAgentConfig must be present (non-nil) when
// AgentType=zed_external — Helix uses presence-of-object as the
// "wire up a runner" signal.
type ExternalAgentConfig struct {
	Resolution    string `json:"resolution,omitempty"`
	DisplayWidth  int    `json:"display_width,omitempty"`
	DisplayHeight int    `json:"display_height,omitempty"`
	DesktopType   string `json:"desktop_type,omitempty"`
}

// SessionChatMessage is one entry in StartChatRequest.Messages.
type SessionChatMessage struct {
	Role    string         `json:"role"`
	Content MessageContent `json:"content"`
}

// MessageContent is the multipart body. helix-org only ever sends a
// single text part. content_type is omitted to match Helix UI's wire
// shape ({"parts":[…]}); Helix infers text from the part type.
type MessageContent struct {
	Parts []any `json:"parts"`
}

// NewTextMessage builds a single user-text message.
func NewTextMessage(role, text string) SessionChatMessage {
	return SessionChatMessage{
		Role:    role,
		Content: MessageContent{Parts: []any{text}},
	}
}

// Output is the polling result for a session — mirrors
// types.SessionOutputResponse.
type Output struct {
	SessionID  string `json:"session_id"`
	Status     string `json:"status"`
	Output     string `json:"output"`
	DurationMs int64  `json:"duration_ms"`
}

// IsTerminal reports whether o.Status is a terminal state.
func (o Output) IsTerminal() bool {
	return o.Status == "complete" || o.Status == "error"
}

// SessionUpdate is one frame from /api/v1/ws/user (and the
// equivalent in-process pubsub topic). interaction_patch frames
// carry EntryPatches[]; session_update / interaction_update frames
// carry final-state snapshots.
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

// EntryPatch is one per-entry delta — mirrors types.EntryPatch.
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

// Session is the subset of Helix's Session helix-org reads.
type Session struct {
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	ProjectID     string         `json:"project_id"`
	ParentApp     string         `json:"parent_app,omitempty"`
	DefaultRepoID string         `json:"default_repo_id,omitempty"`
	Interactions  []*Interaction `json:"interactions,omitempty"`
}

// Interaction is one assistant turn — Helix stores prompt + response
// on the same row.
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

// OpenAIToolCall mirrors openai.ToolCall.
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

// SendMessageOptions are the optional knobs on SendSessionMessage.
type SendMessageOptions struct {
	Interrupt    bool
	NotifyUserID string
}

// SendMessageResponse is the body of POST /sessions/{id}/messages.
type SendMessageResponse struct {
	RequestID     string `json:"request_id"`
	InteractionID string `json:"interaction_id"`
}

// ServerStatus mirrors the slice of /api/v1/config helix-org reads.
type ServerStatus struct {
	MaxConcurrentDesktops    int `json:"max_concurrent_desktops"`
	ActiveConcurrentDesktops int `json:"active_concurrent_desktops"`
}

// HasDesktopRoom reports whether at least one desktop slot is free.
// Max=0 means "unlimited" at the server level.
func (s ServerStatus) HasDesktopRoom() bool {
	if s.MaxConcurrentDesktops <= 0 {
		return true
	}
	return s.ActiveConcurrentDesktops < s.MaxConcurrentDesktops
}
