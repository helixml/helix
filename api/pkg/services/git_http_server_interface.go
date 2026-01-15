package services

import (
	"github.com/gorilla/mux"
)

// GitHTTPServerInterface defines the common interface for git HTTP server implementations.
// Both the CGI-based GitHTTPServer and the pure Go GoGitHTTPServer implement this interface.
type GitHTTPServerInterface interface {
	// RegisterRoutes registers the HTTP routes for the git server
	RegisterRoutes(router *mux.Router)

	// GetCloneURL returns the HTTP clone URL for a repository
	GetCloneURL(repositoryID string) string

	// SetTestMode enables or disables test mode
	SetTestMode(enabled bool)

	// SetMessageSender sets the callback for sending messages to spec task agents
	SetMessageSender(sender SpecTaskMessageSender)
}

// Compile-time interface checks
var _ GitHTTPServerInterface = (*GitHTTPServer)(nil)
var _ GitHTTPServerInterface = (*GoGitHTTPServer)(nil)
