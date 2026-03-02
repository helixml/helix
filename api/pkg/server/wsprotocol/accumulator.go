package wsprotocol

// MessageAccumulator handles the multi-message append/overwrite logic for
// WebSocket sync responses from Zed.
//
// Zed sends entries with a unique message_id per logical message. The same
// message_id streams cumulative content updates (overwrite semantics), while a
// new message_id appends a distinct block separated by "\n\n".
//
// The accumulator tracks the byte offset where the current message_id's
// content begins, so streaming updates replace only that portion without
// destroying earlier messages.
type MessageAccumulator struct {
	Content       string
	LastMessageID string
	Offset        int // byte offset in Content where the current message_id starts
}

// AddMessage processes a new content update from Zed.
//
// If messageID matches the last seen ID, the content from Offset onward is
// replaced (streaming update of the same message). If messageID differs, the
// new content is appended after a "\n\n" separator (new distinct message).
func (a *MessageAccumulator) AddMessage(messageID, content string) {
	if a.LastMessageID == "" {
		// First message ever
		a.Content = content
		a.Offset = 0
		a.LastMessageID = messageID
		return
	}

	if a.LastMessageID == messageID {
		// Same message streaming — replace from offset, keep prefix
		a.Content = a.Content[:a.Offset] + content
		return
	}

	// New distinct message — record offset, append with separator
	if a.Content != "" {
		a.Offset = len(a.Content) + 2 // account for "\n\n"
		a.Content = a.Content + "\n\n" + content
	} else {
		a.Offset = 0
		a.Content = content
	}
	a.LastMessageID = messageID
}
