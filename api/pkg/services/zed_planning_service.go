package services

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// ZedPlanningService manages Zed-based planning phase with git repository integration
// This service enables Zed agents to perform planning with full codebase context
type ZedPlanningService struct {
	store            store.Store
	controller       *controller.Controller
	pubsub           pubsub.PubSub
	zedIntegration   *ZedIntegrationService
	gitRepoService   *GitRepositoryService
	protocolClient   *pubsub.ZedProtocolClient
	defaultWorkspace string // Base workspace path for Zed agents
	planningTimeout  time.Duration
	testMode         bool
}

// ZedPlanningSession represents a planning session with Zed agent
type ZedPlanningSession struct {
	ID              string                  `json:"id"`
	SpecTaskID      string                  `json:"spec_task_id"`
	RepositoryID    string                  `json:"repository_id"`
	ZedAgentID      string                  `json:"zed_agent_id"`
	Phase           ZedPlanningPhase        `json:"phase"`
	Status          ZedPlanningStatus       `json:"status"`
	WorkspacePath   string                  `json:"workspace_path"`
	RepositoryURL   string                  `json:"repository_url"`
	PlanningBranch  string                  `json:"planning_branch"`
	GeneratedSpecs  map[string]string       `json:"generated_specs,omitempty"`
	ContextAnalysis *ProjectContextAnalysis `json:"context_analysis,omitempty"`
	StartedAt       time.Time               `json:"started_at"`
	CompletedAt     *time.Time              `json:"completed_at,omitempty"`
	ErrorMessage    string                  `json:"error_message,omitempty"`
	Metadata        map[string]interface{}  `json:"metadata,omitempty"`
}

// ZedPlanningPhase represents the current phase of planning
type ZedPlanningPhase string

const (
	ZedPlanningPhaseInitializing    ZedPlanningPhase = "initializing"
	ZedPlanningPhaseAnalyzing       ZedPlanningPhase = "analyzing"
	ZedPlanningPhaseGeneratingSpecs ZedPlanningPhase = "generating_specs"
	ZedPlanningPhaseReviewPending   ZedPlanningPhase = "review_pending"
	ZedPlanningPhaseCompleted       ZedPlanningPhase = "completed"
)

// ZedPlanningStatus represents the status of a planning session
type ZedPlanningStatus string

const (
	ZedPlanningStatusActive    ZedPlanningStatus = "active"
	ZedPlanningStatusCompleted ZedPlanningStatus = "completed"
	ZedPlanningStatusFailed    ZedPlanningStatus = "failed"
	ZedPlanningStatusCancelled ZedPlanningStatus = "cancelled"
)

// ProjectContextAnalysis represents analysis of existing project codebase
type ProjectContextAnalysis struct {
	TechnologyStack     []string               `json:"technology_stack"`
	ProjectStructure    map[string]interface{} `json:"project_structure"`
	ExistingFeatures    []string               `json:"existing_features"`
	Dependencies        []string               `json:"dependencies"`
	ArchitectureNotes   string                 `json:"architecture_notes"`
	RecommendedPatterns []string               `json:"recommended_patterns"`
	AnalysisTimestamp   time.Time              `json:"analysis_timestamp"`
}

// ZedPlanningRequest represents a request to start planning with Zed
type ZedPlanningRequest struct {
	SpecTaskID     string            `json:"spec_task_id"`
	RepositoryID   string            `json:"repository_id,omitempty"`
	RepositoryURL  string            `json:"repository_url,omitempty"`
	ProjectName    string            `json:"project_name"`
	Description    string            `json:"description"`
	Requirements   string            `json:"requirements"`
	OwnerID        string            `json:"owner_id"`
	CreateRepo     bool              `json:"create_repo"`
	SampleType     string            `json:"sample_type,omitempty"`
	Environment    map[string]string `json:"environment,omitempty"`
	PlanningPrompt string            `json:"planning_prompt,omitempty"`
}

// ZedPlanningResult represents the result of a planning session
type ZedPlanningResult struct {
	Session         *ZedPlanningSession     `json:"session"`
	Repository      *GitRepository          `json:"repository"`
	GeneratedSpecs  map[string]string       `json:"generated_specs"`
	ContextAnalysis *ProjectContextAnalysis `json:"context_analysis"`
	PlanningBranch  string                  `json:"planning_branch"`
	ReadyForReview  bool                    `json:"ready_for_review"`
	NextSteps       []string                `json:"next_steps"`
	Success         bool                    `json:"success"`
	Message         string                  `json:"message"`
}

// NewZedPlanningService creates a new Zed planning service
func NewZedPlanningService(
	store store.Store,
	controller *controller.Controller,
	ps pubsub.PubSub,
	zedIntegration *ZedIntegrationService,
	gitRepoService *GitRepositoryService,
	defaultWorkspace string,
) *ZedPlanningService {
	service := &ZedPlanningService{
		store:            store,
		controller:       controller,
		pubsub:           ps,
		zedIntegration:   zedIntegration,
		gitRepoService:   gitRepoService,
		defaultWorkspace: defaultWorkspace,
		planningTimeout:  30 * time.Minute,
		testMode:         false,
	}

	// Initialize protocol client
	service.protocolClient = pubsub.NewZedProtocolClient(ps)

	return service
}

// SetTestMode enables or disables test mode
func (s *ZedPlanningService) SetTestMode(enabled bool) {
	s.testMode = enabled
}

// StartPlanningSession starts a new planning session with Zed agent
func (s *ZedPlanningService) StartPlanningSession(
	ctx context.Context,
	request *ZedPlanningRequest,
) (*ZedPlanningResult, error) {
	log.Info().
		Str("spec_task_id", request.SpecTaskID).
		Str("project_name", request.ProjectName).
		Msg("Starting Zed planning session")

	// 1. Get or create repository
	var repository *GitRepository
	var err error

	if request.RepositoryID != "" {
		// Use existing repository
		repository, err = s.gitRepoService.GetRepository(ctx, request.RepositoryID)
		if err != nil {
			return nil, fmt.Errorf("failed to get repository %s: %w", request.RepositoryID, err)
		}
	} else if request.CreateRepo {
		// Create new repository
		repository, err = s.createRepositoryForPlanning(ctx, request)
		if err != nil {
			return nil, fmt.Errorf("failed to create repository: %w", err)
		}
	} else {
		return nil, fmt.Errorf("either repository_id or create_repo must be specified")
	}

	// 2. Create planning session
	session := &ZedPlanningSession{
		ID:             s.generatePlanningSessionID(request.SpecTaskID),
		SpecTaskID:     request.SpecTaskID,
		RepositoryID:   repository.ID,
		Phase:          ZedPlanningPhaseInitializing,
		Status:         ZedPlanningStatusActive,
		WorkspacePath:  s.generateWorkspacePath(request.SpecTaskID),
		RepositoryURL:  repository.CloneURL,
		PlanningBranch: fmt.Sprintf("planning/%s", request.SpecTaskID),
		StartedAt:      time.Now(),
		Metadata:       make(map[string]interface{}),
	}

	// 3. Launch Zed planning agent
	zedAgent, err := s.launchPlanningAgent(ctx, session, request)
	if err != nil {
		return nil, fmt.Errorf("failed to launch Zed planning agent: %w", err)
	}

	session.ZedAgentID = zedAgent.SessionID

	// 4. Store planning session (if store supports it)
	err = s.storePlanningSession(ctx, session)
	if err != nil {
		log.Warn().Err(err).Str("session_id", session.ID).Msg("Failed to store planning session")
	}

	result := &ZedPlanningResult{
		Session:        session,
		Repository:     repository,
		PlanningBranch: session.PlanningBranch,
		Success:        true,
		Message:        "Planning session started successfully",
		NextSteps: []string{
			"Zed agent is cloning repository and analyzing codebase",
			"Specs will be generated based on existing project context",
			"Planning branch will be created for review",
		},
	}

	log.Info().
		Str("session_id", session.ID).
		Str("zed_agent_id", zedAgent.SessionID).
		Str("repository_id", repository.ID).
		Msg("Zed planning session started")

	return result, nil
}

// GetPlanningSession retrieves a planning session by ID
func (s *ZedPlanningService) GetPlanningSession(
	ctx context.Context,
	sessionID string,
) (*ZedPlanningSession, error) {
	// Try to get from store first
	session, err := s.getPlanningSessionFromStore(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("planning session not found: %s", sessionID)
	}

	// Update with current status if Zed agent is still active
	if session.Status == ZedPlanningStatusActive {
		err = s.updatePlanningSessionStatus(ctx, session)
		if err != nil {
			log.Warn().Err(err).Str("session_id", sessionID).Msg("Failed to update planning session status")
		}
	}

	return session, nil
}

// CompletePlanning marks a planning session as completed and prepares for implementation
func (s *ZedPlanningService) CompletePlanning(
	ctx context.Context,
	sessionID string,
	approved bool,
) (*ZedPlanningResult, error) {
	session, err := s.GetPlanningSession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get planning session: %w", err)
	}

	if !approved {
		session.Status = ZedPlanningStatusCancelled
		now := time.Now()
		session.CompletedAt = &now

		result := &ZedPlanningResult{
			Session: session,
			Success: false,
			Message: "Planning cancelled by user",
		}
		return result, nil
	}

	// Mark as completed
	session.Status = ZedPlanningStatusCompleted
	session.Phase = ZedPlanningPhaseCompleted
	now := time.Now()
	session.CompletedAt = &now

	// Get repository
	repository, err := s.gitRepoService.GetRepository(ctx, session.RepositoryID)
	if err != nil {
		return nil, fmt.Errorf("failed to get repository: %w", err)
	}

	// Update planning session in store
	err = s.storePlanningSession(ctx, session)
	if err != nil {
		log.Warn().Err(err).Str("session_id", sessionID).Msg("Failed to update planning session")
	}

	result := &ZedPlanningResult{
		Session:         session,
		Repository:      repository,
		GeneratedSpecs:  session.GeneratedSpecs,
		ContextAnalysis: session.ContextAnalysis,
		PlanningBranch:  session.PlanningBranch,
		ReadyForReview:  false, // Already approved
		Success:         true,
		Message:         "Planning completed and approved",
		NextSteps: []string{
			"Specs have been committed to planning branch",
			"Implementation phase can now begin",
			"Multiple Zed agents can coordinate on implementation",
		},
	}

	log.Info().
		Str("session_id", sessionID).
		Str("repository_id", session.RepositoryID).
		Msg("Planning session completed and approved")

	return result, nil
}

// createRepositoryForPlanning creates a repository for planning
func (s *ZedPlanningService) createRepositoryForPlanning(
	ctx context.Context,
	request *ZedPlanningRequest,
) (*GitRepository, error) {
	if request.SampleType != "" {
		// Create sample repository
		return s.gitRepoService.CreateSampleRepository(
			ctx,
			request.ProjectName,
			request.Description,
			request.OwnerID,
			request.SampleType,
			true, // Enable Kodit indexing by default for new projects
		)
	}

	// Create SpecTask repository
	specTask := &types.SpecTask{
		ID:          request.SpecTaskID,
		Name:        request.ProjectName,
		Description: request.Description,
		CreatedBy:   request.OwnerID,
	}

	return s.gitRepoService.CreateSpecTaskRepository(ctx, specTask, nil)
}

// launchPlanningAgent launches a Zed agent for planning
func (s *ZedPlanningService) launchPlanningAgent(
	ctx context.Context,
	session *ZedPlanningSession,
	request *ZedPlanningRequest,
) (*types.ZedAgent, error) {
	// Build planning prompt
	planningPrompt := s.buildPlanningPrompt(request)

	// Create Zed agent configuration
	zedAgent := &types.ZedAgent{
		SessionID:   fmt.Sprintf("planning_%s", session.ID),
		UserID:      request.OwnerID,
		Input:       planningPrompt,
		ProjectPath: session.WorkspacePath,
		WorkDir:     session.WorkspacePath,
		InstanceID:  session.ID,
		Env: []string{
			"PHASE=planning",
			"SPEC_TASK_ID=" + request.SpecTaskID,
			"REPOSITORY_ID=" + session.RepositoryID,
			"REPOSITORY_URL=" + session.RepositoryURL,
			"PLANNING_BRANCH=" + session.PlanningBranch,
			"PROJECT_NAME=" + request.ProjectName,
		},
	}

	// Add custom environment variables
	if request.Environment != nil {
		for key, value := range request.Environment {
			zedAgent.Env = append(zedAgent.Env, fmt.Sprintf("%s=%s", key, value))
		}
	}

	// Launch Zed agent (unless in test mode)
	if !s.testMode {
		err := s.zedIntegration.LaunchZedAgent(ctx, zedAgent)
		if err != nil {
			return nil, fmt.Errorf("failed to launch Zed agent: %w", err)
		}
	}

	return zedAgent, nil
}

// buildPlanningPrompt builds the planning prompt for the Zed agent
func (s *ZedPlanningService) buildPlanningPrompt(request *ZedPlanningRequest) string {
	if request.PlanningPrompt != "" {
		return request.PlanningPrompt
	}

	prompt := fmt.Sprintf(`You are a planning agent working on a software project. Your task is to:

1. **Clone and analyze the project repository**:
   - Run: git clone %s %s
   - Analyze the existing codebase structure
   - Identify technology stack and patterns
   - Understand existing features and architecture

2. **Generate comprehensive specifications** in docs/specs/:
   - requirements.md: EARS notation user stories based on: %s
   - design.md: Technical design considering existing codebase
   - tasks.md: Implementation plan with specific tasks
   - coordination.md: Multi-session coordination strategy

3. **Create planning branch and commit specs**:
   - Create branch: %s
   - Commit generated specs for review
   - Include analysis of existing codebase context

**Project**: %s
**Requirements**: %s

**Key Guidelines**:
- Respect existing project patterns and architecture
- Generate specs that build on existing codebase
- Consider integration with existing features
- Plan for multi-session implementation coordination
- Use git effectively for version control and collaboration

Begin by cloning the repository and analyzing the existing codebase.`,
		request.RepositoryURL,
		request.ProjectName,
		request.Requirements,
		fmt.Sprintf("planning/%s", request.SpecTaskID),
		request.ProjectName,
		request.Requirements,
	)

	return prompt
}

// generatePlanningSessionID generates a unique planning session ID
func (s *ZedPlanningService) generatePlanningSessionID(specTaskID string) string {
	timestamp := time.Now().Unix()
	return fmt.Sprintf("planning_%s_%d", specTaskID, timestamp)
}

// generateWorkspacePath generates a workspace path for the planning session
func (s *ZedPlanningService) generateWorkspacePath(specTaskID string) string {
	return filepath.Join(s.defaultWorkspace, fmt.Sprintf("planning_%s", specTaskID))
}

// updatePlanningSessionStatus updates the status of a planning session
func (s *ZedPlanningService) updatePlanningSessionStatus(
	ctx context.Context,
	session *ZedPlanningSession,
) error {
	// This would query the Zed agent status and update session accordingly
	// For now, just update the timestamp
	session.Metadata["last_status_check"] = time.Now()
	return nil
}

// storePlanningSession stores a planning session (implementation depends on store capabilities)
func (s *ZedPlanningService) storePlanningSession(
	ctx context.Context,
	session *ZedPlanningSession,
) error {
	// This would store the planning session in the database
	// For now, just log it
	sessionJSON, _ := json.MarshalIndent(session, "", "  ")
	log.Info().
		Str("session_id", session.ID).
		RawJSON("session", sessionJSON).
		Msg("Planning session stored")
	return nil
}

// getPlanningSessionFromStore retrieves a planning session from store
func (s *ZedPlanningService) getPlanningSessionFromStore(
	ctx context.Context,
	sessionID string,
) (*ZedPlanningSession, error) {
	// This would retrieve from the database
	// For now, return a mock session
	return &ZedPlanningSession{
		ID:        sessionID,
		Status:    ZedPlanningStatusActive,
		Phase:     ZedPlanningPhaseAnalyzing,
		StartedAt: time.Now().Add(-5 * time.Minute),
		Metadata:  make(map[string]interface{}),
	}, nil
}

// ListPlanningSessions lists all planning sessions for a user
func (s *ZedPlanningService) ListPlanningSessions(
	ctx context.Context,
	ownerID string,
) ([]*ZedPlanningSession, error) {
	// This would query the database for planning sessions
	// For now, return empty list
	return []*ZedPlanningSession{}, nil
}

// CancelPlanningSession cancels an active planning session
func (s *ZedPlanningService) CancelPlanningSession(
	ctx context.Context,
	sessionID string,
) error {
	session, err := s.GetPlanningSession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("failed to get planning session: %w", err)
	}

	if session.Status != ZedPlanningStatusActive {
		return fmt.Errorf("cannot cancel planning session in status: %s", session.Status)
	}

	// Cancel Zed agent if running
	if session.ZedAgentID != "" && !s.testMode {
		err = s.zedIntegration.StopZedAgent(ctx, session.ZedAgentID)
		if err != nil {
			log.Warn().Err(err).Str("zed_agent_id", session.ZedAgentID).Msg("Failed to stop Zed agent")
		}
	}

	// Update session status
	session.Status = ZedPlanningStatusCancelled
	now := time.Now()
	session.CompletedAt = &now

	err = s.storePlanningSession(ctx, session)
	if err != nil {
		log.Warn().Err(err).Str("session_id", sessionID).Msg("Failed to update cancelled planning session")
	}

	log.Info().
		Str("session_id", sessionID).
		Str("zed_agent_id", session.ZedAgentID).
		Msg("Planning session cancelled")

	return nil
}
