package types

import (
	"time"

	"gorm.io/datatypes"
)

// AgentElicitation is a question an ACP agent asked the user mid-turn, via an ACP
// elicitation (`session/request_elicitation`). The agent's turn is blocked until the
// user answers, so this row is the difference between a question that reaches a human
// and a turn that sits in `waiting` until someone kills it.
//
// This row is authoritative for Status. A mirror of the question also lives inline in
// the interaction's ResponseEntries (Type == ResponseEntryTypeElicitation) so it renders
// in conversation order and stays in the transcript after it is answered. Both are
// written by the same handler, so they cannot drift.
//
// Why a row and not only an entry: status transitions need a conditional write
// (`WHERE status = ?`) to resolve two clients answering at once, authorisation needs an
// indexed lookup by id, and "which tasks are blocked on a human" needs an indexed query
// rather than a scan of every interaction's jsonb.
type AgentElicitation struct {
	// ID is Zed's ElicitationEntryId — opaque to Helix, stable for the elicitation's life.
	ID            string `json:"id" gorm:"primaryKey;size:255"`
	SessionID     string `json:"session_id" gorm:"index;size:255"`
	InteractionID string `json:"interaction_id" gorm:"index;size:255"`
	// RequestID is the turn that asked. Used to route the question to the right
	// interaction, the same way message_added is routed.
	RequestID   string `json:"request_id" gorm:"size:255"`
	AcpThreadID string `json:"acp_thread_id" gorm:"index;size:255"`
	ToolCallID  string `json:"tool_call_id,omitempty" gorm:"size:255"`
	// EntryIndex is the position in Zed's thread entries, which is also the accumulator's
	// message_id — that is what puts the question in the right place in the transcript.
	EntryIndex string `json:"entry_index" gorm:"size:64"`
	Message    string `json:"message" gorm:"type:text"`
	Mode       string `json:"mode" gorm:"size:32"`
	// Schema is the ACP `requestedSchema`, stored verbatim. The frontend renders the
	// question from this, so anything dropped here is a control the user never sees.
	Schema datatypes.JSON `json:"schema" gorm:"type:jsonb"`
	Status string         `json:"status" gorm:"size:32;index"`
	// Content is the answer Helix sent. Zed cannot report it back —
	// ElicitationStatus::Accepted is a unit variant and does not retain the submission —
	// so this is written when we send the answer, not when the agent confirms it.
	Content datatypes.JSON `json:"content,omitempty" gorm:"type:jsonb"`
	// ResolutionReason distinguishes the ways a question stops being answerable, so the
	// UI can say "you replied instead" rather than a bare "cancelled".
	ResolutionReason string `json:"resolution_reason,omitempty" gorm:"size:64"`
	// LastSeenAt is refreshed every time the agent re-affirms it still holds this
	// question (the resync heartbeat). A pending row whose LastSeenAt has gone stale is
	// the only evidence that the agent holding it is gone — a WebSocket reconnect is
	// not, since the commonest cause of one is the Helix API restarting while the
	// desktop container, Zed and its respond_tx all survive untouched.
	LastSeenAt time.Time `json:"last_seen_at" gorm:"index"`
	Created    time.Time `json:"created"`
	Updated    time.Time `json:"updated"`
}

func (AgentElicitation) TableName() string {
	return "agent_elicitations"
}

// Elicitation statuses. pending and submitting are the two live states; the rest are
// terminal and final — a late event for a terminal elicitation is dropped.
const (
	ElicitationStatusPending = "pending"
	// ElicitationStatusSubmitting is Helix-local and optimistic: the answer has been
	// sent to the agent but the agent has not confirmed it. It is never sent to Zed and
	// never shown as "answered".
	ElicitationStatusSubmitting = "submitting"
	ElicitationStatusAccepted   = "accepted"
	ElicitationStatusDeclined   = "declined"
	ElicitationStatusCancelled  = "cancelled"
	ElicitationStatusCompleted  = "completed"
)

// Reasons a question stopped being answerable, for UI copy.
const (
	ElicitationReasonAnswered           = "answered"
	ElicitationReasonSkipped            = "skipped"
	ElicitationReasonFollowUp           = "follow_up"
	ElicitationReasonInterrupted        = "interrupted"
	ElicitationReasonAgentNoLongerHolds = "agent_no_longer_holds"
)

// IsLive reports whether the question can still be answered.
func (e *AgentElicitation) IsLive() bool {
	return e.Status == ElicitationStatusPending || e.Status == ElicitationStatusSubmitting
}

// ElicitationRespondRequest is the body of
// POST /api/v1/sessions/{id}/elicitations/{elicitation_id}/respond.
//
// It deliberately carries no session or thread identifier: the session comes from the URL
// and nothing in the body is trusted to name it.
type ElicitationRespondRequest struct {
	// Action is "accept" or "decline". "decline" is not an abort — the ACP adapter turns
	// it into an empty answers map and the agent's turn continues, which is why the UI
	// labels it "Skip".
	Action string `json:"action"`
	// Content maps schema field names to the user's answers.
	Content map[string]interface{} `json:"content,omitempty"`
}

// ElicitationRespondResponse reports the status the elicitation moved to.
type ElicitationRespondResponse struct {
	ElicitationID string `json:"elicitation_id"`
	Status        string `json:"status"`
}
