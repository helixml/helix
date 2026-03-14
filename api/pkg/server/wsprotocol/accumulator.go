package wsprotocol

import "strings"

// ResponseEntry represents a single typed entry in the response.
// Used to preserve the structural boundary between assistant text and tool calls.
type ResponseEntry struct {
	Type       string `json:"type"` // "text" or "tool_call"
	Content    string `json:"content"`
	MessageID  string `json:"message_id"`
	ToolName   string `json:"tool_name,omitempty"`   // For tool_call: the tool label
	ToolStatus string `json:"tool_status,omitempty"` // For tool_call: "Completed", "In Progress", etc.
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

	// Ordered list of message IDs (insertion order)
	messageOrder []string
	// Map from message_id to its content
	messageContent map[string]string
	// Map from message_id to its entry type ("text" or "tool_call")
	messageType map[string]string
	// Map from message_id to tool metadata (name, status) for tool_call entries
	messageToolName   map[string]string
	messageToolStatus map[string]string
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
// entryType should be "text" for assistant prose or "tool_call" for tool invocations.
// An empty entryType preserves any previously stored type for this message_id.
func (a *MessageAccumulator) AddMessageWithType(messageID, content, entryType string) {
	a.AddMessageWithToolInfo(messageID, content, entryType, "", "")
}

// AddMessageWithToolInfo processes a new content update with full tool metadata.
func (a *MessageAccumulator) AddMessageWithToolInfo(messageID, content, entryType, toolName, toolStatus string) {
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

	// Migrate legacy state: if we have Content but no messageOrder, this
	// accumulator was restored from DB (only LastMessageID/Offset persisted).
	// Treat the existing Content as belonging to LastMessageID so we don't
	// lose it. This only runs once on first AddMessage after restore.
	if len(a.messageOrder) == 0 && a.Content != "" && a.LastMessageID != "" {
		a.messageOrder = []string{a.LastMessageID}
		a.messageContent[a.LastMessageID] = a.Content[a.Offset:]
		if a.Offset > 2 {
			// There was a prefix before this message. We can't recover individual
			// message_ids for the prefix, so store it under a synthetic key that
			// sorts before any real message_id.
			const prefixKey = "\x00__prefix__"
			a.messageOrder = append([]string{prefixKey}, a.messageOrder...)
			a.messageContent[prefixKey] = a.Content[:a.Offset-2] // subtract "\n\n"
		}
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
	a.rebuild()
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
			Type:       t,
			Content:    c,
			MessageID:  id,
			ToolName:   a.messageToolName[id],
			ToolStatus: a.messageToolStatus[id],
		})
	}
	return entries
}

// rebuild reconstructs Content by joining all messages in insertion order.
// Empty messages are included (they may get content later via streaming).
func (a *MessageAccumulator) rebuild() {
	// Collect non-empty parts
	parts := make([]string, 0, len(a.messageOrder))
	for _, id := range a.messageOrder {
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
