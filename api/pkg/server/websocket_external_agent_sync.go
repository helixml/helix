package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
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

// ErrNoExternalAgentWS is returned by sendCommandToExternalAgent when no
// WebSocket connection exists for the target session. This is an expected,
// transient state (the dev container may be sleeping or still booting): the
// caller's persisted interaction will be picked up by the reconnect resume path
// when the agent reconnects, so no retry is needed at the prompt-queue layer.
// Callers distinguish this from real send failures using errors.Is.
var ErrNoExternalAgentWS = errors.New("no external agent WebSocket connection")

// ErrPromptBusyDeferred is returned by sendQueuedPromptToSession when the
// session turned out to be mid-turn and the claimed prompt was therefore NOT
// dispatched. This is the queue working as designed, not a failure: the prompt
// is untouched and will be redelivered on the next drain.
//
// It exists so callers can tell a defer apart from a real dispatch error and put
// the prompt back with RevertPromptToPending instead of MarkPromptAsFailed.
// MarkPromptAsFailed increments retry_count, which is a bounded budget
// (defaultMaxPromptQueueRetries) for GENUINE failures — once it is exhausted the
// GetNext*Prompt selectors stop matching the row and the message is silently
// dropped forever. processAnyPendingPrompt runs on every acknowledged
// cancellation, i.e. every time the user interrupts the agent, so charging the
// budget for defers burnt ~11 retries in 16 minutes on a live session.
// See design/tasks/003021_queued-agent-messages.
var ErrPromptBusyDeferred = errors.New("prompt dispatch deferred: session busy")

// agentCrashErrorMarkers identify thread_load_error events that originate from
// the Claude Agent ACP wrapper inside Zed having exited. Once the wrapper
// process is gone, every subsequent follow-up the user sends bounces with
// "Session not found" — Zed's THREAD_SERVICE has no agent to dispatch to and
// no first-class way to respawn one for an existing thread. We don't auto-retry
// silently because the half-dead container needs to be torn down and rebuilt.
// Instead the queue surfaces a Restart button; restartCrashedAgentThread tears
// down the desktop container and brings up a fresh one via the resume path,
// preserving ZedThreadID so Zed reloads the existing thread from threads.db on
// reconnect and the agent reloads its session from the persistent workspace
// volume — full conversation context is restored.
var agentCrashErrorMarkers = []string{
	"Claude Agent process exited",
	"Session not found",
}

// isAgentCrashError reports whether errMsg matches a known terminal Claude
// Agent failure that no number of auto-retries can recover from.
func isAgentCrashError(errMsg string) bool {
	for _, marker := range agentCrashErrorMarkers {
		if strings.Contains(errMsg, marker) {
			return true
		}
	}
	return false
}

func isMissingCodexRolloutError(errMsg string) bool {
	return strings.Contains(errMsg, "no rollout found for thread id")
}

func isAuthoritativeMissingThreadError(errMsg string) bool {
	if isMissingCodexRolloutError(errMsg) || strings.Contains(errMsg, `no thread found with ID: SessionId("`) {
		return true
	}
	const prefix = "Failed to load thread: Resource not found: "
	remainder, ok := strings.CutPrefix(errMsg, prefix)
	if !ok {
		return false
	}
	threadID, rawMeta, ok := strings.Cut(remainder, ": ")
	if !ok || threadID == "" {
		return false
	}
	var meta map[string]string
	return json.Unmarshal([]byte(rawMeta), &meta) == nil && len(meta) == 1 && meta["uri"] == threadID
}

// acpWedgeCrashThreshold is how many prior retries a prompt must already have
// accumulated before a recurring thread_load_error is treated as terminal
// (crash-marked, surfacing Restart) rather than retried again. A thread_load_error
// has many transport/wrapper wordings ("ede_diagnostic …", "response channel
// cancelled", "send failed because receiver is gone", …) so we gate on recurrence,
// not on the string. Gives a genuinely-transient drain (which Zed itself already
// retries 4×750ms) a couple of normal backoff retries to clear before we give up.
// See design/2026-06-15-wedged-acp-thread-autowake-flood.md.
const acpWedgeCrashThreshold = 2

// streamingContext caches DB query results during token streaming to avoid
// redundant queries. Created on first message_added, cleared on message_completed.
// Also buffers interaction updates: DB writes are throttled to at most once per
// dbWriteInterval, and frontend publishes to once per publishInterval.
type streamingContext struct {
	session     *types.Session
	interaction *types.Interaction
	// Track which interaction this context is for - used to detect transitions
	interactionID string
	// Commenter ID for design review comment streaming (looked up from sessionToCommenterMapping)
	commenterID string
	// DB write throttling
	lastDBWrite time.Time
	dirty       bool // true if interaction has been updated since last DB write
	// Frontend publish throttling
	lastPublish time.Time
	// Trailing-edge flush timer: fires after publishInterval to drain any
	// patches that were skipped by the throttle when no new event arrived.
	flushTimer *time.Timer
	// Trailing-edge DB flush timer: mirrors flushTimer for the throttled DB
	// write. Fires after dbTrailingFlushInterval so the persisted interaction
	// (read by the 3s poll fallback and page-reload snapshots) never sits
	// more than ~dbTrailingFlushInterval behind the live stream during a pause.
	dbFlushTimer *time.Timer
	// Per-entry delta tracking: tracks entries sent to frontend so we can compute per-entry diffs
	previousEntries []wsprotocol.ResponseEntry
	// Message accumulator: persists across handleMessageAdded calls so that
	// out-of-order flush updates (Stopped event) can replace earlier message_ids
	// in-place instead of appending duplicates. A new accumulator per call would
	// lose the message_id→content mapping because the DB only stores the joined string.
	accumulator *wsprotocol.MessageAccumulator
	// priorEntries are (message_id, content) snapshots from earlier completed
	// interactions in this session. Seeded into the accumulator so Zed's
	// flush_streaming_throttle re-sends of prior-turn entries are dropped
	// instead of leaking into the current interaction's response_entries.
	// Compared by (id, content) so wrapper-restart renumbering doesn't drop
	// legitimate new content (see design/2026-04-30-queue-and-other-stuck-state-bugs.md).
	priorEntries []wsprotocol.ResponseEntry
	mu           sync.Mutex
}

const (
	// dbWriteInterval is the minimum time between UpdateInteraction calls during streaming.
	// Intermediate content is buffered in the streamingContext.
	// Risk: up to dbWriteInterval of content lost on crash. Acceptable because
	// message_completed always writes the final state, and Zed has the full content.
	// Note: long-running agent sessions can accumulate 10+ MB of response_entries.
	// Each DB write replaces the entire JSONB blob (TOAST rewrite), so frequent
	// writes cause massive autovacuum pressure. 5s keeps crash-loss minimal while
	// reducing TOAST churn ~25x vs the previous 200ms interval.
	dbWriteInterval = 5 * time.Second

	// publishInterval is the minimum time between frontend pubsub events during streaming.
	// Frontend batches to requestAnimationFrame (~16ms), so faster is wasted work.
	publishInterval = 50 * time.Millisecond

	// dbTrailingFlushInterval is the delay after the last streamed chunk before
	// the trailing-edge DB flush fires. Kept small enough that a fallback/reload
	// read is never badly stale, but large enough that continuous streaming
	// (which keeps resetting the timer) still writes at the dbWriteInterval
	// cadence rather than on every chunk — bounding TOAST churn.
	dbTrailingFlushInterval = 500 * time.Millisecond
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

	// Report is the agent's active-turn report, captured when the session
	// became ready. Zero (Reported=false) when the readiness timeout fired
	// instead of agent_ready arriving.
	Report agentTurnReport
	// ResumeHook decides what to do with a waiting turn once the agent's report
	// is in. It is installed after the connect handler has resolved the turn,
	// which can race agent_ready — setResumeHook fires it immediately when the
	// session is already ready.
	ResumeHook  func(agentTurnReport)
	resumeFired bool
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
	defer apiServer.externalAgentWSManager.unregisterConnection(agentID, wsConn)

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
			defer apiServer.externalAgentWSManager.unregisterConnection(helixSessionID, wsConn)
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
			log.Info().
				Str("session_id", helixSessionID).
				Str("zed_thread_id", helixSession.Metadata.ZedThreadID).
				Str("spec_task_id", helixSession.Metadata.SpecTaskID).
				Str("agent_type", helixSession.Metadata.AgentType).
				Msg("[CONNECT] Session loaded for reconnect")
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

			// Clear stale streaming context from previous connection.
			apiServer.flushAndClearStreamingContext(ctx, helixSessionID)

			// A reconnect means any dispatch in flight on the old connection is
			// gone. Drop its claim so the waiting interaction can be re-resolved
			// against this connection.
			apiServer.releaseSessionDispatchClaims(helixSessionID)

			// Correlate the waiting turn to this connection — request_id maps,
			// durable binding, dispatch claim — but do NOT send it yet. Whether
			// to send at all depends on which turns the agent reports it is
			// already running, and that report only arrives with agent_ready.
			// See external_agent_resume.go.
			resume := apiServer.resolveWaitingInteraction(ctx, helixSessionID, helixSession, agentID, wsConn)
			requestID := ""
			if resume != nil {
				requestID = resume.requestID
				defer close(resume.abandoned)
				apiServer.externalAgentWSManager.setResumeHook(helixSessionID, func(report agentTurnReport) {
					apiServer.applyResumeDecision(resume, report)
				})
			}

			// Send open_thread BEFORE the agent_ready gate so Zed re-establishes its
			// thread subscription immediately on connect. If we wait until after
			// agent_ready, the queued chat_message gets flushed first, and when Zed
			// opens the thread it replays history as message_added events that corrupt
			// the current interaction.
			if helixSession.Metadata.ZedThreadID != "" {
				// Reopen THIS session's own thread — the exact thread every
				// chat_message send path targets (session.Metadata.ZedThreadID).
				// Do NOT substitute a spec-task-global "latest" thread here: the
				// connection — and the waiting interaction queued moments earlier
				// by resolveWaitingInteraction — both belong to helixSession, and
				// that send uses helixSession.Metadata.ZedThreadID. If open_thread
				// addresses a different thread, Zed foregrounds/streams one thread
				// while Helix sends messages into another (opened ≠ sent-to), which
				// the user sees as their messages vanishing into a thread that
				// isn't on screen — without ever switching session in the UI.
				// The global "latest" override never made the send land correctly
				// (the send was always on the session's own thread); it could only
				// ever cause the mismatch. Compaction-created threads are their own
				// Helix sessions (handleUserCreatedThread) and must be driven by
				// reconnecting under that session, not by retargeting this one.
				// See design/2026-06-22-zed-open-thread-send-mismatch.md.
				targetThreadID := helixSession.Metadata.ZedThreadID

				agentNameForOpen := apiServer.getAgentNameForSession(ctx, helixSession)
				log.Info().
					Str("session_id", helixSessionID).
					Str("zed_thread_id", targetThreadID).
					Str("agent_name", agentNameForOpen).
					Msg("[CONNECT] Sending open_thread directly on new connection before agent_ready gate")

				// Send directly on the new wsConn rather than going through
				// sendCommandToExternalAgent — during reconnection, the connection
				// map may briefly have a stale entry or the channel may be closed
				// by a racing defer from the old connection handler.
				data := map[string]interface{}{
					"acp_thread_id": targetThreadID,
					"session_id":    helixSessionID,
				}
				if requestID != "" {
					data["request_id"] = requestID
				}
				if agentNameForOpen != "" {
					data["agent_name"] = agentNameForOpen
				}
				openThreadCmd := types.ExternalAgentCommand{
					Type: "open_thread",
					Data: data,
				}
				// Write directly to the WebSocket, bypassing SendChan and the
				// sender goroutine. During reconnection, the sender goroutine
				// may not have started reading from the channel yet, and we
				// need open_thread to arrive before agent_ready.
				wsConn.mu.Lock()
				writeErr := wsConn.Conn.WriteJSON(openThreadCmd)
				wsConn.mu.Unlock()
				if writeErr != nil {
					log.Error().
						Str("session_id", helixSessionID).
						Err(writeErr).
						Msg("[CONNECT] Failed to write open_thread directly to WebSocket")
				} else {
					log.Info().
						Str("session_id", helixSessionID).
						Str("zed_thread_id", targetThreadID).
						Msg("[CONNECT] ✅ open_thread written directly to WebSocket")
				}
			}
		}
	}

	// Handle incoming messages (blocking)
	apiServer.handleExternalAgentReceiver(ctx, wsConn)
}

// sessionForInteraction resolves the Helix session an interaction belongs to.
func (apiServer *HelixAPIServer) sessionForInteraction(ctx context.Context, interactionID string) (*types.Session, error) {
	if interactionID == "" {
		return nil, fmt.Errorf("empty interaction id")
	}
	interaction, err := apiServer.Controller.Options.Store.GetInteraction(ctx, interactionID)
	if err != nil {
		return nil, err
	}
	if interaction.SessionID == "" {
		return nil, fmt.Errorf("interaction %s has no session", interactionID)
	}
	return apiServer.Controller.Options.Store.GetSession(ctx, interaction.SessionID)
}

// dispatchClaim records which request_id is currently delivering a waiting
// interaction to the external agent, and on whose session.
type dispatchClaim struct {
	requestID string
	sessionID string
}

// claimInteractionDispatch atomically reserves interactionID for delivery to the
// external agent under requestID.
//
// Two independent paths can deliver the same waiting interaction: RunExternalAgent
// (the live chat turn) and resolveWaitingInteraction (agent reconnect, and the
// settings-sync daemon's /agent-config-applied callback after an agent switch).
// Both used to fire during RunExternalAgent's readiness wait, sending two
// chat_message commands carrying the SAME request_id and an empty acp_thread_id.
// Zed has no request_id dedupe, so it opened TWO ACP threads for one turn; the
// second thread_created looked user-initiated and was forked into a throwaway
// "New Conversation" session, which is where the agent's entire response then
// streamed — invisible from the task it belonged to.
//
// Returns the request_id the interaction is being delivered under (the winner's,
// so a loser can still attach its response channels to the live turn) and whether
// this caller won. A loser must NOT send a second chat_message.
func (apiServer *HelixAPIServer) claimInteractionDispatch(sessionID, interactionID, requestID string) (string, bool) {
	apiServer.contextMappingsMutex.Lock()
	defer apiServer.contextMappingsMutex.Unlock()
	return apiServer.claimInteractionDispatchLocked(sessionID, interactionID, requestID)
}

// claimInteractionDispatchLocked is claimInteractionDispatch for callers that
// already hold contextMappingsMutex (resolveWaitingInteraction claims inside the
// same critical section that picks the request_id, so the two cannot diverge).
func (apiServer *HelixAPIServer) claimInteractionDispatchLocked(sessionID, interactionID, requestID string) (string, bool) {
	if interactionID == "" || requestID == "" {
		return requestID, true
	}
	if apiServer.interactionDispatchClaims == nil {
		apiServer.interactionDispatchClaims = make(map[string]dispatchClaim)
	}
	if existing, claimed := apiServer.interactionDispatchClaims[interactionID]; claimed {
		return existing.requestID, false
	}
	apiServer.interactionDispatchClaims[interactionID] = dispatchClaim{requestID: requestID, sessionID: sessionID}
	return requestID, true
}

// releaseInteractionDispatch drops the claim on interactionID so the turn can be
// re-delivered (retry, reconnect).
func (apiServer *HelixAPIServer) releaseInteractionDispatch(interactionID string) {
	if interactionID == "" {
		return
	}
	apiServer.contextMappingsMutex.Lock()
	delete(apiServer.interactionDispatchClaims, interactionID)
	apiServer.contextMappingsMutex.Unlock()
}

// releaseDispatchClaimByRequest drops the claim held under requestID. Called when
// a turn's response channels are torn down, which is the point the turn is over
// regardless of how it ended.
func (apiServer *HelixAPIServer) releaseDispatchClaimByRequest(requestID string) {
	if requestID == "" {
		return
	}
	apiServer.contextMappingsMutex.Lock()
	for interactionID, claim := range apiServer.interactionDispatchClaims {
		if claim.requestID == requestID {
			delete(apiServer.interactionDispatchClaims, interactionID)
		}
	}
	apiServer.contextMappingsMutex.Unlock()
}

// releaseSessionDispatchClaims drops every claim belonging to a session. Called
// when the external agent (re)connects: a reconnect means an in-flight dispatch
// may have died with the old connection, so resolveWaitingInteraction must be
// free to re-deliver the still-waiting interaction.
func (apiServer *HelixAPIServer) releaseSessionDispatchClaims(sessionID string) {
	if sessionID == "" {
		return
	}
	apiServer.contextMappingsMutex.Lock()
	for interactionID, claim := range apiServer.interactionDispatchClaims {
		if claim.sessionID == sessionID {
			delete(apiServer.interactionDispatchClaims, interactionID)
		}
	}
	apiServer.contextMappingsMutex.Unlock()
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
	event := log.Trace().
		Str("agent_session_id", sessionID).
		Str("event_type", syncMsg.EventType)
	if syncMsg.EventType != "message_added" {
		event = event.Interface("data", syncMsg.Data)
	}
	event.Msg("[HELIX] Processing message from external agent")

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
	case "turn_cancelled":
		err = apiServer.handleTurnCancelled(sessionID, syncMsg)
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

	// PRIORITY 4: A second thread_created carrying a request_id that still has an
	// in-flight turn is NOT a user opening a thread in Zed — it is a duplicate
	// thread for a turn Helix itself dispatched. (PRIORITY 1 consumed the
	// request_id → session mapping for the first thread, so without this check
	// the duplicate falls through and forks a throwaway session; the agent then
	// streams its whole response into a session nobody is looking at.)
	//
	// Rebind onto the turn's own session and point it at the newest thread —
	// that is the thread Zed foregrounds and streams the prompt on.
	if requestID != "" {
		apiServer.contextMappingsMutex.RLock()
		inFlightInteractionID, inFlight := apiServer.requestToInteractionMapping[requestID]
		apiServer.contextMappingsMutex.RUnlock()
		if inFlight {
			if boundSession, err := apiServer.sessionForInteraction(context.Background(), inFlightInteractionID); err == nil && boundSession != nil {
				log.Warn().
					Str("agent_session_id", sessionID).
					Str("helix_session_id", boundSession.ID).
					Str("acp_thread_id", acpThreadID).
					Str("request_id", requestID).
					Str("previous_zed_thread_id", boundSession.Metadata.ZedThreadID).
					Msg("⚠️ [HELIX] Duplicate thread_created for an in-flight turn — rebinding to its session instead of forking a new one")

				boundSession.Metadata.ZedThreadID = contextID
				boundSession.Updated = time.Now()
				if _, err := apiServer.Controller.Options.Store.UpdateSession(context.Background(), *boundSession); err != nil {
					return fmt.Errorf("failed to rebind session %s to duplicate thread: %w", boundSession.ID, err)
				}

				apiServer.contextMappingsMutex.Lock()
				if apiServer.contextMappings == nil {
					apiServer.contextMappings = make(map[string]string)
				}
				apiServer.contextMappings[contextID] = boundSession.ID
				apiServer.contextMappingsMutex.Unlock()

				if boundSession.Metadata.SpecTaskID != "" {
					go apiServer.trackSpecTaskZedThread(context.Background(), boundSession, acpThreadID, title)
				}
				return nil
			}
		}
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
		ID:                     "", // Will be generated
		GenerationID:           0,
		Created:                time.Now(),
		Updated:                time.Now(),
		Scheduled:              time.Now(),
		Completed:              time.Time{},
		SessionID:              createdSession.ID,
		UserID:                 createdSession.Owner,
		Mode:                   types.SessionModeInference,
		PromptMessage:          "New conversation started via Zed", // Default message
		State:                  types.InteractionStateWaiting,
		ResponseMessage:        "",
		ExternalAgentRequestID: requestID,
	}
	now := time.Now()
	interaction.ExternalAgentDispatchedAt = &now

	createdInteraction, err := apiServer.Controller.Options.Store.CreateInteraction(context.Background(), interaction)
	if err != nil {
		log.Error().Err(err).
			Str("helix_session_id", createdSession.ID).
			Msg("❌ [HELIX] Failed to create interaction for new thread")
		return fmt.Errorf("failed to create interaction: %w", err)
	}

	// Notify frontend immediately so the chat updates without waiting for poll
	apiServer.publishInteractionUpdateToFrontend(createdSession.ID, createdSession.Owner, createdInteraction)

	// Store request_id → interaction_id mapping so that cancel and completion
	// handlers can look up the correct interaction.
	if requestID != "" {
		apiServer.contextMappingsMutex.Lock()
		if apiServer.requestToInteractionMapping == nil {
			apiServer.requestToInteractionMapping = make(map[string]string)
		}
		apiServer.requestToInteractionMapping[requestID] = createdInteraction.ID
		apiServer.contextMappingsMutex.Unlock()
	}

	log.Info().
		Str("helix_session_id", createdSession.ID).
		Str("interaction_id", createdInteraction.ID).
		Str("request_id", requestID).
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

	// Refuse to push new input to a paused session. This is defence-in-depth:
	// the HTTP ingress paths (sendSessionMessage / startChatSessionHandler /
	// sendQueuedPromptToSession) all check pause state before calling here,
	// but a future caller that bypasses them would otherwise silently wake
	// a frozen-checkpoint session.
	if session.Metadata.Paused {
		log.Warn().
			Str("session_id", sessionID).
			Str("interaction_id", interaction.ID).
			Str("paused_reason", session.Metadata.PausedReason).
			Msg("notify: refusing to push new interaction to paused session")
		return fmt.Errorf("session is paused (reason: %s)", session.Metadata.PausedReason)
	}

	bound, err := apiServer.Store.BindInteractionExternalAgentRequest(context.Background(), interaction.ID, interaction.GenerationID, interaction.ID)
	if err != nil {
		return fmt.Errorf("persist external-agent request mapping: %w", err)
	}
	if !bound {
		return fmt.Errorf("interaction %s is no longer waiting", interaction.ID)
	}
	interaction.ExternalAgentRequestID = interaction.ID

	log.Info().
		Str("session_id", sessionID).
		Str("interaction_id", interaction.ID).
		Str("agent_type", session.Metadata.AgentType).
		Msg("Notifying external agent of new interaction")

	// Build command data - include acp_thread_id if session already has one (for follow-up messages)
	//
	// NOTE: we deliberately do NOT set "role" here. The Zed sync client drops any
	// chat_message with role=="user" as a UI-sync echo (websocket_sync.rs:421), so a
	// genuine prompt sent through this path was being silently discarded (#2642). The
	// queue path (sendQueuedPromptToSession) never set role and works; this matches it.
	commandData := map[string]interface{}{
		"message":                   interaction.PromptMessage,
		"request_id":                interaction.ID, // Use interaction ID as request ID for response tracking
		"interaction_id":            interaction.ID,
		"interaction_generation_id": interaction.GenerationID,
		"track_code_changes":        session.Metadata.SpecTaskID != "",
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

	// Use the unified sendCommandToExternalAgent which handles connection lookup and routing.
	// If no WebSocket connection exists, sendCommandToExternalAgent will auto-start the
	// dev container via autoStartDevContainerForSession. The waiting interaction will be picked up
	// by the reconnect resume path when the agent reconnects.
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

	// entry_type distinguishes assistant prose, tool invocations, and plan snapshots.
	// Optional field — old Zed versions don't send it (defaults to empty string).
	entryType, _ := syncMsg.Data["entry_type"].(string)
	// request_id correlates this response to the chat_message that triggered it.
	// Used to route streaming tokens to the correct interaction.
	messageRequestID, _ := syncMsg.Data["request_id"].(string)
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
		sctx := apiServer.getOrCreateStreamingContext(context.Background(), helixSessionID, messageRequestID)
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
		content = apiServer.redactCredentials(helixSession.OrganizationID, content)
		toolName = apiServer.redactCredentials(helixSession.OrganizationID, toolName)
		syncMsg.Data["content"] = content
		syncMsg.Data["tool_name"] = toolName
		targetInteraction := sctx.interaction

		if targetInteraction != nil {
			// Mark the originating queue prompt as 'sent' the first time Zed
			// emits an assistant event for this interaction. The link comes from
			// the persisted Interaction.PromptID column so it survives API
			// restarts and reconnect-via-resume. Idempotent at
			// the SQL layer — calling MarkPromptAsSent on an already-sent prompt
			// is a no-op write — so it's safe to fire on every message_added.
			if targetInteraction.PromptID != "" {
				if markErr := apiServer.Controller.Options.Store.MarkPromptAsSent(context.Background(), targetInteraction.PromptID); markErr != nil {
					log.Warn().Err(markErr).
						Str("prompt_id", targetInteraction.PromptID).
						Str("interaction_id", targetInteraction.ID).
						Msg("Failed to mark prompt as sent after Zed acknowledged")
				}
			}

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
				sctx.accumulator = wsprotocol.RestoreAccumulator(
					targetInteraction.ResponseMessage,
					targetInteraction.LastZedMessageID,
					targetInteraction.LastZedMessageOffset,
					targetInteraction.ResponseEntries,
				)
				// Drop replays of (message_id, content) pairs that belong to
				// earlier interactions in this session — Zed's
				// flush_streaming_throttle resends ALL ACP thread entries on
				// every event. Content-aware so wrapper-restart renumbering
				// doesn't drop legitimate new content under a reused id.
				sctx.accumulator.SetPriorEntries(sctx.priorEntries)
			}
			acc := sctx.accumulator
			prevMessageID := acc.LastMessageID

			// FORCE-FLUSH on tool_call boundary: When a tool_call entry arrives (a new
			// message_id that will create a new entry), force-publish any pending patches
			// BEFORE adding the tool_call to the accumulator. This ensures the frontend
			// sees the complete text content of the preceding entry before entry_count
			// increases.
			//
			// Without this flush, the tool_call entry is visible while the preceding text
			// entry's final content is still waiting in the 50ms publish throttle buffer,
			// causing truncated sentences (e.g., "...with `Hello" followed by a Write
			// tool call, when it should say "...with `Hello, world!`").
			isNewEntry := prevMessageID != "" && prevMessageID != messageID
			currentEntryCount := len(acc.Entries())
			forceFlushToolCall := isNewEntry && entryType == "tool_call" && currentEntryCount > 0
			if forceFlushToolCall {
				// Force-flush before the tool_call: publish the current state (which has
				// the complete text entry content from Zed's stale-pending flush) before
				// the tool_call entry is added and entry_count increases.
				currentEntries := acc.Entries()
				if err := apiServer.publishEntryPatchesToFrontend(helixSessionID, helixSession.Owner, targetInteraction.ID, sctx.previousEntries, currentEntries, sctx.commenterID); err != nil {
					log.Error().Err(err).
						Str("session_id", helixSessionID).
						Str("interaction_id", targetInteraction.ID).
						Msg("Failed to force-publish entry patches before tool_call")
				} else {
					log.Info().
						Str("interaction_id", targetInteraction.ID).
						Int("entry_count", currentEntryCount).
						Str("entry_type", entryType).
						Msg("📤 [FLUSH] Force-published patches before tool_call entry")
				}
				sctx.previousEntries = currentEntries
			}

			acc.AddMessageWithToolInfo(messageID, content, entryType, toolName, toolStatus)

			if prevMessageID != "" && prevMessageID != messageID {
				log.Info().
					Str("interaction_id", targetInteraction.ID).
					Str("last_message_id", prevMessageID).
					Str("new_message_id", messageID).
					Msg("📝 [HELIX] New distinct message detected (different message_id)")
			}

			targetInteraction.LastZedMessageID = acc.LastMessageID
			targetInteraction.Updated = time.Now()
			sctx.dirty = true

			// THROTTLED DB WRITE: Only flush to DB if enough time has passed.
			// The in-memory interaction always has the latest content.
			// Rebuild Content/Offset and marshal response_entries only when
			// actually writing — avoids joining 17 MB of strings and
			// serializing multi-MB JSON on every message.
			now := time.Now()
			if now.Sub(sctx.lastDBWrite) >= dbWriteInterval {
				// Leading-edge DB write; cancel any pending trailing flush since
				// we're persisting the latest state right now.
				if sctx.dbFlushTimer != nil {
					sctx.dbFlushTimer.Stop()
					sctx.dbFlushTimer = nil
				}
				if err := apiServer.flushStreamingFieldsToDB(sctx); err != nil {
					return fmt.Errorf("failed to update interaction %s: %w", targetInteraction.ID, err)
				}
			} else {
				// Trailing-edge DB flush: mirror the frontend publish flushTimer
				// so the persisted interaction (used by the 3s poll fallback and
				// page-reload snapshots) is never more than dbTrailingFlushInterval
				// behind the live stream during a pause. Without this the DB sits
				// up to dbWriteInterval (5s) stale whenever the agent pauses
				// mid-turn (e.g. before a tool call). Each new chunk resets the
				// timer, so continuous streaming still writes at the dbWriteInterval
				// cadence via the leading-edge branch above.
				if sctx.dbFlushTimer != nil {
					sctx.dbFlushTimer.Stop()
				}
				trailingInteractionID := targetInteraction.ID
				sctx.dbFlushTimer = time.AfterFunc(dbTrailingFlushInterval, func() {
					sctx.mu.Lock()
					defer sctx.mu.Unlock()
					sctx.dbFlushTimer = nil
					if !sctx.dirty {
						return
					}
					if err := apiServer.flushStreamingFieldsToDB(sctx); err != nil {
						log.Error().Err(err).
							Str("interaction_id", trailingInteractionID).
							Msg("Failed to write interaction in trailing DB flush")
					}
				})
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
			// Exception: if we just force-flushed before a tool_call, also publish the tool_call.
			if now.Sub(sctx.lastPublish) >= publishInterval || forceFlushToolCall {
				if sctx.flushTimer != nil {
					sctx.flushTimer.Stop()
					sctx.flushTimer = nil
				}
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
			} else {
				// Trailing-edge timer: ensure pending patches are published
				// even if no new message_added event arrives within the window.
				if sctx.flushTimer != nil {
					sctx.flushTimer.Stop()
				}
				sessionID := helixSessionID
				owner := helixSession.Owner
				interactionID := targetInteraction.ID
				commenterID := sctx.commenterID
				sctx.flushTimer = time.AfterFunc(publishInterval, func() {
					sctx.mu.Lock()
					defer sctx.mu.Unlock()
					sctx.flushTimer = nil
					if sctx.accumulator == nil || sctx.interaction == nil {
						return
					}
					currentEntries := sctx.accumulator.Entries()
					if err := apiServer.publishEntryPatchesToFrontend(sessionID, owner, interactionID, sctx.previousEntries, currentEntries, commenterID); err != nil {
						log.Error().Err(err).
							Str("session_id", sessionID).
							Str("interaction_id", interactionID).
							Msg("Failed to publish entry patches in trailing flush")
					}
					sctx.previousEntries = currentEntries
					sctx.lastPublish = time.Now()
				})
			}
		} else {
			// Reaching here means the chokepoint (getOrCreateStreamingContext) found
			// neither a Waiting interaction nor a restart-interrupted one to recover
			// for this thread's session — i.e. there is genuinely no interaction to
			// route this assistant content to (no prompt was ever created for it).
			// This is the only remaining unroutable case after the #2643 recovery;
			// log loudly with the thread id so it's diagnosable rather than silent.
			log.Warn().
				Str("session_id", sessionID).
				Str("acp_thread_id", contextID).
				Str("helix_session_id", helixSessionID).
				Str("request_id", messageRequestID).
				Msg("⚠️ [HELIX] No interaction to route assistant message_added (no Waiting or recoverable interaction for this thread) — content dropped")
		}
	} else {
		// For user messages, check whether a pre-created Waiting interaction already exists
		// for this session (e.g. created by sendMessageToSpecTaskAgent for approval flows).
		// Zed echoes the sent user message back as message_added(role=user), which would
		// otherwise create a duplicate interaction and overwrite the mapping, causing the
		// assistant response to land in the wrong interaction (Bug 1 fix).
		// Check requestToInteractionMapping: if any request maps to this session via
		// requestToSessionMapping, a pre-created interaction already exists.
		apiServer.contextMappingsMutex.RLock()
		var existingInteractionID string
		for reqID, sessID := range apiServer.requestToSessionMapping {
			if sessID == helixSessionID {
				if intID, ok := apiServer.requestToInteractionMapping[reqID]; ok {
					existingInteractionID = intID
					break
				}
			}
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
				ID:                     "", // Will be generated
				Created:                time.Now(),
				Updated:                time.Now(),
				SessionID:              helixSessionID,
				UserID:                 helixSession.Owner,
				GenerationID:           helixSession.GenerationID, // Must match session's generation for query to find it
				Mode:                   types.SessionModeInference,
				PromptMessage:          content,
				State:                  types.InteractionStateWaiting,
				ExternalAgentRequestID: messageRequestID,
			}
			now := time.Now()
			interaction.ExternalAgentDispatchedAt = &now

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

			log.Info().
				Str("helix_session_id", helixSessionID).
				Str("interaction_id", createdInteraction.ID).
				Msg("🗺️ [HELIX] Created interaction from Zed user message")
		}
	}

	return nil
}

func (apiServer *HelixAPIServer) recordCredential(orgID, _ string, token string) {
	if orgID == "" || token == "" {
		return
	}
	apiServer.credentialTokensMu.Lock()
	defer apiServer.credentialTokensMu.Unlock()
	if apiServer.credentialTokens == nil {
		apiServer.credentialTokens = make(map[string]map[string]struct{})
	}
	if apiServer.credentialTokens[orgID] == nil {
		apiServer.credentialTokens[orgID] = make(map[string]struct{})
	}
	apiServer.credentialTokens[orgID][token] = struct{}{}
}

func (apiServer *HelixAPIServer) redactCredentials(orgID, content string) string {
	apiServer.credentialTokensMu.RLock()
	tokens := make([]string, 0, len(apiServer.credentialTokens[orgID]))
	for token := range apiServer.credentialTokens[orgID] {
		tokens = append(tokens, token)
	}
	apiServer.credentialTokensMu.RUnlock()
	for _, token := range tokens {
		content = strings.ReplaceAll(content, token, "<redacted>")
	}
	return content
}

// getOrCreateStreamingContext returns a cached streaming context for the given
// helix session, or creates one by querying the DB on the first call. This avoids
// redundant GetSession + ListInteractions queries on every streaming token.
//
// requestID is the request_id from the message_added event, used to route tokens
// to the correct interaction via requestToInteractionMapping.
//
// IMPORTANT: Also detects interaction transitions (follow-up messages) and resets
// previousEntries when the target interaction changes. This prevents patch computation
// from using stale entries from the old interaction.
func (apiServer *HelixAPIServer) getOrCreateStreamingContext(ctx context.Context, helixSessionID string, requestID string) *streamingContext {
	// Resolve which interaction this request_id maps to
	var expectedInteractionID string
	if requestID != "" {
		apiServer.contextMappingsMutex.RLock()
		expectedInteractionID = apiServer.requestToInteractionMapping[requestID]
		apiServer.contextMappingsMutex.RUnlock()
	}

	apiServer.streamingContextsMu.RLock()
	sctx, exists := apiServer.streamingContexts[helixSessionID]
	apiServer.streamingContextsMu.RUnlock()

	if exists {
		// Check if interaction has changed (follow-up message scenario)
		sctx.mu.Lock()
		if expectedInteractionID != "" && sctx.interactionID != "" && sctx.interactionID != expectedInteractionID {
			log.Info().
				Str("session_id", helixSessionID).
				Str("old_interaction_id", sctx.interactionID).
				Str("new_interaction_id", expectedInteractionID).
				Str("request_id", requestID).
				Msg("🔄 [PERF] Interaction transition detected! Resetting streaming context for new interaction")

			// Flush any dirty state for the old interaction before switching.
			//
			// Both branches below write through column-scoped helpers
			// (UpdateInteractionStreamingFields for content,
			// MarkInteractionCompleteIfWaiting for the auto-complete state
			// transition) so they cannot clobber a concurrent transition
			// written by handleTurnCancelled / handleMessageCompleted. The
			// in-memory sctx.interaction.State is a snapshot from when the
			// turn started — checking it here is fine because the actual
			// state mutation is guarded server-side.
			if sctx.interaction != nil {
				// Rebuild and persist any pending streaming content first.
				if sctx.accumulator != nil {
					sctx.accumulator.Rebuild()
					sctx.interaction.ResponseMessage = sctx.accumulator.Content
					sctx.interaction.LastZedMessageOffset = sctx.accumulator.Offset
					entries := sctx.accumulator.Entries()
					if len(entries) > 0 {
						if entriesJSON, err := json.Marshal(entries); err == nil {
							_ = json.Unmarshal(entriesJSON, &sctx.interaction.ResponseEntries)
						}
					}
				}

				if sctx.interaction.State == types.InteractionStateWaiting || sctx.dirty {
					if err := apiServer.Controller.Options.Store.UpdateInteractionStreamingFields(
						ctx,
						sctx.interaction.ID,
						sctx.interaction.GenerationID,
						sctx.interaction.ResponseMessage,
						sctx.interaction.ResponseEntries,
						sctx.interaction.LastZedMessageOffset,
						sctx.interaction.LastZedMessageID,
					); err != nil {
						log.Error().Err(err).
							Str("interaction_id", sctx.interactionID).
							Msg("Failed to flush old interaction during transition")
					}
				}

				// Auto-complete the old interaction if it's still Waiting.
				// Claude Code (via the Anthropic API) sometimes starts sending
				// interrupt tokens BEFORE emitting message_completed for the
				// cancelled turn. Without this, the cancelled interaction
				// stays Waiting forever and the E2E ordering check fails
				// ("interrupt tokens before first message_completed").
				//
				// The transition is guarded WHERE state='waiting' so that if
				// handleTurnCancelled has already moved it to Interrupted (or
				// handleMessageCompleted moved it to Complete / Error), this
				// is a no-op — preventing the lost-update that previously
				// resurrected cancelled turns as falsely "complete".
				if sctx.interaction.State == types.InteractionStateWaiting {
					transitioned, err := apiServer.Controller.Options.Store.MarkInteractionCompleteIfWaiting(ctx, sctx.interaction.ID, sctx.interaction.GenerationID)
					if err != nil {
						log.Error().Err(err).
							Str("interaction_id", sctx.interactionID).
							Msg("Failed to auto-complete old interaction during interrupt transition")
					} else if transitioned {
						log.Info().
							Str("interaction_id", sctx.interactionID).
							Msg("⚡ [TRANSITION] Auto-completed cancelled interaction (interrupt arrived before message_completed)")
					} else {
						log.Debug().
							Str("interaction_id", sctx.interactionID).
							Msg("⏭️ [TRANSITION] Skipped auto-complete (DB row is no longer Waiting — another handler already transitioned it)")
					}
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
			if sctx.flushTimer != nil {
				sctx.flushTimer.Stop()
				sctx.flushTimer = nil
			}
			if sctx.dbFlushTimer != nil {
				sctx.dbFlushTimer.Stop()
				sctx.dbFlushTimer = nil
			}
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

	// Find the target interaction: use request_id mapping if available
	var targetInteraction *types.Interaction
	if expectedInteractionID != "" {
		for i := range interactions {
			if interactions[i].ID == expectedInteractionID {
				targetInteraction = interactions[i]
				break
			}
		}
	}
	// Durable correlation: the in-memory request map does not survive an API
	// restart, so an agent that keeps working across one addresses its turn by a
	// request_id Helix can no longer resolve from cache. The
	// ExternalAgentRequestID column exists precisely for this.
	//
	// This also covers the case the cache can never handle: an interaction that
	// was wrongly moved to `error` while its thread stayed alive. The agent
	// naming that request_id is proof the turn is still running, so revive it
	// rather than dropping every remaining chunk as unroutable.
	//
	// Two guards keep this from resurrecting turns that are legitimately dead,
	// because Zed replays thread history as message_added on open_thread and
	// those replays carry the thread's current request_id:
	//
	//   - Completed must be zero. Every deliberate terminal decision
	//     (message_completed, turn_cancelled, thread_load_error) stamps it. Only
	//     a turn killed mid-flight without a terminal handshake leaves it unset,
	//     and that is exactly the wrongly-errored case.
	//   - The error must not be a known agent crash. If the agent process died,
	//     the turn is over; a replayed entry is not evidence otherwise.
	//
	// `complete` is never revived: that turn legitimately finished.
	if targetInteraction == nil && requestID != "" {
		for i := len(interactions) - 1; i >= 0; i-- {
			if interactions[i].ExternalAgentRequestID != requestID {
				continue
			}
			candidate := interactions[i]
			if candidate.State == types.InteractionStateError &&
				candidate.Completed.IsZero() &&
				!isAgentCrashError(candidate.Error) {
				log.Warn().
					Str("session_id", helixSessionID).
					Str("interaction_id", candidate.ID).
					Str("request_id", requestID).
					Str("previous_error", candidate.Error).
					Msg("🔄 [HELIX] Agent is still streaming a turn Helix had marked errored — reviving the interaction")
				candidate.State = types.InteractionStateWaiting
				candidate.Error = ""
				candidate.Completed = time.Time{}
				if _, err := apiServer.Controller.Options.Store.UpdateInteraction(ctx, candidate); err != nil {
					log.Error().Err(err).
						Str("interaction_id", candidate.ID).
						Msg("Failed to revive errored interaction for live turn")
					break
				}
			}
			if candidate.State == types.InteractionStateWaiting {
				targetInteraction = candidate
			}
			break
		}
	}
	// Fallback: find most recent waiting interaction (for backward compat / old Zed without request_id)
	if targetInteraction == nil {
		for i := len(interactions) - 1; i >= 0; i-- {
			if interactions[i].State == types.InteractionStateWaiting {
				targetInteraction = interactions[i]
				break
			}
		}
	}
	// After an API restart, the most recent interaction may have been marked as
	// "error"/"Interrupted" by ResetRunningInteractions. If Zed reconnects and
	// resumes sending messages, recover by reusing that interaction — reset its
	// state back to Waiting so it can continue accumulating responses.
	//
	// Deliberately only the LAST row. A more-recent turn (e.g. a cancelled
	// "interrupted" turn, or a separately errored turn) means there is no older
	// in-flight turn to resume, so we must NOT scan past it and resurrect a stale
	// interrupted interaction behind it. #2643's true cause is a divergence between
	// THIS resolver (most-recent-waiting / restart-recovery) and
	// handleMessageCompleted's request_id-mapping resolver — see
	// architecture-simplifications.md §1; broadening this scan does not fix that and
	// risks misrouting, so it stays conservative.
	if targetInteraction == nil && len(interactions) > 0 {
		last := interactions[len(interactions)-1]
		if last.State == types.InteractionStateError && last.Error == "Interrupted" {
			last.State = types.InteractionStateWaiting
			last.Error = ""
			last.Completed = time.Time{}
			if _, err := apiServer.Controller.Options.Store.UpdateInteraction(ctx, last); err != nil {
				log.Error().Err(err).
					Str("interaction_id", last.ID).
					Msg("Failed to recover interrupted interaction")
			} else {
				log.Info().
					Str("interaction_id", last.ID).
					Str("session_id", helixSessionID).
					Msg("🔄 [HELIX] Recovered interrupted interaction after API restart")
			}
			targetInteraction = last
		}
	}

	// Set interactionID for tracking transitions
	var newInteractionID string
	if targetInteraction != nil {
		newInteractionID = targetInteraction.ID
	}

	// Populate requestToInteractionMapping for Zed-initiated messages.
	// When the user types in Zed, the interaction is created without a mapping.
	// Zed reuses the same request_id for the response, so we need to register
	// it here so handleMessageCompleted can route correctly.
	//
	// CRITICAL: distinguish "request_id never seen" from "request_id already
	// consumed by handleMessageCompleted (sentinel '')". The wrapper inside Zed
	// buffers events that aren't direct ACP responses (background bash, hooks,
	// subagent completions, etc. — see auto_wake_stuck_interactions.go) and
	// flushes them later tagged with the *last* request_id it saw. Those
	// flushed events show up here with a request_id whose mapping was already
	// consumed; rebinding that consumed sentinel to the current Waiting
	// interaction defeats handleMessageCompleted's duplicate-completion dedup
	// and prematurely marks an unrelated mid-turn interaction Complete. See
	// design/2026-04-28-stale-request-id-rebind-loses-zed-updates.md.
	if requestID != "" && newInteractionID != "" {
		apiServer.contextMappingsMutex.Lock()
		if apiServer.requestToInteractionMapping == nil {
			apiServer.requestToInteractionMapping = make(map[string]string)
		}
		existing, alreadySeen := apiServer.requestToInteractionMapping[requestID]
		staleRequestID := false
		switch {
		case alreadySeen && existing == "":
			staleRequestID = true
			// Stale wrapper replay — leave the consumed sentinel in place so the
			// follow-up message_completed is dropped by the dedup. Streaming
			// tokens still flow into the current interaction via the most-recent-
			// Waiting fallback at line 1551-1558, so no content is lost.
			apiServer.contextMappingsMutex.Unlock()
			log.Debug().
				Str("session_id", helixSessionID).
				Str("request_id", requestID).
				Str("would_have_bound", newInteractionID).
				Msg("🛡️ [HELIX] Ignoring stale request_id rebind (mapping previously consumed by completion)")
		case existing != newInteractionID:
			apiServer.requestToInteractionMapping[requestID] = newInteractionID
			if apiServer.requestToSessionMapping == nil {
				apiServer.requestToSessionMapping = make(map[string]string)
			}
			apiServer.requestToSessionMapping[requestID] = helixSessionID
			apiServer.contextMappingsMutex.Unlock()
			log.Info().
				Str("session_id", helixSessionID).
				Str("request_id", requestID).
				Str("interaction_id", newInteractionID).
				Msg("🗺️ [HELIX] Populated requestToInteractionMapping from streaming context (Zed-initiated message)")
		default:
			if apiServer.requestToSessionMapping == nil {
				apiServer.requestToSessionMapping = make(map[string]string)
			}
			apiServer.requestToSessionMapping[requestID] = helixSessionID
			apiServer.contextMappingsMutex.Unlock()
		}

		if !staleRequestID && (targetInteraction.ExternalAgentRequestID != requestID || targetInteraction.ExternalAgentDispatchedAt == nil) {
			if updated, err := apiServer.Store.MarkInteractionExternalAgentDispatched(ctx, targetInteraction.ID, targetInteraction.GenerationID, requestID); err != nil {
				log.Error().Err(err).
					Str("session_id", helixSessionID).
					Str("interaction_id", targetInteraction.ID).
					Str("request_id", requestID).
					Msg("Failed to persist recovered external-agent dispatch")
			} else if updated {
				now := time.Now()
				targetInteraction.ExternalAgentRequestID = requestID
				targetInteraction.ExternalAgentDispatchedAt = &now
			}
		}
		if !staleRequestID && targetInteraction.ExternalAgentCancelRequestedAt != nil {
			go apiServer.retryPendingExternalAgentCancellation(helixSessionID, targetInteraction.ID, requestID)
		}
	}

	priorEntries := collectPriorEntries(interactions, newInteractionID)

	// If we have an existing context (from a transition), update it instead of creating new
	if exists && sctx != nil {
		sctx.mu.Lock()
		sctx.session = helixSession
		sctx.interaction = targetInteraction
		sctx.interactionID = newInteractionID
		sctx.priorEntries = priorEntries
		sctx.mu.Unlock()

		log.Info().
			Str("session_id", helixSessionID).
			Str("interaction_id", newInteractionID).
			Msg("📦 [PERF] Updated streaming context for new interaction (transition)")

		return sctx
	}

	// Create new context
	sctx = &streamingContext{
		session:       helixSession,
		interaction:   targetInteraction,
		interactionID: newInteractionID,
		priorEntries:  priorEntries,
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

// collectPriorEntries harvests (message_id, content) pairs from response_entries
// of completed interactions in this session, excluding the target interaction.
// These are seeded into the new interaction's accumulator so Zed's
// flush_streaming_throttle replays of prior-turn entries are dropped instead
// of leaking into the new interaction's response_entries.
//
// We carry content (not just IDs) because Zed's wrapper may restart and
// renumber message_ids; without comparing content, legitimate new entries
// under reused IDs would be silently dropped. See
// design/2026-04-30-queue-and-other-stuck-state-bugs.md for the empty-response
// bounce that motivated this.
func collectPriorEntries(interactions []*types.Interaction, targetInteractionID string) []wsprotocol.ResponseEntry {
	if len(interactions) == 0 {
		return nil
	}
	var entries []wsprotocol.ResponseEntry
	for _, i := range interactions {
		if i == nil || i.ID == targetInteractionID {
			continue
		}
		if len(i.ResponseEntries) == 0 {
			continue
		}
		var ie []wsprotocol.ResponseEntry
		if err := json.Unmarshal(i.ResponseEntries, &ie); err != nil {
			continue
		}
		for _, e := range ie {
			if e.MessageID != "" {
				entries = append(entries, e)
			}
		}
	}
	return entries
}

// flushAndClearStreamingContext flushes any dirty interaction state to the DB,
// then removes the cached streaming context for a session.
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

	if sctx.flushTimer != nil {
		sctx.flushTimer.Stop()
		sctx.flushTimer = nil
	}
	if sctx.dbFlushTimer != nil {
		sctx.dbFlushTimer.Stop()
		sctx.dbFlushTimer = nil
	}

	if sctx.interaction != nil {
		if sctx.dirty {
			// Rebuild Content/ResponseEntries before flushing — the streaming
			// loop defers these to the DB write throttle, so they may be stale.
			if sctx.accumulator != nil {
				sctx.accumulator.Rebuild()
				sctx.interaction.ResponseMessage = sctx.accumulator.Content
				sctx.interaction.LastZedMessageOffset = sctx.accumulator.Offset
				if entriesJSON, err := json.Marshal(sctx.accumulator.Entries()); err == nil {
					_ = json.Unmarshal(entriesJSON, &sctx.interaction.ResponseEntries)
				}
			}
			// Column-scoped write: never touch state/completed/error here,
			// so a concurrent transition from handleTurnCancelled /
			// handleMessageCompleted cannot be clobbered by this flush.
			err := apiServer.Controller.Options.Store.UpdateInteractionStreamingFields(
				ctx,
				sctx.interaction.ID,
				sctx.interaction.GenerationID,
				sctx.interaction.ResponseMessage,
				sctx.interaction.ResponseEntries,
				sctx.interaction.LastZedMessageOffset,
				sctx.interaction.LastZedMessageID,
			)
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

// sendChatMessageToExternalAgent creates a waiting interaction with a
// caller-supplied request_id and sends the WebSocket command directly, with no
// busy-check.
//
// PRODUCTION SENDS DO NOT USE THIS. All production message sending now goes
// through the session-scoped prompt queue (enqueueAgentMessage →
// processPendingPromptsForSession → sendQueuedPromptToSession), which honours
// interrupt as defer-until-idle (false) vs cancel-then-send (true). This
// function is retained ONLY as the low-level primitive the WebSocket-sync e2e
// test harness drives (test_helpers.go SendChatMessage, used by the cross-repo
// Zed e2e server which passes its own request_id and asserts routing on it).
func (apiServer *HelixAPIServer) sendChatMessageToExternalAgent(sessionID, message, requestID string, interrupt bool) (interactionID string, err error) {
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
		configSnapshot, snapshotErr := apiServer.codeAgentConfigSnapshot(ctx, session)
		if snapshotErr != nil {
			return "", fmt.Errorf("resolve code-agent configuration for chat message: %w", snapshotErr)
		}
		interaction := &types.Interaction{
			Created:                 time.Now(),
			Updated:                 time.Now(),
			SessionID:               sessionID,
			UserID:                  session.Owner,
			GenerationID:            session.GenerationID,
			Mode:                    types.SessionModeInference,
			PromptMessage:           message,
			State:                   types.InteractionStateWaiting,
			CodeAgentConfigSnapshot: configSnapshot,
			ExternalAgentRequestID:  requestID,
		}

		createdInteraction, createErr := apiServer.Controller.Options.Store.CreateInteraction(ctx, interaction)
		if createErr != nil {
			log.Warn().Err(createErr).Str("session_id", sessionID).Msg("Failed to create interaction for chat message")
		} else {
			interactionID = createdInteraction.ID
			// Notify frontend immediately so the chat updates without waiting for poll
			apiServer.publishInteractionUpdateToFrontend(sessionID, session.Owner, createdInteraction)
			// Map request_id → interaction_id so handleMessageCompleted can
			// route responses to the correct interaction.
			apiServer.contextMappingsMutex.Lock()
			if apiServer.requestToInteractionMapping == nil {
				apiServer.requestToInteractionMapping = make(map[string]string)
			}
			apiServer.requestToInteractionMapping[requestID] = interactionID
			// When there is no existing Zed thread (acpThreadID == nil), the agent
			// will create a NEW thread and emit thread_created. Register
			// request_id → session so handleThreadCreated (PRIORITY 1) reattaches
			// that new thread to THIS session instead of spawning an orphan
			// session — which is what happens after a session is cleared
			// (ClearSession resets ZedThreadID to ""). Only registered for the
			// new-thread case so we don't leak entries on normal same-thread
			// continuations, which emit no thread_created to consume the mapping.
			if acpThreadID == nil {
				if apiServer.requestToSessionMapping == nil {
					apiServer.requestToSessionMapping = make(map[string]string)
				}
				apiServer.requestToSessionMapping[requestID] = sessionID
			}
			apiServer.contextMappingsMutex.Unlock()
		}

		// Update session timestamp so findConnectedSessionForSpecTask
		// picks the most recently active session.
		_ = apiServer.Controller.Options.Store.TouchSession(ctx, sessionID)
	}

	// Forked sessions get the parent's transcript prepended on the very first
	// message (when ZedThreadID=="" so Zed will create a new thread). No-op
	// on regular sessions and on follow-up messages.
	outgoingMessage := apiServer.maybePrependTranscript(ctx, session, message)

	command := types.ExternalAgentCommand{
		Type: "chat_message",
		Data: map[string]interface{}{
			"message":        outgoingMessage,
			"request_id":     requestID,
			"acp_thread_id":  acpThreadID, // Use existing thread if available, nil = create new
			"agent_name":     agentName,   // Which agent to use (e.g., "claude", "qwen", "zed-agent")
			"interrupt":      interrupt,   // Tell agent to cancel current turn before sending (mirrors prompt-queue path)
			"interaction_id": interactionID,
			"interaction_generation_id": func() int {
				if session != nil {
					return session.GenerationID
				}
				return 0
			}(),
			"track_code_changes": session != nil && session.Metadata.SpecTaskID != "",
		},
	}

	err = apiServer.sendCommandToExternalAgent(sessionID, command)
	return interactionID, err
}

func externalAgentCommandIDs(command types.ExternalAgentCommand) (interactionID, requestID string, generationID int) {
	if command.Type != "chat_message" || command.Data == nil {
		return "", "", 0
	}
	interactionID, _ = command.Data["interaction_id"].(string)
	requestID, _ = command.Data["request_id"].(string)
	generationID, _ = command.Data["interaction_generation_id"].(int)
	return interactionID, requestID, generationID
}

// markExternalAgentCommandDispatched persists the correlation and dispatch
// boundary before a chat command enters the WebSocket queue. The returned
// rollback is used when enqueueing fails, so a later cancel can distinguish a
// durable queued turn from one the external runtime may be executing.
func (apiServer *HelixAPIServer) markExternalAgentCommandDispatched(ctx context.Context, command types.ExternalAgentCommand) (func(), error) {
	interactionID, requestID, generationID := externalAgentCommandIDs(command)
	if interactionID == "" {
		// Recovery-only continue prompts have no interaction and are outside the
		// user-turn cancellation lifecycle.
		return func() {}, nil
	}
	if requestID == "" {
		return nil, fmt.Errorf("chat_message for interaction %s has no request_id", interactionID)
	}
	updated, err := apiServer.Store.MarkInteractionExternalAgentDispatched(ctx, interactionID, generationID, requestID)
	if err != nil {
		return nil, fmt.Errorf("persist dispatch for interaction %s: %w", interactionID, err)
	}
	if !updated {
		return nil, fmt.Errorf("interaction %s is no longer waiting; refusing external-agent dispatch", interactionID)
	}
	return func() {
		if err := apiServer.Store.ClearInteractionExternalAgentDispatched(context.Background(), interactionID, generationID, requestID); err != nil {
			log.Error().Err(err).
				Str("interaction_id", interactionID).
				Str("request_id", requestID).
				Msg("Failed to roll back external-agent dispatch marker")
		}
	}, nil
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
		// No connection — auto-start the dev container if this session belongs to a spec task.
		// The caller's interaction/prompt is already persisted; the reconnect
		// resume path will deliver it when the agent reconnects via WebSocket. Wrap the sentinel so
		// callers (e.g. sendQueuedPromptToSession) can recognise this as an expected
		// transient state via errors.Is and avoid surfacing it as a queue failure.
		go apiServer.autoStartDevContainerForSession(sessionID)
		return fmt.Errorf("session %s: %w", sessionID, ErrNoExternalAgentWS)
	}

	// Capture the complete workspace before the agent receives a chat turn.
	// This is synchronous by design: doing it after the send would race the
	// agent's first file edit and make historical per-turn diffs incorrect.
	apiServer.captureInteractionBeforeCheckpoint(sessionID, command)

	interactionID, _, _ := externalAgentCommandIDs(command)
	if interactionID != "" {
		apiServer.contextMappingsMutex.Lock()
		defer apiServer.contextMappingsMutex.Unlock()
	}
	rollbackDispatch, err := apiServer.markExternalAgentCommandDispatched(context.Background(), command)
	if err != nil {
		return err
	}

	// Send command to the specific Zed agent.
	// Use a deferred recover to handle the case where the connection's SendChan
	// was closed between getConnection and the send (race on reconnection).
	var sendErr error
	func() {
		defer func() {
			if r := recover(); r != nil {
				log.Warn().
					Str("session_id", sessionID).
					Interface("panic", r).
					Msg("Recovered from panic sending to external agent (connection likely replaced during reconnect)")
				sendErr = fmt.Errorf("connection replaced during send for session %s", sessionID)
			}
		}()
		select {
		case wsConn.SendChan <- command:
			log.Trace().
				Str("session_id", sessionID).
				Str("command_type", command.Type).
				Msg("Sent command to specific external Zed agent")
		default:
			sendErr = fmt.Errorf("external agent send channel full for session %s", sessionID)
		}
	}()
	if sendErr != nil {
		rollbackDispatch()
	}
	return sendErr
}

// sendCancelToExternalAgent sends a cancel_current_turn command to Zed and waits
// for a turn_cancelled response (up to timeout). Returns the status ("cancelled" or "noop")
// or an error if the timeout expires or the command can't be sent.
func (apiServer *HelixAPIServer) sendCancelToExternalAgent(sessionID, requestID string, timeout time.Duration) (string, error) {
	// Create a channel to receive the turn_cancelled response
	ch := make(chan string, 1)
	apiServer.contextMappingsMutex.Lock()
	apiServer.pendingCancelChannels[requestID] = ch
	apiServer.contextMappingsMutex.Unlock()

	// Clean up on exit
	defer func() {
		apiServer.contextMappingsMutex.Lock()
		delete(apiServer.pendingCancelChannels, requestID)
		apiServer.contextMappingsMutex.Unlock()
	}()

	// Send cancel command
	command := types.ExternalAgentCommand{
		Type: "cancel_current_turn",
		Data: map[string]interface{}{
			"request_id": requestID,
		},
	}
	if err := apiServer.sendCommandToExternalAgent(sessionID, command); err != nil {
		return "", fmt.Errorf("failed to send cancel command: %w", err)
	}

	// Wait for response or timeout
	select {
	case status := <-ch:
		return status, nil
	case <-time.After(timeout):
		log.Warn().
			Str("session_id", sessionID).
			Str("request_id", requestID).
			Msg("Timeout waiting for turn_cancelled from Zed")
		return "", fmt.Errorf("timeout waiting for turn_cancelled")
	}
}

// handleTurnCancelled processes the turn_cancelled event from Zed
func (apiServer *HelixAPIServer) handleTurnCancelled(sessionID string, syncMsg *types.SyncMessage) error {
	requestID, _ := syncMsg.Data["request_id"].(string)
	status, _ := syncMsg.Data["status"].(string)
	if requestID == "" {
		return fmt.Errorf("turn_cancelled missing request_id")
	}
	if status != "cancelled" && status != "noop" {
		return fmt.Errorf("turn_cancelled for %s has invalid status %q", requestID, status)
	}

	log.Info().
		Str("session_id", sessionID).
		Str("request_id", requestID).
		Str("status", status).
		Msg("Received turn_cancelled from Zed")

	apiServer.contextMappingsMutex.RLock()
	interactionID := apiServer.requestToInteractionMapping[requestID]
	apiServer.contextMappingsMutex.RUnlock()

	var interaction *types.Interaction
	var err error
	if interactionID != "" {
		interaction, err = apiServer.Store.GetInteraction(context.Background(), interactionID)
	} else {
		interaction, err = apiServer.Store.GetInteractionByExternalAgentRequestID(context.Background(), requestID)
	}
	if err != nil {
		return fmt.Errorf("resolve interaction for acknowledged cancellation %s: %w", requestID, err)
	}
	if interaction == nil {
		return fmt.Errorf("resolve interaction for acknowledged cancellation %s: interaction not found", requestID)
	}

	transitioned, err := apiServer.Store.MarkInteractionInterruptedIfWaiting(context.Background(), interaction.ID, interaction.GenerationID)
	if err != nil {
		return fmt.Errorf("persist acknowledged cancellation for %s: %w", interaction.ID, err)
	}
	if transitioned {
		interaction, err = apiServer.Store.GetInteraction(context.Background(), interaction.ID)
		if err != nil {
			return fmt.Errorf("reload acknowledged cancellation for %s: %w", interaction.ID, err)
		}
		session, sessionErr := apiServer.Store.GetSession(context.Background(), interaction.SessionID)
		if sessionErr != nil {
			return fmt.Errorf("load session for acknowledged cancellation %s: %w", interaction.ID, sessionErr)
		}
		if err := apiServer.publishInteractionUpdateToFrontend(interaction.SessionID, session.Owner, interaction); err != nil {
			log.Warn().Err(err).Str("interaction_id", interaction.ID).Msg("Failed to publish acknowledged cancellation")
		}
		log.Info().
			Str("interaction_id", interaction.ID).
			Str("runtime_status", status).
			Msg("Persisted acknowledged external-agent cancellation")
	}

	// Acknowledge the HTTP caller only after the interaction state transition is
	// durable, so its immediate refetch observes the interrupted turn.
	apiServer.contextMappingsMutex.RLock()
	ch, exists := apiServer.pendingCancelChannels[requestID]
	apiServer.contextMappingsMutex.RUnlock()
	if exists {
		select {
		case ch <- status:
		default:
			// Channel full or already received — ignore
		}
	}

	go apiServer.processAnyPendingPrompt(context.Background(), interaction.SessionID)

	return nil
}

func (apiServer *HelixAPIServer) retryPendingExternalAgentCancellation(sessionID, interactionID, requestID string) {
	if interactionID == "" || requestID == "" {
		return
	}
	if _, loaded := apiServer.pendingCancelRetries.LoadOrStore(interactionID, struct{}{}); loaded {
		return
	}
	defer apiServer.pendingCancelRetries.Delete(interactionID)

	for attempt := 0; ; attempt++ {
		interaction, err := apiServer.Store.GetInteraction(context.Background(), interactionID)
		if err != nil || interaction.State != types.InteractionStateWaiting || interaction.ExternalAgentCancelRequestedAt == nil {
			return
		}
		if attempt > 0 {
			delay := time.Duration(1<<uint(min(attempt-1, 5))) * time.Second
			time.Sleep(delay)
		}
		unlock := apiServer.lockCancelTurn(sessionID)
		_, err = apiServer.sendCancelToExternalAgent(sessionID, requestID, 3*time.Second)
		unlock()
		if err == nil {
			return
		}
		if attempt < 5 || attempt%10 == 9 {
			log.Warn().Err(err).
				Str("session_id", sessionID).
				Str("interaction_id", interactionID).
				Str("request_id", requestID).
				Int("attempt", attempt+1).
				Msg("Durable external-agent cancellation retry was not acknowledged")
		}
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

// unregisterConnection unregisters an external agent connection.
// Only removes if it matches the currently registered connection,
// preventing a stale defer from closing a newer connection's channel
// after reconnection.
func (manager *ExternalAgentWSManager) unregisterConnection(sessionID string, conn *ExternalAgentWSConnection) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if current, exists := manager.connections[sessionID]; exists && current == conn {
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
	// If we don't receive agent_ready within 60s, assume ready and send anyway.
	// The zero agentTurnReport is deliberate: a timeout means the agent never
	// told us what it was running, which is exactly the "cannot tell" case
	// decideResume handles conservatively.
	state.TimeoutTimer = time.AfterFunc(60*time.Second, func() {
		log.Warn().
			Str("session_id", sessionID).
			Msg("⏰ [READINESS] Timeout waiting for agent_ready, proceeding with queued messages")
		manager.markSessionReady(sessionID, agentTurnReport{}, onReady)
	})

	manager.readinessState[sessionID] = state

	log.Trace().
		Str("session_id", sessionID).
		Bool("needs_continue", needsContinue).
		Msg("[READINESS] Initialized readiness tracking for session")
}

// markSessionReady marks a session as ready and flushes pending messages.
// report is the agent's active-turn report from agent_ready, or the zero value
// when readiness was assumed after a timeout.
func (manager *ExternalAgentWSManager) markSessionReady(sessionID string, report agentTurnReport, onReady func()) {
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
	state.Report = report

	// The resume hook must run after the pending queue is flushed (so a turn we
	// decide to deliver lands behind anything already queued) but outside the
	// lock (it does DB work and can send). Capture it here, fire it below.
	resumeHook := state.ResumeHook
	if resumeHook != nil && !state.resumeFired {
		state.resumeFired = true
	} else {
		resumeHook = nil
	}
	pendingQueue := state.PendingQueue
	state.PendingQueue = nil // Clear the queue

	log.Trace().
		Str("session_id", sessionID).
		Int("pending_count", len(pendingQueue)).
		Msg("[READINESS] Session marked as ready, flushing pending messages")

	// Enqueue pending messages before releasing readinessMu. Cancellation removes
	// a not-yet-ready chat command under this same lock; if it loses that race,
	// the chat command is guaranteed to be ahead of cancel_current_turn in the
	// WebSocket SendChan rather than being sent after a misleading noop cancel.
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
	manager.readinessMu.Unlock()

	// Decide what to do with the turn that was waiting when this connection came
	// up, now that the agent has (or has not) reported what it is running.
	if resumeHook != nil {
		resumeHook(report)
	}

	// Call the onReady callback (e.g., to send continue prompt)
	if onReady != nil {
		onReady()
	}
}

// setResumeHook installs the reconnect resume decision for a session. The
// connect handler resolves the waiting turn after readiness tracking is already
// initialised, so this can race agent_ready: if the session is already ready,
// the hook fires immediately with the report captured at that moment.
func (manager *ExternalAgentWSManager) setResumeHook(sessionID string, hook func(agentTurnReport)) {
	manager.readinessMu.Lock()
	state, exists := manager.readinessState[sessionID]
	if !exists {
		manager.readinessMu.Unlock()
		return
	}
	if state.IsReady {
		if state.resumeFired {
			manager.readinessMu.Unlock()
			return
		}
		state.resumeFired = true
		report := state.Report
		manager.readinessMu.Unlock()
		hook(report)
		return
	}
	state.ResumeHook = hook
	manager.readinessMu.Unlock()
}

// cancelQueuedChatMessage removes a chat command that has not crossed the
// readiness gate. Returning true proves the external agent never received it.
func (manager *ExternalAgentWSManager) cancelQueuedChatMessage(sessionID, requestID string) bool {
	manager.readinessMu.Lock()
	defer manager.readinessMu.Unlock()
	state := manager.readinessState[sessionID]
	if state == nil || len(state.PendingQueue) == 0 {
		return false
	}
	before := len(state.PendingQueue)
	state.PendingQueue = slices.DeleteFunc(state.PendingQueue, func(command types.ExternalAgentCommand) bool {
		return command.Type == "chat_message" && command.Data["request_id"] == requestID
	})
	return len(state.PendingQueue) < before
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

	// Match the request_id from message_completed to the correct interaction.
	//
	// Strategy (no timing — purely state-based):
	// 1. Try requestToInteractionMapping (populated by both sendMessageToSpecTaskAgent
	//    and getOrCreateStreamingContext for Zed-initiated messages).
	// 2. If no mapping, peek at the streaming context: if the agent is actively
	//    streaming to a different interaction, this message_completed is stale → skip.
	// 3. If no mapping AND no streaming context, DB fallback — but only match
	//    interactions that have response content (API restart recovery). Empty
	//    interactions are freshly-created and the message_completed is stale.
	messageRequestID, _ := syncMsg.Data["request_id"].(string)

	var targetInteractionID string

	// Step 1: Try the authoritative request_id → interaction_id mapping.
	// Consumed mappings are stored as empty strings — a stale duplicate will find
	// the consumed entry and be dropped. getOrCreateStreamingContext overwrites
	// consumed entries when a request_id is reused for a new turn.
	apiServer.contextMappingsMutex.Lock()
	var mappingConsumed bool // true if request_id was previously processed (stale duplicate)
	if messageRequestID != "" {
		if mappedID, ok := apiServer.requestToInteractionMapping[messageRequestID]; ok {
			if mappedID == "" {
				// This request_id was already consumed by a previous message_completed.
				// This is a stale duplicate (e.g. interrupt race sending two completions
				// for the same cancelled turn). Skip it.
				mappingConsumed = true
			} else {
				targetInteractionID = mappedID
				// Mark consumed (keep the key so subsequent duplicates are caught)
				apiServer.requestToInteractionMapping[messageRequestID] = ""
			}
		}
	}
	apiServer.contextMappingsMutex.Unlock()

	if mappingConsumed {
		log.Warn().
			Str("helix_session_id", helixSessionID).
			Str("request_id", messageRequestID).
			Msg("⚠️ [HELIX] Duplicate message_completed for consumed request_id mapping — ignoring")
		return nil
	}

	if targetInteractionID != "" {
		log.Info().
			Str("helix_session_id", helixSessionID).
			Str("interaction_id", targetInteractionID).
			Str("request_id", messageRequestID).
			Msg("✅ [HELIX] Matched interaction by request_id mapping")
	}

	// Step 2: No mapping found at all — DB fallback (API restart recovery, or
	// request_id was never registered in the mapping system).
	// Find the most recent waiting interaction.
	if targetInteractionID == "" {
		helixSession, err := apiServer.Controller.Options.Store.GetSession(context.Background(), helixSessionID)
		if err != nil {
			return fmt.Errorf("failed to get Helix session %s: %w", helixSessionID, err)
		}

		interactions, _, err := apiServer.Controller.Options.Store.ListInteractions(context.Background(), &types.ListInteractionsQuery{
			SessionID:    helixSessionID,
			GenerationID: helixSession.GenerationID,
			PerPage:      1000,
		})
		if err != nil {
			return fmt.Errorf("failed to list interactions for session %s: %w", helixSessionID, err)
		}

		for i := len(interactions) - 1; i >= 0; i-- {
			if interactions[i].State == types.InteractionStateWaiting {
				targetInteractionID = interactions[i].ID
				log.Info().
					Str("helix_session_id", helixSessionID).
					Str("interaction_id", interactions[i].ID).
					Msg("✅ [HELIX] Fallback: found waiting interaction via DB scan (no request_id mapping)")
				break
			}
		}
	}

	if targetInteractionID == "" {
		log.Warn().
			Str("helix_session_id", helixSessionID).
			Str("request_id", messageRequestID).
			Msg("⚠️ [HELIX] No matching interaction found for message_completed — skipping")
		// Still unblock any waiter registered under this request_id so the
		// HTTP path cannot hang until the 180s timeout.
		apiServer.signalExternalAgentResponseDone(helixSessionID, messageRequestID)
		return nil
	}

	// Always unblock waitForExternalAgentResponse for this turn, even on early
	// returns below (already-complete, interrupted, empty-response bounce).
	// Missing this signal is what left the /sessions/chat waiter blocked for
	// 180s after a successful stream and then clobbered the reply.
	// Signal under both the event's request_id and the interaction id — they
	// are usually the same after the fix, but may differ for in-flight turns.
	defer func() {
		apiServer.signalExternalAgentResponseDone(helixSessionID, messageRequestID)
		if targetInteractionID != "" && targetInteractionID != messageRequestID {
			apiServer.signalExternalAgentResponseDone(helixSessionID, targetInteractionID)
		}
	}()

	// Now that we have the target, flush the streaming context to DB.
	// This ensures the latest streamed content is persisted before we reload.
	responseEntries := apiServer.flushAndClearStreamingContext(context.Background(), helixSessionID)

	// Get the session (may already be loaded from fallback path above, but the
	// mapping path skips session loading, so load it here unconditionally).
	helixSession, err := apiServer.Controller.Options.Store.GetSession(context.Background(), helixSessionID)
	if err != nil {
		return fmt.Errorf("failed to get Helix session %s: %w", helixSessionID, err)
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

	// If the interaction was already completed (e.g. auto-completed by the streaming
	// context transition logic during an interrupt), skip redundant completion.
	if targetInteraction.State == types.InteractionStateComplete {
		log.Info().
			Str("helix_session_id", helixSessionID).
			Str("interaction_id", targetInteraction.ID).
			Msg("ℹ️ [HELIX] Interaction already complete (auto-completed during transition) — skipping")
		return nil
	}

	// An interaction already in Interrupted state is the expected terminal state
	// after handleTurnCancelled — message_completed arrives shortly after as the
	// agent finishes in-flight work, but the response can legitimately be empty
	// if the cancel landed before any token streamed (Phase 13 of the Zed WS E2E
	// test exercises exactly this race). Don't treat that as a bounce.
	if targetInteraction.State == types.InteractionStateInterrupted &&
		targetInteraction.ResponseMessage == "" && len(responseEntries) == 0 {
		log.Info().
			Str("helix_session_id", helixSessionID).
			Str("interaction_id", targetInteraction.ID).
			Msg("⏭️ [HELIX] message_completed for already-interrupted interaction with empty response — preserving interrupted state")
		return nil
	}

	// Empty response: mark as error and re-queue. If a prior
	// chat_response_error already populated a real error, preserve it.
	if targetInteraction.ResponseMessage == "" && len(responseEntries) == 0 {
		if targetInteraction.State == types.InteractionStateError && targetInteraction.Error != "" {
			log.Info().
				Str("helix_session_id", helixSessionID).
				Str("interaction_id", targetInteraction.ID).
				Str("preserved_error", targetInteraction.Error).
				Msg("message_completed: preserving prior chat_response_error")
			return nil
		}

		log.Warn().
			Str("helix_session_id", helixSessionID).
			Str("interaction_id", targetInteraction.ID).
			Msg("⚠️ [HELIX] message_completed with EMPTY response — marking as error and re-queuing")

		targetInteraction.State = types.InteractionStateError
		targetInteraction.Error = "Agent unresponsive: it returned an empty response. Retrying automatically."
		targetInteraction.Updated = time.Now()

		if _, err := apiServer.Controller.Options.Store.UpdateInteraction(context.Background(), targetInteraction); err != nil {
			return fmt.Errorf("failed to update bounced interaction %s: %w", targetInteraction.ID, err)
		}

		// Re-queue the bounced prompt so it will be retried (non-fatal if no matching prompt)
		if err := apiServer.Controller.Options.Store.RequeueBouncedPrompt(context.Background(), helixSessionID); err != nil {
			log.Debug().Err(err).
				Str("session_id", helixSessionID).
				Msg("No prompt_history_entry to re-queue (Zed user message or already retried)")
		}

		// Publish the error state to frontend
		apiServer.publishInteractionUpdateToFrontend(helixSessionID, helixSession.Owner, targetInteraction)
		_ = apiServer.publishSessionUpdateToFrontend(helixSession, targetInteraction)

		// Trigger queue processing so the re-queued prompt gets picked up
		go apiServer.processPromptQueue(context.Background(), helixSessionID)

		return nil
	}

	// Mark the interaction as complete — but don't overwrite "interrupted" state.
	// When a turn is cancelled, handleTurnCancelled marks the interaction as
	// interrupted. The message_completed event may arrive shortly after (the agent
	// finishes its in-flight work), and we must not clobber the interrupted state.
	if targetInteraction.State == types.InteractionStateInterrupted {
		log.Info().
			Str("helix_session_id", helixSessionID).
			Str("interaction_id", targetInteraction.ID).
			Msg("⏭️ [HELIX] Interaction already interrupted — skipping state transition to complete")
	} else {
		targetInteraction.State = types.InteractionStateComplete
	}
	// Clear a premature waiter timeout (or any non-fatal error stamp) once the
	// agent actually finishes. The HTTP/SSE waiter used to mark
	// "External agent response timeout" at 180s while the WS path kept
	// streaming for many more minutes — leaving Error set on a complete row.
	targetInteraction.Error = ""
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

	// Complete the immutable before/after workspace receipt before publishing
	// the terminal interaction update, so the chat never needs to derive an old
	// turn from the current working tree.
	apiServer.finalizeInteractionCodeChanges(context.Background(), helixSessionID, targetInteraction)

	// Snapshot the effective model before this interaction becomes idle. A
	// model switch keeps the same Helix session, so reading the task later would
	// otherwise attribute this completed turn to the newly selected model.
	if targetInteraction.CodeAgentConfigSnapshot == nil {
		snapshot, snapshotErr := apiServer.codeAgentConfigSnapshot(context.Background(), helixSession)
		if snapshotErr != nil {
			return snapshotErr
		}
		targetInteraction.CodeAgentConfigSnapshot = snapshot
	}
	if err := applyACPInteractionUsage(targetInteraction, syncMsg); err != nil {
		return err
	}
	if err := apiServer.applyACPTotalProcessedUsage(context.Background(), targetInteraction); err != nil {
		return err
	}

	// A completed turn proves any latched launch failure on the owning spec task
	// is no longer true. Work can resume through session inference (the chat's
	// Retry, or just a message), which never touches the task, leaving it at
	// backlog with a stale error while its agent runs.
	apiServer.reconcileSpecTaskAfterTurn(context.Background(), helixSession)

	_, err = apiServer.Controller.Options.Store.UpdateInteraction(context.Background(), targetInteraction)
	if err != nil {
		return fmt.Errorf("failed to update interaction %s: %w", targetInteraction.ID, err)
	}

	// Reaching message_completed means the agent is alive and produced a turn,
	// so any consumed auto-restart budget is refunded: a recovered autonomous
	// session that runs a successful turn gets a fresh restart budget. The
	// counter lives on the session (not the prompt) precisely so it survives
	// ResetCrashedPromptsForSession and only clears here, on proven success.
	if helixSession.Metadata.AutoRestartOnCrash && helixSession.Metadata.AutoRestartCount > 0 {
		helixSession.Metadata.AutoRestartCount = 0
		if _, err := apiServer.Controller.Options.Store.UpdateSession(context.Background(), *helixSession); err != nil {
			log.Warn().Err(err).Str("helix_session_id", helixSessionID).
				Msg("Failed to reset auto-restart budget after successful turn")
		} else {
			log.Info().Str("helix_session_id", helixSessionID).
				Msg("♻️ [HELIX] Reset auto-restart budget after successful turn")
		}
	}

	// Continuous reconciliation: if the queued prompt was never marked sent
	// via handleMessageAdded (e.g. the agent went straight to message_completed
	// without intermediate streaming, or the streaming path missed it), this
	// is the last reliable point at which we know the interaction completed
	// successfully. Idempotent — a no-op for prompts already in 'sent'.
	if targetInteraction.PromptID != "" {
		if markErr := apiServer.Controller.Options.Store.MarkPromptAsSent(context.Background(), targetInteraction.PromptID); markErr != nil {
			log.Warn().Err(markErr).
				Str("prompt_id", targetInteraction.PromptID).
				Str("interaction_id", targetInteraction.ID).
				Msg("Failed to mark prompt as sent at message_completed reconciliation")
		}
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

		// Emit attention event: agent interaction completed.
		//
		// Skip the notification when the user is clearly already active in the
		// session — either they interrupted the agent (sent a new message that
		// caused this completion) or they sent a quick follow-up. In both cases
		// a newer interaction with state=waiting already exists, and notifying
		// is just noise because the user is looking right at the UI.
		skipAttention := false
		latestInteractions, _, listErr := apiServer.Controller.Options.Store.ListInteractions(context.Background(), &types.ListInteractionsQuery{
			SessionID:    helixSessionID,
			GenerationID: helixSession.GenerationID,
			PerPage:      1,
		})
		if listErr != nil {
			log.Warn().Err(listErr).
				Str("session_id", helixSessionID).
				Msg("Failed to list interactions for attention-event suppression check; emitting event as default")
		} else if len(latestInteractions) > 0 {
			latest := latestInteractions[len(latestInteractions)-1]
			if latest.State == types.InteractionStateWaiting && latest.Created.After(targetInteraction.Created) {
				log.Debug().
					Str("session_id", helixSessionID).
					Str("completed_interaction_id", targetInteraction.ID).
					Str("newer_waiting_interaction_id", latest.ID).
					Msg("Skipping agent_interaction_completed attention event: user already active in session")
				skipAttention = true
			}
		}

		if !skipAttention && apiServer.attentionService != nil {
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
	// (messageRequestID already extracted above for FIFO queue matching)

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

	// doneChan is signaled via the defer above (covers this success path and
	// every early return after targetInteractionID is resolved).

	// Process next non-interrupt prompt from queue (if any)
	go apiServer.processPromptQueue(context.Background(), helixSessionID)

	if err := apiServer.recordACPUsage(context.Background(), helixSession, targetInteraction, syncMsg); err != nil {
		return err
	}

	return nil
}

// lockPromptDrain serialises queue-drain dispatch for a single session and
// returns the unlock func (call via defer). Without it, two drain goroutines —
// e.g. message_completed → processPromptQueue and agent_ready →
// processAnyPendingPrompt — can each atomically claim a DIFFERENT pending prompt
// and dispatch both to Zed at the same instant. Zed then serialises them in
// arrival order, which is non-deterministic w.r.t. submission order, producing
// out-of-order interactions (the streaming one ends up above an already-complete
// one in the transcript). Holding this lock across claim → [cancel] → send →
// CreateInteraction guarantees the next drain's busy re-check (in
// sendQueuedPromptToSession) observes the committed Waiting interaction and
// defers (queue mode) or cancels the freshly-started turn in order (interrupt
// mode). See design/2026-06-23-queue-drain-out-of-order-dispatch.md.
func (apiServer *HelixAPIServer) lockPromptDrain(sessionID string) func() {
	muIface, _ := apiServer.promptDrainMutexes.LoadOrStore(sessionID, &sync.Mutex{})
	mu := muIface.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// requeueUndispatchedPrompt records the outcome of a failed dispatch attempt for
// a prompt that was already claimed (status='sending'), and is the ONLY place the
// three drain paths decide between "put it back" and "count it as a failure".
//
// A busy-defer means the prompt was never sent and nothing is wrong, so it goes
// straight back to pending with its retry budget intact. Anything else is a real
// failure and is recorded with backoff so the UI can show "Failed - retrying".
func (apiServer *HelixAPIServer) requeueUndispatchedPrompt(ctx context.Context, sessionID string, prompt *types.PromptHistoryEntry, dispatchErr error) {
	if errors.Is(dispatchErr, ErrPromptBusyDeferred) {
		log.Info().
			Str("session_id", sessionID).
			Str("prompt_id", prompt.ID).
			Int("retry_count", prompt.RetryCount).
			Msg("⏸️  [QUEUE] Session busy — returning prompt to queue without charging the retry budget")
		if revertErr := apiServer.Store.RevertPromptToPending(ctx, prompt.ID); revertErr != nil {
			log.Error().Err(revertErr).Str("prompt_id", prompt.ID).Msg("Failed to revert deferred prompt to pending")
		}
		return
	}

	log.Error().
		Err(dispatchErr).
		Str("session_id", sessionID).
		Str("prompt_id", prompt.ID).
		Msg("Failed to dispatch queued prompt to session")

	// Mark as failed (records error so the UI can show it under "Failed - retrying")
	if markErr := apiServer.Store.MarkPromptAsFailed(ctx, prompt.ID, dispatchErr.Error()); markErr != nil {
		log.Error().Err(markErr).Str("prompt_id", prompt.ID).Msg("Failed to mark prompt as failed")
	}
}

// processPromptQueue checks for pending non-interrupt prompts and sends the next one
// This is called after a message is completed to process queued non-interrupt messages
func (apiServer *HelixAPIServer) processPromptQueue(ctx context.Context, sessionID string) {
	// Serialise drains for this session — see lockPromptDrain. Acquired before
	// the busy-check so the check + claim + dispatch are atomic w.r.t. other drains.
	defer apiServer.lockPromptDrain(sessionID)()

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
		// CRITICAL: Order=id DESC so PerPage=1 returns the NEWEST interaction.
		// Without the explicit order, ListInteractions returns id ASC (oldest
		// first) and we would ALWAYS check the very first interaction in the
		// session — which has been Complete for hours — so the busy check
		// would never fire and queue prompts would dispatch on top of an
		// actively-streaming Zed turn.
		interactions, _, err := apiServer.Store.ListInteractions(ctx, &types.ListInteractionsQuery{
			SessionID:    sessionID,
			GenerationID: session.GenerationID,
			PerPage:      1,
			Order:        "id DESC",
		})
		if err != nil {
			log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to list interactions for queue processing")
			return
		}
		if len(interactions) > 0 && interactions[0].State == types.InteractionStateWaiting {
			log.Info().
				Str("session_id", sessionID).
				Str("interaction_id", interactions[0].ID).
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
		apiServer.requeueUndispatchedPrompt(ctx, sessionID, nextPrompt, err)
		return
	}

	// NOTE: Don't mark as sent here. sendQueuedPromptToSession persists the
	// interaction with PromptID set; handleMessageAdded reads that column and
	// marks the prompt 'sent' when Zed actually starts streaming a response,
	// so the queue UI keeps showing it as in-flight until then.

	log.Info().
		Str("session_id", sessionID).
		Str("prompt_id", nextPrompt.ID).
		Msg("✅ [QUEUE] Successfully dispatched queued prompt to session (waiting for Zed to start)")
}

// processAnyPendingPrompt checks for any pending prompt (interrupt or non-interrupt) and sends it
// This is used when the session is idle to process ALL pending prompts, not just non-interrupt ones
func (apiServer *HelixAPIServer) processAnyPendingPrompt(ctx context.Context, sessionID string) {
	// Serialise drains for this session — see lockPromptDrain. Without this, this
	// readiness-path drain can claim one pending prompt while processPromptQueue
	// concurrently claims another and both dispatch to Zed at once, reordering them.
	defer apiServer.lockPromptDrain(sessionID)()

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
	// This drain runs on every acknowledged cancellation, so a busy-defer here is
	// routine — requeueUndispatchedPrompt keeps it off the retry budget.
	if err := apiServer.sendQueuedPromptToSession(ctx, sessionID, nextPrompt); err != nil {
		apiServer.requeueUndispatchedPrompt(ctx, sessionID, nextPrompt, err)
		return
	}

	// NOTE: Don't mark as sent here — see comment in processPromptQueue. The
	// prompt is marked sent when Zed sends the first message_added, by
	// handleMessageAdded reading the Interaction.PromptID column.

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

	// Drop queued prompts targeting a paused session. The user paused the
	// session (or it was paused by being forked) after this prompt was
	// queued; delivering it now would resurrect a frozen checkpoint.
	if session.Metadata.Paused {
		log.Info().
			Str("session_id", sessionID).
			Str("prompt_id", prompt.ID).
			Str("paused_reason", session.Metadata.PausedReason).
			Msg("queue: dropping queued prompt — target session is paused")
		if markErr := apiServer.Store.MarkPromptAsFailed(ctx, prompt.ID,
			fmt.Sprintf("session paused (%s)", session.Metadata.PausedReason)); markErr != nil {
			log.Warn().Err(markErr).Str("prompt_id", prompt.ID).Msg("queue: failed to mark prompt failed after paused-session drop")
		}
		return nil
	}

	// Re-check session idle state right before creating the interaction.
	// A Zed user message may have arrived between processPromptQueue's initial
	// check and now, creating a new Waiting interaction. Without this guard the
	// queue prompt would be sent while the agent is already processing the Zed
	// user message, causing the agent to bounce it with an empty response.
	// See: design/2026-04-16-lost-responses-race-condition.md
	//
	// CRITICAL: Order=id DESC so PerPage=1 returns the NEWEST interaction.
	// ListInteractions defaults to id ASC (oldest first) — without the explicit
	// order, we would get the very first interaction in the session (always
	// Complete) and the busy check would never fire.
	//
	// Interrupt prompts (Cmd/Ctrl+Enter on the spec-task chat) are exempt:
	// their entire purpose is to land while the agent is mid-turn. The Zed
	// e2e Phase 8 test (zed/crates/external_websocket_sync/e2e-test/
	// helix-ws-test-server/main.go:780) covers exactly this — sends the
	// interrupt the moment the first assistant token arrives and asserts
	// the cancelled turn's message_completed AND the interrupt's
	// message_completed both arrive. If we defer here the interrupt
	// silently fails, the frontend marks it as failed, and the user has
	// to manually retry. Pre-2026-04-26 the guard was latent because of
	// the ASC/DESC ListInteractions ordering bug fixed in 853492e14;
	// fixing the ordering exposed this missing branch.
	//
	// BOOT-RACE EXCEPTION to the interrupt exemption: the interrupt bypass is only
	// safe once the Zed thread EXISTS. The very first message of a session is sent
	// with an empty acp_thread_id — that empty id is what makes Zed create the
	// thread. If a second message (even an interrupt) is dispatched before
	// thread_created has populated ZedThreadID, it ALSO goes out with an empty
	// acp_thread_id and Zed forks a SECOND, divorced thread: the initial message's
	// work lands in thread A, the follow-up in thread B, and the agent answers the
	// follow-up with "a previous conversation context that I don't have". So until
	// the thread is established, even an interrupt must respect the busy-defer; it
	// is redelivered into the SAME thread once ZedThreadID is set. The poller-side
	// barrier in prompt_history_handlers.go stops the interrupt path; this guards
	// the processAnyPendingPrompt / readiness path that funnels here too.
	// See design/2026-06-19-incident-interrupt-during-boot-context-loss.md.
	threadNotEstablished := session.Metadata.ZedThreadID == ""
	if !prompt.Interrupt || threadNotEstablished {
		latestInteractions, _, recheckErr := apiServer.Store.ListInteractions(ctx, &types.ListInteractionsQuery{
			SessionID:    sessionID,
			GenerationID: session.GenerationID,
			PerPage:      1,
			Order:        "id DESC",
		})
		if recheckErr == nil && len(latestInteractions) > 0 && latestInteractions[0].State == types.InteractionStateWaiting {
			// Before reporting "busy", check whether this Waiting interaction is
			// the one we already created for THIS prompt on a previous dispatch
			// attempt (e.g. the no-WS path persisted I1, the agent then connected
			// and the reconnect resume path sent it, and now the prompt's retry
			// timer is firing while I1 is still mid-turn). The PromptID column
			// on the interaction is the authoritative link to the originating
			// prompt; if it points back at us, the message is already in flight
			// and we must NOT create a duplicate. handleMessageAdded will mark
			// this prompt 'sent' when Zed acknowledges the existing interaction.
			if latestInteractions[0].PromptID == prompt.ID {
				log.Info().
					Str("session_id", sessionID).
					Str("interaction_id", latestInteractions[0].ID).
					Str("prompt_id", prompt.ID).
					Msg("✅ [QUEUE] Prompt is already in flight via existing Waiting interaction — skipping duplicate dispatch")
				return nil
			}
			return fmt.Errorf("session %s became busy (interaction %s is Waiting), deferring queue prompt: %w", sessionID, latestInteractions[0].ID, ErrPromptBusyDeferred)
		}
	}

	// CRITICAL: Create an interaction BEFORE sending the message
	// This ensures that when the agent responds, handleMessageAdded has an interaction to update.
	// PromptID persists the link back to the prompt_history_entry so handleMessageAdded /
	// handleMessageCompleted can mark the prompt 'sent' from the DB row, not from an in-memory
	// map that doesn't survive API restart. Replaces interactionToPromptMapping.
	// See design/2026-04-30-queue-and-other-stuck-state-bugs.md.
	configSnapshot, err := apiServer.codeAgentConfigSnapshot(ctx, session)
	if err != nil {
		return fmt.Errorf("resolve code-agent configuration for queue prompt: %w", err)
	}
	interactionID := system.GenerateInteractionID()
	interaction := &types.Interaction{
		ID:                      interactionID,
		Created:                 time.Now(),
		Updated:                 time.Now(),
		Scheduled:               time.Now(),
		SessionID:               sessionID,
		UserID:                  session.Owner,
		GenerationID:            session.GenerationID, // Must match session's generation for query to find it
		Mode:                    types.SessionModeInference,
		PromptMessage:           prompt.Content,
		State:                   types.InteractionStateWaiting,
		PromptID:                prompt.ID,
		CodeAgentConfigSnapshot: configSnapshot,
		ExternalAgentRequestID:  interactionID,
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

	// Determine agent name
	agentName := apiServer.getAgentNameForSession(ctx, session)

	// Use interaction ID as request ID for better tracing
	requestID := createdInteraction.ExternalAgentRequestID

	// Commenter response routing + design-review comment linkage. These carry the
	// context the old synchronous direct send set up at call time; on the queue
	// path we set them at dispatch, once the interaction (and its request_id)
	// exist. NotifyUserID on the prompt row is the user the response should be
	// streamed to (a design-review commenter); the comment linkage backfills the
	// comment's RequestID/InteractionID so all the existing comment finalize /
	// streaming / timeout / reconcile machinery (which keys off those fields)
	// keeps working unchanged.
	if prompt.NotifyUserID != "" {
		apiServer.contextMappingsMutex.Lock()
		if apiServer.requestToCommenterMapping == nil {
			apiServer.requestToCommenterMapping = make(map[string]string)
		}
		apiServer.requestToCommenterMapping[requestID] = prompt.NotifyUserID
		if apiServer.sessionToCommenterMapping == nil {
			apiServer.sessionToCommenterMapping = make(map[string]string)
		}
		apiServer.sessionToCommenterMapping[sessionID] = prompt.NotifyUserID
		apiServer.contextMappingsMutex.Unlock()
	}
	apiServer.backfillCommentLinkageForPrompt(ctx, prompt.ID, requestID, createdInteraction.ID)

	// Store request_id->session mapping so thread_created can find the right session
	// (needed for the FIRST message when ZedThreadID is empty and Zed will create a
	// new thread). The interaction → prompt link no longer lives in an in-memory map;
	// it's persisted on the Interaction.PromptID column at create time, so it
	// survives API restart and reconnect-driven re-delivery.
	apiServer.contextMappingsMutex.Lock()
	if apiServer.requestToSessionMapping == nil {
		apiServer.requestToSessionMapping = make(map[string]string)
	}
	apiServer.requestToSessionMapping[requestID] = sessionID
	// Map request_id → interaction_id for FIFO queue matching
	if apiServer.requestToInteractionMapping == nil {
		apiServer.requestToInteractionMapping = make(map[string]string)
	}
	apiServer.requestToInteractionMapping[requestID] = createdInteraction.ID
	apiServer.contextMappingsMutex.Unlock()
	log.Info().
		Str("request_id", requestID).
		Str("session_id", sessionID).
		Msg("🔗 [QUEUE] Stored request_id->session mapping for thread creation")

	// Forked sessions: prepend parent transcript on the first outgoing message
	// (when ZedThreadID is empty so Zed will create a new thread). No-op on
	// regular / follow-up sends. maybePrependTranscript loads the seed from
	// the fork_seed interaction's ResponseMessage exactly once.
	outgoingMessage := apiServer.maybePrependTranscript(ctx, session, prompt.Content)

	// Create the command to send to the external agent
	// NOTE: acp_thread_id can be empty on first message - this triggers thread creation in Zed
	command := types.ExternalAgentCommand{
		Type: "chat_message",
		Data: map[string]interface{}{
			"acp_thread_id":             session.Metadata.ZedThreadID, // Empty on first message triggers thread creation
			"message":                   outgoingMessage,
			"request_id":                requestID,
			"agent_name":                agentName,
			"from_queue":                true,             // Indicate this came from the queue
			"interrupt":                 prompt.Interrupt, // Tell Zed to cancel the current turn before sending
			"interaction_id":            createdInteraction.ID,
			"interaction_generation_id": createdInteraction.GenerationID,
			"track_code_changes":        session.Metadata.SpecTaskID != "",
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
	// adds session_id to data, and updates agent work state.
	//
	// Two failure modes need different treatment:
	//
	//  1. No WebSocket connection (ErrNoExternalAgentWS): the agent is sleeping
	//     or still booting. sendCommandToExternalAgent has already kicked off
	//     autoStartDevContainerForSession; the reconnect resume path will deliver
	//     the persisted Waiting interaction once the agent reconnects. The
	//     Interaction.PromptID column we set on createInteraction survives the
	//     failure (it's in the DB row), so when Zed acknowledges I1 the prompt
	//     gets marked 'sent' via handleMessageAdded reading the column. We must
	//     NOT mark the prompt as failed here — doing so used to surface a
	//     misleading "no WebSocket connection" error and trigger a retry that
	//     collided with the in-flight interaction (PR #2311).
	//
	//  2. Real dispatch failures (channel full, connection replaced mid-send):
	//     the connection exists but the send didn't take. Mark the prompt failed
	//     so the queue's exponential-backoff retry kicks in. Tear down the
	//     request_id mappings so the next attempt starts clean.
	if err := apiServer.sendCommandToExternalAgent(sessionID, command); err != nil {
		if errors.Is(err, ErrNoExternalAgentWS) {
			log.Info().
				Str("session_id", sessionID).
				Str("interaction_id", createdInteraction.ID).
				Str("prompt_id", prompt.ID).
				Msg("⏸️ [QUEUE] Agent not connected — interaction persisted, awaiting the reconnect resume path")
			return nil
		}

		log.Warn().Err(err).
			Str("session_id", sessionID).
			Str("interaction_id", createdInteraction.ID).
			Str("prompt_id", prompt.ID).
			Msg("❌ [QUEUE] Failed to send to agent (real dispatch failure, will retry)")

		// Clean up request_id mappings so the next retry starts clean. Without
		// this, stale message_completed events from a prior agent context could
		// match this interaction. The interaction → prompt link lives in the
		// DB column, so it survives this cleanup.
		apiServer.contextMappingsMutex.Lock()
		delete(apiServer.requestToSessionMapping, requestID)
		delete(apiServer.requestToInteractionMapping, requestID)
		apiServer.contextMappingsMutex.Unlock()

		if markErr := apiServer.Store.MarkPromptAsFailed(context.Background(), prompt.ID, err.Error()); markErr != nil {
			log.Error().Err(markErr).Str("prompt_id", prompt.ID).Msg("Failed to mark prompt as failed after dispatch failure")
		}
		return nil
	}

	return nil
}

// autoStartDevContainerForSession boots the dev container for any zed_external
// session that has no live WebSocket connection. Fire-and-forget — the caller's
// message is already persisted and will be picked up by the reconnect resume path
// when the agent reconnects.
//
// Handles three session shapes via startDevContainerForSession:
//   - spec-task sessions  (session.Metadata.SpecTaskID set)
//   - exploratory sessions (session.Metadata.ProjectID set)
//   - legacy sessions      (session.ProjectID set)
//
// Sessions with none of the above are logged and skipped (we cannot invent project
// config). Non-zed_external sessions are also skipped (they have no desktop to wake).
func (apiServer *HelixAPIServer) autoStartDevContainerForSession(sessionID string) {
	ctx := context.Background()
	session, err := apiServer.Controller.Options.Store.GetSession(ctx, sessionID)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("autoStartDevContainerForSession: failed to get session")
		return
	}
	if session == nil {
		log.Warn().Str("session_id", sessionID).Msg("autoStartDevContainerForSession: session is nil")
		return
	}
	if session.Metadata.AgentType != "zed_external" {
		log.Debug().
			Str("session_id", sessionID).
			Str("agent_type", session.Metadata.AgentType).
			Msg("autoStartDevContainerForSession: not a zed_external session, skipping")
		return
	}

	log.Info().
		Str("session_id", sessionID).
		Str("spec_task_id", session.Metadata.SpecTaskID).
		Bool("has_spec_task", session.Metadata.SpecTaskID != "").
		Msg("Auto-starting dev container for session with no WebSocket connection")

	if startErr := apiServer.startDevContainerForSession(ctx, session); startErr != nil {
		log.Error().Err(startErr).
			Str("session_id", sessionID).
			Str("spec_task_id", session.Metadata.SpecTaskID).
			Msg("Failed to auto-start dev container")
	}
}

// truncateString truncates a string to maxLen characters
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// recoverMissingThread clears an authoritative stale ACP thread and replays
// its existing waiting interaction as the first message of a replacement
// thread. The persisted thread ID prevents duplicate errors from replaying it.
// replayableInteraction resolves the turn requestID names, if this session is
// still waiting on it — the turn a thread recovery replays. Returns nil when
// there is nothing to replay (no request id, turn already finished, or the id
// belongs to another session's turn).
func (apiServer *HelixAPIServer) replayableInteraction(ctx context.Context, sessionID, requestID string) (*types.Interaction, error) {
	apiServer.contextMappingsMutex.RLock()
	interactionID := apiServer.requestToInteractionMapping[requestID]
	apiServer.contextMappingsMutex.RUnlock()
	if interactionID == "" {
		interactionID = requestID
	}
	if interactionID == "" {
		return nil, nil
	}
	interaction, err := apiServer.Controller.Options.Store.GetInteraction(ctx, interactionID)
	if err != nil {
		return nil, fmt.Errorf("load interaction for stale thread recovery: %w", err)
	}
	if interaction == nil || interaction.State != types.InteractionStateWaiting {
		return nil, nil
	}
	if interaction.SessionID != "" && interaction.SessionID != sessionID {
		return nil, nil
	}
	return interaction, nil
}

// reseedContext asks for the replayed turn to carry a summary of what the agent
// was doing. It is set only when the agent CANNOT restore sessions at all: for
// a thread that merely went missing, the agent still keeps its own session
// store, and paying tokens to re-explain the task on every such recovery is not
// obviously wanted. See buildThreadReseedPreamble.
func (apiServer *HelixAPIServer) recoverMissingThread(ctx context.Context, sessionID, acpThreadID, requestID string, reseedContext bool) (bool, error) {
	session, err := apiServer.Controller.Options.Store.GetSession(ctx, sessionID)
	if err != nil {
		return false, fmt.Errorf("load session for stale thread recovery: %w", err)
	}
	if session == nil || session.Metadata.ZedThreadID != acpThreadID {
		return true, nil
	}

	// With no turn to replay there is nothing to carry context on, and for an
	// agent that cannot restore sessions the dead thread id is the only thing
	// that will tell us to reseed. Clearing it here would let the user's NEXT
	// message open a fresh thread quietly — no error, no context, exactly the
	// silent amnesia the reseed exists to prevent. Leaving it costs that message
	// one failed load (no model call, sub-second) and routes it back here with a
	// request_id, which is the path that seeds. A thread that merely went
	// missing has no such need, so it is still cleared eagerly.
	interaction, err := apiServer.replayableInteraction(ctx, sessionID, requestID)
	if err != nil {
		return false, err
	}
	if reseedContext && interaction == nil {
		log.Info().Str("session_id", sessionID).Str("unrestorable_thread_id", acpThreadID).
			Msg("Keeping unrestorable ACP thread id so the next message is reseeded rather than silently starting fresh")
		return true, nil
	}

	session.Metadata.ZedThreadID = ""
	if err := apiServer.Controller.Options.Store.UpdateSessionMetadata(ctx, sessionID, session.Metadata); err != nil {
		return false, fmt.Errorf("clear stale ACP thread: %w", err)
	}

	apiServer.contextMappingsMutex.Lock()
	delete(apiServer.contextMappings, acpThreadID)
	apiServer.contextMappingsMutex.Unlock()
	if interaction == nil {
		return true, nil
	}

	apiServer.contextMappingsMutex.Lock()
	apiServer.requestToSessionMapping[requestID] = sessionID
	apiServer.requestToInteractionMapping[requestID] = interaction.ID
	apiServer.contextMappingsMutex.Unlock()

	apiServer.externalAgentWSManager.readinessMu.Lock()
	if state := apiServer.externalAgentWSManager.readinessState[sessionID]; state != nil {
		state.PendingQueue = slices.DeleteFunc(state.PendingQueue, func(command types.ExternalAgentCommand) bool {
			return command.Type == "chat_message" && command.Data["request_id"] == requestID
		})
	}
	apiServer.externalAgentWSManager.readinessMu.Unlock()

	// The replay lands in a brand-new agent session, so whatever the agent knew
	// about this task died with the old one. Rebuild enough of it to carry on —
	// otherwise the turn succeeds mechanically and the agent answers a follow-up
	// with no idea what it was working on.
	message := interaction.PromptMessage
	if reseedContext {
		if preamble := apiServer.buildThreadReseedPreamble(ctx, session, interaction.ID); preamble != "" {
			message = preamble + message
		}
	}
	if interaction.SystemPrompt != "" {
		message = interaction.SystemPrompt + "\n\n**User Request:**\n" + message
	}
	command := types.ExternalAgentCommand{
		Type: "chat_message",
		Data: map[string]interface{}{
			"message":                   message,
			"request_id":                requestID,
			"interaction_id":            interaction.ID,
			"interaction_generation_id": interaction.GenerationID,
			"agent_name":                apiServer.getAgentNameForSession(ctx, session),
		},
	}
	if err := apiServer.sendCommandToExternalAgent(sessionID, command); err != nil {
		if errors.Is(err, ErrNoExternalAgentWS) {
			return true, nil
		}
		log.Warn().Err(err).Str("session_id", sessionID).Str("interaction_id", interaction.ID).
			Msg("Failed to replay interaction after clearing stale ACP thread")
		return false, fmt.Errorf("replay interaction after clearing stale ACP thread: %w", err)
	}
	log.Info().Str("session_id", sessionID).Str("stale_thread_id", acpThreadID).
		Str("interaction_id", interaction.ID).Str("request_id", requestID).
		Msg("Replayed interaction as first message after clearing stale ACP thread")
	return true, nil
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
	// Both conditions mean the same thing for delivery: this thread can never be
	// loaded again, so the turn must be replayed into a new one rather than
	// failed. They differ only in why — the thread is gone, or the agent has no
	// way to re-enter it (see isUnrestorableThreadError).
	if (isAuthoritativeMissingThreadError(errorMsg) || isUnrestorableThreadError(errorMsg)) && acpThreadID != "" {
		recoverySessionID := helixSessionID
		if recoverySessionID == "" {
			recoverySessionID = sessionID
		}
		helixSessionID = recoverySessionID
		handled, err := apiServer.recoverMissingThread(context.Background(), recoverySessionID, acpThreadID, requestID, isUnrestorableThreadError(errorMsg))
		if err != nil {
			return err
		}
		if handled {
			return nil
		}
	}
	taskSessionID := helixSessionID
	if taskSessionID == "" {
		taskSessionID = sessionID
	}
	apiServer.failRunningTriggerExecution(taskSessionID, fmt.Sprintf("Thread load failed: %s", errorMsg))

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
			// A hard agent crash ("Claude Agent process exited", "Session not
			// found") on an autonomous session must auto-recover regardless of how
			// the crashing turn was sent — a queued planning prompt (PromptID set)
			// OR a blocking follow-up via handleBlockingSession (no PromptID). The
			// prompt crash-marking below is rightly PromptID-gated, but the RESTART
			// decision is not: it keys only on "crash + autonomous". maybeAutoRestart
			// self-gates on the flag and dedupes, so this is safe even when the
			// in-loop crash-mark path also fires.
			if isAgentCrashError(errorMsg) && helixSession.Metadata.AutoRestartOnCrash {
				go apiServer.maybeAutoRestartCrashedAgent(helixSessionID)
			}
			// Resolve the turn this failure actually names. Selecting by
			// request_id rather than scanning for the newest waiting
			// interaction matters because a thread_load_error is a DELIVERY
			// failure: if Helix re-sent a turn the agent was already running,
			// the rejection carries that turn's request_id and a newest-waiting
			// scan could fail an unrelated turn instead.
			//
			// An UNCORRELATED failure (empty request_id — Zed could not tie the
			// open_thread failure to a turn) has no key to select by, so it
			// keeps the newest-waiting behaviour: something failed on this
			// session and the turn in flight is the one to surface it on.
			target := apiServer.interactionForRequest(context.Background(), requestID)
			if requestID == "" {
				target = apiServer.newestWaitingInteraction(context.Background(), helixSession)
			}
			if target != nil && target.SessionID != helixSessionID {
				log.Warn().
					Str("helix_session_id", helixSessionID).
					Str("interaction_session_id", target.SessionID).
					Str("request_id", requestID).
					Msg("[HELIX] thread_load_error request_id resolves to another session's turn — ignoring")
				target = nil
			}
			if target != nil && target.State == types.InteractionStateWaiting {
				// A delivery failure against a turn that is still producing
				// output is proof the delivery was a duplicate, not that the
				// turn failed. Ignore it outright (unlike an abort, it can never
				// become true later — the turn is demonstrably reachable).
				lastPublish, streaming := apiServer.streamingEvidence(helixSessionID, target.ID)
				if streaming && time.Since(lastPublish) < liveTurnEvidenceWindow {
					log.Warn().
						Str("helix_session_id", helixSessionID).
						Str("interaction_id", target.ID).
						Str("request_id", requestID).
						Time("last_publish", lastPublish).
						Msg("⏸️ [HELIX] thread_load_error names a turn that is still streaming — ignoring rejected duplicate delivery")
				} else {
					target.State = types.InteractionStateError
					target.Error = fmt.Sprintf("Thread load failed: %s", errorMsg)
					target.Updated = time.Now()
					target.Completed = time.Now()
					apiServer.Controller.Options.Store.UpdateInteraction(context.Background(), target)

					// If this interaction came from a queue prompt that's still
					// in 'sending' state (deferred MarkPromptAsSent flow), mark
					// the prompt as failed so the user sees retry, not a
					// stuck "queued" entry.
					//
					// Distinguish terminal Claude Agent crashes (process exit,
					// "Session not found") from transient errors. For crashes,
					// auto-retry is futile — every subsequent send hits the same
					// dead process and rebounds. We pin next_retry_at far in the
					// future via MarkPromptAsCrashed so the queue stops looping,
					// and the frontend's crash detector renders a Restart button.
					// Read the prompt_id directly from the interaction column so this
					// works after API restart too (the in-memory map used to be the
					// only source of this link).
					if target.PromptID != "" {
						failureMsg := fmt.Sprintf("Thread load failed: %s", errorMsg)
						var markErr error
						// A thread_load_error that RECURS is terminal: it means Zed
						// cannot deliver the follow-up to the agent (wedged ACP thread,
						// dead connection, …) and re-sending to the same thread can
						// never succeed. The exact wrapper/transport wording varies
						// ("ede_diagnostic …", "response channel cancelled", "send
						// failed because receiver is gone", …) so we do NOT match on
						// the string — recurrence is the signal. After a couple of
						// normal backoff retries (acpWedgeCrashThreshold) we crash-mark
						// the prompt (pins next_retry_at to the far-future sentinel,
						// surfacing Restart) instead of looping forever. The first
						// occurrence still gets normal retries in case it was a genuinely
						// transient drain that Zed's own retry just missed.
						recurringThreadLoadFailure := false
						if !isAgentCrashError(errorMsg) {
							// Only the recurrence gate needs the prior retry_count; a
							// hard crash is terminal immediately and short-circuits.
							if p, gErr := apiServer.Controller.Options.Store.GetPromptHistoryEntry(context.Background(), target.PromptID); gErr == nil && p != nil && p.RetryCount >= acpWedgeCrashThreshold {
								recurringThreadLoadFailure = true
							}
						}
						if isAgentCrashError(errorMsg) || recurringThreadLoadFailure {
							log.Warn().
								Str("prompt_id", target.PromptID).
								Str("interaction_id", target.ID).
								Str("acp_thread_id", acpThreadID).
								Bool("recurring_thread_load_failure", recurringThreadLoadFailure).
								Msg("💥 [HELIX] Agent thread terminal (hard crash or recurring thread_load_error) — marking prompt crashed (suppress auto-retry, awaits user Restart)")
							markErr = apiServer.Controller.Options.Store.MarkPromptAsCrashed(context.Background(), target.PromptID, failureMsg)
							// The hard-crash case already triggered auto-restart above
							// (outside this loop, PromptID-independent). Here we also
							// cover the RECURRING thread_load_error case — a wedged queue
							// prompt that isn't a hard-crash marker — which only reaches
							// terminal after retries and so always has a PromptID.
							if recurringThreadLoadFailure && helixSession.Metadata.AutoRestartOnCrash {
								go apiServer.maybeAutoRestartCrashedAgent(helixSessionID)
							}
						} else {
							markErr = apiServer.Controller.Options.Store.MarkPromptAsFailed(context.Background(), target.PromptID, failureMsg)
						}
						if markErr != nil {
							log.Error().Err(markErr).
								Str("prompt_id", target.PromptID).
								Str("interaction_id", target.ID).
								Msg("Failed to mark prompt after thread load error")
						}
					}

					log.Info().
						Str("helix_session_id", helixSessionID).
						Str("interaction_id", target.ID).
						Msg("✅ [HELIX] Marked interaction as error due to thread load failure")
				}
			}
		}
	}

	return nil
}

// handleChatResponseError persists the agent's error onto the
// matching Interaction (WS chat path) and/or forwards it to the
// legacy HTTP-stream response channel. Missing mappings degrade
// silently — chat_response_error for an unknown request_id is a no-op
// (e.g. desktop replay during reconnect).
// genericACPAbortMarker identifies the generic mid-turn abort string that Zed
// emits when an ACP turn errors without a cause (see
// zed/crates/external_websocket_sync/src/thread_service.rs). It is the only
// message we attempt to reclassify — specific errors are left untouched.
const genericACPAbortMarker = "exited mid-turn or hit max tokens"

// providerFailureLookback bounds how far back a failed provider call may be and
// still be accepted as the explanation for a turn abort. A coding agent gives
// up seconds after its last failed attempt, so anything older than this belongs
// to an earlier, already-recovered part of the session and must not be
// presented as the cause.
const providerFailureLookback = 3 * time.Minute

// maybeExplainProviderFailure replaces the generic ACP mid-turn abort message
// with the provider error that actually caused it.
//
// Zed's AcpThreadEvent::Error carries no cause (see
// zed/crates/acp_thread/src/acp_thread.rs — run_turn logs the error and emits a
// payload-free event), so the harness can only report "the process exited or
// hit max tokens" and point at a Zed.log inside a sandbox container that is
// deleted when the task ends. Helix already recorded the real reason: every
// proxied model request lands in llm_calls with its provider error. Read it
// back rather than asking the user to go looking for a log they cannot reach.
//
// Returns errorMsg unchanged when the message is not the generic one, when no
// recent call failed, or when the store is unavailable — a wrong explanation is
// worse than a vague one.
func (apiServer *HelixAPIServer) maybeExplainProviderFailure(ctx context.Context, helixSessionID, errorMsg string) string {
	if !strings.Contains(errorMsg, genericACPAbortMarker) || helixSessionID == "" {
		return errorMsg
	}
	calls, _, err := apiServer.Store.ListLLMCalls(ctx, &store.ListLLMCallsQuery{
		SessionID: helixSessionID,
		Page:      1,
		PerPage:   10,
	})
	if err != nil || len(calls) == 0 {
		return errorMsg
	}
	// ListLLMCalls orders by created DESC, so the first failure we meet is the
	// most recent one.
	var failure *types.LLMCall
	for _, call := range calls {
		if call == nil {
			continue
		}
		if strings.TrimSpace(call.Error) != "" {
			failure = call
			break
		}
	}
	if failure == nil || time.Since(failure.Created) > providerFailureLookback {
		return errorMsg
	}

	// Count how many consecutive recent calls failed the same way. One failure
	// reads as a blip; several says the provider was down and the agent
	// exhausted its retries, which is the difference between "try again" and
	// "go look at the provider".
	attempts := 0
	for _, call := range calls {
		if call == nil || strings.TrimSpace(call.Error) == "" {
			break
		}
		attempts++
	}

	detail := strings.TrimSpace(failure.Error)
	if len(detail) > 300 {
		detail = detail[:300] + "…"
	}
	msg := fmt.Sprintf("Agent turn aborted: the model provider failed and the coding agent gave up retrying. "+
		"Last provider error: %s", detail)
	if failure.Model != "" {
		msg += fmt.Sprintf(" (model %s)", failure.Model)
	}
	if attempts > 1 {
		msg += fmt.Sprintf(". %d consecutive requests failed", attempts)
	}
	msg += "."
	return msg
}

// maybeReclassifySubscriptionAuthError rewrites the generic ACP mid-turn abort
// message into a legible Claude-subscription auth error when (a) the message is
// the generic one, (b) the session's agent runs Claude Code in subscription
// mode, and (c) the session owner's subscription is missing or fails a fresh
// liveness probe. Otherwise it returns errorMsg unchanged.
func (apiServer *HelixAPIServer) maybeReclassifySubscriptionAuthError(ctx context.Context, helixSessionID, errorMsg string) string {
	if !strings.Contains(errorMsg, genericACPAbortMarker) {
		return errorMsg
	}
	session, err := apiServer.Store.GetSession(ctx, helixSessionID)
	if err != nil || session == nil || session.ParentApp == "" {
		return errorMsg
	}
	app, err := apiServer.Store.GetApp(ctx, session.ParentApp)
	if err != nil || len(app.Config.Helix.Assistants) == 0 {
		return errorMsg
	}
	asst := app.Config.Helix.Assistants[0]
	if asst.CodeAgentRuntime != types.CodeAgentRuntimeClaudeCode || !asst.CodeAgentCredentialType.IsSubscription() {
		return errorMsg
	}

	owner := apiServer.displayNameForUser(ctx, session.Owner)
	sub, err := apiServer.Store.GetSessionClaudeSubscription(ctx, session)
	if errors.Is(err, store.ErrNotFound) || sub == nil {
		return fmt.Sprintf("Claude subscription authentication failed: %s has no Claude subscription connected. "+
			"Connect one in Settings, or switch the agent to API-key mode.", owner)
	}
	if err != nil {
		return errorMsg
	}

	// Confirm it's really an auth problem before rewriting. revalidate persists
	// the fresh Status/LastError so Settings reflects it too.
	sub = apiServer.revalidateClaudeSubscription(ctx, sub)
	if sub.Status == "error" {
		return fmt.Sprintf("Claude subscription authentication failed for %s (invalid or expired token). "+
			"Reconnect the subscription in Settings.", owner)
	}
	return errorMsg
}

func (apiServer *HelixAPIServer) handleChatResponseError(sessionID string, syncMsg *types.SyncMessage) error {
	requestID, ok := syncMsg.Data["request_id"].(string)
	if !ok {
		log.Warn().Str("session_id", sessionID).Msg("Chat response error missing request_id")
		return nil
	}

	errorMsg, ok := syncMsg.Data["error"].(string)
	if !ok || errorMsg == "" {
		errorMsg = "Unknown error from external agent"
	}

	interaction := apiServer.interactionForRequest(context.Background(), requestID)
	if interaction != nil {
		// applyTurnError, not a direct state write: an error carrying this
		// request_id may be the rejection of a duplicate delivery rather than the
		// abort of the turn itself. See external_agent_turn_error.go.
		apiServer.applyTurnError(context.Background(), interaction, errorMsg)
	}

	if _, _, errorChan, exists := apiServer.getResponseChannel(sessionID, requestID); exists {
		select {
		case errorChan <- fmt.Errorf("%s", errorMsg):
		default:
			log.Warn().Str("session_id", sessionID).Str("request_id", requestID).Msg("Error channel full")
		}
	} else if interaction == nil {
		log.Warn().Str("session_id", sessionID).Str("request_id", requestID).Msg("chat_response_error: no mapping or channel")
	}

	return nil
}

// interactionForRequest resolves a request_id to its interaction. The in-memory
// map is a cache that does not survive an API restart, so it falls back to the
// durable ExternalAgentRequestID column — which is the whole reason that column
// exists. Without this, an agent that keeps talking across a restart addresses
// turns Helix can no longer name.
func (apiServer *HelixAPIServer) interactionForRequest(ctx context.Context, requestID string) *types.Interaction {
	if requestID == "" {
		return nil
	}
	apiServer.contextMappingsMutex.RLock()
	interactionID := apiServer.requestToInteractionMapping[requestID]
	apiServer.contextMappingsMutex.RUnlock()

	if interactionID != "" {
		interaction, err := apiServer.Controller.Options.Store.GetInteraction(ctx, interactionID)
		if err == nil && interaction != nil {
			return interaction
		}
		log.Warn().Err(err).Str("request_id", requestID).Str("interaction_id", interactionID).
			Msg("[HELIX] Mapped interaction could not be loaded; falling back to durable request lookup")
	}

	interaction, err := apiServer.Store.GetInteractionByExternalAgentRequestID(ctx, requestID)
	if err != nil || interaction == nil {
		return nil
	}
	// Repopulate the cache so subsequent events on this turn skip the query.
	apiServer.contextMappingsMutex.Lock()
	if apiServer.requestToInteractionMapping == nil {
		apiServer.requestToInteractionMapping = make(map[string]string)
	}
	apiServer.requestToInteractionMapping[requestID] = interaction.ID
	apiServer.contextMappingsMutex.Unlock()
	return interaction
}

// handleAgentReady processes the agent_ready event from Zed
// This is sent when the agent (e.g., qwen-code) has finished initialization and is ready for prompts
func (apiServer *HelixAPIServer) handleAgentReady(sessionID string, syncMsg *types.SyncMessage) error {
	log.Trace().
		Str("session_id", sessionID).
		Interface("data", syncMsg.Data).
		Msg("[READINESS] Received agent_ready event from Zed")

	// An agent on the wire disproves any latched "desktop could not start"
	// error on the owning spec task. Releasing it here — not only at turn
	// completion — covers the session that connects, works, and then idles out
	// without a turn ever completing.
	apiServer.reconcileSpecTaskLaunchFailure(context.Background(), sessionID)

	// Extract optional metadata from the ready event
	agentName, _ := syncMsg.Data["agent_name"].(string)
	threadID, _ := syncMsg.Data["thread_id"].(string)

	// Which turns does the agent say it is already running? This is what lets a
	// reconnect re-attach to a live turn instead of re-sending it. An agent
	// build that predates active_turns yields the zero report, which
	// decideResume handles conservatively.
	report := parseAgentTurnReport(syncMsg.Data)
	if report.Reported {
		log.Info().
			Str("session_id", sessionID).
			Int("active_turns", len(report.Active)).
			Interface("turns", report.Active).
			Msg("[READINESS] Agent reported its active turns")
	}

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

	// Mark as ready (this flushes queued messages, runs the reconnect resume
	// decision with the agent's report, and calls onReady)
	apiServer.externalAgentWSManager.markSessionReady(sessionID, report, onReadyCallback)

	// NOTE: open_thread is now sent on connect in handleExternalAgentConnection,
	// BEFORE the agent_ready gate. This ensures Zed re-establishes its thread
	// subscription before any queued chat_message is flushed, preventing history
	// replay message_added events from corrupting the current interaction.

	// Process any pending prompts (including interrupt=true ones)
	// When agent is ready/idle, we should process ALL pending prompts, not just non-interrupt ones
	go apiServer.processAnyPendingPrompt(context.Background(), sessionID)

	return nil
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

// flushStreamingFieldsToDB rebuilds the accumulator content for the current
// interaction and persists the streaming columns (response_message,
// response_entries, offset, last message id). It is a column-scoped write: it
// never touches state/completed/error, so a concurrent
// handleTurnCancelled / handleMessageCompleted transition can't be clobbered by
// an in-flight streaming flush. The caller must hold sctx.mu. It is a no-op if
// there is no interaction or accumulator to flush. On success it updates
// lastDBWrite and clears the dirty flag.
func (apiServer *HelixAPIServer) flushStreamingFieldsToDB(sctx *streamingContext) error {
	if sctx.accumulator == nil || sctx.interaction == nil {
		return nil
	}
	acc := sctx.accumulator
	it := sctx.interaction
	acc.Rebuild()
	it.ResponseMessage = acc.Content
	it.LastZedMessageOffset = acc.Offset
	if entriesJSON, err := json.Marshal(acc.Entries()); err == nil {
		_ = json.Unmarshal(entriesJSON, &it.ResponseEntries)
	}
	if err := apiServer.Controller.Options.Store.UpdateInteractionStreamingFields(
		context.Background(),
		it.ID,
		it.GenerationID,
		it.ResponseMessage,
		it.ResponseEntries,
		it.LastZedMessageOffset,
		it.LastZedMessageID,
	); err != nil {
		return err
	}
	sctx.lastDBWrite = time.Now()
	sctx.dirty = false
	return nil
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

	// PHANTOM-DRAFT GUARD (belt-and-braces against helixml/zed Fix 1a not being
	// in this Zed binary): on every container restart, Zed's agent panel
	// speculatively calls new_session() to back its empty input editor — see
	// crates/agent_ui/src/agent_panel.rs `activate_draft`. That fires
	// UserCreatedThread to us even though the user never typed anything in the
	// new "draft" thread. Without this guard, every restart leaks an empty
	// "New Chat" row in spec_task_zed_threads and a duplicate Claude spawn that
	// races against the existing one for npm `_npx/<hash>` cache, surfacing as
	// 180s `chrome-devtools/github context server failed to start` errors.
	//
	// If this spec_task already has an active work_session whose helix_session
	// has no interactions, the incoming UserCreatedThread is almost certainly
	// such a phantom draft. Refuse and log loudly. The user creating a genuine
	// new chat is unaffected: they only do that AFTER typing in the existing
	// thread (which gives it ≥1 interaction), so the dedup wouldn't fire.
	//
	// Full diagnosis: design/2026-05-13-mcp-cache-contention-and-duplicate-claude-spawn.md
	if specTaskID := existingSession.Metadata.SpecTaskID; specTaskID != "" {
		existingThreads, listErr := apiServer.Controller.Options.Store.ListSpecTaskZedThreads(ctx, specTaskID)
		if listErr == nil {
			for _, et := range existingThreads {
				if et.Status != types.SpecTaskZedStatusActive {
					continue
				}
				ws, wErr := apiServer.Controller.Options.Store.GetSpecTaskWorkSession(ctx, et.WorkSessionID)
				if wErr != nil || ws == nil {
					continue
				}
				_, count, iErr := apiServer.Controller.Options.Store.ListInteractions(ctx, &types.ListInteractionsQuery{
					SessionID: ws.HelixSessionID,
				})
				if iErr == nil && count == 0 {
					log.Warn().
						Str("acp_thread_id", acpThreadID).
						Str("spec_task_id", specTaskID).
						Str("phantom_zed_thread_id", et.ZedThreadID).
						Str("phantom_helix_session", ws.HelixSessionID).
						Msg("⚠️ [HELIX] Refusing to create new session — spec_task already has empty active work_session (probable phantom draft from Zed agent panel; see design/2026-05-13-mcp-cache-contention-and-duplicate-claude-spawn.md)")
					return nil
				}
			}
		}
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

	// The coding agent generates a useful thread title near the beginning of a
	// run. Planning tasks later get their canonical name from requirements.md;
	// just-do-it tasks skip that artifact, so reuse the already-generated thread
	// title instead. This works with the task's configured coding model and does
	// not depend on the optional Kodit enrichment model being configured.
	if taskID := session.Metadata.SpecTaskID; taskID != "" && strings.TrimSpace(newTitle) != "" {
		task, taskErr := apiServer.Controller.Options.Store.GetSpecTask(ctx, taskID)
		if taskErr != nil {
			log.Warn().Err(taskErr).Str("spec_task_id", taskID).Msg("Failed to load task for thread title sync")
		} else if task.JustDoItMode && task.UserShortTitle == "" && task.Name != newTitle {
			task.Name = newTitle
			task.UpdatedAt = time.Now()
			if taskErr := apiServer.Controller.Options.Store.UpdateSpecTask(ctx, task); taskErr != nil {
				log.Warn().Err(taskErr).Str("spec_task_id", taskID).Msg("Failed to update just-do-it task name from thread title")
			}
		}
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

	// A SpecTask work session maps 1:1 to its Helix session. Model switches
	// replace the ACP thread inside that session; they must not create another
	// work session (the unique helix_session_id constraint correctly rejects
	// that). Reuse the existing row and repoint its current Zed thread record.
	workSessions, err := st.ListWorkSessionsBySpecTask(ctx, specTaskID, nil)
	if err != nil {
		log.Error().Err(err).
			Str("spec_task_id", specTaskID).
			Str("helix_session_id", helixSession.ID).
			Msg("Failed to list SpecTaskWorkSessions for thread tracking")
		return
	}
	var workSession *types.SpecTaskWorkSession
	for _, candidate := range workSessions {
		if candidate.HelixSessionID == helixSession.ID {
			workSession = candidate
			break
		}
	}
	if workSession == nil {
		workSession = &types.SpecTaskWorkSession{
			SpecTaskID:     specTaskID,
			HelixSessionID: helixSession.ID,
			Name:           title,
			Phase:          types.SpecTaskPhaseImplementation,
			Status:         types.SpecTaskWorkSessionStatusActive,
		}
		if createErr := st.CreateSpecTaskWorkSession(ctx, workSession); createErr != nil {
			log.Error().Err(createErr).
				Str("spec_task_id", specTaskID).
				Str("helix_session_id", helixSession.ID).
				Msg("Failed to create SpecTaskWorkSession for thread tracking")
			return
		}
	}

	now := time.Now()
	if workSession.ZedThread != nil {
		workSession.ZedThread.ZedThreadID = acpThreadID
		workSession.ZedThread.Status = types.SpecTaskZedStatusActive
		workSession.ZedThread.LastActivityAt = &now
		if updateErr := st.UpdateSpecTaskZedThread(ctx, workSession.ZedThread); updateErr != nil {
			log.Error().Err(updateErr).
				Str("spec_task_id", specTaskID).
				Str("work_session_id", workSession.ID).
				Str("acp_thread_id", acpThreadID).
				Msg("Failed to repoint SpecTaskZedThread after coding configuration switch")
		}
		return
	}

	// Create the SpecTaskZedThread record
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
