package chat

import (
	"context"
	"net/http"
)

// Backend is the surface the HTTP server wires to /ui/chat/* and the
// UI handler reads. The sole implementation is *HelixBridge — it
// drives a Helix chat session via helixclient and translates
// `interaction_update` / `interaction_patch` frames into the SSE
// shape the UI expects. The dev-only claude-subprocess Bridge was
// deleted in B9; production has always run on Helix's external-agent
// infrastructure.
type Backend interface {
	StreamHandler() http.Handler
	SendHandler() http.Handler
	NewHandler() http.Handler
	SwitchHandler() http.Handler
	CommandsHandler() http.Handler
	// CWD is the working directory the backend is anchored to. The
	// helix backend returns the server's cwd as a stable label.
	CWD() string
	// HistoryStartsFresh reports whether the chat page should render
	// nothing as initial history because the user just clicked New
	// chat and the freshly-created session hasn't produced output yet.
	HistoryStartsFresh() bool
	// Label is a short footer string for the chat page indicating the
	// active LLM backend, e.g. "helix · minimax-m2.7". Rendered next
	// to the Send button so the operator can tell at a glance which
	// stack their chat is on.
	Label() string
	// History returns the rendered HTML fragments for the current
	// session's prior turns, in display order. Used by the chat page
	// to re-render the conversation on refresh / navigation back.
	// Returns nil if no current session or the backend cannot
	// reconstruct history.
	History(ctx context.Context) []string
}

// Compile-time assertion: the helix bridge satisfies Backend.
var _ Backend = (*HelixBridge)(nil)
