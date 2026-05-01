package chat

import "net/http"

// Backend is the surface the HTTP server wires to /ui/chat/* and the
// UI handler reads. Two implementations live in this package today:
//
//   - *Bridge — runs a long-lived `claude` subprocess in the server's
//     cwd and bridges its stream-json output to SSE. Used when
//     `chat.backend=claude`. Development-only (the North Star is that
//     all LLM calls flow through Helix).
//   - *HelixBridge — drives a Helix chat session via helixclient and
//     translates `interaction_update` / `interaction_patch` frames
//     into the same SSE shape the UI expects. Used when
//     `chat.backend=helix`.
//
// Keeping the claude implementation around is a dev convenience — if
// a contributor doesn't have a Helix to point at, they can still drive
// the org graph end-to-end. Both backends MUST be safe to use through
// this interface alone; the UI handler never type-asserts.
type Backend interface {
	StreamHandler() http.Handler
	SendHandler() http.Handler
	NewHandler() http.Handler
	SwitchHandler() http.Handler
	CommandsHandler() http.Handler
	// CWD is the working directory the backend is anchored to. The
	// claude backend uses it to find per-cwd session jsonls; the
	// helix backend returns the server's cwd as a stable label.
	CWD() string
	// HistoryStartsFresh reports whether the chat page should render
	// nothing as initial history because the user just clicked New
	// chat and the freshly-created session hasn't produced output yet.
	HistoryStartsFresh() bool
	// Label is a short footer string for the chat page indicating the
	// active LLM backend, e.g. "helix · minimax-m2.7" or
	// "claude · sonnet 4.6". Rendered next to the Send button so the
	// operator can tell at a glance which stack their chat is on.
	Label() string
}

// Compile-time assertions: both bridges satisfy Backend.
var (
	_ Backend = (*Bridge)(nil)
	_ Backend = (*HelixBridge)(nil)
)
