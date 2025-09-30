package external_agent

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// NATSExternalAgentService manages external agents via NATS
type NATSExternalAgentService struct {
	pubsub               pubsub.PubSub
	registeredAgents     map[string]*RegisteredAgent
	agentSessions        map[string]*AgentSession
	sessionToAgent       map[string]string // sessionID -> agentID mapping
	zedInstanceToSession map[string]string // instanceID -> sessionID mapping
	mutex                sync.RWMutex

	// Agent assignment strategy
	roundRobinIndex int

	// Configuration
	heartbeatTimeout time.Duration
	maxRetries       int
}

// RegisteredAgent represents an external agent that has registered with the control plane
type RegisteredAgent struct {
	ID           string                 `json:"id"`
	Type         string                 `json:"type"`   // "zed_external", etc.
	Status       string                 `json:"status"` // "available", "busy", "offline"
	Capabilities []string               `json:"capabilities"`
	LastSeen     time.Time              `json:"last_seen"`
	ActiveTasks  int                    `json:"active_tasks"`
	MaxTasks     int                    `json:"max_tasks"`
	Metadata     map[string]interface{} `json:"metadata"`

	// NATS communication
	ReplySubject string `json:"reply_subject"`
}

// AgentSession represents an active session between Helix and an external agent
type AgentSession struct {
	ID           string    `json:"id"`
	AgentID      string    `json:"agent_id"`
	HelixSession string    `json:"helix_session_id"`
	InstanceID   string    `json:"instance_id,omitempty"` // For multi-session Zed instances
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
	LastActivity time.Time `json:"last_activity"`

	// WebSocket sync info
	WebSocketURL string `json:"websocket_url"`
	AuthToken    string `json:"auth_token"`

	// Context mapping for multi-thread scenarios
	ZedContexts map[string]string `json:"zed_contexts,omitempty"` // contextID -> interactionID
}

// NewNATSExternalAgentService creates a new NATS-based external agent service
func NewNATSExternalAgentService(pubsub pubsub.PubSub) *NATSExternalAgentService {
	service := &NATSExternalAgentService{
		pubsub:               pubsub,
		registeredAgents:     make(map[string]*RegisteredAgent),
		agentSessions:        make(map[string]*AgentSession),
		sessionToAgent:       make(map[string]string),
		zedInstanceToSession: make(map[string]string),
		heartbeatTimeout:     5 * time.Minute,
		maxRetries:           3,
	}

	return service
}

// Start initializes the service and starts listening for agent registrations
func (s *NATSExternalAgentService) Start(ctx context.Context) error {
	log.Info().Msg("Starting NATS external agent service")

	// Subscribe to agent registrations
	_, err := s.pubsub.QueueSubscribe(ctx, pubsub.ZedAgentRunnerStream, pubsub.GetExternalAgentRegistrationQueue(), s.handleAgentRegistration)
	if err != nil {
		return fmt.Errorf("failed to subscribe to agent registrations: %w", err)
	}

	// Subscribe to agent heartbeats
	_, err = s.pubsub.QueueSubscribe(ctx, pubsub.ZedAgentRunnerStream, pubsub.GetExternalAgentHeartbeatQueue(), s.handleAgentHeartbeat)
	if err != nil {
		return fmt.Errorf("failed to subscribe to agent heartbeats: %w", err)
	}

	// Subscribe to agent task responses
	_, err = s.pubsub.QueueSubscribe(ctx, pubsub.ZedAgentRunnerStream, pubsub.GetExternalAgentResponseQueue(), s.handleAgentResponse)
	if err != nil {
		return fmt.Errorf("failed to subscribe to agent responses: %w", err)
	}

	// Start cleanup routine for stale agents
	go s.cleanupRoutine(ctx)

	return nil
}

// AssignExternalAgent assigns a task to an available external agent
func (s *NATSExternalAgentService) AssignExternalAgent(ctx context.Context, request *types.ZedAgent) (*types.ZedAgentResponse, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	log.Debug().
		Str("session_id", request.SessionID).
		Str("user_id", request.UserID).
		Msg("Looking for available external agent")

	// Find an available agent
	agent := s.selectAvailableAgent("zed_external")
	if agent == nil {
		// Debug: List all registered agents
		allAgents := s.ListRegisteredAgents()
		log.Warn().
			Int("total_agents", len(allAgents)).
			Str("session_id", request.SessionID).
			Msg("No real agents available, creating mock agent session for development")

		// Create a mock agent session for development/testing
		return s.createMockAgentSession(request)
	}

	log.Info().
		Str("session_id", request.SessionID).
		Str("agent_id", agent.ID).
		Str("agent_status", agent.Status).
		Int("active_tasks", agent.ActiveTasks).
		Msg("Selected agent for external agent task")

	// Create agent session
	session := &AgentSession{
		ID:           request.SessionID,
		AgentID:      agent.ID,
		HelixSession: request.SessionID,
		InstanceID:   request.InstanceID,
		Status:       "starting",
		CreatedAt:    time.Now(),
		LastActivity: time.Now(),
		WebSocketURL: fmt.Sprintf("ws://api/v1/external-agents/sync?session_id=%s", request.SessionID),
		AuthToken:    s.generateAuthToken(request.SessionID),
		ZedContexts:  make(map[string]string),
	}

	// Store session mappings
	s.agentSessions[session.ID] = session
	s.sessionToAgent[request.SessionID] = agent.ID
	if request.InstanceID != "" {
		s.zedInstanceToSession[request.InstanceID] = request.SessionID
	}

	// Mark agent as busy
	agent.ActiveTasks++
	if agent.ActiveTasks >= agent.MaxTasks {
		agent.Status = "busy"
	}
	agent.LastSeen = time.Now()

	// Create ZedAgent with RDP password for task assignment
	agentWithPassword := &types.ZedAgent{
		SessionID:   request.SessionID,
		InstanceID:  request.InstanceID,
		ThreadID:    request.ThreadID,
		UserID:      request.UserID,
		Input:       request.Input,
		ProjectPath: request.ProjectPath,
		WorkDir:     request.WorkDir,
		Env:         request.Env,
	}

	// Send task assignment to agent via NATS
	taskMessage := &types.ZedTaskMessage{
		Type:      "zed_task",
		Agent:     *agentWithPassword,
		AuthToken: session.AuthToken,
	}

	messageBytes, err := json.Marshal(taskMessage)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal task message: %w", err)
	}

	// Send to agent's reply subject
	err = s.pubsub.Publish(ctx, agent.ReplySubject, messageBytes)
	if err != nil {
		// Revert state changes on error
		delete(s.agentSessions, session.ID)
		delete(s.sessionToAgent, request.SessionID)
		if request.InstanceID != "" {
			delete(s.zedInstanceToSession, request.InstanceID)
		}
		agent.ActiveTasks--
		if agent.ActiveTasks < agent.MaxTasks {
			agent.Status = "available"
		}
		return nil, fmt.Errorf("failed to send task to agent: %w", err)
	}

	log.Info().
		Str("session_id", request.SessionID).
		Str("agent_id", agent.ID).
		Str("instance_id", request.InstanceID).
		Msg("Successfully assigned external agent task")

	return &types.ZedAgentResponse{
		SessionID:    session.ID,
		WebSocketURL: session.WebSocketURL,
		AuthToken:    session.AuthToken,
		Status:       "assigned",
	}, nil
}

// GetAgentSession retrieves an agent session by session ID
func (s *NATSExternalAgentService) GetAgentSession(sessionID string) (*AgentSession, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	log.Debug().
		Str("session_id", sessionID).
		Int("total_sessions", len(s.agentSessions)).
		Msg("Looking up agent session")

	session, exists := s.agentSessions[sessionID]
	if !exists {
		log.Debug().
			Str("session_id", sessionID).
			Msg("Session not found, listing all available sessions")

		for id, sess := range s.agentSessions {
			log.Debug().
				Str("available_session_id", id).
				Str("agent_id", sess.AgentID).
				Str("status", sess.Status).
				Msg("Available session")
		}

		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	log.Debug().
		Str("session_id", sessionID).
		Str("agent_id", session.AgentID).
		Str("status", session.Status).
		Msg("Found agent session")

	return session, nil
}

// StopAgentSession stops an agent session
func (s *NATSExternalAgentService) StopAgentSession(ctx context.Context, sessionID string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	session, exists := s.agentSessions[sessionID]
	if !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	// Send stop message to agent
	agentID, exists := s.sessionToAgent[sessionID]
	if exists {
		agent, exists := s.registeredAgents[agentID]
		if exists {
			stopMessage := map[string]interface{}{
				"type":       "stop_task",
				"session_id": sessionID,
			}

			messageBytes, _ := json.Marshal(stopMessage)
			s.pubsub.Publish(ctx, agent.ReplySubject, messageBytes)

			// Update agent status
			agent.ActiveTasks--
			if agent.ActiveTasks < agent.MaxTasks {
				agent.Status = "available"
			}
		}
	}

	// Clean up session mappings
	delete(s.agentSessions, sessionID)
	delete(s.sessionToAgent, sessionID)
	if session.InstanceID != "" {
		delete(s.zedInstanceToSession, session.InstanceID)
	}

	log.Info().
		Str("session_id", sessionID).
		Str("agent_id", session.AgentID).
		Msg("Stopped external agent session")

	return nil
}

// ListRegisteredAgents returns all registered agents
func (s *NATSExternalAgentService) ListRegisteredAgents() []*RegisteredAgent {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	agents := make([]*RegisteredAgent, 0, len(s.registeredAgents))
	for _, agent := range s.registeredAgents {
		agents = append(agents, agent)
	}

	return agents
}

// ListActiveSessions returns all active agent sessions
func (s *NATSExternalAgentService) ListActiveSessions() []*AgentSession {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	sessions := make([]*AgentSession, 0, len(s.agentSessions))
	for _, session := range s.agentSessions {
		sessions = append(sessions, session)
	}

	return sessions
}

// MapZedContextToInteraction maps a Zed context to a Helix interaction
func (s *NATSExternalAgentService) MapZedContextToInteraction(sessionID, contextID, interactionID string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	session, exists := s.agentSessions[sessionID]
	if !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	if session.ZedContexts == nil {
		session.ZedContexts = make(map[string]string)
	}

	session.ZedContexts[contextID] = interactionID

	log.Debug().
		Str("session_id", sessionID).
		Str("context_id", contextID).
		Str("interaction_id", interactionID).
		Msg("Mapped Zed context to Helix interaction")

	return nil
}

// GetInteractionForContext retrieves the interaction ID for a Zed context
func (s *NATSExternalAgentService) GetInteractionForContext(sessionID, contextID string) (string, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	session, exists := s.agentSessions[sessionID]
	if !exists {
		return "", fmt.Errorf("session not found: %s", sessionID)
	}

	interactionID, exists := session.ZedContexts[contextID]
	if !exists {
		return "", fmt.Errorf("context not mapped: %s", contextID)
	}

	return interactionID, nil
}

// Private helper methods

func (s *NATSExternalAgentService) handleAgentRegistration(msg *pubsub.Message) error {
	var registration struct {
		AgentID      string                 `json:"agent_id"`
		AgentType    string                 `json:"agent_type"`
		Capabilities []string               `json:"capabilities"`
		MaxTasks     int                    `json:"max_tasks"`
		ReplySubject string                 `json:"reply_subject"`
		Metadata     map[string]interface{} `json:"metadata"`
	}

	if err := json.Unmarshal(msg.Data, &registration); err != nil {
		log.Error().Err(err).Msg("Failed to unmarshal agent registration")
		return msg.Nak()
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()

	agent := &RegisteredAgent{
		ID:           registration.AgentID,
		Type:         registration.AgentType,
		Status:       "available",
		Capabilities: registration.Capabilities,
		LastSeen:     time.Now(),
		ActiveTasks:  0,
		MaxTasks:     registration.MaxTasks,
		ReplySubject: registration.ReplySubject,
		Metadata:     registration.Metadata,
	}

	s.registeredAgents[agent.ID] = agent

	log.Info().
		Str("agent_id", agent.ID).
		Str("agent_type", agent.Type).
		Int("max_tasks", agent.MaxTasks).
		Str("reply_subject", agent.ReplySubject).
		Int("total_registered", len(s.registeredAgents)).
		Msg("External agent registered")

	return msg.Ack()
}

func (s *NATSExternalAgentService) handleAgentHeartbeat(msg *pubsub.Message) error {
	var heartbeat struct {
		AgentID     string `json:"agent_id"`
		Status      string `json:"status"`
		ActiveTasks int    `json:"active_tasks"`
	}

	if err := json.Unmarshal(msg.Data, &heartbeat); err != nil {
		log.Error().Err(err).Msg("Failed to unmarshal agent heartbeat")
		return msg.Nak()
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()

	agent, exists := s.registeredAgents[heartbeat.AgentID]
	if !exists {
		log.Warn().Str("agent_id", heartbeat.AgentID).Msg("Received heartbeat from unknown agent")
		return msg.Ack()
	}

	agent.Status = heartbeat.Status
	agent.ActiveTasks = heartbeat.ActiveTasks
	agent.LastSeen = time.Now()

	log.Debug().Str("agent_id", heartbeat.AgentID).Str("status", heartbeat.Status).Msg("Agent heartbeat received")

	return msg.Ack()
}

func (s *NATSExternalAgentService) handleAgentResponse(msg *pubsub.Message) error {
	// Handle task completion responses from agents
	var response struct {
		AgentID   string `json:"agent_id"`
		SessionID string `json:"session_id"`
		Status    string `json:"status"` // "completed", "failed", "progress"
		Message   string `json:"message,omitempty"`
	}

	if err := json.Unmarshal(msg.Data, &response); err != nil {
		log.Error().Err(err).Msg("Failed to unmarshal agent response")
		return msg.Nak()
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()

	session, exists := s.agentSessions[response.SessionID]
	if exists {
		session.Status = response.Status
		session.LastActivity = time.Now()

		// If task completed, free up agent
		if response.Status == "completed" || response.Status == "failed" {
			if agent, exists := s.registeredAgents[response.AgentID]; exists {
				agent.ActiveTasks--
				if agent.ActiveTasks < agent.MaxTasks {
					agent.Status = "available"
				}
			}
		}
	}

	log.Info().
		Str("agent_id", response.AgentID).
		Str("session_id", response.SessionID).
		Str("status", response.Status).
		Msg("Agent task response received")

	return msg.Ack()
}

func (s *NATSExternalAgentService) selectAvailableAgent(agentType string) *RegisteredAgent {
	// Simple round-robin selection of available agents
	availableAgents := make([]*RegisteredAgent, 0)

	log.Debug().
		Str("agent_type", agentType).
		Int("total_registered", len(s.registeredAgents)).
		Msg("Selecting available agent")

	for _, agent := range s.registeredAgents {
		log.Debug().
			Str("agent_id", agent.ID).
			Str("agent_type", agent.Type).
			Str("status", agent.Status).
			Int("active_tasks", agent.ActiveTasks).
			Int("max_tasks", agent.MaxTasks).
			Msg("Checking agent availability")

		if agent.Type == agentType && agent.Status == "available" {
			availableAgents = append(availableAgents, agent)
		}
	}

	log.Debug().
		Str("agent_type", agentType).
		Int("available_count", len(availableAgents)).
		Msg("Found available agents")

	if len(availableAgents) == 0 {
		return nil
	}

	// Round-robin selection
	agent := availableAgents[s.roundRobinIndex%len(availableAgents)]
	s.roundRobinIndex++

	return agent
}

func (s *NATSExternalAgentService) cleanupRoutine(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.cleanupStaleAgents()
		}
	}
}

func (s *NATSExternalAgentService) cleanupStaleAgents() {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	now := time.Now()

	// Remove agents that haven't sent heartbeats
	for agentID, agent := range s.registeredAgents {
		if now.Sub(agent.LastSeen) > s.heartbeatTimeout {
			delete(s.registeredAgents, agentID)
			log.Warn().
				Str("agent_id", agentID).
				Dur("last_seen", now.Sub(agent.LastSeen)).
				Msg("Removed stale external agent")
		}
	}

	// Clean up orphaned sessions
	for sessionID, session := range s.agentSessions {
		if _, exists := s.registeredAgents[session.AgentID]; !exists {
			delete(s.agentSessions, sessionID)
			delete(s.sessionToAgent, sessionID)
			if session.InstanceID != "" {
				delete(s.zedInstanceToSession, session.InstanceID)
			}
			log.Warn().
				Str("session_id", sessionID).
				Str("agent_id", session.AgentID).
				Msg("Cleaned up orphaned agent session")
		}
	}
}

func (s *NATSExternalAgentService) generatePassword() string {
	// Generate a cryptographically secure random password for RDP access
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	const length = 16

	b := make([]byte, length)
	_, err := rand.Read(b)
	if err != nil {
		// Fail securely - no fallback passwords
		log.Error().Err(err).Msg("Failed to generate secure random password - cannot proceed")
		panic(fmt.Sprintf("Failed to generate secure RDP password: %v", err))
	}

	// Convert random bytes to charset characters
	password := make([]byte, length)
	for i := 0; i < length; i++ {
		password[i] = charset[b[i]%byte(len(charset))]
	}

	return fmt.Sprintf("zed_%s_%d", string(password), time.Now().Unix())
}

func (s *NATSExternalAgentService) generateAuthToken(sessionID string) string {
	return fmt.Sprintf("ext_token_%s_%d", sessionID, time.Now().UnixNano())
}

// createMockAgentSession creates a mock agent session when no real agents are available
func (s *NATSExternalAgentService) createMockAgentSession(request *types.ZedAgent) (*types.ZedAgentResponse, error) {
	log.Info().
		Str("session_id", request.SessionID).
		Msg("Creating mock external agent session for development")

	// Create mock agent session
	session := &AgentSession{
		ID:           request.SessionID,
		AgentID:      "mock-agent-dev",
		HelixSession: request.SessionID,
		InstanceID:   request.InstanceID,
		Status:       "mock-ready",
		CreatedAt:    time.Now(),
		LastActivity: time.Now(),
		WebSocketURL: fmt.Sprintf("ws://api/v1/external-agents/sync?session_id=%s", request.SessionID),
		AuthToken:    s.generateAuthToken(request.SessionID),
		ZedContexts:  make(map[string]string),
	}

	// Store session mappings
	s.agentSessions[session.ID] = session
	s.sessionToAgent[request.SessionID] = session.AgentID
	if request.InstanceID != "" {
		s.zedInstanceToSession[request.InstanceID] = request.SessionID
	}

	log.Info().
		Str("session_id", request.SessionID).
		Str("status", session.Status).
		Msg("Mock external agent session created successfully")

	return &types.ZedAgentResponse{
		SessionID:    session.ID,
		WebSocketURL: session.WebSocketURL,
		AuthToken:    session.AuthToken,
		Status:       "mock-ready",
	}, nil
}
