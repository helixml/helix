package server

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// chatMessagePaceWindow is a drain-window backoff for the claude-agent-acp
// wrapper: when a follow-up prompt arrives on the same session before the
// wrapper has finalised a previous cancelled turn the wrapper returns
// "Internal error: [ede_diagnostic] result_type=user last_content_type=n/a stop_reason=null".
// The cross-repo E2E test driver fires chat_messages programmatically with
// zero delay, which hammers this race; the existing 2s gap in
// advanceAfterCompletion is empirically not enough. This pacing in
// QueueCommand sets a per-session floor; tune up if CI continues to flake.
// Production chat_message paths flow through sendChatMessageToExternalAgent
// and do not call QueueCommand, so this is test-traffic-only.
const chatMessagePaceWindow = 4 * time.Second

var (
	lastChatMessageMu   sync.Mutex
	lastChatMessageTime = map[string]time.Time{}
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
		requestToSessionMapping:     make(map[string]string),
		requestToInteractionMapping: make(map[string]string),
		pendingCancelChannels:       make(map[string]chan string),
		externalAgentSessionMapping: make(map[string]string),
		externalAgentUserMapping:    make(map[string]string),
		sessionCommentTimeout:       make(map[string]*time.Timer),
		requestToCommenterMapping:   make(map[string]string),
		streamingContexts:          make(map[string]*streamingContext),
		streamingRateLimiter:        make(map[string]time.Time),
		activeStreamProxies:         make(map[string]*activeStreamProxy),
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
//
// For chat_message commands, calls are paced per-session by
// chatMessagePaceWindow to work around the claude-agent-acp drain race
// (see the chatMessagePaceWindow doc).
func (s *HelixAPIServer) QueueCommand(sessionID string, cmd types.ExternalAgentCommand) bool {
	if cmd.Type == "chat_message" {
		paceChatMessage(sessionID)
	}
	return s.externalAgentWSManager.queueOrSend(sessionID, cmd)
}

// paceChatMessage enforces a minimum spacing between chat_message sends on
// the same session by reserving the next available slot at least
// chatMessagePaceWindow after the previous send. Concurrent callers serialise
// through reserved slots so two parallel calls do not both fire immediately.
func paceChatMessage(sessionID string) {
	lastChatMessageMu.Lock()
	target := time.Now()
	if last, ok := lastChatMessageTime[sessionID]; ok {
		if scheduled := last.Add(chatMessagePaceWindow); scheduled.After(target) {
			target = scheduled
		}
	}
	lastChatMessageTime[sessionID] = target
	wait := time.Until(target)
	lastChatMessageMu.Unlock()
	if wait > 0 {
		time.Sleep(wait)
	}
}

// SendChatMessage sends a chat message via the low-level
// sendChatMessageToExternalAgent primitive (creates an interaction with the
// caller-supplied request_id and sends the WebSocket command). This is a
// TEST-HARNESS entry point for the WebSocket-sync e2e server, which needs to
// supply its own request_id and assert routing on it. Production sends go
// through the session-scoped prompt queue, not this path.
//
// Defaults interrupt=false to preserve historical behaviour for the cross-repo
// e2e test server (zed-repo). Use SendChatMessageWithInterrupt for tests that
// need to exercise the interrupt path.
func (s *HelixAPIServer) SendChatMessage(sessionID, message, requestID string) error {
	_, err := s.sendChatMessageToExternalAgent(sessionID, message, requestID, false)
	return err
}

// SendChatMessageWithInterrupt sends a chat message with the interrupt flag set,
// matching the semantic used by design-review comments and the prompt-history queue.
func (s *HelixAPIServer) SendChatMessageWithInterrupt(sessionID, message, requestID string, interrupt bool) error {
	_, err := s.sendChatMessageToExternalAgent(sessionID, message, requestID, interrupt)
	return err
}

// EnqueueQueuedPrompt drives the PRODUCTION send path used by all agent
// messaging: it enqueues a prompt onto the session-scoped prompt queue and
// drains it synchronously (dispatch when idle, or defer / cancel-then-send per
// interrupt), exactly as processPendingPromptsForSession does in production.
//
// Unlike SendChatMessage (the low-level primitive), this exercises
// enqueueAgentMessage → processPendingPromptsForSession → processPromptQueue /
// processInterruptPrompt → sendQueuedPromptToSession. The WS-sync e2e uses it to
// validate the real production queue behaviour (busy-defer and interrupt) against
// a live Zed binary. Returns the queue-entry (prompt) id. The drain is
// synchronous so tests observe the outcome deterministically without polling.
func (s *HelixAPIServer) EnqueueQueuedPrompt(sessionID, message string, interrupt bool) (string, error) {
	promptID, err := s.persistQueuedPrompt(context.Background(), sessionID, message, interrupt, "", "")
	if err != nil {
		return "", err
	}
	s.processPendingPromptsForSession(context.Background(), sessionID)
	return promptID, nil
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

// FindConnectedSessionForSpecTask exposes the production routing logic
// for tests. Returns the session ID that would be used to send a message
// to the given spectask.
func (s *HelixAPIServer) FindConnectedSessionForSpecTask(ctx context.Context, specTask *types.SpecTask) (string, error) {
	return s.findConnectedSessionForSpecTask(ctx, specTask)
}

// SendCancelToExternalAgent sends a cancel_current_turn command and waits
// for the turn_cancelled response. Exposed for E2E tests.
func (s *HelixAPIServer) SendCancelToExternalAgent(sessionID, requestID string, timeout time.Duration) (string, error) {
	return s.sendCancelToExternalAgent(sessionID, requestID, timeout)
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
