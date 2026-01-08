package services

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// ExternalAgentPool manages a pool of external agent instances
// Agents can be reused across multiple Helix sessions for the same SpecTask
type ExternalAgentPool struct {
	store      store.Store
	controller *controller.Controller
	agents     map[string]*ExternalAgentInstance
	mutex      sync.RWMutex
}

// ExternalAgentInstance represents a single external agent (Zed instance)
type ExternalAgentInstance struct {
	InstanceID       string              `json:"instance_id"`
	ZedInstanceID    string              `json:"zed_instance_id"`
	SpecTaskID       string              `json:"spec_task_id"`
	CurrentSessionID string              `json:"current_session_id"`
	HelixSessions    []string            `json:"helix_sessions"` // Track all sessions this agent has worked on
	WorkingDir       string              `json:"working_dir"`
	DesignDocsPath   string              `json:"design_docs_path"`
	RepoPath         string              `json:"repo_path"`
	Status           ExternalAgentStatus `json:"status"`
	LastActivity     time.Time           `json:"last_activity"`
	CreatedAt        time.Time           `json:"created_at"`
}

// ExternalAgentStatus represents the status of an external agent
type ExternalAgentStatus string

const (
	AgentStatusIdle          ExternalAgentStatus = "idle"
	AgentStatusWorking       ExternalAgentStatus = "working"
	AgentStatusTransitioning ExternalAgentStatus = "transitioning" // Moving between sessions
	AgentStatusStopped       ExternalAgentStatus = "stopped"
	AgentStatusFailed        ExternalAgentStatus = "failed"
)

// NewExternalAgentPool creates a new agent pool
func NewExternalAgentPool(store store.Store, controller *controller.Controller) *ExternalAgentPool {
	return &ExternalAgentPool{
		store:      store,
		controller: controller,
		agents:     make(map[string]*ExternalAgentInstance),
	}
}

// GetOrCreateForTask gets an existing agent for a task or creates a new one
func (p *ExternalAgentPool) GetOrCreateForTask(ctx context.Context, specTask *types.SpecTask, repoPath, designDocsPath string) (*ExternalAgentInstance, error) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	// Check if task already has an agent assigned
	for _, agent := range p.agents {
		if agent.SpecTaskID == specTask.ID && agent.Status != AgentStatusStopped {
			log.Info().
				Str("task_id", specTask.ID).
				Str("agent_id", agent.InstanceID).
				Msg("Reusing existing agent for task")

			agent.LastActivity = time.Now()
			return agent, nil
		}
	}

	// No agent found, create new one
	agent, err := p.createAgentForTask(ctx, specTask, repoPath, designDocsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create agent: %w", err)
	}

	p.agents[agent.InstanceID] = agent

	log.Info().
		Str("task_id", specTask.ID).
		Str("agent_id", agent.InstanceID).
		Msg("Created new external agent for task")

	return agent, nil
}

// createAgentForTask creates a new external agent instance
func (p *ExternalAgentPool) createAgentForTask(ctx context.Context, specTask *types.SpecTask, repoPath, designDocsPath string) (*ExternalAgentInstance, error) {
	// Generate instance ID
	instanceID := fmt.Sprintf("ext_agent_%s_%d", specTask.ID, time.Now().Unix())

	// TODO: Start external agent via executor
	// This would create Zed container via Wolf with environment variables:
	// - SPEC_TASK_ID={specTask.ID}
	// - DESIGN_DOCS_PATH={designDocsPath}
	// - REPO_PATH={repoPath}

	agent := &ExternalAgentInstance{
		InstanceID:       instanceID,
		ZedInstanceID:    "", // Will be set when Zed instance is created
		SpecTaskID:       specTask.ID,
		CurrentSessionID: "",
		HelixSessions:    []string{},
		WorkingDir:       repoPath,
		DesignDocsPath:   designDocsPath,
		RepoPath:         repoPath,
		Status:           AgentStatusIdle,
		LastActivity:     time.Now(),
		CreatedAt:        time.Now(),
	}

	return agent, nil
}

// TransitionToNewSession transitions agent from one Helix session to another
// This allows the agent to work across multiple sessions while maintaining context
func (p *ExternalAgentPool) TransitionToNewSession(ctx context.Context, agentID, newSessionID string) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	agent, exists := p.agents[agentID]
	if !exists {
		return fmt.Errorf("agent %s not found", agentID)
	}

	log.Info().
		Str("agent_id", agentID).
		Str("old_session", agent.CurrentSessionID).
		Str("new_session", newSessionID).
		Msg("Transitioning agent to new session")

	// Update agent state
	agent.Status = AgentStatusTransitioning

	// Add old session to history if not empty
	if agent.CurrentSessionID != "" {
		agent.HelixSessions = append(agent.HelixSessions, agent.CurrentSessionID)
	}

	// Set new session
	agent.CurrentSessionID = newSessionID
	agent.Status = AgentStatusWorking
	agent.LastActivity = time.Now()

	return nil
}

// MarkWorking marks an agent as actively working
func (p *ExternalAgentPool) MarkWorking(agentID string) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	agent, exists := p.agents[agentID]
	if !exists {
		return fmt.Errorf("agent %s not found", agentID)
	}

	agent.Status = AgentStatusWorking
	agent.LastActivity = time.Now()

	return nil
}

// MarkIdle marks an agent as idle and available
func (p *ExternalAgentPool) MarkIdle(agentID string) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	agent, exists := p.agents[agentID]
	if !exists {
		return fmt.Errorf("agent %s not found", agentID)
	}

	agent.Status = AgentStatusIdle
	agent.LastActivity = time.Now()

	return nil
}

// MarkFailed marks an agent as failed
func (p *ExternalAgentPool) MarkFailed(agentID string) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	agent, exists := p.agents[agentID]
	if !exists {
		return fmt.Errorf("agent %s not found", agentID)
	}

	agent.Status = AgentStatusFailed
	agent.LastActivity = time.Now()

	log.Error().
		Str("agent_id", agentID).
		Str("task_id", agent.SpecTaskID).
		Msg("Agent marked as failed")

	return nil
}

// StopAgent stops and removes an agent from the pool
func (p *ExternalAgentPool) StopAgent(ctx context.Context, agentID string) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	agent, exists := p.agents[agentID]
	if !exists {
		return fmt.Errorf("agent %s not found", agentID)
	}

	agent.Status = AgentStatusStopped
	agent.LastActivity = time.Now()

	// TODO: Actually stop the Zed instance via external agent executor

	delete(p.agents, agentID)

	log.Info().
		Str("agent_id", agentID).
		Str("task_id", agent.SpecTaskID).
		Msg("Stopped and removed agent from pool")

	return nil
}

// GetAgent returns an agent by ID
func (p *ExternalAgentPool) GetAgent(agentID string) (*ExternalAgentInstance, error) {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	agent, exists := p.agents[agentID]
	if !exists {
		return nil, fmt.Errorf("agent %s not found", agentID)
	}

	return agent, nil
}

// GetAgentByTask returns the agent assigned to a task
func (p *ExternalAgentPool) GetAgentByTask(taskID string) (*ExternalAgentInstance, error) {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	for _, agent := range p.agents {
		if agent.SpecTaskID == taskID && agent.Status != AgentStatusStopped {
			return agent, nil
		}
	}

	return nil, fmt.Errorf("no agent found for task %s", taskID)
}

// ListActiveAgents returns all active agents
func (p *ExternalAgentPool) ListActiveAgents() []*ExternalAgentInstance {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	agents := []*ExternalAgentInstance{}
	for _, agent := range p.agents {
		if agent.Status != AgentStatusStopped {
			agents = append(agents, agent)
		}
	}

	return agents
}

// CleanupStaleAgents removes agents that haven't been active recently
func (p *ExternalAgentPool) CleanupStaleAgents(ctx context.Context, maxIdleTime time.Duration) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	now := time.Now()
	staleAgents := []string{}

	for agentID, agent := range p.agents {
		if now.Sub(agent.LastActivity) > maxIdleTime {
			staleAgents = append(staleAgents, agentID)
		}
	}

	for _, agentID := range staleAgents {
		agent := p.agents[agentID]
		agent.Status = AgentStatusStopped

		// TODO: Actually stop the Zed instance via external agent executor

		delete(p.agents, agentID)

		log.Info().
			Str("agent_id", agentID).
			Str("task_id", agent.SpecTaskID).
			Dur("idle_time", now.Sub(agent.LastActivity)).
			Msg("Cleaned up stale agent")
	}

	if len(staleAgents) > 0 {
		log.Info().
			Int("count", len(staleAgents)).
			Msg("Cleaned up stale agents")
	}

	return nil
}

// GetPoolStats returns statistics about the agent pool
func (p *ExternalAgentPool) GetPoolStats() *AgentPoolStats {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	stats := &AgentPoolStats{
		TotalAgents:   len(p.agents),
		IdleAgents:    0,
		WorkingAgents: 0,
		FailedAgents:  0,
		AgentsByTask:  make(map[string]int),
	}

	for _, agent := range p.agents {
		switch agent.Status {
		case AgentStatusIdle:
			stats.IdleAgents++
		case AgentStatusWorking, AgentStatusTransitioning:
			stats.WorkingAgents++
		case AgentStatusFailed:
			stats.FailedAgents++
		}

		stats.AgentsByTask[agent.SpecTaskID]++
	}

	return stats
}

// AgentPoolStats contains statistics about the agent pool
type AgentPoolStats struct {
	TotalAgents   int            `json:"total_agents"`
	IdleAgents    int            `json:"idle_agents"`
	WorkingAgents int            `json:"working_agents"`
	FailedAgents  int            `json:"failed_agents"`
	AgentsByTask  map[string]int `json:"agents_by_task"`
}
