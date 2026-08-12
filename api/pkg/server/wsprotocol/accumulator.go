package wsprotocol

import (
	"encoding/json"
	"strings"

	"github.com/helixml/helix/api/pkg/util/sanitize"
	"gorm.io/datatypes"
)

// RestoreAccumulator rebuilds an accumulator from persisted DB state.
// If structured response_entries are available, it restores the full
// message_id→content map with correct types. Otherwise falls back to
// the legacy Content/LastMessageID/Offset restore which loses structure.
func RestoreAccumulator(content string, lastMessageID string, offset int, responseEntries datatypes.JSON) *MessageAccumulator {
	// Try structured restore from response_entries
	if len(responseEntries) > 0 {
		var entries []ResponseEntry
		if err := json.Unmarshal(responseEntries, &entries); err == nil && len(entries) > 0 {
			acc := &MessageAccumulator{
				Content:        content,
				LastMessageID:  lastMessageID,
				Offset:         offset,
				contentDirty:   false,
				messageOrder:   make([]string, 0, len(entries)),
				messageContent: make(map[string]string, len(entries)),
				messageType:    make(map[string]string, len(entries)),
				messageToolName:   make(map[string]string),
				messageToolStatus: make(map[string]string),
				messageElicitation: make(map[string]*ElicitationEntry),
			}
			for _, entry := range entries {
				id := entry.MessageID
				if id == "" {
					continue
				}
				acc.messageOrder = append(acc.messageOrder, id)
				acc.messageContent[id] = entry.Content
				acc.messageType[id] = entry.Type
				if entry.ToolName != "" {
					acc.messageToolName[id] = entry.ToolName
				}
				if entry.ToolStatus != "" {
					acc.messageToolStatus[id] = entry.ToolStatus
				}
				// Restoring this is what lets a question survive an API restart still
				// answerable, rather than degrading to inert text.
				if entry.Elicitation != nil {
					acc.messageElicitation[id] = entry.Elicitation
				}
			}
			if lastMessageID == "" && len(acc.messageOrder) > 0 {
				acc.LastMessageID = acc.messageOrder[len(acc.messageOrder)-1]
			}
			return acc
		}
	}

	// No structured entries — start fresh.
	return &MessageAccumulator{}
}

// ResponseEntry represents a single typed entry in the response.
// Used to preserve structural boundaries between assistant text, tool calls,
// and plan snapshots.
type ResponseEntry struct {
	Type       string `json:"type"` // "text", "tool_call", "plan" or "elicitation"
	Content    string `json:"content"`
	MessageID  string `json:"message_id"`
	ToolName   string `json:"tool_name,omitempty"`   // For tool_call: the tool label
	ToolStatus string `json:"tool_status,omitempty"` // For tool_call: "Completed", "In Progress", etc.
	// Elicitation is set for Type == "elicitation": a question the agent asked the user.
	// The frontend renders the answerable card from this payload.
	Elicitation *ElicitationEntry `json:"elicitation,omitempty"`
}

// Entry types. "elicitation" entries are questions the agent asked mid-turn; they render
// as an answerable card and remain in the transcript showing what was chosen.
const (
	ResponseEntryTypeText        = "text"
	ResponseEntryTypeToolCall    = "tool_call"
	ResponseEntryTypeElicitation = "elicitation"
)

// ElicitationEntry is the render payload for a question the agent asked. Schema is the
// ACP `requestedSchema` verbatim — the frontend builds the form from it generically, so
// anything dropped here is a control the user never gets to use.
type ElicitationEntry struct {
	ID         string          `json:"id"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
	Message    string          `json:"message"`
	Mode       string          `json:"mode,omitempty"`
	Schema     json.RawMessage `json:"schema,omitempty"`
	Status     string          `json:"status"`
	// Content is what the user submitted, once answered.
	Content json.RawMessage `json:"content,omitempty"`
	// ResolutionReason explains a non-answered ending so the card can say "you replied
	// instead" or "expired — the agent restarted" rather than a bare "cancelled".
	ResolutionReason string `json:"resolution_reason,omitempty"`
}

// MessageAccumulator handles the multi-message append/overwrite logic for
// WebSocket sync responses from Zed.
//
// Zed sends entries with a unique message_id per logical message (the entry_idx
// in the AcpThread). The same message_id streams cumulative content updates
// (overwrite semantics), while a new message_id represents a distinct entry
// (assistant text block, tool call, etc.).
//
// During streaming, EntryUpdated events fire with 100ms throttling. The content
// snapshot may be mid-word if the Markdown buffer is still being populated.
// When the turn completes (Stopped event), flush_streaming_throttle() sends
// corrected content for ALL entries — including earlier message_ids whose
// content was previously truncated.
//
// The accumulator must handle these out-of-order updates: a flush might send
// message_id "2" again after "18" was already the most recent. The old
// single-offset design treated this as a new append, duplicating content.
// This implementation tracks each message_id separately and reconstructs
// the full content string on every update.
type MessageAccumulator struct {
	Content       string
	LastMessageID string
	Offset        int // kept for DB backward compat; not used in new logic

	// contentDirty tracks whether Content/Offset need rebuilding.
	// rebuild() is deferred until Content is actually needed (DB write or
	// completion) to avoid joining 17 MB of strings on every message.
	contentDirty bool

	// Ordered list of message IDs (insertion order)
	messageOrder []string
	// Map from message_id to its content
	messageContent map[string]string
	// Map from message_id to its entry type ("text", "tool_call", or "plan")
	messageType map[string]string
	// Map from message_id to tool metadata (name, status) for tool_call entries
	messageToolName   map[string]string
	messageToolStatus map[string]string
	// Map from message_id to the question payload, for elicitation entries
	messageElicitation map[string]*ElicitationEntry

	// priorMessageContent holds (message_id → content) snapshots of entries
	// from earlier completed interactions in the same session. Zed's
	// flush_streaming_throttle replays ALL ACP thread entries on every event,
	// so on a follow-up turn the new accumulator keeps receiving message_added
	// events for the previous turn's entries. Without filtering those entries
	// leak into the new interaction's response_entries — the failure mode
	// caught by the e2e RESPONSE ENTRIES ISOLATION VALIDATION step.
	//
	// Filtering by message_id ALONE was too aggressive: when the wrapper
	// restarts inside Zed, message_ids reset and are reused for legitimately
	// new content; dropping them produces an empty interaction that bounces
	// with "Agent returned empty response" (incident on
	// ses_01kq8cnnkmww35bacpscbrehn0 / int_01kqjsrhndcpwb9zv068dn7mv9 on
	// 2026-05-01 — see design/2026-04-30-queue-and-other-stuck-state-bugs.md).
	// We now also compare content: a replay of a prior entry has identical
	// content; a renumbered new entry has different content. Drop only on
	// exact content match.
	priorMessageContent map[string]string
}

// AddMessage processes a new content update from Zed.
//
// If messageID was seen before, its content is replaced in-place (streaming
// update or flush correction). If messageID is new, it is appended to the
// ordered list. The full Content string is rebuilt after every call.
func (a *MessageAccumulator) AddMessage(messageID, content string) {
	a.AddMessageWithType(messageID, content, "")
}

// AddMessageWithType processes a new content update from Zed, with an explicit entry type.
// entryType should be "text" for assistant prose, "tool_call" for tool
// invocations, or "plan" for a replaceable plan snapshot.
// An empty entryType preserves any previously stored type for this message_id.
func (a *MessageAccumulator) AddMessageWithType(messageID, content, entryType string) {
	a.AddMessageWithToolInfo(messageID, content, entryType, "", "")
}

// SetPriorEntries records (message_id → content) snapshots from earlier
// interactions in the same session. AddMessage* calls whose (id, content)
// pair exactly matches a prior entry are dropped as wrapper replays.
// Idempotent and additive: calling it multiple times unions the maps; later
// calls for the same id win (last-writer).
func (a *MessageAccumulator) SetPriorEntries(entries []ResponseEntry) {
	if len(entries) == 0 {
		return
	}
	if a.priorMessageContent == nil {
		a.priorMessageContent = make(map[string]string, len(entries))
	}
	for _, e := range entries {
		if e.MessageID == "" {
			continue
		}
		a.priorMessageContent[e.MessageID] = e.Content
	}
}

// SetPriorMessageIDs is kept for backwards compat with callers that only had
// id-level information. New callers should use SetPriorEntries which can also
// distinguish wrapper replays (same id+content) from wrapper-restart-renumbered
// new content (same id, different content). When called via this id-only path,
// we conservatively store an empty string for content — matching only when
// Zed's replay also happens to be empty content (unusual). Most callers should
// migrate to SetPriorEntries.
func (a *MessageAccumulator) SetPriorMessageIDs(ids []string) {
	if len(ids) == 0 {
		return
	}
	if a.priorMessageContent == nil {
		a.priorMessageContent = make(map[string]string, len(ids))
	}
	for _, id := range ids {
		if id == "" {
			continue
		}
		if _, exists := a.priorMessageContent[id]; !exists {
			a.priorMessageContent[id] = ""
		}
	}
}

// AddMessageWithToolInfo processes a new content update with full tool metadata.
func (a *MessageAccumulator) AddMessageWithToolInfo(messageID, content, entryType, toolName, toolStatus string) {
	if priorContent, isPrior := a.priorMessageContent[messageID]; isPrior && priorContent == content {
		// Same (id, content) as an earlier interaction's entry — Zed's
		// flush_streaming_throttle replayed it. Drop silently. Different
		// content under the same id means the wrapper restarted and renumbered;
		// accept it as new content for this interaction.
		return
	}

	// Sanitize content to prevent PostgreSQL errors from null bytes in
	// terminal output or binary data that Zed captures from tool calls.
	content = sanitize.ForPostgres(content)

	if a.messageContent == nil {
		a.messageContent = make(map[string]string)
	}
	if a.messageType == nil {
		a.messageType = make(map[string]string)
	}
	if a.messageToolName == nil {
		a.messageToolName = make(map[string]string)
	}
	if a.messageToolStatus == nil {
		a.messageToolStatus = make(map[string]string)
	}

	if _, exists := a.messageContent[messageID]; exists {
		// Known message_id — replace content in-place
		a.messageContent[messageID] = content
		// Only update type if explicitly provided (don't overwrite with empty)
		if entryType != "" {
			a.messageType[messageID] = entryType
		}
		if toolName != "" {
			a.messageToolName[messageID] = toolName
		}
		if toolStatus != "" {
			a.messageToolStatus[messageID] = toolStatus
		}
	} else {
		// New message_id — append to order
		a.messageOrder = append(a.messageOrder, messageID)
		a.messageContent[messageID] = content
		if entryType != "" {
			a.messageType[messageID] = entryType
		}
		if toolName != "" {
			a.messageToolName[messageID] = toolName
		}
		if toolStatus != "" {
			a.messageToolStatus[messageID] = toolStatus
		}
	}

	a.LastMessageID = messageID
	a.contentDirty = true
}

// UpsertElicitation records (or updates) a question the agent asked at messageID.
//
// Unlike AddMessage*, this is safe to call repeatedly with the same id: the agent
// re-affirms outstanding questions on a heartbeat, and status changes arrive as further
// updates to the same entry. The message text doubles as the entry's Content so
// TextFromEntries and search keep working without knowing about elicitations.
func (a *MessageAccumulator) UpsertElicitation(messageID string, elicitation *ElicitationEntry) {
	if messageID == "" || elicitation == nil {
		return
	}
	if a.messageContent == nil {
		a.messageContent = make(map[string]string)
	}
	if a.messageType == nil {
		a.messageType = make(map[string]string)
	}
	if a.messageElicitation == nil {
		a.messageElicitation = make(map[string]*ElicitationEntry)
	}

	if _, exists := a.messageContent[messageID]; !exists {
		a.messageOrder = append(a.messageOrder, messageID)
	}
	// Never empty: Entries() drops entries with no content, which would make an
	// unanswerable question vanish from the transcript.
	content := sanitize.ForPostgres(elicitation.Message)
	if content == "" {
		content = "The agent asked a question."
	}
	a.messageContent[messageID] = content
	a.messageType[messageID] = ResponseEntryTypeElicitation
	a.messageElicitation[messageID] = elicitation
	a.LastMessageID = messageID
	a.contentDirty = true
}

// ElicitationEntryFor returns the question recorded at messageID, if any.
func (a *MessageAccumulator) ElicitationEntryFor(messageID string) *ElicitationEntry {
	return a.messageElicitation[messageID]
}

// MessageIDForElicitation finds the entry carrying a given elicitation id. Status updates
// arrive keyed by elicitation id, not by message_id.
func (a *MessageAccumulator) MessageIDForElicitation(elicitationID string) (string, bool) {
	for messageID, entry := range a.messageElicitation {
		if entry != nil && entry.ID == elicitationID {
			return messageID, true
		}
	}
	return "", false
}

// Entries returns the structured response entries in insertion order,
// preserving the type information for each message_id.
// Entries with empty content are omitted.
func (a *MessageAccumulator) Entries() []ResponseEntry {
	entries := make([]ResponseEntry, 0, len(a.messageOrder))
	for _, id := range a.messageOrder {
		c := a.messageContent[id]
		if c == "" {
			continue
		}
		t := a.messageType[id]
		if t == "" {
			// Infer type from content for backward compat (no entry_type from old Zed)
			if strings.HasPrefix(c, "**Tool Call:") {
				t = "tool_call"
			} else {
				t = "text"
			}
		}
		entries = append(entries, ResponseEntry{
			Type:        t,
			Content:     c,
			MessageID:   id,
			ToolName:    a.messageToolName[id],
			ToolStatus:  a.messageToolStatus[id],
			Elicitation: a.messageElicitation[id],
		})
	}
	return entries
}

// Rebuild reconstructs Content and Offset from the accumulated messages.
// Call this before reading Content (e.g. before a DB write or completion).
// No-op if content hasn't changed since the last rebuild.
func (a *MessageAccumulator) Rebuild() {
	if !a.contentDirty {
		return
	}
	a.rebuild()
	a.contentDirty = false
}

// rebuild reconstructs the legacy flat Content by joining prose and tool
// entries in insertion order. Plan snapshots remain structured-only so their
// JSON never leaks into assistant prose or downstream transcripts.
func (a *MessageAccumulator) rebuild() {
	// Collect non-empty parts
	parts := make([]string, 0, len(a.messageOrder))
	for _, id := range a.messageOrder {
		if a.messageType[id] == "plan" {
			continue
		}
		c := a.messageContent[id]
		if c != "" {
			parts = append(parts, c)
		}
	}
	a.Content = strings.Join(parts, "\n\n")

	// Update Offset for backward compat: point to the start of LastMessageID's content
	a.Offset = 0
	if a.LastMessageID != "" {
		offset := 0
		for _, id := range a.messageOrder {
			if a.messageType[id] == "plan" {
				continue
			}
			c := a.messageContent[id]
			if c == "" {
				continue
			}
			if id == a.LastMessageID {
				a.Offset = offset
				break
			}
			offset += len(c) + 2 // +2 for "\n\n" separator
		}
	}
}
