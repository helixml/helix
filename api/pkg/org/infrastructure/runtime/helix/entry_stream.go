package helix

import "github.com/helixml/helix/api/pkg/types"

// EntryTopic is a per-session translator from Helix's response-entry
// patch wire format into stable, "settled" transcript events.
//
// Helix topics `types.EntryPatch[]` frames over the WebSocket. Each patch
// targets one entry in the session's response-entry array (indexed
// by `Index`, identified by `MessageID`). Text and tool_call entries
// can be extended in place; a new MessageID at the same Index means
// the previous entry is sealed and a new one takes its slot. Tool
// calls additionally carry `ToolStatus` ("In Progress" | "Completed"
// | …).
//
// `EntryTopic` accumulates the full content of each entry and
// invokes the supplied callback once an entry is *settled*:
//
//   - `text` entries settle when a different MessageID appears at
//     the same Index, or when `Flush()` is called (session end).
//   - `tool_call` entries settle when `ToolStatus` reaches
//     `Completed` or `Failed`. The first time a tool_call entry is
//     observed, an `EventToolUse` is emitted with the args; the
//     final event is `EventToolResult` (or `EventToolResultError`).
//
// The callback's view is the same shape for both AI Worker
// activation transcripts and the chat SSE bridge, so they can render
// identically without needing to know about EntryPatches.
type EntryTopic struct {
	emit    func(Event)
	entries map[int]*entryState
}

// Event is one settled transcript event surfaced by EntryTopic. The
// text/tool_use/tool_result distinction is the canonical line shape
// for activation transcripts; both helix-org's transcript and
// the chat HTML bridge consume the same set.
type Event struct {
	Kind     string // "assistant" | "tool_use" | "tool_result" | "tool_result-error" | "error"
	Text     string
	ToolName string
}

// EventKind constants are the canonical activation-transcript line
// tags every consumer (s-transcript topic, chat bridge) reads.
const (
	EventAssistant       = "assistant"
	EventToolUse         = "tool_use"
	EventToolResult      = "tool_result"
	EventToolResultError = "tool_result-error"
	EventError           = "error"
)

type entryState struct {
	messageID  string
	kind       string // "text" | "tool_call"
	content    string
	toolName   string
	toolStatus string
	announced  bool // true once the opening event for this entry has been emitted
	settled    bool // true once the closing event for this entry has been emitted
}

// NewEntryTopic returns a fresh translator. emit is called once per
// settled event. emit must be safe to call from the goroutine
// driving Apply.
func NewEntryTopic(emit func(Event)) *EntryTopic {
	return &EntryTopic{emit: emit, entries: map[int]*entryState{}}
}

// Apply consumes one types.WebsocketEvent frame from SubscribeUpdates. It
// processes EntryPatches into the per-entry state and emits any
// settled events. Frames with no EntryPatches (session_update /
// interaction_update snapshots) are also handled — they may carry
// terminal `types.Interaction.State` indicating end-of-turn, which flushes
// any open text entries.
func (s *EntryTopic) Apply(u types.WebsocketEvent) {
	for _, p := range u.EntryPatches {
		s.applyPatch(p)
	}
	if u.Interaction != nil {
		switch u.Interaction.State {
		case "complete":
			s.Flush()
		case "error":
			if u.Interaction.Error != "" {
				s.emit(Event{Kind: EventError, Text: u.Interaction.Error})
			}
			s.Flush()
		}
	}
}

func (s *EntryTopic) applyPatch(p types.EntryPatch) {
	cur, exists := s.entries[p.Index]
	if !exists || cur.messageID != p.MessageID {
		// New entry at this index — seal the previous occupant first.
		if exists {
			s.seal(cur)
		}
		cur = &entryState{messageID: p.MessageID, kind: p.Type}
		s.entries[p.Index] = cur
	}
	cur.content = spliceUTF16(cur.content, p.Patch, p.PatchOffset)
	if p.ToolName != "" {
		cur.toolName = p.ToolName
	}
	if p.ToolStatus != "" {
		cur.toolStatus = p.ToolStatus
	}
	// Announce tool_use the first time a tool_call entry is seen.
	if cur.kind == "tool_call" && !cur.announced {
		cur.announced = true
		s.emit(Event{Kind: EventToolUse, Text: cur.content, ToolName: cur.toolName})
	}
	// Settle tool_call when the runtime reports a terminal status.
	if cur.kind == "tool_call" && (cur.toolStatus == "Completed" || cur.toolStatus == "Failed") && !cur.settled {
		s.seal(cur)
	}
}

// Flush emits closing events for any unsealed entries. Call at end
// of session (terminal status) or on disconnect when the caller
// wants to drain whatever's been accumulated.
func (s *EntryTopic) Flush() {
	// Iterate in deterministic order so transcripts are stable.
	for i := 0; i <= maxIndex(s.entries); i++ {
		e, ok := s.entries[i]
		if !ok || e.settled {
			continue
		}
		s.seal(e)
	}
}

func maxIndex(m map[int]*entryState) int {
	max := -1
	for i := range m {
		if i > max {
			max = i
		}
	}
	return max
}

func (s *EntryTopic) seal(e *entryState) {
	if e.settled {
		return
	}
	e.settled = true
	switch e.kind {
	case "text":
		if e.content != "" {
			s.emit(Event{Kind: EventAssistant, Text: e.content})
		}
	case "tool_call":
		kind := EventToolResult
		if e.toolStatus == "Failed" {
			kind = EventToolResultError
		}
		s.emit(Event{Kind: kind, Text: e.content, ToolName: e.toolName})
	}
}

// spliceUTF16 inserts `patch` into `s` at byte offset `offset`.
//
// Helix's `PatchOffset` is documented as a UTF-16 offset; in
// practice the patches helix-org consumes are append-only ASCII /
// short UTF-8 strings where the byte offset matches the UTF-16
// offset. We treat it as a byte offset and clamp to len(s) so
// out-of-range patches (e.g. snapshot replay where offset > current
// length) append rather than panic. If real-world patches surface
// surrogate-pair edits this approximation will need fixing.
func spliceUTF16(s, patch string, offset int) string {
	if offset >= len(s) {
		return s + patch
	}
	if offset < 0 {
		offset = 0
	}
	end := offset + len(patch)
	if end > len(s) {
		end = len(s)
	}
	return s[:offset] + patch + s[end:]
}
