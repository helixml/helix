package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/server/wsprotocol"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// streamingContext caches DB query results during token streaming to avoid
// redundant queries. Created on first message_added, cleared on message_completed.
// Also buffers interaction updates: DB writes are throttled to at most once per
// dbWriteInterval, and frontend publishes to once per publishInterval.
type streamingContext struct {
	session     *types.Session
	interaction *types.Interaction
	// Track which interaction this context is for - used to detect transitions
	interactionID string
	// Message IDs from previous completed interactions in this session.
	// Used to prevent re-accumulating old entries when Zed flushes the entire thread.
	excludedMessageIDs map[string]bool
	// Commenter ID for design review comment streaming (looked up from sessionToCommenterMapping)
	commenterID string
	// DB write throttling
	lastDBWrite time.Time
	dirty       bool // true if interaction has been updated since last DB write
	// Frontend publish throttling
	lastPublish time.Time
	// Per-entry delta tracking: tracks entries sent to frontend so we can compute per-entry diffs
	previousEntries []wsprotocol.ResponseEntry
	// Message accumulator: persists across handleMessageAdded calls so that
	// out-of-order flush updates (Stopped event) can replace earlier message_ids
	// in-place instead of appending duplicates. A new accumulator per call would
	// lose the message_id→content mapping because the DB only stores the joined string.
	accumulator *wsprotocol.MessageAccumulator
	mu          sync.Mutex
}

const (
	// dbWriteInterval is the minimum time between UpdateInteraction calls during streaming.
	// Intermediate content is buffered in the streamingContext.
	// Risk: up to dbWriteInterval of content lost on crash. Acceptable because
	// message_completed always writes the final state, and Zed has the full content.
	dbWriteInterval = 200 * time.Millisecond

	// publishInterval is the minimum time between frontend pubsub events during streaming.
	// Frontend batches to requestAnimationFrame (~16ms), so faster is wasted work.
	publishInterval = 50 * time.Millisecond
)

// External agent WebSocket connections
type ExternalAgentWSManager struct {
	connections map[string]*ExternalAgentWSConnection
	mu          sync.RWMutex
	upgrader    websocket.Upgrader

	// Session readiness tracking - prevents sending messages before agent is ready
	readinessState map[string]*SessionReadinessState
	readinessMu    sync.RWMutex
}

// SessionReadinessState tracks whether an agent session is ready to receive messages
// This prevents race conditions where we send prompts before Zed has loaded the agent
type SessionReadinessState struct {
	IsReady       bool                         // True when agent_ready received
	ReadyAt       time.Time                    // When agent became ready
	PendingQueue  []types.ExternalAgentCommand // Commands queued before ready
	TimeoutTimer  *time.Timer                  // Fallback timeout (60s)
	SessionID     string                       // For logging
	NeedsContinue bool                         // Whether to send continue prompt when ready
}

type ExternalAgentWSConnection struct {
	SessionID   string
	Conn        *websocket.Conn
	ConnectedAt time.Time
	LastPing    time.Time
	SendChan    chan types.ExternalAgentCommand
	mu          sync.Mutex
}

func NewExternalAgentWSManager() *ExternalAgentWSManager {
	return &ExternalAgentWSManager{
		connections:    make(map[string]*ExternalAgentWSConnection),
		readinessState: make(map[string]*SessionReadinessState),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // TODO: Add proper origin validation
			},
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		},
	}
}

// External agent runner connection manager (tracks /ws/external-agent-runner connections)
type ExternalAgentRunnerManager struct {
	runnerConnections map[string][]*ExternalAgentRunnerConnection // map[runnerID][]connections
	mu                sync.RWMutex
}

type ExternalAgentRunnerConnection struct {
	ConnectionID string
	RunnerID     string
	ConnectedAt  time.Time
	LastPing     time.Time
	Concurrency  int
	Status       string
}

func NewExternalAgentRunnerManager() *ExternalAgentRunnerManager {
	return &ExternalAgentRunnerManager{
		runnerConnections: make(map[string][]*ExternalAgentRunnerConnection),
	}
}

// addConnection adds a runner connection with unique connection ID
func (manager *ExternalAgentRunnerManager) addConnection(runnerID string, concurrency int) string {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	now := time.Now()
	// Generate unique connection ID: runnerID + timestamp + microseconds
	connectionID := fmt.Sprintf("%s-%d-%d", runnerID, now.Unix(), now.Nanosecond()/1000)

	newConnection := &ExternalAgentRunnerConnection{
		ConnectionID: connectionID,
		RunnerID:     runnerID,
		ConnectedAt:  now,
		LastPing:     now,
		Concurrency:  concurrency,
		Status:       "connected",
	}

	// Add to runner's connection array
	manager.runnerConnections[runnerID] = append(manager.runnerConnections[runnerID], newConnection)

	log.Info().
		Str("runner_id", runnerID).
		Str("connection_id", connectionID).
		Int("concurrency", concurrency).
		Int("total_connections", len(manager.runnerConnections[runnerID])).
		Msg("🔗 External agent runner connection added to manager")

	return connectionID
}

// removeConnection removes a specific connection by runner ID and connection ID
func (manager *ExternalAgentRunnerManager) removeConnection(runnerID, connectionID string) {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	// Find and remove the connection from the specific runner's array
	connections, exists := manager.runnerConnections[runnerID]
	if !exists {
		log.Warn().
			Str("runner_id", runnerID).
			Str("connection_id", connectionID).
			Msg("⚠️ Attempted to remove connection from non-existent runner")
		return
	}

	for i, conn := range connections {
		if conn.ConnectionID == connectionID {
			// Remove this connection from the slice
			manager.runnerConnections[runnerID] = append(connections[:i], connections[i+1:]...)

			// If no connections left for this runner, remove the runner entry
			if len(manager.runnerConnections[runnerID]) == 0 {
				delete(manager.runnerConnections, runnerID)
			}

			log.Info().
				Str("runner_id", runnerID).
				Str("connection_id", connectionID).
				Int("remaining_connections", len(manager.runnerConnections[runnerID])).
				Msg("🔌 External agent runner connection removed from manager")
			return
		}
	}

	log.Warn().
		Str("runner_id", runnerID).
		Str("connection_id", connectionID).
		Msg("⚠️ Attempted to remove non-existent connection")
}

// updatePingByRunner updates the last ping time for the most recent connection of a runner
func (manager *ExternalAgentRunnerManager) updatePingByRunner(runnerID string) {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	connections, exists := manager.runnerConnections[runnerID]
	if !exists || len(connections) == 0 {
		log.Warn().
			Str("runner_id", runnerID).
			Msg("⚠️ Attempted to update ping for non-existent runner connection")
		return
	}

	// Find the most recent connection for this runner
	var mostRecentConn *ExternalAgentRunnerConnection
	var mostRecentTime time.Time

	for _, conn := range connections {
		if conn.ConnectedAt.After(mostRecentTime) {
			mostRecentConn = conn
			mostRecentTime = conn.ConnectedAt
		}
	}

	if mostRecentConn != nil {
		oldPing := mostRecentConn.LastPing
		mostRecentConn.LastPing = time.Now()

		log.Info().
			Str("runner_id", runnerID).
			Str("connection_id", mostRecentConn.ConnectionID).
			Time("old_ping", oldPing).
			Time("new_ping", mostRecentConn.LastPing).
			Dur("ping_interval", mostRecentConn.LastPing.Sub(oldPing)).
			Int("total_connections", len(connections)).
			Msg("🏓 External agent runner ping timestamp updated")
	}
}

// handleExternalAgentSync handles WebSocket connections from external agents (Zed instances)
func (apiServer *HelixAPIServer) handleExternalAgentSync(res http.ResponseWriter, req *http.Request) {
	log.Trace().
		Str("method", req.Method).
		Str("url", req.URL.String()).
		Str("remote_addr", req.RemoteAddr).
		Msg("[HELIX] External agent WebSocket connection attempt")
	// Extract session ID from query parameters (checks both session_id and agent_id for compatibility)
	agentID := req.URL.Query().Get("session_id")
	if agentID == "" {
		agentID = req.URL.Query().Get("agent_id")
	}
	if agentID == "" {
		// Generate a unique agent ID for this connection
		agentID = fmt.Sprintf("external-agent-%d", time.Now().UnixNano())
		log.Info().Str("agent_id", agentID).Msg("Generated agent ID for external agent connection")
	}

	// Validate auth token
	authHeader := req.Header.Get("Authorization")
	if authHeader == "" {
		http.Error(res, "Authorization header required", http.StatusUnauthorized)
		return
	}

	// Extract token from "Bearer <token>" format
	token := ""
	if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
		token = authHeader[7:]
	}

	if !apiServer.validateExternalAgentToken(agentID, token) {
		http.Error(res, "Invalid token", http.StatusUnauthorized)
		return
	}

	// Upgrade to WebSocket
	conn, err := apiServer.externalAgentWSManager.upgrader.Upgrade(res, req, nil)
	if err != nil {
		log.Error().Err(err).Str("agent_id", agentID).Msg("Failed to upgrade WebSocket")
		return
	}

	log.Info().Str("agent_id", agentID).Msg("External agent WebSocket connected")

	// Create connection wrapper
	wsConn := &ExternalAgentWSConnection{
		SessionID:   agentID, // Using agent ID as connection identifier
		Conn:        conn,
		ConnectedAt: time.Now(),
		LastPing:    time.Now(),
		SendChan:    make(chan types.ExternalAgentCommand, 100),
	}

	// Register connection with agent ID
	apiServer.externalAgentWSManager.registerConnection(agentID, wsConn)
	defer apiServer.externalAgentWSManager.unregisterConnection(agentID)

	// Check if this agent has a Helix session mapping
	// agentID could be either agent_session_id (req_*) or helix_session_id (ses_*)
	helixSessionID := ""
	if strings.HasPrefix(agentID, "ses_") {
		// Direct Helix session ID
		helixSessionID = agentID
		log.Trace().
			Str("agent_session_id", agentID).
			Str("helix_session_id", helixSessionID).
			Msg("[HELIX] External agent connected with Helix session ID, checking for initial message")
	} else {
		apiServer.contextMappingsMutex.RLock()
		mappedHelixID, exists := apiServer.externalAgentSessionMapping[agentID]
		apiServer.contextMappingsMutex.RUnlock()
		if exists {
			// Agent session ID mapping - register connection with BOTH IDs for routing
			helixSessionID = mappedHelixID
			apiServer.externalAgentWSManager.registerConnection(helixSessionID, wsConn)
			defer apiServer.externalAgentWSManager.unregisterConnection(helixSessionID)
			log.Info().
				Str("agent_session_id", agentID).
				Str("helix_session_id", helixSessionID).
				Msg("🚀 [HELIX] External agent connected with agent session ID, registered with BOTH IDs for routing")
		}
	}

	// Start goroutines for handling connection
	ctx, cancel := context.WithCancel(req.Context())
	defer cancel()

	// Start message sender
	go apiServer.handleExternalAgentSender(ctx, wsConn)

	if helixSessionID != "" {

		// Get the Helix session to find the initial interaction
		helixSession, err := apiServer.Controller.Options.Store.GetSession(ctx, helixSessionID)
		if err == nil && helixSession != nil {
			// CRITICAL: Rebuild contextMappings from persisted ZedThreadID if present
			// This ensures message routing works after API server restarts
			if helixSession.Metadata.ZedThreadID != "" {
				apiServer.contextMappingsMutex.Lock()
				apiServer.contextMappings[helixSession.Metadata.ZedThreadID] = helixSessionID
				apiServer.contextMappingsMutex.Unlock()
				log.Trace().
					Str("helix_session_id", helixSessionID).
					Str("zed_thread_id", helixSession.Metadata.ZedThreadID).
					Msg("[HELIX] Restored contextMappings from session metadata")
			}

			// Check if agent was working before disconnect (to determine if continue prompt needed)
			needsContinue := false

			// Initialize readiness tracking - we'll wait for agent_ready before sending continue prompt
			// This prevents race conditions where we send prompts before the agent is ready
			apiServer.externalAgentWSManager.initReadinessState(helixSessionID, needsContinue, nil)
			defer apiServer.externalAgentWSManager.cleanupReadinessState(helixSessionID)

			// Find and queue the waiting interaction for the agent
			apiServer.pickupWaitingInteraction(ctx, helixSessionID, helixSession, agentID)
		}
	}

	// Handle incoming messages (blocking)
	apiServer.handleExternalAgentReceiver(ctx, wsConn)
}

// pickupWaitingInteraction finds the most recent waiting interaction for a session
// and queues the initial chat_message for the external agent. If no
// requestToSessionMapping entry exists (e.g. session created via session handler
// rather than sendMessageToSpecTaskAgent), it falls back to using the interaction
// ID as request_id — the same convention sendMessageToSpecTaskAgent uses.
func (apiServer *HelixAPIServer) pickupWaitingInteraction(ctx context.Context, helixSessionID string, helixSession *types.Session, agentID string) {
	interactions, _, err := apiServer.Controller.Options.Store.ListInteractions(ctx, &types.ListInteractionsQuery{
		SessionID:    helixSessionID,
		GenerationID: helixSession.GenerationID,
		PerPage:      1000,
	})
	if err != nil || len(interactions) == 0 {
		return
	}

	// Find the most recent waiting interaction
	for i := len(interactions) - 1; i >= 0; i-- {
		if interactions[i].State != types.InteractionStateWaiting {
			continue
		}

		// Look up request_id under lock (requestToSessionMapping is written
		// concurrently by sendMessageToSpecTaskAgent). If no mapping exists,
		// fall back to the interaction ID.
		interactionID := interactions[i].ID

		apiServer.contextMappingsMutex.Lock()
		var requestID string
		for rid, sid := range apiServer.requestToSessionMapping {
			if sid == helixSessionID {
				requestID = rid
				break
			}
		}
		if requestID == "" {
			requestID = interactionID
			if apiServer.requestToSessionMapping == nil {
				apiServer.requestToSessionMapping = make(map[string]string)
			}
			apiServer.requestToSessionMapping[requestID] = helixSessionID
			if apiServer.sessionToWaitingInteraction == nil {
				apiServer.sessionToWaitingInteraction = make(map[string][]string)
			}
			apiServer.sessionToWaitingInteraction[helixSessionID] = append(
				apiServer.sessionToWaitingInteraction[helixSessionID], interactionID)
			log.Info().
				Str("helix_session_id", helixSessionID).
				Str("request_id", requestID).
				Msg("🔧 [HELIX] Created request_id mapping from waiting interaction ID")
		}
		apiServer.contextMappingsMutex.Unlock()

		// Combine system prompt and user message into a single message
		fullMessage := interactions[i].PromptMessage
		if interactions[i].SystemPrompt != "" {
			fullMessage = interactions[i].SystemPrompt + "\n\n**User Request:**\n" + interactions[i].PromptMessage
		}

		// Determine which agent to use based on the spec task's code agent config
		agentName := apiServer.getAgentNameForSession(ctx, helixSession)

		// Use existing thread if available, otherwise create new
		var acpThreadID interface{} = nil
		if helixSession.Metadata.ZedThreadID != "" {
			acpThreadID = helixSession.Metadata.ZedThreadID
			log.Info().
				Str("helix_session_id", helixSessionID).
				Str("zed_thread_id", helixSession.Metadata.ZedThreadID).
				Msg("🔗 [HELIX] Resuming in existing Zed thread after reconnect")
		}

		command := types.ExternalAgentCommand{
			Type: "chat_message",
			Data: map[string]interface{}{
				"message":       fullMessage,
				"request_id":    requestID,
				"acp_thread_id": acpThreadID,
				"agent_name":    agentName,
			},
		}

		if apiServer.externalAgentWSManager.queueOrSend(helixSessionID, command) {
			log.Info().
				Str("agent_session_id", agentID).
				Str("request_id", requestID).
				Str("helix_session_id", helixSessionID).
				Msg("✅ [HELIX] Queued initial chat_message for Zed (will send when agent_ready)")
		} else {
			log.Warn().
				Str("agent_session_id", agentID).
				Msg("⚠️ [HELIX] Failed to queue initial message")
		}
		return
	}
}

// handleExternalAgentReceiver handles incoming messages from external agent
func (apiServer *HelixAPIServer) handleExternalAgentReceiver(ctx context.Context, wsConn *ExternalAgentWSConnection) {
	defer wsConn.Conn.Close()

	// Set read deadline and pong handler
	wsConn.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	wsConn.Conn.SetPongHandler(func(appData string) error {
		wsConn.LastPing = time.Now()
		wsConn.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		select {
		case <-ctx.Done():
			return
		default:
			var syncMsg types.SyncMessage
			if err := wsConn.Conn.ReadJSON(&syncMsg); err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					log.Error().Err(err).Str("session_id", wsConn.SessionID).Msg("WebSocket read error")
				}
				return
			}

			// Process sync message
			if err := apiServer.processExternalAgentSyncMessage(wsConn.SessionID, &syncMsg); err != nil {
				log.Error().Err(err).Str("session_id", wsConn.SessionID).Str("event_type", syncMsg.EventType).Msg("Failed to process sync message")
			}
		}
	}
}

// handleExternalAgentSender handles outgoing messages to external agent
func (apiServer *HelixAPIServer) handleExternalAgentSender(ctx context.Context, wsConn *ExternalAgentWSConnection) {
	ticker := time.NewTicker(54 * time.Second) // Ping every 54 seconds
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case command := <-wsConn.SendChan:
			wsConn.mu.Lock()
			if err := wsConn.Conn.WriteJSON(command); err != nil {
				log.Error().Err(err).Str("session_id", wsConn.SessionID).Msg("Failed to send command to external agent")
				wsConn.mu.Unlock()
				return
			}
			wsConn.mu.Unlock()
		case <-ticker.C:
			wsConn.mu.Lock()
			if err := wsConn.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				log.Error().Err(err).Str("session_id", wsConn.SessionID).Msg("Failed to send ping")
				wsConn.mu.Unlock()
				return
			}
			wsConn.mu.Unlock()
		}
	}
}

// processExternalAgentSyncMessage processes incoming sync messages from external agents
func (apiServer *HelixAPIServer) processExternalAgentSyncMessage(sessionID string, syncMsg *types.SyncMessage) error {
	log.Trace().
		Str("agent_session_id", sessionID).
		Str("event_type", syncMsg.EventType).
		Interface("data", syncMsg.Data).
		Msg("[HELIX] Processing message from external agent")

	// Process sync message directly
	var err error
	switch syncMsg.EventType {
	case "thread_created":
		err = apiServer.handleThreadCreated(sessionID, syncMsg)
	case "user_created_thread":
		err = apiServer.handleUserCreatedThread(sessionID, syncMsg)
	case "thread_title_changed":
		err = apiServer.handleThreadTitleChanged(sessionID, syncMsg)
	case "context_created": // Legacy support - redirect to thread_created
		err = apiServer.handleThreadCreated(sessionID, syncMsg)
	case "message_added":
		err = apiServer.handleMessageAdded(sessionID, syncMsg)
	case "message_updated":
		err = apiServer.handleMessageUpdated(sessionID, syncMsg)
	case "context_title_changed":
		err = apiServer.handleContextTitleChanged(sessionID, syncMsg)
	case "chat_response":
		err = apiServer.handleChatResponse(sessionID, syncMsg)
	case "chat_response_chunk":
		err = apiServer.handleChatResponseChunk(sessionID, syncMsg)
	case "chat_response_done":
		err = apiServer.handleChatResponseDone(sessionID, syncMsg)
	case "message_completed":
		err = apiServer.handleMessageCompleted(sessionID, syncMsg)
	case "thread_load_error":
		err = apiServer.handleThreadLoadError(sessionID, syncMsg)
	case "chat_response_error":
		err = apiServer.handleChatResponseError(sessionID, syncMsg)
	case "agent_ready":
		err = apiServer.handleAgentReady(sessionID, syncMsg)
	case "ping":
		// no-op
	default:
		log.Warn().Str("event_type", syncMsg.EventType).Msg("Unknown sync message type")
	}

	// Fire test hook if registered (nil in production)
	if apiServer.syncEventHook != nil {
		apiServer.syncEventHook(sessionID, syncMsg)
	}

	return err
}

// handleThreadCreated processes thread creation from external agent (new protocol)
func (apiServer *HelixAPIServer) handleThreadCreated(sessionID string, syncMsg *types.SyncMessage) error {
	// NEW PROTOCOL: use acp_thread_id
	acpThreadID, ok := syncMsg.Data["acp_thread_id"].(string)
	if !ok {
		// FALLBACK: try old context_id for compatibility
		acpThreadID, ok = syncMsg.Data["context_id"].(string)
		if !ok {
			return fmt.Errorf("missing or invalid acp_thread_id/context_id")
		}
	}

	contextID := acpThreadID // Use contextID as alias for compatibility with rest of code

	title, _ := syncMsg.Data["title"].(string)
	if title == "" {
		title = "New Conversation"
	}

	// NEW PROTOCOL: Extract request_id for correlation
	requestID, _ := syncMsg.Data["request_id"].(string)

	// Check if this is a response to a Helix-initiated request
	// If syncMsg has a session_id, this is a response to an existing Helix session
	helixSessionID := syncMsg.SessionID

	log.Info().
		Str("agent_session_id", sessionID).
		Str("helix_session_id", helixSessionID).
		Str("acp_thread_id", acpThreadID).
		Str("request_id", requestID).
		Str("title", title).
		Msg("🔧 [HELIX] Processing thread_created from external agent")

	// PRIORITY 1: Check if request_id maps to an existing Helix session
	// This handles the case where API sent chat_message to Zed with a request_id
	if requestID != "" {
		apiServer.contextMappingsMutex.RLock()
		mappedSessionID, exists := apiServer.requestToSessionMapping[requestID]
		apiServer.contextMappingsMutex.RUnlock()
		if exists {
			log.Info().
				Str("request_id", requestID).
				Str("helix_session_id", mappedSessionID).
				Str("acp_thread_id", acpThreadID).
				Msg("✅ [HELIX] Found existing Helix session via request_id mapping")

			helixSessionID = mappedSessionID // Use the mapped session

			// Clean up only the session mapping - we still need requestToCommenterMapping
			// for streaming updates (message_added, message_completed come AFTER user_created_thread)
			apiServer.contextMappingsMutex.Lock()
			delete(apiServer.requestToSessionMapping, requestID)
			apiServer.contextMappingsMutex.Unlock()
			// NOTE: Do NOT delete requestToCommenterMapping here - it's needed for message streaming
			log.Info().
				Str("request_id", requestID).
				Msg("🧹 [HELIX] Cleaned up request_id → session mapping")
		}
	}

	// PRIORITY 2: Check if helixSessionID is provided or was found via request_id
	// If helixSessionID is provided, REUSE that session instead of creating a new one
	if helixSessionID != "" {
		// This is a response to a Helix-initiated request - store zed_context_id on the session
		log.Info().
			Str("agent_session_id", sessionID).
			Str("helix_session_id", helixSessionID).
			Str("zed_context_id", contextID).
			Msg("✅ [HELIX] Storing Zed context ID on existing Helix session")

		// Get the existing session
		helixSession, err := apiServer.Controller.Options.Store.GetSession(context.Background(), helixSessionID)
		if err != nil {
			return fmt.Errorf("failed to get Helix session %s: %w", helixSessionID, err)
		}

		// Store the zed_context_id and agent name on the session metadata.
		// The agent name is persisted so we use the correct agent for this thread
		// even if the project's default agent changes later.
		helixSession.Metadata.ZedThreadID = contextID
		if helixSession.Metadata.ZedAgentName == "" {
			helixSession.Metadata.ZedAgentName = apiServer.getAgentNameForSession(context.Background(), helixSession)
		}
		helixSession.Updated = time.Now()

		// Update the session in the database
		_, err = apiServer.Controller.Options.Store.UpdateSession(context.Background(), *helixSession)
		if err != nil {
			return fmt.Errorf("failed to update session with zed_context_id: %w", err)
		}

		// CRITICAL: Store the mapping so message_added can find the session
		apiServer.contextMappingsMutex.Lock()
		apiServer.contextMappings[contextID] = helixSessionID
		apiServer.contextMappingsMutex.Unlock()

		log.Info().
			Str("helix_session_id", helixSessionID).
			Str("zed_context_id", contextID).
			Msg("✅ [HELIX] Successfully stored zed_context_id on session and populated contextMappings")

		// If this session belongs to a spectask, also create a SpecTaskZedThread record
		if helixSession.Metadata.SpecTaskID != "" {
			go apiServer.trackSpecTaskZedThread(context.Background(), helixSession, acpThreadID, title)
		}

		return nil
	}

	// PRIORITY 3: Check if a session already exists with this ZedThreadID.
	// This prevents creating duplicate sessions when the same thread is reported again
	// (e.g., after Zed reconnects and re-reports an existing thread).
	existingSession, err := apiServer.findSessionByZedThreadID(context.Background(), contextID)
	if err == nil && existingSession != nil {
		log.Info().
			Str("agent_session_id", sessionID).
			Str("existing_session_id", existingSession.ID).
			Str("zed_thread_id", contextID).
			Msg("✅ [HELIX] Found existing session by ZedThreadID, reusing instead of creating duplicate")

		apiServer.contextMappingsMutex.Lock()
		apiServer.contextMappings[contextID] = existingSession.ID
		apiServer.contextMappingsMutex.Unlock()

		if existingSession.Metadata.SpecTaskID != "" {
			go apiServer.trackSpecTaskZedThread(context.Background(), existingSession, acpThreadID, title)
		}

		return nil
	}

	// If no helixSessionID provided and no existing session found, this is a genuinely NEW context
	log.Info().
		Str("agent_session_id", sessionID).
		Str("context_id", contextID).
		Msg("🆕 [HELIX] Creating NEW Helix session for user-initiated Zed context")

	// Get the real user ID who created this external agent session
	apiServer.contextMappingsMutex.RLock()
	userID, exists := apiServer.externalAgentUserMapping[sessionID]
	apiServer.contextMappingsMutex.RUnlock()
	if !exists || userID == "" {
		log.Warn().
			Str("agent_session_id", sessionID).
			Msg("⚠️ [HELIX] No user mapping found for external agent, using default")
		userID = "external-agent-user" // Fallback for safety
	}

	log.Info().
		Str("agent_session_id", sessionID).
		Str("user_id", userID).
		Msg("✅ [HELIX] Using real user ID for Helix session")

	// Create a new Helix session for this Zed context
	helixSession := types.Session{
		ID:        "", // Will be generated
		Name:      title,
		Owner:     userID,
		OwnerType: types.OwnerTypeUser,
		Type:      types.SessionTypeText,
		Mode:      types.SessionModeInference,
		ModelName: "claude-3.5-sonnet", // Default model, could be configurable
		Created:   time.Now(),
		Updated:   time.Now(),
		Metadata: types.SessionMetadata{
			SystemPrompt: "You are a helpful AI assistant integrated with Zed editor.",
			AgentType:    "zed_external",
			ZedThreadID:  contextID,
		},
	}

	// Create the session in the store
	createdSession, err := apiServer.Controller.Options.Store.CreateSession(context.Background(), helixSession)
	if err != nil {
		return fmt.Errorf("failed to create Helix session: %w", err)
	}

	log.Info().
		Str("agent_session_id", sessionID).
		Str("context_id", contextID).
		Str("helix_session_id", createdSession.ID).
		Str("title", title).
		Msg("Created Helix session for user-initiated Zed context")

	// Store the context mapping for future message routing
	apiServer.contextMappingsMutex.Lock()
	if apiServer.contextMappings == nil {
		apiServer.contextMappings = make(map[string]string)
	}
	apiServer.contextMappings[contextID] = createdSession.ID
	apiServer.contextMappingsMutex.Unlock()

	// Register the WebSocket connection for the child session ID so
	// sendCommandToExternalAgent can route commands to it. The agent
	// connected under sessionID (the parent/connection ID), but child
	// sessions need their own routing entry.
	if wsConn, exists := apiServer.externalAgentWSManager.getConnection(sessionID); exists && wsConn != nil {
		apiServer.externalAgentWSManager.registerConnection(createdSession.ID, wsConn)
	}

	// CRITICAL: Create an interaction for this new session
	// The request_id from thread_created contains the message that triggered this thread
	log.Info().
		Str("context_id", contextID).
		Str("helix_session_id", createdSession.ID).
		Str("request_id", requestID).
		Msg("🆕 [HELIX] Creating initial interaction for new Zed thread")

	interaction := &types.Interaction{
		ID:              "", // Will be generated
		GenerationID:    0,
		Created:         time.Now(),
		Updated:         time.Now(),
		Scheduled:       time.Now(),
		Completed:       time.Time{},
		SessionID:       createdSession.ID,
		UserID:          createdSession.Owner,
		Mode:            types.SessionModeInference,
		PromptMessage:   "New conversation started via Zed", // Default message
		State:           types.InteractionStateWaiting,
		ResponseMessage: "",
	}

	createdInteraction, err := apiServer.Controller.Options.Store.CreateInteraction(context.Background(), interaction)
	if err != nil {
		log.Error().Err(err).
			Str("helix_session_id", createdSession.ID).
			Msg("❌ [HELIX] Failed to create interaction for new thread")
		return fmt.Errorf("failed to create interaction: %w", err)
	}

	// Notify frontend immediately so the chat updates without waiting for poll
	apiServer.publishInteractionUpdateToFrontend(createdSession.ID, createdSession.Owner, createdInteraction)

	// Enqueue the interaction so handleMessageAdded routes responses correctly
	apiServer.contextMappingsMutex.Lock()
	if apiServer.sessionToWaitingInteraction == nil {
		apiServer.sessionToWaitingInteraction = make(map[string][]string)
	}
	apiServer.sessionToWaitingInteraction[createdSession.ID] = append(apiServer.sessionToWaitingInteraction[createdSession.ID], createdInteraction.ID)
	apiServer.contextMappingsMutex.Unlock()

	log.Info().
		Str("helix_session_id", createdSession.ID).
		Str("interaction_id", createdInteraction.ID).
		Msg("✅ [HELIX] Created initial interaction and stored mapping")

	// Check if this external agent belongs to a spectask session
	// The sessionID (agent connection ID) is the Helix session ID when it starts with "ses_"
	// Look up that session to get its SpecTaskID
	if strings.HasPrefix(sessionID, "ses_") {
		originalSession, err := apiServer.Controller.Options.Store.GetSession(context.Background(), sessionID)
		if err == nil && originalSession != nil && originalSession.Metadata.SpecTaskID != "" {
			// Set the SpecTaskID on the new session too
			createdSession.Metadata.SpecTaskID = originalSession.Metadata.SpecTaskID
			createdSession.Metadata.ZedThreadID = contextID
			createdSession.Metadata.ZedAgentName = apiServer.getAgentNameForSession(context.Background(), originalSession)
			_, _ = apiServer.Controller.Options.Store.UpdateSession(context.Background(), *createdSession)

			go apiServer.trackSpecTaskZedThread(context.Background(), createdSession, acpThreadID, title)

			log.Info().
				Str("original_session_id", sessionID).
				Str("new_session_id", createdSession.ID).
				Str("spec_task_id", originalSession.Metadata.SpecTaskID).
				Str("acp_thread_id", acpThreadID).
				Msg("✅ [HELIX] Linked new user-initiated thread to spec task")
		}
	} else {
		// Fallback: check the agent session mapping (for non-ses_ agent IDs)
		apiServer.contextMappingsMutex.RLock()
		originalHelixSessionID, hasOriginal := apiServer.externalAgentSessionMapping[sessionID]
		apiServer.contextMappingsMutex.RUnlock()
		if hasOriginal {
			originalSession, err := apiServer.Controller.Options.Store.GetSession(context.Background(), originalHelixSessionID)
			if err == nil && originalSession != nil && originalSession.Metadata.SpecTaskID != "" {
				createdSession.Metadata.SpecTaskID = originalSession.Metadata.SpecTaskID
				createdSession.Metadata.ZedThreadID = contextID
				createdSession.Metadata.ZedAgentName = apiServer.getAgentNameForSession(context.Background(), originalSession)
				_, _ = apiServer.Controller.Options.Store.UpdateSession(context.Background(), *createdSession)

				go apiServer.trackSpecTaskZedThread(context.Background(), createdSession, acpThreadID, title)
			}
		}
	}

	return nil
}

// NotifyExternalAgentOfNewInteraction sends a message to external agent when a new interaction is created
func (apiServer *HelixAPIServer) NotifyExternalAgentOfNewInteraction(sessionID string, interaction *types.Interaction) error {
	// Get the session to check if it has an external agent
	session, err := apiServer.Controller.Options.Store.GetSession(context.Background(), sessionID)
	if err != nil {
		return fmt.Errorf("failed to get session: %w", err)
	}

	// Check if this session uses an external Zed agent
	if session.Metadata.AgentType != "zed_external" {
		// Not an external agent session, nothing to do
		return nil
	}

	log.Info().
		Str("session_id", sessionID).
		Str("interaction_id", interaction.ID).
		Str("agent_type", session.Metadata.AgentType).
		Msg("Notifying external agent of new interaction")

	// Build command data - include acp_thread_id if session already has one (for follow-up messages)
	commandData := map[string]interface{}{
		"message":    interaction.PromptMessage,
		"role":       "user",
		"request_id": interaction.ID, // Use interaction ID as request ID for response tracking
	}

	if session.Metadata.ZedThreadID != "" {
		commandData["acp_thread_id"] = session.Metadata.ZedThreadID
		log.Info().
			Str("session_id", sessionID).
			Str("acp_thread_id", session.Metadata.ZedThreadID).
			Msg("🔗 [HELIX] Sending follow-up message to existing Zed thread")
	}

	command := types.ExternalAgentCommand{
		Type: "chat_message",
		Data: commandData,
	}

	// If no WebSocket connection exists and this session belongs to a spec task,
	// auto-start the desktop. The waiting interaction will be picked up by
	// pickupWaitingInteraction when the agent reconnects via WebSocket.
	if _, exists := apiServer.externalAgentWSManager.getConnection(sessionID); !exists {
		if session.Metadata.SpecTaskID != "" {
			specTask, err := apiServer.Controller.Options.Store.GetSpecTask(context.Background(), session.Metadata.SpecTaskID)
			if err != nil {
				log.Error().Err(err).Str("spec_task_id", session.Metadata.SpecTaskID).Msg("Failed to load spec task for desktop auto-start")
			} else {
				log.Info().
					Str("session_id", sessionID).
					Str("spec_task_id", session.Metadata.SpecTaskID).
					Msg("No WebSocket connection, auto-starting desktop — agent will pick up waiting interaction on reconnect")
				go func() {
					if startErr := apiServer.startDesktopForSpecTask(context.Background(), specTask); startErr != nil {
						log.Error().Err(startErr).Str("spec_task_id", session.Metadata.SpecTaskID).Msg("Failed to auto-start desktop")
					}
				}()
			}
			return nil // interaction is in "waiting" state; agent will pick it up
		}
	}

	// Use the unified sendCommandToExternalAgent which handles connection lookup and routing
	return apiServer.sendCommandToExternalAgent(sessionID, command)
}

// handleMessageAdded processes message addition from external agent
func (apiServer *HelixAPIServer) handleMessageAdded(sessionID string, syncMsg *types.SyncMessage) error {
	// NEW PROTOCOL: use acp_thread_id
	contextID, ok := syncMsg.Data["acp_thread_id"].(string)
	if !ok {
		// FALLBACK: try old context_id for compatibility
		contextID, ok = syncMsg.Data["context_id"].(string)
		if !ok {
			return fmt.Errorf("missing or invalid acp_thread_id/context_id")
		}
	}

	messageID, ok := syncMsg.Data["message_id"].(string)
	if !ok {
		return fmt.Errorf("missing or invalid message_id")
	}

	content, ok := syncMsg.Data["content"].(string)
	if !ok {
		return fmt.Errorf("missing or invalid content")
	}

	var role string
	role, ok = syncMsg.Data["role"].(string)
	if !ok {
		return fmt.Errorf("missing or invalid role")
	}

	// entry_type distinguishes "text" (assistant prose) from "tool_call" (tool invocations).
	// Optional field — old Zed versions don't send it (defaults to empty string).
	entryType, _ := syncMsg.Data["entry_type"].(string)
	// Structured tool call metadata — sent by Zed for tool_call entries.
	toolName, _ := syncMsg.Data["tool_name"].(string)
	toolStatus, _ := syncMsg.Data["tool_status"].(string)

	log.Info().
		Str("session_id", sessionID).
		Str("context_id", contextID).
		Str("message_id", messageID).
		Str("role", role).
		Msg("External agent added message")

	// Find the Helix session that corresponds to this Zed context
	apiServer.contextMappingsMutex.RLock()
	helixSessionID, exists := apiServer.contextMappings[contextID]
	apiServer.contextMappingsMutex.RUnlock()
	if !exists {
		// FALLBACK: contextMappings may be empty after API restart
		// Try to find session by ZedThreadID in database
		log.Info().
			Str("context_id", contextID).
			Msg("🔍 [HELIX] contextMappings miss, attempting database fallback lookup by ZedThreadID")

		foundSession, err := apiServer.findSessionByZedThreadID(context.Background(), contextID)
		if err != nil || foundSession == nil {
			// No session found for this thread. For user messages, create a session on-the-fly.
			// This handles the race condition where MessageAdded(role=user) arrives before UserCreatedThread.
			if role != "assistant" {
				log.Info().
					Str("context_id", contextID).
					Str("agent_session_id", sessionID).
					Msg("🔧 [HELIX] No session for user message - creating session on-the-fly")

				// Get the existing agent session to copy config from
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()

				existingSession, err := apiServer.Controller.Options.Store.GetSession(ctx, sessionID)
				if err != nil {
					return fmt.Errorf("failed to load agent session to copy config: %w", err)
				}

				// Create new session with same config as agent session
				newSession := &types.Session{
					ID:             system.GenerateSessionID(),
					Created:        time.Now(),
					Updated:        time.Now(),
					Mode:           types.SessionModeInference,
					Type:           existingSession.Type,
					ModelName:      existingSession.ModelName,
					ParentApp:      existingSession.ParentApp,
					OrganizationID: existingSession.OrganizationID,
					Owner:          existingSession.Owner,
					OwnerType:      existingSession.OwnerType,
					Metadata: types.SessionMetadata{
						ZedThreadID:         contextID,
						AgentType:           existingSession.Metadata.AgentType,
						ExternalAgentConfig: existingSession.Metadata.ExternalAgentConfig,
					},
					Name: "Zed Chat", // Default name, will be updated by thread_title_changed
				}

				_, err = apiServer.Controller.Options.Store.CreateSession(ctx, *newSession)
				if err != nil {
					return fmt.Errorf("failed to create on-the-fly session: %w", err)
				}

				helixSessionID = newSession.ID
				apiServer.contextMappingsMutex.Lock()
				apiServer.contextMappings[contextID] = helixSessionID
				apiServer.contextMappingsMutex.Unlock()

				log.Info().
					Str("context_id", contextID).
					Str("helix_session_id", helixSessionID).
					Msg("✅ [HELIX] Created on-the-fly session for user message")
			} else {
				// Assistant message with no session is an error - shouldn't happen
				return fmt.Errorf("no Helix session found for context_id: %s (in-memory miss, database fallback failed)", contextID)
			}
		} else {
			helixSessionID = foundSession.ID
			// Restore the mapping for future messages
			apiServer.contextMappingsMutex.Lock()
			apiServer.contextMappings[contextID] = helixSessionID
			apiServer.contextMappingsMutex.Unlock()
			log.Info().
				Str("context_id", contextID).
				Str("helix_session_id", helixSessionID).
				Msg("✅ [HELIX] Found session via database fallback, restored contextMappings")
		}
	}

	if role == "assistant" {
		// PERFORMANCE OPTIMIZATION: Use streaming context cache to avoid
		// redundant DB queries during token streaming. GetSession and
		// ListInteractions are called once on the first token, then cached
		// for all subsequent tokens until message_completed.
		sctx := apiServer.getOrCreateStreamingContext(context.Background(), helixSessionID)
		if sctx == nil {
			return fmt.Errorf("failed to get or create streaming context for session %s", helixSessionID)
		}

		sctx.mu.Lock()
		defer sctx.mu.Unlock()

		// Look up commenter by session ID (sessionToCommenterMapping is set when comment is sent to agent)
		// message_added events from Zed don't include request_id, so we use session-based lookup
		if sctx.commenterID == "" {
			if apiServer.sessionToCommenterMapping != nil {
				if commenterID, exists := apiServer.sessionToCommenterMapping[helixSessionID]; exists {
					sctx.commenterID = commenterID
					log.Debug().
						Str("session_id", helixSessionID).
						Str("commenter_id", commenterID).
						Msg("📝 [HELIX] Found commenter for session via sessionToCommenterMapping")
				}
			}
		}

		helixSession := sctx.session
		targetInteraction := sctx.interaction

		if targetInteraction != nil {
			// Update the existing interaction with the AI response content
			// IMPORTANT: Keep state as Waiting - only message_completed marks it as Complete
			//
			// MULTI-MESSAGE HANDLING using wsprotocol.MessageAccumulator:
			// The accumulator is stored in the streaming context so it persists
			// across calls. This is critical: the Stopped flush sends corrected
			// content for earlier message_ids (out of order), and the accumulator
			// needs its message_id→content map to replace them in-place.
			// Creating a new accumulator per call would lose this mapping because
			// the DB only stores the joined Content string + LastMessageID/Offset.
			if sctx.accumulator == nil {
				sctx.accumulator = &wsprotocol.MessageAccumulator{
					Content:            targetInteraction.ResponseMessage,
					LastMessageID:      targetInteraction.LastZedMessageID,
					Offset:             targetInteraction.LastZedMessageOffset,
					ExcludedMessageIDs: sctx.excludedMessageIDs,
				}
			}
			acc := sctx.accumulator
			prevMessageID := acc.LastMessageID

			acc.AddMessageWithToolInfo(messageID, content, entryType, toolName, toolStatus)

			if prevMessageID != "" && prevMessageID != messageID {
				log.Info().
					Str("interaction_id", targetInteraction.ID).
					Str("last_message_id", prevMessageID).
					Str("new_message_id", messageID).
					Msg("📝 [HELIX] New distinct message detected (different message_id)")
			}

			targetInteraction.ResponseMessage = acc.Content
			if entriesJSON, entErr := json.Marshal(acc.Entries()); entErr == nil {
				_ = json.Unmarshal(entriesJSON, &targetInteraction.ResponseEntries)
			}
			targetInteraction.LastZedMessageID = acc.LastMessageID
			targetInteraction.LastZedMessageOffset = acc.Offset
			targetInteraction.Updated = time.Now()
			sctx.dirty = true

			// THROTTLED DB WRITE: Only flush to DB if enough time has passed.
			// The in-memory interaction always has the latest content.
			now := time.Now()
			if now.Sub(sctx.lastDBWrite) >= dbWriteInterval {
				_, err := apiServer.Controller.Options.Store.UpdateInteraction(context.Background(), targetInteraction)
				if err != nil {
					return fmt.Errorf("failed to update interaction %s: %w", targetInteraction.ID, err)
				}
				sctx.lastDBWrite = now
				sctx.dirty = false
			}

			log.Debug().
				Str("session_id", sessionID).
				Str("helix_session_id", helixSessionID).
				Str("interaction_id", targetInteraction.ID).
				Int("content_length", len(acc.Content)).
				Bool("db_written", !sctx.dirty).
				Msg("📝 [HELIX] Updated interaction in-memory")

			// THROTTLED FRONTEND PUBLISH: Only publish if enough time has passed.
			// Uses per-entry patches to reduce wire traffic from O(N) to O(delta).
			if now.Sub(sctx.lastPublish) >= publishInterval {
				currentEntries := acc.Entries()
				err := apiServer.publishEntryPatchesToFrontend(helixSessionID, helixSession.Owner, targetInteraction.ID, sctx.previousEntries, currentEntries, sctx.commenterID)
				if err != nil {
					log.Error().Err(err).
						Str("session_id", helixSessionID).
						Str("interaction_id", targetInteraction.ID).
						Msg("Failed to publish entry patches to frontend")
				}
				sctx.previousEntries = currentEntries
				sctx.lastPublish = now
			}
		} else {
			log.Warn().
				Str("session_id", sessionID).
				Str("context_id", contextID).
				Str("helix_session_id", helixSessionID).
				Msg("No interaction found to update with assistant response")
		}
	} else {
		// For user messages, check whether a pre-created Waiting interaction already exists
		// for this session (e.g. created by sendMessageToSpecTaskAgent for approval flows).
		// Zed echoes the sent user message back as message_added(role=user), which would
		// otherwise create a duplicate interaction and overwrite the mapping, causing the
		// assistant response to land in the wrong interaction (Bug 1 fix).
		// Peek at the front of the FIFO queue (don't pop — message_completed does that)
		apiServer.contextMappingsMutex.RLock()
		var existingInteractionID string
		if q := apiServer.sessionToWaitingInteraction[helixSessionID]; len(q) > 0 {
			existingInteractionID = q[0]
		}
		apiServer.contextMappingsMutex.RUnlock()

		if existingInteractionID != "" {
			// A pre-created Waiting interaction exists — this is the Zed echo of a message
			// sent by sendMessageToSpecTaskAgent. Reuse the pre-created interaction and do
			// NOT overwrite the mapping so the assistant response lands in the right place.
			log.Info().
				Str("session_id", sessionID).
				Str("context_id", contextID).
				Str("helix_session_id", helixSessionID).
				Str("existing_interaction_id", existingInteractionID).
				Msg("💬 [HELIX] Reusing pre-created interaction for Zed user-message echo (skipping duplicate creation)")
		} else {
			// No pre-created interaction — this is a genuine user message from Zed.
			// Create a new interaction and map it so the AI response goes to it.
			helixSession, err := apiServer.Controller.Options.Store.GetSession(context.Background(), helixSessionID)
			if err != nil {
				return fmt.Errorf("failed to get Helix session %s: %w", helixSessionID, err)
			}

			interaction := &types.Interaction{
				ID:            "", // Will be generated
				Created:       time.Now(),
				Updated:       time.Now(),
				SessionID:     helixSessionID,
				UserID:        helixSession.Owner,
				GenerationID:  helixSession.GenerationID, // Must match session's generation for query to find it
				Mode:          types.SessionModeInference,
				PromptMessage: content,
				State:         types.InteractionStateWaiting,
			}

			// Create the interaction in the store
			createdInteraction, err := apiServer.Controller.Options.Store.CreateInteraction(context.Background(), interaction)
			if err != nil {
				return fmt.Errorf("failed to create interaction: %w", err)
			}

			log.Info().
				Str("session_id", sessionID).
				Str("context_id", contextID).
				Str("helix_session_id", helixSessionID).
				Str("interaction_id", createdInteraction.ID).
				Str("role", role).
				Msg("💬 [HELIX] Created interaction for user message from Zed")

			// Notify frontend immediately so the chat updates without waiting for poll
			if helixSession != nil {
				apiServer.publishInteractionUpdateToFrontend(helixSessionID, helixSession.Owner, createdInteraction)
			}

			// Update session timestamp so findConnectedSessionForSpecTask
			// picks the session with the most recent activity.
			_ = apiServer.Controller.Options.Store.TouchSession(context.Background(), helixSessionID)

			// CRITICAL: Enqueue this interaction so the AI response goes to it
			apiServer.contextMappingsMutex.Lock()
			if apiServer.sessionToWaitingInteraction == nil {
				apiServer.sessionToWaitingInteraction = make(map[string][]string)
			}
			apiServer.sessionToWaitingInteraction[helixSessionID] = append(apiServer.sessionToWaitingInteraction[helixSessionID], createdInteraction.ID)
			apiServer.contextMappingsMutex.Unlock()
			log.Info().
				Str("helix_session_id", helixSessionID).
				Str("interaction_id", createdInteraction.ID).
				Msg("🗺️ [HELIX] Mapped session to new interaction from Zed user message")
		}
	}

	return nil
}

// getOrCreateStreamingContext returns a cached streaming context for the given
// helix session, or creates one by querying the DB on the first call. This avoids
// redundant GetSession + ListInteractions queries on every streaming token.
//
// IMPORTANT: Also detects interaction transitions (follow-up messages) and resets
// previousEntries when the target interaction changes. This prevents patch computation
// from using stale entries from the old interaction.
func (apiServer *HelixAPIServer) getOrCreateStreamingContext(ctx context.Context, helixSessionID string) *streamingContext {
	// Check what interaction we SHOULD be targeting: peek at front of the FIFO queue.
	// Do NOT pop here — message_completed pops after marking the interaction complete.
	apiServer.contextMappingsMutex.RLock()
	var expectedInteractionID string
	var hasMapping bool
	if q := apiServer.sessionToWaitingInteraction[helixSessionID]; len(q) > 0 {
		expectedInteractionID = q[0]
		hasMapping = true
	}
	apiServer.contextMappingsMutex.RUnlock()

	apiServer.streamingContextsMu.RLock()
	sctx, exists := apiServer.streamingContexts[helixSessionID]
	apiServer.streamingContextsMu.RUnlock()

	if exists {
		// Check if interaction has changed (follow-up message scenario)
		sctx.mu.Lock()
		if hasMapping && sctx.interactionID != "" && sctx.interactionID != expectedInteractionID {
			log.Info().
				Str("session_id", helixSessionID).
				Str("old_interaction_id", sctx.interactionID).
				Str("new_interaction_id", expectedInteractionID).
				Msg("🔄 [PERF] Interaction transition detected! Resetting streaming context for new interaction")

			// Flush any dirty state for the old interaction before switching
			if sctx.dirty && sctx.interaction != nil {
				_, err := apiServer.Controller.Options.Store.UpdateInteraction(ctx, sctx.interaction)
				if err != nil {
					log.Error().Err(err).
						Str("interaction_id", sctx.interactionID).
						Msg("Failed to flush old interaction during transition")
				}
			}

			// Reset for new interaction - will be populated below
			sctx.interaction = nil
			sctx.interactionID = ""
			sctx.previousEntries = nil
			sctx.dirty = false
			sctx.lastDBWrite = time.Time{}
			sctx.lastPublish = time.Time{}
			sctx.accumulator = nil // clear stale message_id mappings from old interaction
		}
		sctx.mu.Unlock()

		// If context still has valid interaction, return it
		if sctx.interaction != nil {
			return sctx
		}
		// Otherwise fall through to re-query and UPDATE the existing context
	}

	// First token for this session (or transition) — do the DB lookups
	helixSession, err := apiServer.Controller.Options.Store.GetSession(ctx, helixSessionID)
	if err != nil {
		log.Error().Err(err).Str("session_id", helixSessionID).
			Msg("Failed to get session for streaming context")
		return nil
	}

	interactions, _, err := apiServer.Controller.Options.Store.ListInteractions(ctx, &types.ListInteractionsQuery{
		SessionID:    helixSessionID,
		GenerationID: helixSession.GenerationID,
		PerPage:      1000,
	})
	if err != nil {
		log.Error().Err(err).Str("session_id", helixSessionID).
			Msg("Failed to list interactions for streaming context")
		return nil
	}

	// Find the target interaction: use front of FIFO queue if available
	var targetInteraction *types.Interaction
	apiServer.contextMappingsMutex.RLock()
	var mappedInteractionID string
	var hasMappingForLookup bool
	if q := apiServer.sessionToWaitingInteraction[helixSessionID]; len(q) > 0 {
		mappedInteractionID = q[0]
		hasMappingForLookup = true
	}
	apiServer.contextMappingsMutex.RUnlock()
	if hasMappingForLookup {
		for i := range interactions {
			if interactions[i].ID == mappedInteractionID {
				targetInteraction = interactions[i]
				break
			}
		}
	}
	if targetInteraction == nil {
		for i := len(interactions) - 1; i >= 0; i-- {
			if interactions[i].State == types.InteractionStateWaiting ||
				(interactions[i].State == types.InteractionStateComplete && interactions[i].ResponseMessage == "") {
				targetInteraction = interactions[i]
				break
			}
		}
	}

	// Set interactionID for tracking transitions
	var newInteractionID string
	if targetInteraction != nil {
		newInteractionID = targetInteraction.ID
	}

	// Collect message_ids from ALL other completed interactions in this session.
	// When Zed's flush_streaming_throttle fires, it resends every entry in the ACP
	// thread — including entries from previous turns. Without an exclusion set the
	// accumulator would treat those old entries as new and append them, causing
	// response_entries to balloon across interactions.
	excludedIDs := collectExcludedMessageIDs(interactions, newInteractionID)

	// If we have an existing context (from a transition), update it instead of creating new
	if exists && sctx != nil {
		sctx.mu.Lock()
		sctx.session = helixSession
		sctx.interaction = targetInteraction
		sctx.interactionID = newInteractionID
		sctx.excludedMessageIDs = excludedIDs
		sctx.mu.Unlock()

		log.Info().
			Str("session_id", helixSessionID).
			Str("interaction_id", newInteractionID).
			Msg("📦 [PERF] Updated streaming context for new interaction (transition)")

		return sctx
	}

	// Create new context
	sctx = &streamingContext{
		session:            helixSession,
		interaction:        targetInteraction,
		interactionID:      newInteractionID,
		excludedMessageIDs: excludedIDs,
	}

	apiServer.streamingContextsMu.Lock()
	// Double-check: another goroutine may have created it while we were querying
	if existing, ok := apiServer.streamingContexts[helixSessionID]; ok {
		apiServer.streamingContextsMu.Unlock()
		return existing
	}
	apiServer.streamingContexts[helixSessionID] = sctx
	apiServer.streamingContextsMu.Unlock()

	log.Info().
		Str("session_id", helixSessionID).
		Str("interaction_id", newInteractionID).
		Bool("has_interaction", targetInteraction != nil).
		Msg("📦 [PERF] Created streaming context cache (will skip DB queries on subsequent tokens)")

	return sctx
}

// collectExcludedMessageIDs extracts message_ids from ALL completed interactions
// in the session other than the target interaction. These IDs are used to prevent
// the accumulator from re-adding old entries when Zed's flush resends the entire
// ACP thread history.
func collectExcludedMessageIDs(interactions []*types.Interaction, targetInteractionID string) map[string]bool {
	excluded := make(map[string]bool)
	for _, inter := range interactions {
		if inter.ID == targetInteractionID {
			continue
		}
		if len(inter.ResponseEntries) == 0 {
			continue
		}
		var entries []wsprotocol.ResponseEntry
		if err := json.Unmarshal(inter.ResponseEntries, &entries); err != nil {
			continue
		}
		for _, e := range entries {
			if e.MessageID != "" {
				excluded[e.MessageID] = true
			}
		}
	}
	if len(excluded) == 0 {
		return nil
	}
	return excluded
}

// flushAndClearStreamingContext flushes any dirty interaction state to the DB,
// then removes the cached streaming context for a session.
// Called on message_completed to ensure the DB has the latest content before
// the interaction is marked as complete.
func (apiServer *HelixAPIServer) flushAndClearStreamingContext(ctx context.Context, helixSessionID string) []wsprotocol.ResponseEntry {
	apiServer.streamingContextsMu.Lock()
	sctx, exists := apiServer.streamingContexts[helixSessionID]
	delete(apiServer.streamingContexts, helixSessionID)
	apiServer.streamingContextsMu.Unlock()

	if !exists || sctx == nil {
		return nil
	}

	sctx.mu.Lock()
	defer sctx.mu.Unlock()

	if sctx.interaction != nil {
		if sctx.dirty {
			_, err := apiServer.Controller.Options.Store.UpdateInteraction(ctx, sctx.interaction)
			if err != nil {
				log.Error().Err(err).
					Str("session_id", helixSessionID).
					Str("interaction_id", sctx.interaction.ID).
					Msg("Failed to flush dirty interaction on streaming context clear")
			} else {
				log.Info().
					Str("session_id", helixSessionID).
					Str("interaction_id", sctx.interaction.ID).
					Int("content_length", len(sctx.interaction.ResponseMessage)).
					Msg("📦 [PERF] Flushed dirty interaction to DB before message_completed")
			}
		}

		// CRITICAL: Publish one final set of entry patches to the frontend with the
		// complete corrected content, bypassing the publish throttle. During streaming,
		// the throttle may have sent truncated snapshots. The Stopped flush corrects
		// the accumulator, but the throttle can swallow these corrections if
		// message_completed arrives immediately after.
		if sctx.session != nil && sctx.accumulator != nil {
			currentEntries := sctx.accumulator.Entries()
			err := apiServer.publishEntryPatchesToFrontend(
				helixSessionID, sctx.session.Owner, sctx.interaction.ID,
				sctx.previousEntries, currentEntries, sctx.commenterID,
			)
			if err != nil {
				log.Error().Err(err).
					Str("session_id", helixSessionID).
					Str("interaction_id", sctx.interaction.ID).
					Msg("Failed to publish final corrected entry patches to frontend")
			} else {
				log.Info().
					Str("session_id", helixSessionID).
					Str("interaction_id", sctx.interaction.ID).
					Msg("📦 [FLUSH] Published final corrected entry patches to frontend before completion")
			}
		}
	}

	// Extract structured entries from the accumulator before it's destroyed.
	// These preserve the type (text vs tool_call) and ordering of each message_id.
	if sctx.accumulator != nil {
		return sctx.accumulator.Entries()
	}
	return nil
}

// handleMessageUpdated processes message updates from external agent
func (apiServer *HelixAPIServer) handleMessageUpdated(sessionID string, syncMsg *types.SyncMessage) error {
	// TODO: Handle message updates (e.g., editing)
	log.Debug().Str("session_id", sessionID).Msg("Message updated")
	return nil
}

// handleContextTitleChanged processes context title changes
func (apiServer *HelixAPIServer) handleContextTitleChanged(sessionID string, syncMsg *types.SyncMessage) error {
	// TODO: Update context title in Helix
	log.Debug().Str("session_id", sessionID).Msg("Context title changed")
	return nil
}

// sendChatMessageToExternalAgent is the canonical function for sending a message
// to an external agent. It creates a waiting interaction, enqueues it for response
// routing, and sends the WebSocket command. All callers that need to send a message
// to an agent should use this function.
func (apiServer *HelixAPIServer) sendChatMessageToExternalAgent(sessionID, message, requestID string) (interactionID string, err error) {
	ctx := context.Background()

	// Look up the session to get its ZedThreadID and agent name
	var acpThreadID interface{} = nil
	var agentName string
	session, err := apiServer.Controller.Options.Store.GetSession(ctx, sessionID)
	if err == nil && session != nil {
		agentName = apiServer.getAgentNameForSession(ctx, session)
		if session.Metadata.ZedThreadID != "" {
			acpThreadID = session.Metadata.ZedThreadID
		}
	}

	// Create a waiting interaction so handleMessageCompleted can find it.
	// Each message gets its own interaction to properly track the conversation.
	if session != nil {
		interaction := &types.Interaction{
			Created:       time.Now(),
			Updated:       time.Now(),
			SessionID:     sessionID,
			UserID:        session.Owner,
			GenerationID:  session.GenerationID,
			Mode:          types.SessionModeInference,
			PromptMessage: message,
			State:         types.InteractionStateWaiting,
		}

		createdInteraction, createErr := apiServer.Controller.Options.Store.CreateInteraction(ctx, interaction)
		if createErr != nil {
			log.Warn().Err(createErr).Str("session_id", sessionID).Msg("Failed to create interaction for chat message")
		} else {
			interactionID = createdInteraction.ID
			// Notify frontend immediately so the chat updates without waiting for poll
			apiServer.publishInteractionUpdateToFrontend(sessionID, session.Owner, createdInteraction)
			apiServer.contextMappingsMutex.Lock()
			if apiServer.sessionToWaitingInteraction == nil {
				apiServer.sessionToWaitingInteraction = make(map[string][]string)
			}
			apiServer.sessionToWaitingInteraction[sessionID] = append(
				apiServer.sessionToWaitingInteraction[sessionID], interactionID)
			apiServer.contextMappingsMutex.Unlock()
		}

		// Update session timestamp so findConnectedSessionForSpecTask
		// picks the most recently active session.
		_ = apiServer.Controller.Options.Store.TouchSession(ctx, sessionID)
	}

	command := types.ExternalAgentCommand{
		Type: "chat_message",
		Data: map[string]interface{}{
			"message":       message,
			"request_id":    requestID,
			"acp_thread_id": acpThreadID, // Use existing thread if available, nil = create new
			"agent_name":    agentName,   // Which agent to use (e.g., "claude", "qwen", "zed-agent")
		},
	}

	err = apiServer.sendCommandToExternalAgent(sessionID, command)
	return interactionID, err
}

// sendCommandToExternalAgent sends a command to the external agent
func (apiServer *HelixAPIServer) sendCommandToExternalAgent(sessionID string, command types.ExternalAgentCommand) error {
	// Add session_id to the command data for context
	if command.Data == nil {
		command.Data = make(map[string]interface{})
	}
	command.Data["session_id"] = sessionID

	// Get the WebSocket connection for this session
	wsConn, exists := apiServer.externalAgentWSManager.getConnection(sessionID)
	if !exists || wsConn == nil {
		return fmt.Errorf("no WebSocket connection found for session %s", sessionID)
	}

	// Send command to the specific Zed agent
	select {
	case wsConn.SendChan <- command:
		log.Trace().
			Str("session_id", sessionID).
			Str("command_type", command.Type).
			Msg("Sent command to specific external Zed agent")

		return nil
	default:
		return fmt.Errorf("external agent send channel full for session %s", sessionID)
	}
}

// registerConnection registers a new external agent connection
func (manager *ExternalAgentWSManager) registerConnection(sessionID string, conn *ExternalAgentWSConnection) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	manager.connections[sessionID] = conn
	log.Trace().
		Str("session_id", sessionID).
		Int("total_connections", len(manager.connections)).
		Msg("[HELIX] Registered external agent connection")
}

// unregisterConnection unregisters an external agent connection
func (manager *ExternalAgentWSManager) unregisterConnection(sessionID string) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if conn, exists := manager.connections[sessionID]; exists {
		close(conn.SendChan)
		delete(manager.connections, sessionID)
	}
}

// getConnection gets an external agent connection
func (manager *ExternalAgentWSManager) getConnection(sessionID string) (*ExternalAgentWSConnection, bool) {
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	conn, exists := manager.connections[sessionID]
	return conn, exists
}

// listConnections returns all active connections
func (manager *ExternalAgentWSManager) listConnections() []types.ExternalAgentConnection {
	manager.mu.RLock()
	defer manager.mu.RUnlock()

	connections := make([]types.ExternalAgentConnection, 0, len(manager.connections))
	for sessionID, conn := range manager.connections {
		connections = append(connections, types.ExternalAgentConnection{
			SessionID:   sessionID,
			ConnectedAt: conn.ConnectedAt,
			LastPing:    conn.LastPing,
			Status:      "connected",
		})
	}
	return connections
}

// Session Readiness Management
// These methods track whether the agent in a session is ready to receive messages.
// This prevents race conditions where we send prompts before Zed has loaded the agent.

// initReadinessState initializes readiness tracking for a session
// Returns a callback that should be called when agent_ready is received (or on timeout)
func (manager *ExternalAgentWSManager) initReadinessState(sessionID string, needsContinue bool, onReady func()) {
	manager.readinessMu.Lock()
	defer manager.readinessMu.Unlock()

	// Clean up any existing state for this session
	if existing, exists := manager.readinessState[sessionID]; exists && existing.TimeoutTimer != nil {
		existing.TimeoutTimer.Stop()
	}

	state := &SessionReadinessState{
		IsReady:       false,
		SessionID:     sessionID,
		PendingQueue:  make([]types.ExternalAgentCommand, 0),
		NeedsContinue: needsContinue,
	}

	// Set up fallback timeout (60 seconds)
	// If we don't receive agent_ready within 60s, assume ready and send anyway
	state.TimeoutTimer = time.AfterFunc(60*time.Second, func() {
		log.Warn().
			Str("session_id", sessionID).
			Msg("⏰ [READINESS] Timeout waiting for agent_ready, proceeding with queued messages")
		manager.markSessionReady(sessionID, onReady)
	})

	manager.readinessState[sessionID] = state

	log.Trace().
		Str("session_id", sessionID).
		Bool("needs_continue", needsContinue).
		Msg("[READINESS] Initialized readiness tracking for session")
}

// markSessionReady marks a session as ready and flushes pending messages
func (manager *ExternalAgentWSManager) markSessionReady(sessionID string, onReady func()) {
	manager.readinessMu.Lock()

	state, exists := manager.readinessState[sessionID]
	if !exists {
		manager.readinessMu.Unlock()
		log.Debug().Str("session_id", sessionID).Msg("No readiness state found for session")
		return
	}

	if state.IsReady {
		manager.readinessMu.Unlock()
		log.Debug().Str("session_id", sessionID).Msg("Session already marked as ready")
		return
	}

	// Stop the timeout timer
	if state.TimeoutTimer != nil {
		state.TimeoutTimer.Stop()
	}

	state.IsReady = true
	state.ReadyAt = time.Now()
	pendingQueue := state.PendingQueue
	state.PendingQueue = nil // Clear the queue

	manager.readinessMu.Unlock()

	log.Trace().
		Str("session_id", sessionID).
		Int("pending_count", len(pendingQueue)).
		Msg("[READINESS] Session marked as ready, flushing pending messages")

	// Flush pending messages (after releasing lock to avoid deadlock)
	if len(pendingQueue) > 0 {
		conn, exists := manager.getConnection(sessionID)
		if exists {
			for _, cmd := range pendingQueue {
				select {
				case conn.SendChan <- cmd:
					log.Debug().
						Str("session_id", sessionID).
						Str("type", cmd.Type).
						Msg("📤 [READINESS] Sent queued message")
				default:
					log.Warn().
						Str("session_id", sessionID).
						Str("type", cmd.Type).
						Msg("⚠️ [READINESS] SendChan full, dropped queued message")
				}
			}
		}
	}

	// Call the onReady callback (e.g., to send continue prompt)
	if onReady != nil {
		onReady()
	}
}

// isSessionReady checks if a session is ready to receive messages
func (manager *ExternalAgentWSManager) isSessionReady(sessionID string) bool {
	manager.readinessMu.RLock()
	defer manager.readinessMu.RUnlock()

	state, exists := manager.readinessState[sessionID]
	if !exists {
		// No readiness tracking = assume ready (for backward compatibility)
		return true
	}
	return state.IsReady
}

// queueOrSend queues a command if session isn't ready, or sends immediately if ready
func (manager *ExternalAgentWSManager) queueOrSend(sessionID string, cmd types.ExternalAgentCommand) bool {
	manager.readinessMu.Lock()
	state, exists := manager.readinessState[sessionID]
	if !exists || state.IsReady {
		manager.readinessMu.Unlock()
		// Session is ready or not tracked - send immediately
		conn, connExists := manager.getConnection(sessionID)
		if !connExists {
			log.Warn().Str("session_id", sessionID).Msg("No connection found for session")
			return false
		}
		select {
		case conn.SendChan <- cmd:
			return true
		default:
			log.Warn().Str("session_id", sessionID).Msg("SendChan full, could not send command")
			return false
		}
	}

	// Session not ready - queue the message
	state.PendingQueue = append(state.PendingQueue, cmd)
	manager.readinessMu.Unlock()

	log.Debug().
		Str("session_id", sessionID).
		Str("type", cmd.Type).
		Int("queue_size", len(state.PendingQueue)).
		Msg("📥 [READINESS] Queued message (waiting for agent_ready)")
	return true
}

// cleanupReadinessState removes readiness tracking for a session
func (manager *ExternalAgentWSManager) cleanupReadinessState(sessionID string) {
	manager.readinessMu.Lock()
	defer manager.readinessMu.Unlock()

	if state, exists := manager.readinessState[sessionID]; exists {
		if state.TimeoutTimer != nil {
			state.TimeoutTimer.Stop()
		}
		delete(manager.readinessState, sessionID)
		log.Debug().Str("session_id", sessionID).Msg("Cleaned up readiness state")
	}
}

// handleChatResponse processes complete chat response from external agent
func (apiServer *HelixAPIServer) handleChatResponse(sessionID string, syncMsg *types.SyncMessage) error {
	log.Info().
		Str("session_id", sessionID).
		Str("event_type", syncMsg.EventType).
		Interface("data", syncMsg.Data).
		Msg("🔵 [HELIX] RECEIVED CHAT_RESPONSE FROM EXTERNAL AGENT")
	requestID, ok := syncMsg.Data["request_id"].(string)
	if !ok {
		log.Warn().Str("session_id", sessionID).Msg("Chat response missing request_id")
		return nil
	}

	content, ok := syncMsg.Data["content"].(string)
	if !ok {
		log.Warn().Str("session_id", sessionID).Str("request_id", requestID).Msg("Chat response missing content")
		return nil
	}

	// Skip placeholder acknowledgment responses - they should not trigger completion
	if content == "🤖 Processing your request with AI... (Real response will follow via async system)" {
		log.Info().
			Str("session_id", sessionID).
			Str("request_id", requestID).
			Msg("Skipping placeholder acknowledgment response - waiting for real AI response")
		return nil
	}

	// CRITICAL FIX: Use the Helix Session ID from the message, not the Agent Session ID
	helixSessionID := syncMsg.SessionID
	log.Info().
		Str("agent_session_id", sessionID).
		Str("helix_session_id", helixSessionID).
		Str("request_id", requestID).
		Msg("🔧 [HELIX] USING HELIX SESSION ID FOR RESPONSE CHANNEL LOOKUP")

	// Handle response via legacy channel handling
	responseChan, doneChan, _, exists := apiServer.getResponseChannel(helixSessionID, requestID)
	if !exists {
		log.Warn().Str("session_id", sessionID).Str("request_id", requestID).Msg("No response channel found for request")
		return nil
	}

	log.Info().
		Str("session_id", sessionID).
		Str("request_id", requestID).
		Str("content", content).
		Msg("🔵 [HELIX] SENDING RESPONSE TO RESPONSE CHANNEL")

	// Send content as single chunk
	select {
	case responseChan <- content:
		log.Info().
			Str("session_id", sessionID).
			Str("request_id", requestID).
			Msg("✅ [HELIX] RESPONSE SENT TO CHANNEL SUCCESSFULLY")
	default:
		log.Warn().Str("session_id", sessionID).Str("request_id", requestID).Msg("Response channel full")
	}

	// Send completion signal
	select {
	case doneChan <- true:
	default:
		log.Warn().Str("session_id", sessionID).Str("request_id", requestID).Msg("Done channel full")
	}

	// Try to link agent response to design review comment (if this request came from a comment)
	go func() {
		if err := apiServer.linkAgentResponseToCommentByRequestID(context.Background(), requestID, content); err != nil {
			log.Debug().
				Err(err).
				Str("request_id", requestID).
				Msg("No design review comment linked to this request (this is normal for non-comment requests)")
		}
	}()

	return nil
}

// handleChatResponseChunk processes streaming chat response chunk from external agent
func (apiServer *HelixAPIServer) handleChatResponseChunk(sessionID string, syncMsg *types.SyncMessage) error {
	requestID, ok := syncMsg.Data["request_id"].(string)
	if !ok {
		log.Warn().Str("session_id", sessionID).Msg("Chat response chunk missing request_id")
		return nil
	}

	chunk, ok := syncMsg.Data["chunk"].(string)
	if !ok {
		log.Warn().Str("session_id", sessionID).Str("request_id", requestID).Msg("Chat response chunk missing chunk")
		return nil
	}

	// Handle response chunk via legacy channel handling
	responseChan, _, _, exists := apiServer.getResponseChannel(sessionID, requestID)
	if !exists {
		log.Warn().Str("session_id", sessionID).Str("request_id", requestID).Msg("No response channel found for chunk")
		return nil
	}

	// Send chunk
	select {
	case responseChan <- chunk:
	default:
		log.Warn().Str("session_id", sessionID).Str("request_id", requestID).Msg("Response channel full for chunk")
	}

	return nil
}

// handleChatResponseDone processes completion signal from external agent
func (apiServer *HelixAPIServer) handleChatResponseDone(sessionID string, syncMsg *types.SyncMessage) error {
	log.Info().
		Str("session_id", sessionID).
		Str("event_type", syncMsg.EventType).
		Interface("data", syncMsg.Data).
		Msg("🔵 [HELIX] RECEIVED CHAT_RESPONSE_DONE FROM EXTERNAL AGENT")
	requestID, ok := syncMsg.Data["request_id"].(string)
	if !ok {
		log.Warn().Str("session_id", sessionID).Msg("Chat response done missing request_id")
		return nil
	}

	// CRITICAL FIX: Use the Helix Session ID from the message, not the Agent Session ID
	helixSessionID := syncMsg.SessionID
	log.Info().
		Str("agent_session_id", sessionID).
		Str("helix_session_id", helixSessionID).
		Str("request_id", requestID).
		Msg("🔧 [HELIX] USING HELIX SESSION ID FOR DONE CHANNEL LOOKUP")

	// Handle response completion via legacy channel handling
	_, doneChan, _, exists := apiServer.getResponseChannel(helixSessionID, requestID)
	if !exists {
		log.Warn().Str("session_id", sessionID).Str("request_id", requestID).Msg("No response channel found for done signal")
		return nil
	}

	log.Info().
		Str("session_id", sessionID).
		Str("request_id", requestID).
		Msg("🔵 [HELIX] SENDING DONE SIGNAL TO DONE CHANNEL")

	// Send completion signal
	select {
	case doneChan <- true:
		log.Info().
			Str("session_id", sessionID).
			Str("request_id", requestID).
			Msg("✅ [HELIX] DONE SIGNAL SENT TO CHANNEL SUCCESSFULLY")
	default:
		log.Warn().Str("session_id", sessionID).Str("request_id", requestID).Msg("Done channel full")
	}

	return nil
}

// handleMessageCompleted marks the interaction as complete when AI finishes responding
func (apiServer *HelixAPIServer) handleMessageCompleted(sessionID string, syncMsg *types.SyncMessage) error {
	log.Info().
		Str("session_id", sessionID).
		Str("event_type", syncMsg.EventType).
		Interface("data", syncMsg.Data).
		Msg("🎯 [HELIX] RECEIVED MESSAGE_COMPLETED FROM EXTERNAL AGENT")

	// Extract acp_thread_id from the data
	acpThreadID, ok := syncMsg.Data["acp_thread_id"].(string)
	if !ok || acpThreadID == "" {
		return fmt.Errorf("missing acp_thread_id in message_completed data")
	}

	// Look up helix_session_id from context mapping
	apiServer.contextMappingsMutex.RLock()
	helixSessionID, ok := apiServer.contextMappings[acpThreadID]
	apiServer.contextMappingsMutex.RUnlock()
	if !ok {
		// FALLBACK: contextMappings may be empty after API restart
		// Try to find session by ZedThreadID in database
		log.Info().
			Str("acp_thread_id", acpThreadID).
			Msg("🔍 [HELIX] contextMappings miss in message_completed, attempting database fallback")

		foundSession, err := apiServer.findSessionByZedThreadID(context.Background(), acpThreadID)
		if err != nil || foundSession == nil {
			log.Warn().
				Str("acp_thread_id", acpThreadID).
				Msg("⚠️ [HELIX] No Helix session mapping found for this thread (database fallback failed) - skipping message_completed")
			return nil
		}
		helixSessionID = foundSession.ID
		// Restore the mapping for future messages
		apiServer.contextMappingsMutex.Lock()
		apiServer.contextMappings[acpThreadID] = helixSessionID
		apiServer.contextMappingsMutex.Unlock()
		log.Info().
			Str("acp_thread_id", acpThreadID).
			Str("helix_session_id", helixSessionID).
			Msg("✅ [HELIX] Found session via database fallback in message_completed, restored contextMappings")
	}

	log.Info().
		Str("acp_thread_id", acpThreadID).
		Str("helix_session_id", helixSessionID).
		Msg("✅ [HELIX] Found Helix session mapping for message_completed")

	// Flush any dirty streaming context to DB before clearing.
	// The throttled DB writes in handleMessageAdded may have left unflushed content.
	// Also extract the structured response entries (typed, ordered) from the accumulator
	// before it's destroyed — these are stored on the interaction for structured rendering.
	responseEntries := apiServer.flushAndClearStreamingContext(context.Background(), helixSessionID)

	// Get the session
	helixSession, err := apiServer.Controller.Options.Store.GetSession(context.Background(), helixSessionID)
	if err != nil {
		return fmt.Errorf("failed to get Helix session %s: %w", helixSessionID, err)
	}

	// Load interactions for this session (GetSession doesn't load them)
	interactions, _, err := apiServer.Controller.Options.Store.ListInteractions(context.Background(), &types.ListInteractionsQuery{
		SessionID:    helixSessionID,
		GenerationID: helixSession.GenerationID,
		PerPage:      1000,
	})
	if err != nil {
		return fmt.Errorf("failed to list interactions for session %s: %w", helixSessionID, err)
	}

	log.Info().
		Str("helix_session_id", helixSessionID).
		Int("interaction_count", len(interactions)).
		Msg("🔍 [DEBUG] Loaded interactions for message_completed")

	// PRIMARY: use the front of the FIFO queue — the interaction that was being processed.
	// Pop it now so the next message_completed will target the next queued interaction.
	// This fixes the off-by-one bug where sendMessageToSpecTaskAgent creates a second
	// waiting interaction while the first is still streaming.
	var targetInteractionID string
	apiServer.contextMappingsMutex.Lock()
	if q := apiServer.sessionToWaitingInteraction[helixSessionID]; len(q) > 0 {
		targetInteractionID = q[0]
		if len(q) == 1 {
			delete(apiServer.sessionToWaitingInteraction, helixSessionID)
		} else {
			apiServer.sessionToWaitingInteraction[helixSessionID] = q[1:]
		}
		log.Info().
			Str("helix_session_id", helixSessionID).
			Str("interaction_id", targetInteractionID).
			Int("remaining_queue", len(apiServer.sessionToWaitingInteraction[helixSessionID])).
			Msg("✅ [HELIX] Popped interaction from FIFO queue for message_completed")
	}
	apiServer.contextMappingsMutex.Unlock()

	// FALLBACK: after API restart the in-memory queue is lost — find most recent waiting interaction in DB
	if targetInteractionID == "" {
		for i := len(interactions) - 1; i >= 0; i-- {
			if interactions[i].State == types.InteractionStateWaiting {
				targetInteractionID = interactions[i].ID
				log.Info().
					Str("helix_session_id", helixSessionID).
					Str("interaction_id", interactions[i].ID).
					Msg("✅ [HELIX] Fallback: found waiting interaction via DB scan (queue was empty)")
				break
			}
		}
	}

	if targetInteractionID == "" {
		log.Warn().
			Str("helix_session_id", helixSessionID).
			Msg("⚠️ [HELIX] No waiting interaction found to mark as complete")
		return nil
	}

	// CRITICAL: Reload the interaction from database to get latest response_message
	// The message_added handler may have just updated it, so we need the freshest data
	targetInteraction, err := apiServer.Controller.Options.Store.GetInteraction(context.Background(), targetInteractionID)
	if err != nil {
		return fmt.Errorf("failed to reload interaction %s: %w", targetInteractionID, err)
	}

	log.Info().
		Str("helix_session_id", helixSessionID).
		Str("interaction_id", targetInteraction.ID).
		Int("response_length", len(targetInteraction.ResponseMessage)).
		Str("response_preview", targetInteraction.ResponseMessage).
		Str("state_before", string(targetInteraction.State)).
		Msg("🔄 [HELIX] Reloaded interaction with latest response content")

	// Warn if the response is suspiciously empty — likely indicates content was lost
	// during the streaming→flush→reload pipeline
	if targetInteraction.ResponseMessage == "" {
		log.Warn().
			Str("helix_session_id", helixSessionID).
			Str("interaction_id", targetInteraction.ID).
			Msg("⚠️ [HELIX] message_completed but response_message is EMPTY — content may have been lost during streaming flush")
	}

	// Mark the interaction as complete
	targetInteraction.State = types.InteractionStateComplete
	targetInteraction.Completed = time.Now()
	targetInteraction.Updated = time.Now()

	// Store structured response entries if available (from accumulator).
	// This preserves the type and ordering of each entry (text vs tool_call)
	// so the frontend can render them with the correct component in order.
	if len(responseEntries) > 0 {
		entriesJSON, err := json.Marshal(responseEntries)
		if err != nil {
			log.Error().Err(err).Msg("Failed to marshal response entries")
		} else {
			targetInteraction.ResponseEntries = entriesJSON
		}
	}

	_, err = apiServer.Controller.Options.Store.UpdateInteraction(context.Background(), targetInteraction)
	if err != nil {
		return fmt.Errorf("failed to update interaction %s: %w", targetInteraction.ID, err)
	}

	log.Info().
		Str("helix_session_id", helixSessionID).
		Str("interaction_id", targetInteraction.ID).
		Int("final_response_length", len(targetInteraction.ResponseMessage)).
		Str("final_state", string(targetInteraction.State)).
		Msg("✅ [HELIX] Marked interaction as complete")

	// Update session timestamp so findConnectedSessionForSpecTask
	// picks the most recently active session.
	_ = apiServer.Controller.Options.Store.TouchSession(context.Background(), helixSessionID)

	// Update SpecTaskZedThread activity if this is a spectask session
	if helixSession.Metadata.SpecTaskID != "" {
		go apiServer.updateSpecTaskZedThreadActivity(context.Background(), acpThreadID)

		// Emit attention event: agent interaction completed
		if apiServer.attentionService != nil {
			go func() {
				task, err := apiServer.Controller.Options.Store.GetSpecTask(context.Background(), helixSession.Metadata.SpecTaskID)
				if err != nil {
					log.Warn().Err(err).
						Str("spec_task_id", helixSession.Metadata.SpecTaskID).
						Msg("Failed to load spectask for attention event")
					return
				}
				_, err = apiServer.attentionService.EmitEvent(
					context.Background(),
					types.AttentionEventAgentInteractionCompleted,
					task,
					targetInteraction.ID, // qualifier = interaction ID for idempotency
					map[string]interface{}{
						"interaction_id": targetInteraction.ID,
						"session_id":     helixSessionID,
					},
				)
				if err != nil {
					log.Warn().Err(err).
						Str("spec_task_id", task.ID).
						Msg("Failed to emit agent_interaction_completed attention event")
				}
			}()
		}
	}

	// Extract request_id from message data for commenter notification
	// This needs to be done before publishing so we can pass it to publishSessionUpdateToFrontend
	messageRequestID, _ := syncMsg.Data["request_id"].(string)

	// FINALIZE COMMENT RESPONSE before notifying the frontend.
	// Running this synchronously (not in a goroutine) ensures comment.agent_response is
	// written to the DB before the frontend receives the completion event and refetches
	// the comment list. Previously this ran in a goroutine after the publish, causing a
	// race where the frontend refetch could beat finalization and see an empty
	// agent_response — eventually triggering the 2-minute timeout error message.
	if messageRequestID != "" {
		log.Info().
			Str("request_id", messageRequestID).
			Str("helix_session_id", helixSessionID).
			Msg("🎯 [HELIX] Using request_id from message_completed data to finalize comment")

		if err := apiServer.finalizeCommentResponse(context.Background(), messageRequestID); err != nil {
			log.Debug().
				Err(err).
				Str("request_id", messageRequestID).
				Msg("No comment found for request_id (this is normal for non-comment interactions)")
		} else {
			log.Info().
				Str("request_id", messageRequestID).
				Msg("✅ [HELIX] Finalized comment response via request_id from message data")
		}

		// Clean up requestToCommenterMapping now that response is complete
		if apiServer.requestToCommenterMapping != nil {
			delete(apiServer.requestToCommenterMapping, messageRequestID)
			log.Debug().Str("request_id", messageRequestID).Msg("🧹 [HELIX] Cleaned up requestToCommenterMapping")
		}
	} else {
		// FALLBACK: Session-based lookup (for agents that don't echo request_id)
		// This may fail if helixSessionID != planning_session_id, but we try anyway
		log.Debug().
			Str("helix_session_id", helixSessionID).
			Msg("No request_id in message_completed data, falling back to session-based lookup")

		pendingComment, err := apiServer.Store.GetPendingCommentByPlanningSessionID(context.Background(), helixSessionID)
		if err == nil && pendingComment != nil {
			requestID := pendingComment.RequestID
			commentID := pendingComment.ID

			if err := apiServer.finalizeCommentResponse(context.Background(), requestID); err != nil {
				log.Error().
					Err(err).
					Str("comment_id", commentID).
					Str("request_id", requestID).
					Msg("Failed to finalize comment response")
			} else {
				log.Info().
					Str("comment_id", commentID).
					Str("request_id", requestID).
					Msg("✅ [HELIX] Finalized comment response via session-based lookup (fallback)")
			}

			// Clean up requestToCommenterMapping now that response is complete
			if apiServer.requestToCommenterMapping != nil {
				delete(apiServer.requestToCommenterMapping, requestID)
				log.Debug().Str("request_id", requestID).Msg("🧹 [HELIX] Cleaned up requestToCommenterMapping (fallback path)")
			}
		} else {
			log.Debug().
				Str("session_id", helixSessionID).
				Msg("No pending design review comment to finalize for session (this is normal for non-comment interactions)")
		}
	}

	// CRITICAL: Publish completion through BOTH event channels:
	// 1. interaction_update — same channel used during streaming, ensures useLiveInteraction sees state=complete
	// 2. session_update — full session for React Query cache consistency
	// The frontend's session_update handler has rejection logic (interaction count checks)
	// that can silently drop events, so interaction_update is the reliable path.
	err = apiServer.publishInteractionUpdateToFrontend(helixSessionID, helixSession.Owner, targetInteraction, messageRequestID)
	if err != nil {
		log.Error().Err(err).
			Str("session_id", helixSessionID).
			Str("interaction_id", targetInteraction.ID).
			Msg("Failed to publish interaction completion update to frontend")
	}

	// Also publish full session update for cache consistency
	reloadedSession, err := apiServer.Controller.Options.Store.GetSession(context.Background(), helixSessionID)
	if err != nil {
		log.Error().Err(err).Str("session_id", helixSessionID).Msg("Failed to reload session for final publish")
	} else {
		allInteractions, _, err := apiServer.Controller.Options.Store.ListInteractions(context.Background(), &types.ListInteractionsQuery{
			SessionID:    helixSessionID,
			GenerationID: reloadedSession.GenerationID,
			PerPage:      1000,
		})
		if err == nil && len(allInteractions) > 0 {
			reloadedSession.Interactions = allInteractions
			log.Info().
				Str("session_id", helixSessionID).
				Int("interaction_count", len(allInteractions)).
				Int("last_interaction_response_len", len(allInteractions[len(allInteractions)-1].ResponseMessage)).
				Str("last_interaction_state", string(allInteractions[len(allInteractions)-1].State)).
				Msg("🔍 [DEBUG] Publishing final session update after message_completed")

			err = apiServer.publishSessionUpdateToFrontend(reloadedSession, targetInteraction, messageRequestID)
			if err != nil {
				log.Error().Err(err).
					Str("session_id", helixSessionID).
					Str("interaction_id", targetInteraction.ID).
					Str("request_id", messageRequestID).
					Msg("Failed to publish final session update to frontend")
			}
		}
	}

	// Signal the waiting HTTP handler that this interaction is complete.
	// This is needed for non-streaming (blocking) requests that call waitForExternalAgentResponse.
	// In the normal case, chat_response_done sends the signal. But for Stopped/cancelled turns,
	// Zed sends message_completed without chat_response_done, so we must signal here.
	// The doneChan is buffered(1) so a double-send (normal case) is harmless.
	if messageRequestID != "" {
		_, doneChan, _, exists := apiServer.getResponseChannel(helixSessionID, messageRequestID)
		if exists {
			select {
			case doneChan <- true:
				log.Debug().Str("request_id", messageRequestID).Msg("✅ [HELIX] Sent done signal from message_completed")
			default:
				log.Debug().Str("request_id", messageRequestID).Msg("Done channel already full (normal for streaming case)")
			}
		}
	}

	// Process next non-interrupt prompt from queue (if any)
	go apiServer.processPromptQueue(context.Background(), helixSessionID)

	return nil
}

// processPromptQueue checks for pending non-interrupt prompts and sends the next one
// This is called after a message is completed to process queued non-interrupt messages
func (apiServer *HelixAPIServer) processPromptQueue(ctx context.Context, sessionID string) {
	// Check if the session is busy (last interaction is waiting for a response).
	// This prevents sending a queue-mode prompt while Zed is already processing
	// a locally-submitted message. The check uses DB state which is race-free:
	// handleMessageAdded creates the interaction synchronously before returning.
	session, err := apiServer.Store.GetSession(ctx, sessionID)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to get session for queue processing")
		return
	}
	if session != nil {
		interactions, _, err := apiServer.Store.ListInteractions(ctx, &types.ListInteractionsQuery{
			SessionID:    sessionID,
			GenerationID: session.GenerationID,
			PerPage:      1,
		})
		if err != nil {
			log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to list interactions for queue processing")
			return
		}
		if len(interactions) > 0 && interactions[len(interactions)-1].State == types.InteractionStateWaiting {
			log.Info().
				Str("session_id", sessionID).
				Str("interaction_id", interactions[len(interactions)-1].ID).
				Msg("Session is busy (last interaction waiting), deferring queue-mode prompt")
			return
		}
	}

	// Get the next pending non-interrupt prompt for this session
	nextPrompt, err := apiServer.Store.GetNextPendingPrompt(ctx, sessionID)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to get next pending prompt")
		return
	}

	if nextPrompt == nil {
		log.Debug().Str("session_id", sessionID).Msg("No pending non-interrupt prompts in queue")
		return
	}

	isRetry := nextPrompt.Status == "failed"
	log.Info().
		Str("session_id", sessionID).
		Str("prompt_id", nextPrompt.ID).
		Str("content_preview", truncateString(nextPrompt.Content, 50)).
		Bool("is_retry", isRetry).
		Msg("📤 [QUEUE] Processing next non-interrupt prompt from queue")

	// The prompt was atomically claimed by GetNextPendingPrompt (status set to 'sending').
	// Send it to the session.
	err = apiServer.sendQueuedPromptToSession(ctx, sessionID, nextPrompt)
	if err != nil {
		log.Error().
			Err(err).
			Str("session_id", sessionID).
			Str("prompt_id", nextPrompt.ID).
			Msg("Failed to send queued prompt to session")

		// Mark as failed
		if markErr := apiServer.Store.MarkPromptAsFailed(ctx, nextPrompt.ID); markErr != nil {
			log.Error().Err(markErr).Str("prompt_id", nextPrompt.ID).Msg("Failed to mark prompt as failed")
		}
		return
	}

	// Mark as sent
	if err := apiServer.Store.MarkPromptAsSent(ctx, nextPrompt.ID); err != nil {
		log.Error().Err(err).Str("prompt_id", nextPrompt.ID).Msg("Failed to mark prompt as sent")
	}

	log.Info().
		Str("session_id", sessionID).
		Str("prompt_id", nextPrompt.ID).
		Msg("✅ [QUEUE] Successfully sent queued prompt to session")
}

// processAnyPendingPrompt checks for any pending prompt (interrupt or non-interrupt) and sends it
// This is used when the session is idle to process ALL pending prompts, not just non-interrupt ones
func (apiServer *HelixAPIServer) processAnyPendingPrompt(ctx context.Context, sessionID string) {
	// Get the next pending prompt (any type)
	nextPrompt, err := apiServer.Store.GetAnyPendingPrompt(ctx, sessionID)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to get any pending prompt")
		return
	}

	if nextPrompt == nil {
		log.Trace().Str("session_id", sessionID).Msg("No pending prompts in queue")
		return
	}

	isRetry := nextPrompt.Status == "failed"
	log.Info().
		Str("session_id", sessionID).
		Str("prompt_id", nextPrompt.ID).
		Str("content_preview", truncateString(nextPrompt.Content, 50)).
		Bool("interrupt", nextPrompt.Interrupt).
		Bool("is_retry", isRetry).
		Msg("📤 [QUEUE] Processing pending prompt")

	// GetAnyPendingPrompt already atomically claimed this prompt (set status='sending').
	// No additional ClaimPromptForSending call needed — that would fail because
	// the status is already 'sending', causing every queued prompt to be silently dropped.

	// Send the prompt to the session (creates interaction and sends to agent)
	if err := apiServer.sendQueuedPromptToSession(ctx, sessionID, nextPrompt); err != nil {
		// Interaction creation failed - revert to 'failed' so it can be retried
		log.Error().
			Err(err).
			Str("session_id", sessionID).
			Str("prompt_id", nextPrompt.ID).
			Msg("Failed to create interaction for pending prompt - reverting to failed")
		if markErr := apiServer.Store.MarkPromptAsFailed(ctx, nextPrompt.ID); markErr != nil {
			log.Error().Err(markErr).Str("prompt_id", nextPrompt.ID).Msg("Failed to mark prompt as failed after interaction creation error")
		}
		return
	}

	// Mark as fully sent after successful delivery
	if markErr := apiServer.Store.MarkPromptAsSent(ctx, nextPrompt.ID); markErr != nil {
		log.Warn().Err(markErr).Str("prompt_id", nextPrompt.ID).Msg("Failed to mark prompt as sent after delivery (non-fatal)")
	}

	log.Info().
		Str("session_id", sessionID).
		Str("prompt_id", nextPrompt.ID).
		Bool("interrupt", nextPrompt.Interrupt).
		Msg("✅ [QUEUE] Successfully processed pending prompt")
}

// sendQueuedPromptToSession sends a queued prompt to an external agent session
// CRITICAL: Creates an interaction BEFORE sending so that agent responses have somewhere to go
// NOTE: On the FIRST message, ZedThreadID will be empty - this triggers thread creation in Zed.
// The thread_created event will come back with the new thread ID, which we store via requestToSessionMapping.
func (apiServer *HelixAPIServer) sendQueuedPromptToSession(ctx context.Context, sessionID string, prompt *types.PromptHistoryEntry) error {
	// Get the session to retrieve the ZedThreadID and owner
	session, err := apiServer.Store.GetSession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("failed to get session: %w", err)
	}

	// CRITICAL: Create an interaction BEFORE sending the message
	// This ensures that when the agent responds, handleMessageAdded has an interaction to update
	interaction := &types.Interaction{
		ID:            "", // Will be generated
		Created:       time.Now(),
		Updated:       time.Now(),
		Scheduled:     time.Now(),
		SessionID:     sessionID,
		UserID:        session.Owner,
		GenerationID:  session.GenerationID, // Must match session's generation for query to find it
		Mode:          types.SessionModeInference,
		PromptMessage: prompt.Content,
		State:         types.InteractionStateWaiting,
	}

	createdInteraction, err := apiServer.Controller.Options.Store.CreateInteraction(ctx, interaction)
	if err != nil {
		return fmt.Errorf("failed to create interaction for queue prompt: %w", err)
	}

	log.Info().
		Str("session_id", sessionID).
		Str("interaction_id", createdInteraction.ID).
		Str("content_preview", truncateString(prompt.Content, 30)).
		Msg("✅ [QUEUE] Created interaction for queue prompt")

	// Notify frontend immediately so the chat updates without waiting for poll
	apiServer.publishInteractionUpdateToFrontend(sessionID, session.Owner, createdInteraction)

	// CRITICAL: Enqueue the interaction so handleMessageAdded routes the response correctly.
	// Using append (not overwrite) prevents the race where sendMessageToSpecTaskAgent
	// creates a second interaction while the first is still streaming, which would cause
	// the first interaction's streaming content to land in the second interaction (off-by-one bug).
	apiServer.contextMappingsMutex.Lock()
	if apiServer.sessionToWaitingInteraction == nil {
		apiServer.sessionToWaitingInteraction = make(map[string][]string)
	}
	apiServer.sessionToWaitingInteraction[sessionID] = append(apiServer.sessionToWaitingInteraction[sessionID], createdInteraction.ID)
	apiServer.contextMappingsMutex.Unlock()

	// Determine agent name
	agentName := apiServer.getAgentNameForSession(ctx, session)

	// Use interaction ID as request ID for better tracing
	requestID := createdInteraction.ID

	// CRITICAL: Store request_id->session mapping so thread_created can find the right session
	// This is needed for the FIRST message when ZedThreadID is empty and a new thread will be created
	apiServer.contextMappingsMutex.Lock()
	if apiServer.requestToSessionMapping == nil {
		apiServer.requestToSessionMapping = make(map[string]string)
	}
	apiServer.requestToSessionMapping[requestID] = sessionID
	apiServer.contextMappingsMutex.Unlock()
	log.Info().
		Str("request_id", requestID).
		Str("session_id", sessionID).
		Msg("🔗 [QUEUE] Stored request_id->session mapping for thread creation")

	// Create the command to send to the external agent
	// NOTE: acp_thread_id can be empty on first message - this triggers thread creation in Zed
	command := types.ExternalAgentCommand{
		Type: "chat_message",
		Data: map[string]interface{}{
			"acp_thread_id": session.Metadata.ZedThreadID, // Empty on first message triggers thread creation
			"message":       prompt.Content,
			"request_id":    requestID,
			"agent_name":    agentName,
			"from_queue":    true, // Indicate this came from the queue
		},
	}

	log.Info().
		Str("session_id", sessionID).
		Str("request_id", requestID).
		Str("interaction_id", createdInteraction.ID).
		Str("acp_thread_id", session.Metadata.ZedThreadID).
		Bool("first_message", session.Metadata.ZedThreadID == "").
		Str("content_preview", truncateString(prompt.Content, 30)).
		Msg("📤 [QUEUE] Sending queued prompt via sendCommandToExternalAgent")

	// Use the unified sendCommandToExternalAgent which handles connection lookup,
	// adds session_id to data, and updates agent work state
	//
	// IMPORTANT: We don't return error if sending to agent fails, because the
	// interaction was already created. The queue's job is to persist the message
	// to the backend - which succeeded. Agent send failures are logged but don't
	// affect the prompt status. The user will see the interaction in the session
	// and can retry if needed.
	if err := apiServer.sendCommandToExternalAgent(sessionID, command); err != nil {
		log.Warn().Err(err).
			Str("session_id", sessionID).
			Str("interaction_id", createdInteraction.ID).
			Str("prompt_id", prompt.ID).
			Msg("❌ [QUEUE] Failed to send to agent — auto-starting desktop if stopped")

		// Auto-start the desktop if the session belongs to a spec task.
		// The interaction is in "waiting" state; pickupWaitingInteraction will
		// deliver it when the agent reconnects via WebSocket.
		if session.Metadata.SpecTaskID != "" {
			specTask, taskErr := apiServer.Controller.Options.Store.GetSpecTask(ctx, session.Metadata.SpecTaskID)
			if taskErr != nil {
				log.Error().Err(taskErr).Str("spec_task_id", session.Metadata.SpecTaskID).Msg("Failed to load spec task for desktop auto-start")
			} else {
				go func() {
					if startErr := apiServer.startDesktopForSpecTask(context.Background(), specTask); startErr != nil {
						log.Error().Err(startErr).Str("spec_task_id", session.Metadata.SpecTaskID).Msg("Failed to auto-start desktop from prompt queue")
					}
				}()
			}
		}
	}

	return nil
}

// truncateString truncates a string to maxLen characters
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// handleThreadLoadError handles thread load failures from Zed
// This happens when Zed tries to load an existing thread but fails (e.g., session already active via UI)
// We need to treat this like a completion so the UI clears the text box and shows an error
func (apiServer *HelixAPIServer) handleThreadLoadError(sessionID string, syncMsg *types.SyncMessage) error {
	log.Warn().
		Str("session_id", sessionID).
		Str("event_type", syncMsg.EventType).
		Interface("data", syncMsg.Data).
		Msg("⚠️ [HELIX] RECEIVED THREAD_LOAD_ERROR FROM EXTERNAL AGENT")

	// Extract error details
	acpThreadID, _ := syncMsg.Data["acp_thread_id"].(string)
	requestID, _ := syncMsg.Data["request_id"].(string)
	errorMsg, _ := syncMsg.Data["error"].(string)

	log.Error().
		Str("acp_thread_id", acpThreadID).
		Str("request_id", requestID).
		Str("error", errorMsg).
		Msg("❌ [HELIX] Thread load failed in Zed - session may be active via UI click")

	// Look up helix_session_id from context mapping (if thread was previously mapped)
	var helixSessionID string
	if acpThreadID != "" {
		apiServer.contextMappingsMutex.RLock()
		helixSessionID = apiServer.contextMappings[acpThreadID]
		apiServer.contextMappingsMutex.RUnlock()
	}

	// If we have a request_id, try to send error to the done channel
	// This allows the HTTP streaming to complete with an error message
	if requestID != "" {
		lookupSessionID := helixSessionID
		if lookupSessionID == "" {
			// Fall back to the WebSocket session ID
			lookupSessionID = sessionID
		}

		_, doneChan, errorChan, exists := apiServer.getResponseChannel(lookupSessionID, requestID)
		if exists {
			// Send error message
			if errorChan != nil {
				select {
				case errorChan <- fmt.Errorf("thread load failed: %s", errorMsg):
					log.Info().
						Str("request_id", requestID).
						Msg("✅ [HELIX] Sent error to error channel")
				default:
					log.Debug().Msg("Error channel full")
				}
			}

			// Send completion signal so UI clears
			if doneChan != nil {
				select {
				case doneChan <- true:
					log.Info().
						Str("request_id", requestID).
						Msg("✅ [HELIX] Sent done signal (after error)")
				default:
					log.Debug().Msg("Done channel full")
				}
			}
		}
	}

	// If we have a helix session, update the interaction to show error
	if helixSessionID != "" {
		helixSession, err := apiServer.Controller.Options.Store.GetSession(context.Background(), helixSessionID)
		if err == nil && helixSession != nil {
			// Find the waiting interaction and mark it with error
			interactions, _, err := apiServer.Controller.Options.Store.ListInteractions(context.Background(), &types.ListInteractionsQuery{
				SessionID:    helixSessionID,
				GenerationID: helixSession.GenerationID,
				PerPage:      1000,
			})
			if err == nil {
				for i := len(interactions) - 1; i >= 0; i-- {
					if interactions[i].State == types.InteractionStateWaiting {
						interactions[i].State = types.InteractionStateError
						interactions[i].Error = fmt.Sprintf("Thread load failed: %s", errorMsg)
						interactions[i].Updated = time.Now()
						interactions[i].Completed = time.Now()
						apiServer.Controller.Options.Store.UpdateInteraction(context.Background(), interactions[i])

						log.Info().
							Str("helix_session_id", helixSessionID).
							Str("interaction_id", interactions[i].ID).
							Msg("✅ [HELIX] Marked interaction as error due to thread load failure")
						break
					}
				}
			}
		}
	}

	return nil
}

// handleChatResponseError processes error from external agent
func (apiServer *HelixAPIServer) handleChatResponseError(sessionID string, syncMsg *types.SyncMessage) error {
	requestID, ok := syncMsg.Data["request_id"].(string)
	if !ok {
		log.Warn().Str("session_id", sessionID).Msg("Chat response error missing request_id")
		return nil
	}

	errorMsg, ok := syncMsg.Data["error"].(string)
	if !ok {
		errorMsg = "Unknown error from external agent"
	}

	// Handle response error via legacy channel handling
	_, _, errorChan, exists := apiServer.getResponseChannel(sessionID, requestID)
	if !exists {
		log.Warn().Str("session_id", sessionID).Str("request_id", requestID).Msg("No response channel found for error")
		return nil
	}

	// Send error
	select {
	case errorChan <- fmt.Errorf("%s", errorMsg):
	default:
		log.Warn().Str("session_id", sessionID).Str("request_id", requestID).Msg("Error channel full")
	}

	return nil
}

// handleAgentReady processes the agent_ready event from Zed
// This is sent when the agent (e.g., qwen-code) has finished initialization and is ready for prompts
func (apiServer *HelixAPIServer) handleAgentReady(sessionID string, syncMsg *types.SyncMessage) error {
	log.Trace().
		Str("session_id", sessionID).
		Interface("data", syncMsg.Data).
		Msg("[READINESS] Received agent_ready event from Zed")

	// Extract optional metadata from the ready event
	agentName, _ := syncMsg.Data["agent_name"].(string)
	threadID, _ := syncMsg.Data["thread_id"].(string)

	// Mark the session as ready, which will:
	// 1. Flush any queued messages
	// 2. Trigger the onReady callback (which sends continue prompt if needed)
	apiServer.externalAgentWSManager.readinessMu.RLock()
	state, exists := apiServer.externalAgentWSManager.readinessState[sessionID]
	apiServer.externalAgentWSManager.readinessMu.RUnlock()

	if !exists {
		// No readiness tracking for this session - that's fine, just log
		log.Debug().
			Str("session_id", sessionID).
			Msg("No readiness state found for session (may be already ready or legacy connection)")
		return nil
	}

	// Get the connection for sending continue prompt
	wsConn, connExists := apiServer.externalAgentWSManager.getConnection(sessionID)

	// Create the onReady callback that will send the continue prompt
	var onReadyCallback func()
	if state.NeedsContinue && connExists {
		onReadyCallback = func() {
			log.Info().
				Str("session_id", sessionID).
				Str("agent_name", agentName).
				Str("thread_id", threadID).
				Msg("🔄 [READINESS] Agent ready, now sending continue prompt")
			apiServer.sendContinuePromptIfNeeded(context.Background(), sessionID, wsConn)
		}
	}

	// Mark as ready (this flushes queued messages and calls onReady)
	apiServer.externalAgentWSManager.markSessionReady(sessionID, onReadyCallback)

	// After container/API restart, Zed reconnects and sends agent_ready with thread_id=null.
	// At this point, Zed has lost its thread event subscriptions (SUBSCRIBED_THREADS static
	// is cleared on process restart). Without a subscription, messages the user types directly
	// into Zed's agent panel won't sync back to Helix over the WebSocket.
	//
	// Fix: if the session has an existing ZedThreadID and the agent_ready came without a
	// thread_id (meaning Zed reconnected fresh), send an open_thread command so Zed loads
	// the thread and re-establishes its event subscription via ensure_thread_subscription().
	if threadID == "" && connExists {
		helixSession, err := apiServer.Controller.Options.Store.GetSession(context.Background(), sessionID)
		if err == nil && helixSession != nil && helixSession.Metadata.ZedThreadID != "" {
			// Use the latest thread for this spectask (not necessarily the one stored
			// on this session). When the user creates new threads in Zed, the latest
			// thread is the one they're working in — reopening the original thread
			// would jump them back to stale context.
			targetThreadID := helixSession.Metadata.ZedThreadID
			if helixSession.Metadata.SpecTaskID != "" {
				latestThreadID := apiServer.findLatestZedThreadForSpecTask(context.Background(), helixSession.Metadata.SpecTaskID)
				if latestThreadID != "" {
					targetThreadID = latestThreadID
				}
			}

			agentNameForOpen := apiServer.getAgentNameForSession(context.Background(), helixSession)
			log.Trace().
				Str("session_id", sessionID).
				Str("zed_thread_id", targetThreadID).
				Str("agent_name", agentNameForOpen).
				Msg("[READINESS] Sending open_thread to re-establish Zed subscription after reconnect")
			if err := apiServer.sendOpenThreadCommand(sessionID, targetThreadID, agentNameForOpen); err != nil {
				log.Warn().
					Str("session_id", sessionID).
					Err(err).
					Msg("⚠️ [READINESS] Failed to send open_thread for reconnect subscription recovery")
			}
		}
	}

	// Process any pending prompts (including interrupt=true ones)
	// When agent is ready/idle, we should process ALL pending prompts, not just non-interrupt ones
	go apiServer.processAnyPendingPrompt(context.Background(), sessionID)

	return nil
}

// findLatestZedThreadForSpecTask returns the most recently active Zed thread ID
// for a spectask. Used on reconnect to open the user's current thread rather than
// the original one.
func (apiServer *HelixAPIServer) findLatestZedThreadForSpecTask(ctx context.Context, specTaskID string) string {
	workSessions, err := apiServer.Controller.Options.Store.ListSpecTaskWorkSessions(ctx, specTaskID)
	if err != nil || len(workSessions) == 0 {
		return ""
	}

	// Find the work session with the most recent activity
	var latestThread string
	var latestTime time.Time
	for _, ws := range workSessions {
		session, err := apiServer.Controller.Options.Store.GetSession(ctx, ws.HelixSessionID)
		if err != nil || session == nil {
			continue
		}
		if session.Metadata.ZedThreadID != "" && session.Updated.After(latestTime) {
			latestTime = session.Updated
			latestThread = session.Metadata.ZedThreadID
		}
	}
	return latestThread
}

// findSessionByZedThreadID finds a session by its ZedThreadID metadata
// This is a database fallback when contextMappings is empty (e.g., after API restart)
func (apiServer *HelixAPIServer) findSessionByZedThreadID(ctx context.Context, zedThreadID string) (*types.Session, error) {
	// Query sessions with matching ZedThreadID in metadata
	// The ZedThreadID is stored in session.Metadata.ZedThreadID
	// For now, we iterate through recent sessions (this could be optimized with a DB index on metadata)
	sessions, _, err := apiServer.Controller.Options.Store.ListSessions(ctx, store.ListSessionsQuery{
		PerPage: 100,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list sessions: %w", err)
	}

	for _, session := range sessions {
		if session.Metadata.ZedThreadID == zedThreadID {
			log.Info().
				Str("session_id", session.ID).
				Str("zed_thread_id", zedThreadID).
				Msg("🔍 [HELIX] Found session by ZedThreadID in database")
			return session, nil
		}
	}

	return nil, fmt.Errorf("no session found with ZedThreadID: %s", zedThreadID)
}

// validateExternalAgentToken validates the auth token for external agent
func (apiServer *HelixAPIServer) validateExternalAgentToken(sessionID, token string) bool {
	// TODO: Implement proper token validation
	// For now, just check if token is not empty and session exists
	if token == "" {
		return false
	}

	// TODO: Check if session exists in store
	// TODO: Validate token against stored session tokens
	// TODO: Check token expiration

	return true // Placeholder - always valid for now
}

// generateExternalAgentToken generates an auth token for external agent
func (apiServer *HelixAPIServer) generateExternalAgentToken(sessionID string) (string, error) {
	// TODO: Generate secure token and store it
	// For now, return a simple token
	token := fmt.Sprintf("ext-agent-%s-%d", sessionID, time.Now().Unix())

	// TODO: Store token with expiration in database/cache
	log.Debug().Str("session_id", sessionID).Str("token", token).Msg("Generated external agent token")

	return token, nil
}

// publishSessionUpdateToFrontend publishes a session update to the frontend via pubsub
// If requestID is provided, it will also publish to the commenter's queue (for design review streaming)
func (apiServer *HelixAPIServer) publishSessionUpdateToFrontend(session *types.Session, interaction *types.Interaction, requestID ...string) error {
	// Create websocket event for frontend
	event := &types.WebsocketEvent{
		Type:          types.WebsocketEventSessionUpdate,
		SessionID:     session.ID,
		InteractionID: interaction.ID,
		Owner:         session.Owner,
		Session:       session,
	}

	// Marshal to JSON
	messageBytes, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal websocket event: %w", err)
	}

	// Publish to session owner's queue
	err = apiServer.pubsub.Publish(context.Background(), pubsub.GetSessionQueue(session.Owner, session.ID), messageBytes)
	if err != nil {
		return fmt.Errorf("failed to publish to pubsub: %w", err)
	}

	log.Info().
		Str("session_id", session.ID).
		Str("interaction_id", interaction.ID).
		Str("owner", session.Owner).
		Msg("📤 [HELIX] Published session update to frontend (owner)")

	// If requestID is provided, check if there's a commenter who should also receive the update
	// This handles the case where the design review commenter is different from the session owner
	if len(requestID) > 0 && requestID[0] != "" {
		if apiServer.requestToCommenterMapping != nil {
			if commenterID, exists := apiServer.requestToCommenterMapping[requestID[0]]; exists && commenterID != session.Owner {
				// Publish to commenter's queue as well
				err = apiServer.pubsub.Publish(context.Background(), pubsub.GetSessionQueue(commenterID, session.ID), messageBytes)
				if err != nil {
					log.Warn().
						Err(err).
						Str("session_id", session.ID).
						Str("commenter_id", commenterID).
						Msg("Failed to publish session update to commenter")
				} else {
					log.Info().
						Str("session_id", session.ID).
						Str("interaction_id", interaction.ID).
						Str("commenter_id", commenterID).
						Msg("📤 [HELIX] Published session update to commenter")
				}
			}
		}
	}

	return nil
}

// publishInteractionUpdateToFrontend sends only the updated interaction to the frontend.
// This is an optimization over publishSessionUpdateToFrontend - instead of sending the full
// session with all interactions (O(n) data), we send just the single updated interaction (O(1)).
// This dramatically reduces WebSocket traffic during streaming updates.
func (apiServer *HelixAPIServer) publishInteractionUpdateToFrontend(sessionID, owner string, interaction *types.Interaction, requestID ...string) error {
	// Create websocket event with just the interaction, not the full session
	event := &types.WebsocketEvent{
		Type:          types.WebsocketEventInteractionUpdate,
		SessionID:     sessionID,
		InteractionID: interaction.ID,
		Owner:         owner,
		Interaction:   interaction,
	}

	// Marshal to JSON
	messageBytes, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal websocket event: %w", err)
	}

	// Publish to session owner's queue
	err = apiServer.pubsub.Publish(context.Background(), pubsub.GetSessionQueue(owner, sessionID), messageBytes)
	if err != nil {
		return fmt.Errorf("failed to publish to pubsub: %w", err)
	}

	log.Debug().
		Str("session_id", sessionID).
		Str("interaction_id", interaction.ID).
		Str("owner", owner).
		Int("response_len", len(interaction.ResponseMessage)).
		Msg("📤 [HELIX] Published interaction update to frontend (optimized)")

	// If requestID is provided, check if there's a commenter who should also receive the update
	if len(requestID) > 0 && requestID[0] != "" {
		if apiServer.requestToCommenterMapping != nil {
			if commenterID, exists := apiServer.requestToCommenterMapping[requestID[0]]; exists && commenterID != owner {
				// Publish to commenter's queue as well
				err = apiServer.pubsub.Publish(context.Background(), pubsub.GetSessionQueue(commenterID, sessionID), messageBytes)
				if err != nil {
					log.Warn().
						Err(err).
						Str("session_id", sessionID).
						Str("commenter_id", commenterID).
						Msg("Failed to publish interaction update to commenter")
				} else {
					log.Debug().
						Str("session_id", sessionID).
						Str("interaction_id", interaction.ID).
						Str("commenter_id", commenterID).
						Msg("📤 [HELIX] Published interaction update to commenter")
				}
			}
		}
	}

	return nil
}

// utf16RuneLen returns the number of UTF-16 code units needed to encode the rune.
// BMP characters (U+0000 to U+FFFF) use 1 code unit; supplementary characters use 2.
func utf16RuneLen(r rune) int {
	if r >= 0x10000 {
		return 2
	}
	return 1
}

// utf16Len returns the number of UTF-16 code units in s.
// This matches JavaScript's string.length property.
func utf16Len(s string) int {
	n := 0
	for _, r := range s {
		n += utf16RuneLen(r)
	}
	return n
}

// computePatch computes the minimal patch between previousContent and newContent.
// Returns (patchOffset, patch, totalLength) where offsets are in UTF-16 code units
// to match JavaScript's string.slice() behavior.
// The caller can reconstruct newContent as:
//
//	newContent = previousContent.slice(0, patchOffset) + patch   (in JS)
//
// Fast path: if newContent starts with previousContent, patchOffset = utf16Len(previousContent).
func computePatch(previousContent, newContent string) (patchOffset int, patch string, totalLength int) {
	totalLength = utf16Len(newContent)

	// Fast path: pure append (99% of streaming tokens)
	if len(newContent) >= len(previousContent) && newContent[:len(previousContent)] == previousContent {
		return utf16Len(previousContent), newContent[len(previousContent):], totalLength
	}

	// Slow path: find first differing rune, tracking both byte and UTF-16 positions.
	// We iterate by rune (not byte) to avoid splitting multi-byte characters, and
	// return the UTF-16 code unit offset so JavaScript can apply the patch correctly.
	utf16Off := 0
	byteOff := 0
	prevLen := len(previousContent)
	newLen := len(newContent)

	for byteOff < prevLen && byteOff < newLen {
		prevRune, prevSize := utf8.DecodeRuneInString(previousContent[byteOff:])
		newRune, newSize := utf8.DecodeRuneInString(newContent[byteOff:])
		if prevRune != newRune || prevSize != newSize {
			break
		}
		utf16Off += utf16RuneLen(newRune)
		byteOff += newSize
	}

	return utf16Off, newContent[byteOff:], totalLength
}

// publishEntryPatchesToFrontend sends per-entry delta patches for structured streaming.
// Each entry gets its own string patch (offset/patch/length) so unchanged entries cost
// zero bytes on the wire. The frontend maintains a ResponseEntry[] and applies patches
// per-entry to reconstruct content with correct type boundaries (text vs tool_call).
//
// If commenterID is provided, also publishes to the commenter's queue (for design review).
func (apiServer *HelixAPIServer) publishEntryPatchesToFrontend(
	sessionID, owner, interactionID string,
	previousEntries []wsprotocol.ResponseEntry,
	currentEntries []wsprotocol.ResponseEntry,
	commenterID ...string,
) error {
	if len(currentEntries) == 0 {
		return nil
	}

	event := &types.WebsocketEvent{
		Type:          types.WebsocketEventInteractionPatch,
		SessionID:     sessionID,
		InteractionID: interactionID,
		Owner:         owner,
		EntryCount:    len(currentEntries),
	}

	var entryPatches []types.EntryPatch
	for i, entry := range currentEntries {
		var prevContent string
		if i < len(previousEntries) {
			prevContent = previousEntries[i].Content
		}
		// Skip entries that haven't changed at all
		if i < len(previousEntries) &&
			prevContent == entry.Content &&
			previousEntries[i].Type == entry.Type &&
			previousEntries[i].MessageID == entry.MessageID &&
			previousEntries[i].ToolName == entry.ToolName &&
			previousEntries[i].ToolStatus == entry.ToolStatus {
			continue
		}
		epOffset, epPatch, epTotalLen := computePatch(prevContent, entry.Content)
		entryPatches = append(entryPatches, types.EntryPatch{
			Index:       i,
			MessageID:   entry.MessageID,
			Type:        entry.Type,
			Patch:       epPatch,
			PatchOffset: epOffset,
			TotalLength: epTotalLen,
			ToolName:    entry.ToolName,
			ToolStatus:  entry.ToolStatus,
		})
	}

	if len(entryPatches) == 0 {
		return nil // Nothing changed
	}
	event.EntryPatches = entryPatches

	messageBytes, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal entry patch event: %w", err)
	}

	if err := apiServer.pubsub.Publish(context.Background(), pubsub.GetSessionQueue(owner, sessionID), messageBytes); err != nil {
		return fmt.Errorf("failed to publish entry patches to pubsub: %w", err)
	}

	log.Debug().
		Str("session_id", sessionID).
		Str("interaction_id", interactionID).
		Int("entry_patches", len(entryPatches)).
		Int("entry_count", event.EntryCount).
		Msg("📤 [HELIX] Published entry patches to frontend")

	// Also publish to commenter if applicable (for design review comments)
	if len(commenterID) > 0 && commenterID[0] != "" && commenterID[0] != owner {
		if err := apiServer.pubsub.Publish(context.Background(), pubsub.GetSessionQueue(commenterID[0], sessionID), messageBytes); err != nil {
			log.Warn().Err(err).
				Str("session_id", sessionID).
				Str("commenter_id", commenterID[0]).
				Msg("Failed to publish entry patches to commenter")
		}
	}

	return nil
}

// buildFullStatePatchEvent builds a serialized interaction_patch event containing the
// full content of all entries (computed with no previous entries, so patch_offset=0
// for each). Used to catch up a late-joining WebSocket client that missed earlier
// streaming patches.
func buildFullStatePatchEvent(sessionID, owner, interactionID string, entries []wsprotocol.ResponseEntry) ([]byte, error) {
	if len(entries) == 0 {
		return nil, nil
	}
	event := &types.WebsocketEvent{
		Type:          types.WebsocketEventInteractionPatch,
		SessionID:     sessionID,
		InteractionID: interactionID,
		Owner:         owner,
		EntryCount:    len(entries),
	}
	entryPatches := make([]types.EntryPatch, 0, len(entries))
	for i, entry := range entries {
		// previousContent="" → computePatch returns patchOffset=0, patch=full content
		epOffset, epPatch, epTotalLen := computePatch("", entry.Content)
		entryPatches = append(entryPatches, types.EntryPatch{
			Index:       i,
			MessageID:   entry.MessageID,
			Type:        entry.Type,
			Patch:       epPatch,
			PatchOffset: epOffset,
			TotalLength: epTotalLen,
			ToolName:    entry.ToolName,
			ToolStatus:  entry.ToolStatus,
		})
	}
	event.EntryPatches = entryPatches
	return json.Marshal(event)
}

// handleUserCreatedThread processes user-created thread event from Zed UI
// Creates a new Helix session and maps it to the Zed thread
func (apiServer *HelixAPIServer) handleUserCreatedThread(agentSessionID string, syncMsg *types.SyncMessage) error {
	log.Info().
		Str("agent_session_id", agentSessionID).
		Interface("data", syncMsg.Data).
		Msg("🆕 [HELIX] User created new thread in Zed UI")

	// Extract thread ID and title
	acpThreadID, ok := syncMsg.Data["acp_thread_id"].(string)
	if !ok || acpThreadID == "" {
		return fmt.Errorf("missing or invalid acp_thread_id in user_created_thread event")
	}

	title, _ := syncMsg.Data["title"].(string)
	if title == "" {
		title = "New Chat" // Default title
	}

	// IDEMPOTENCY: Check if we already have a session for this thread
	// This can happen if MessageAdded(role=user) arrived before UserCreatedThread
	// and created the session on-the-fly
	apiServer.contextMappingsMutex.RLock()
	existingMappedSession, alreadyExists := apiServer.contextMappings[acpThreadID]
	apiServer.contextMappingsMutex.RUnlock()

	if alreadyExists {
		log.Info().
			Str("acp_thread_id", acpThreadID).
			Str("existing_session_id", existingMappedSession).
			Msg("✅ [HELIX] Session already exists for thread (created on-the-fly), skipping creation")
		return nil
	}

	// Get the existing Helix session (agentSessionID is the session ID)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	existingSession, err := apiServer.Controller.Options.Store.GetSession(ctx, agentSessionID)
	if err != nil {
		return fmt.Errorf("failed to load existing session: %w", err)
	}

	// Create new Helix session for this user-created thread.
	// Copy ALL metadata from existing session so the new session is properly
	// associated with the spectask, project, and agent runtime.
	session := &types.Session{
		ID:             system.GenerateSessionID(),
		Created:        time.Now(),
		Updated:        time.Now(),
		Mode:           types.SessionModeInference,
		Type:           existingSession.Type,
		ModelName:      existingSession.ModelName,
		ParentApp:      existingSession.ParentApp,
		OrganizationID: existingSession.OrganizationID,
		ProjectID:      existingSession.ProjectID,
		Owner:          existingSession.Owner,
		OwnerType:      existingSession.OwnerType,
		Metadata: types.SessionMetadata{
			ZedThreadID:         acpThreadID,
			AgentType:           existingSession.Metadata.AgentType,
			ExternalAgentConfig: existingSession.Metadata.ExternalAgentConfig,
			SpecTaskID:          existingSession.Metadata.SpecTaskID,
			CodeAgentRuntime:    existingSession.Metadata.CodeAgentRuntime,
		},
		Name: title,
	}

	// Store session in database
	_, err = apiServer.Controller.Options.Store.CreateSession(ctx, *session)
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}

	// Create SpecTaskWorkSession + SpecTaskZedThread if this is a spectask session.
	// This wires the new thread into the multi-session model so it appears in
	// the session dropdown and gets proper lifecycle management.
	specTaskID := existingSession.Metadata.SpecTaskID
	if specTaskID != "" {
		// Determine phase from existing work session
		phase := types.SpecTaskPhaseImplementation
		existingWorkSession, wsErr := apiServer.Controller.Options.Store.GetSpecTaskWorkSessionByHelixSession(ctx, agentSessionID)
		if wsErr == nil && existingWorkSession != nil {
			phase = existingWorkSession.Phase
		}

		workSession := &types.SpecTaskWorkSession{
			SpecTaskID:     specTaskID,
			HelixSessionID: session.ID,
			Name:           title,
			Phase:          phase,
			Status:         types.SpecTaskWorkSessionStatusActive,
		}
		if wsErr := apiServer.Controller.Options.Store.CreateSpecTaskWorkSession(ctx, workSession); wsErr != nil {
			log.Warn().Err(wsErr).Msg("Failed to create work session for user-created thread (session still created)")
		} else {
			now := time.Now()
			zedThread := &types.SpecTaskZedThread{
				WorkSessionID:  workSession.ID,
				SpecTaskID:     specTaskID,
				ZedThreadID:    acpThreadID,
				Status:         types.SpecTaskZedStatusActive,
				LastActivityAt: &now,
			}
			if ztErr := apiServer.Controller.Options.Store.CreateSpecTaskZedThread(ctx, zedThread); ztErr != nil {
				log.Warn().Err(ztErr).Msg("Failed to create zed thread record (work session still created)")
			}
		}
	}

	// Map Zed thread to Helix session (same as handleThreadCreated)
	apiServer.contextMappingsMutex.Lock()
	apiServer.contextMappings[acpThreadID] = session.ID
	apiServer.contextMappingsMutex.Unlock()

	// Register the WebSocket connection for the child session so
	// sendCommandToExternalAgent can route commands to it.
	if wsConn, exists := apiServer.externalAgentWSManager.getConnection(agentSessionID); exists && wsConn != nil {
		apiServer.externalAgentWSManager.registerConnection(session.ID, wsConn)
	}

	log.Info().
		Str("acp_thread_id", acpThreadID).
		Str("helix_session_id", session.ID).
		Str("spec_task_id", specTaskID).
		Str("title", title).
		Msg("✅ [HELIX] Created new session + work session for user-created Zed thread")

	return nil
}

// handleThreadTitleChanged processes thread title change event from Zed
// Updates the corresponding Helix session name
func (apiServer *HelixAPIServer) handleThreadTitleChanged(agentSessionID string, syncMsg *types.SyncMessage) error {
	log.Info().
		Str("agent_session_id", agentSessionID).
		Interface("data", syncMsg.Data).
		Msg("📝 [HELIX] Thread title changed in Zed")

	// Extract thread ID and new title
	acpThreadID, ok := syncMsg.Data["acp_thread_id"].(string)
	if !ok || acpThreadID == "" {
		return fmt.Errorf("missing or invalid acp_thread_id in thread_title_changed event")
	}

	newTitle, ok := syncMsg.Data["title"].(string)
	if !ok {
		return fmt.Errorf("missing or invalid title in thread_title_changed event")
	}

	// Find corresponding Helix session (same as handleThreadCreated uses)
	apiServer.contextMappingsMutex.RLock()
	helixSessionID, exists := apiServer.contextMappings[acpThreadID]
	apiServer.contextMappingsMutex.RUnlock()

	if !exists {
		log.Warn().
			Str("acp_thread_id", acpThreadID).
			Msg("⚠️ [HELIX] Thread title changed but no Helix session found for thread")
		return nil // Not an error - thread might not have a session yet
	}

	// Load session
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	session, err := apiServer.Controller.Options.Store.GetSession(ctx, helixSessionID)
	if err != nil {
		return fmt.Errorf("failed to load session: %w", err)
	}

	// Update session name
	session.Name = newTitle
	session.Updated = time.Now()

	_, err = apiServer.Controller.Options.Store.UpdateSession(ctx, *session)
	if err != nil {
		return fmt.Errorf("failed to update session name: %w", err)
	}

	log.Info().
		Str("acp_thread_id", acpThreadID).
		Str("helix_session_id", helixSessionID).
		Str("new_title", newTitle).
		Msg("✅ [HELIX] Updated session name from Zed thread title")

	// Publish a session update so frontend refetches sessions list with new title
	event := &types.WebsocketEvent{
		Type:      types.WebsocketEventSessionUpdate,
		SessionID: helixSessionID,
		Owner:     session.Owner,
		Session:   session,
	}
	messageBytes, err := json.Marshal(event)
	if err == nil {
		apiServer.pubsub.Publish(context.Background(), pubsub.GetSessionQueue(session.Owner, session.ID), messageBytes)
	}

	return nil
}

// sendContinuePromptIfNeeded checks agent work state and sends continue prompt if agent was working
// Called when WebSocket reconnects after container restart
func (apiServer *HelixAPIServer) sendContinuePromptIfNeeded(ctx context.Context, sessionID string, wsConn *ExternalAgentWSConnection) {
	log.Info().
		Str("session_id", sessionID).
		// Str("spec_task_id", activity.SpecTaskID).
		Msg("Agent was working before disconnect, sending continue prompt")

	// Get session for thread ID and agent config
	session, err := apiServer.Store.GetSession(ctx, sessionID)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to get session for continue prompt")
		return
	}

	// Determine agent name from session config
	agentName := apiServer.getAgentNameForSession(ctx, session)

	// Build continue prompt
	continueMessage := `The sandbox was restarted. Please continue working on your current task.

If you were in the middle of something, please resume from where you left off.
If you need to verify the current state, check the git status and any running processes.`

	command := types.ExternalAgentCommand{
		Type: "chat_message",
		Data: map[string]interface{}{
			"message":       continueMessage,
			"request_id":    system.GenerateRequestID(),
			"acp_thread_id": session.Metadata.ZedThreadID,
			"agent_name":    agentName,
			"is_continue":   true, // Flag so agent knows this is a recovery prompt
		},
	}

	// Send via channel
	select {
	case wsConn.SendChan <- command:
		log.Info().
			Str("session_id", sessionID).
			Msg("Sent continue prompt to agent after reconnect")
	default:
		log.Warn().
			Str("session_id", sessionID).
			Msg("Failed to send continue prompt - channel full")
	}
}

// trackSpecTaskZedThread creates a SpecTaskWorkSession + SpecTaskZedThread pair
// for a Zed thread that belongs to a spectask. This runs in a background goroutine.
func (apiServer *HelixAPIServer) trackSpecTaskZedThread(ctx context.Context, helixSession *types.Session, acpThreadID string, title string) {
	specTaskID := helixSession.Metadata.SpecTaskID
	st := apiServer.Controller.Options.Store

	// Check if a SpecTaskZedThread already exists for this acpThreadID
	existing, err := st.GetSpecTaskZedThreadByZedThreadID(ctx, acpThreadID)
	if err == nil && existing != nil {
		log.Info().
			Str("spec_task_id", specTaskID).
			Str("acp_thread_id", acpThreadID).
			Str("zed_thread_record_id", existing.ID).
			Msg("SpecTaskZedThread already exists for this thread, skipping creation")
		return
	}

	// Create a SpecTaskWorkSession for this thread
	workSession := &types.SpecTaskWorkSession{
		SpecTaskID:     specTaskID,
		HelixSessionID: helixSession.ID,
		Name:           title,
		Phase:          types.SpecTaskPhaseImplementation,
		Status:         types.SpecTaskWorkSessionStatusActive,
	}

	err = st.CreateSpecTaskWorkSession(ctx, workSession)
	if err != nil {
		log.Error().Err(err).
			Str("spec_task_id", specTaskID).
			Str("helix_session_id", helixSession.ID).
			Msg("Failed to create SpecTaskWorkSession for thread tracking")
		return
	}

	// Create the SpecTaskZedThread record
	now := time.Now()
	zedThread := &types.SpecTaskZedThread{
		WorkSessionID:  workSession.ID,
		SpecTaskID:     specTaskID,
		ZedThreadID:    acpThreadID,
		Status:         types.SpecTaskZedStatusActive,
		LastActivityAt: &now,
	}

	err = st.CreateSpecTaskZedThread(ctx, zedThread)
	if err != nil {
		log.Error().Err(err).
			Str("spec_task_id", specTaskID).
			Str("work_session_id", workSession.ID).
			Str("acp_thread_id", acpThreadID).
			Msg("Failed to create SpecTaskZedThread for thread tracking")
		return
	}

	log.Info().
		Str("spec_task_id", specTaskID).
		Str("work_session_id", workSession.ID).
		Str("zed_thread_id", zedThread.ID).
		Str("acp_thread_id", acpThreadID).
		Str("helix_session_id", helixSession.ID).
		Msg("✅ Created SpecTaskZedThread for multi-thread tracking")
}

// updateSpecTaskZedThreadActivity updates the LastActivityAt timestamp on a SpecTaskZedThread.
// This runs in a background goroutine.
func (apiServer *HelixAPIServer) updateSpecTaskZedThreadActivity(ctx context.Context, acpThreadID string) {
	st := apiServer.Controller.Options.Store

	zedThread, err := st.GetSpecTaskZedThreadByZedThreadID(ctx, acpThreadID)
	if err != nil {
		// Not a tracked spectask thread - this is normal for non-spectask sessions
		return
	}

	now := time.Now()
	zedThread.LastActivityAt = &now
	err = st.UpdateSpecTaskZedThread(ctx, zedThread)
	if err != nil {
		log.Error().Err(err).
			Str("acp_thread_id", acpThreadID).
			Str("zed_thread_id", zedThread.ID).
			Msg("Failed to update SpecTaskZedThread activity")
	}
}
