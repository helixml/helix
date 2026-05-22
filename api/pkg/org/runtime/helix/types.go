package helix

import (
	"github.com/helixml/helix/api/pkg/types"
)

// Type aliases — canonical definitions live in api/pkg/types. We
// re-export them under shorter names here so call sites in the
// helix runtime read naturally (`helix.Session` instead of
// `types.Session`, etc.) without duplicating shape definitions.
type (
	ExternalAgentConfig = types.ExternalAgentConfig
	EntryPatch          = types.EntryPatch
	Interaction         = types.Interaction
	Session             = types.Session
	// SessionUpdate is the per-session WebSocket / pubsub frame.
	// Helix calls it WebsocketEvent on the wire.
	SessionUpdate = types.WebsocketEvent
	// Output is the polling result for a session.
	Output = types.SessionOutputResponse
)

// IsTerminalOutput reports whether o.Status is a terminal state.
// Free function instead of method because we don't own the type.
func IsTerminalOutput(o Output) bool {
	return o.Status == "complete" || o.Status == "error"
}

// StartChatRequest is the body of POST /sessions/chat. Mirrors
// types.SessionChatRequest on the wire but adds the OnSessionID
// callback — a Go-only hook for early WS attach that doesn't
// serialise. Kept local because the callback field has no place on
// the canonical types.SessionChatRequest.
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
	// attach a WS subscriber early via this hook. Not serialised.
	OnSessionID func(sessionID string) `json:"-"`
}

// SessionChatMessage is one entry in StartChatRequest.Messages. We
// keep a local minimum-fields struct rather than aliasing
// types.Message because Helix's /sessions/chat endpoint expects this
// trimmed shape (role + content only); the full types.Message
// carries id / created_at / state fields that the request body
// would include with default-zero values, which Helix rejects.
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
