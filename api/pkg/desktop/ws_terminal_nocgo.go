//go:build !cgo

package desktop

import (
	"net/http"
)

// TerminalMessage represents a message from the web terminal client
type TerminalMessage struct {
	Type string `json:"type"`
	Data string `json:"data,omitempty"`
	Rows uint16 `json:"rows,omitempty"`
	Cols uint16 `json:"cols,omitempty"`
}

// PromptRequest represents a request to inject a prompt into Claude Code
type PromptRequest struct {
	Prompt string `json:"prompt"`
}

// handleWSTerminal is a stub for non-cgo builds
func (s *Server) handleWSTerminal(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Terminal not available in nocgo build", http.StatusNotImplemented)
}

// handleClaudePrompt is a stub for non-cgo builds
func (s *Server) handleClaudePrompt(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Claude prompt not available in nocgo build", http.StatusNotImplemented)
}

// handleClaudeInteractions is a stub for non-cgo builds
func (s *Server) handleClaudeInteractions(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Claude interactions not available in nocgo build", http.StatusNotImplemented)
}
