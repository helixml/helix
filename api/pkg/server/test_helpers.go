package server

import (
	"net/http"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// NewTestServer creates a minimal HelixAPIServer for testing the WebSocket
// sync protocol. It initializes only the fields needed by the sync handlers
// (handleThreadCreated, handleMessageAdded, handleMessageCompleted, etc.),
// using the provided store and pubsub implementations.
//
// The returned server has a fully functional WebSocket handler that processes
// sync events using the same production code paths. Use ExternalAgentSyncHandler()
// to get the HTTP handler, and QueueCommand() to send commands to connected agents.
func NewTestServer(s store.Store, ps pubsub.PubSub) *HelixAPIServer {
	ctrl := &controller.Controller{
		Options: controller.Options{
			Store:  s,
			PubSub: ps,
		},
	}
	return &HelixAPIServer{
		Cfg: &config.ServerConfig{
			WebServer: config.WebServer{
				URL:         "http://localhost:0",
				Host:        "localhost",
				Port:        0,
				RunnerToken: "test-runner-token",
			},
		},
		Store:                       s,
		pubsub:                      ps,
		Controller:                  ctrl,
		externalAgentWSManager:      NewExternalAgentWSManager(),
		externalAgentRunnerManager:  NewExternalAgentRunnerManager(),
		contextMappings:             make(map[string]string),
		sessionToWaitingInteraction: make(map[string][]string),
		requestToSessionMapping:     make(map[string]string),
		externalAgentSessionMapping: make(map[string]string),
		externalAgentUserMapping:    make(map[string]string),
		sessionCommentTimeout:       make(map[string]*time.Timer),
		requestToCommenterMapping:   make(map[string]string),
		streamingContexts:          make(map[string]*streamingContext),
		streamingRateLimiter:        make(map[string]time.Time),
	}
}

// ExternalAgentSyncHandler returns the production WebSocket handler for
// external agent sync connections. This is the same handler registered at
// /api/v1/external-agents/sync in the production router.
func (s *HelixAPIServer) ExternalAgentSyncHandler() http.HandlerFunc {
	return s.handleExternalAgentSync
}

// QueueCommand sends a command to an agent connected via WebSocket.
// Returns true if the command was queued/sent, false if no connection exists.
func (s *HelixAPIServer) QueueCommand(sessionID string, cmd types.ExternalAgentCommand) bool {
	return s.externalAgentWSManager.queueOrSend(sessionID, cmd)
}

// SendChatMessage sends a chat message through the production code path,
// creating an interaction and sending the WebSocket command. This is the
// same path used by sendMessageToSpecTaskAgent.
func (s *HelixAPIServer) SendChatMessage(sessionID, message, requestID string) error {
	_, err := s.sendChatMessageToExternalAgent(sessionID, message, requestID)
	return err
}

// ConnectedAgentIDs returns the IDs of all currently connected agents.
func (s *HelixAPIServer) ConnectedAgentIDs() []string {
	s.externalAgentWSManager.mu.RLock()
	defer s.externalAgentWSManager.mu.RUnlock()
	ids := make([]string, 0, len(s.externalAgentWSManager.connections))
	for id := range s.externalAgentWSManager.connections {
		ids = append(ids, id)
	}
	return ids
}

// WaitForAgent polls until at least one agent connects, then returns its ID.
func (s *HelixAPIServer) WaitForAgent(timeout time.Duration) (string, bool) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		ids := s.ConnectedAgentIDs()
		if len(ids) > 0 {
			return ids[0], true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return "", false
}

// ContextMappings returns the current Zed thread → Helix session mappings.
// Used by test binaries to discover which sessions were created for which threads.
func (s *HelixAPIServer) ContextMappings() map[string]string {
	s.contextMappingsMutex.RLock()
	defer s.contextMappingsMutex.RUnlock()
	cp := make(map[string]string, len(s.contextMappings))
	for k, v := range s.contextMappings {
		cp[k] = v
	}
	return cp
}

// SetExternalAgentUserMapping sets the user ID mapping for an agent session.
// The production handler uses this to determine the owner when creating sessions.
func (s *HelixAPIServer) SetExternalAgentUserMapping(agentSessionID, userID string) {
	s.contextMappingsMutex.Lock()
	defer s.contextMappingsMutex.Unlock()
	s.externalAgentUserMapping[agentSessionID] = userID
}

// ProcessSyncEvent injects a sync event as if it came from a connected agent.
// Used by E2E tests to simulate events that the Zed binary can't send in
// headless mode (e.g. user_created_thread which requires UI interaction).
func (s *HelixAPIServer) ProcessSyncEvent(sessionID string, syncMsg *types.SyncMessage) error {
	return s.processExternalAgentSyncMessage(sessionID, syncMsg)
}

// SyncEventHook is a callback invoked after each sync event is processed.
// Set via SetSyncEventHook for test observability. The hook field is on
// HelixAPIServer (syncEventHook), nil in production.
type SyncEventHook func(sessionID string, syncMsg *types.SyncMessage)

// SetSyncEventHook registers a callback that fires after every sync event
// is processed by processExternalAgentSyncMessage. Pass nil to clear.
func (s *HelixAPIServer) SetSyncEventHook(hook SyncEventHook) {
	s.syncEventHook = hook
}
