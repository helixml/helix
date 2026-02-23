package services

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// GoldenBuildService manages golden Docker cache builds for projects.
// When a merge to main happens and the project has AutoWarmDockerCache enabled,
// it triggers a golden build session that runs the startup script to populate
// the Docker cache, then promotes the result to the project's golden snapshot.
//
// Builds are fanned out to ALL online sandboxes so every sandbox has a warm cache.
type GoldenBuildService struct {
	store             store.Store
	containerExecutor ContainerExecutor
	specTaskService   *SpecDrivenTaskService

	// Track running golden builds to prevent duplicates.
	// Key: "projectID/sandboxID" -> build start time.
	// Entries older than 30 min are treated as stale.
	mu       sync.Mutex
	building map[string]time.Time
}

// NewGoldenBuildService creates a new golden build service.
func NewGoldenBuildService(
	store store.Store,
	containerExecutor ContainerExecutor,
	specTaskService *SpecDrivenTaskService,
) *GoldenBuildService {
	return &GoldenBuildService{
		store:             store,
		containerExecutor: containerExecutor,
		specTaskService:   specTaskService,
		building:          make(map[string]time.Time),
	}
}

// buildKey returns the debounce map key for a project+sandbox pair.
func buildKey(projectID, sandboxID string) string {
	return projectID + "/" + sandboxID
}

// updateSandboxCacheStatus updates the per-sandbox DockerCacheState in project metadata.
func (g *GoldenBuildService) updateSandboxCacheStatus(ctx context.Context, projectID, sandboxID string, update func(*types.SandboxCacheState)) {
	g.mu.Lock()
	defer g.mu.Unlock()

	project, err := g.store.GetProject(ctx, projectID)
	if err != nil {
		log.Warn().Err(err).Str("project_id", projectID).Msg("Golden build: failed to get project for status update")
		return
	}
	if project.Metadata.DockerCacheStatus == nil {
		project.Metadata.DockerCacheStatus = &types.DockerCacheState{
			Sandboxes: make(map[string]*types.SandboxCacheState),
		}
	}
	if project.Metadata.DockerCacheStatus.Sandboxes == nil {
		project.Metadata.DockerCacheStatus.Sandboxes = make(map[string]*types.SandboxCacheState)
	}
	state, ok := project.Metadata.DockerCacheStatus.Sandboxes[sandboxID]
	if !ok {
		state = &types.SandboxCacheState{}
		project.Metadata.DockerCacheStatus.Sandboxes[sandboxID] = state
	}
	update(state)
	err = g.store.UpdateProject(ctx, project)
	if err != nil {
		log.Warn().Err(err).Str("project_id", projectID).Str("sandbox_id", sandboxID).Msg("Golden build: failed to update project docker cache status")
	}
}

// TriggerGoldenBuild starts golden builds on all online sandboxes if the setting is enabled
// and no build is already running on each sandbox. Called when code is merged to main.
func (g *GoldenBuildService) TriggerGoldenBuild(ctx context.Context, project *types.Project) {
	if project == nil {
		return
	}

	if !project.Metadata.AutoWarmDockerCache {
		return
	}

	g.fanOutBuilds(ctx, project)
}

// TriggerManualGoldenBuild starts golden builds on all online sandboxes regardless of the
// AutoWarmDockerCache setting. Used by the "Prime Cache" button in the UI.
func (g *GoldenBuildService) TriggerManualGoldenBuild(ctx context.Context, project *types.Project) error {
	if project == nil {
		return fmt.Errorf("project is nil")
	}

	sandboxes, err := g.store.ListSandboxes(ctx)
	if err != nil {
		return fmt.Errorf("failed to list sandboxes: %w", err)
	}

	started := 0
	for _, sb := range sandboxes {
		if sb.Status != "online" {
			continue
		}
		key := buildKey(project.ID, sb.ID)
		g.mu.Lock()
		if startedAt, ok := g.building[key]; ok {
			if time.Since(startedAt) < 30*time.Minute {
				g.mu.Unlock()
				log.Info().Str("project_id", project.ID).Str("sandbox_id", sb.ID).
					Msg("Golden build already running on sandbox, skipping")
				continue
			}
			delete(g.building, key)
		}
		g.building[key] = time.Now()
		g.mu.Unlock()

		log.Info().
			Str("project_id", project.ID).
			Str("sandbox_id", sb.ID).
			Msg("Triggering manual golden build on sandbox")

		go g.runGoldenBuildOnSandbox(ctx, project, sb.ID)
		started++
	}

	if started == 0 {
		return fmt.Errorf("no online sandboxes available for golden build")
	}
	return nil
}

// CancelGoldenBuilds stops all running golden builds for a project.
func (g *GoldenBuildService) CancelGoldenBuilds(ctx context.Context, project *types.Project) error {
	if project == nil {
		return fmt.Errorf("project is nil")
	}

	if project.Metadata.DockerCacheStatus == nil || len(project.Metadata.DockerCacheStatus.Sandboxes) == 0 {
		return fmt.Errorf("no golden builds to cancel")
	}

	cancelled := 0
	for sbID, sbState := range project.Metadata.DockerCacheStatus.Sandboxes {
		if sbState.Status != "building" || sbState.BuildSessionID == "" {
			continue
		}

		// Stop the container
		sessionID := sbState.BuildSessionID
		if err := g.containerExecutor.StopDesktop(ctx, sessionID); err != nil {
			log.Warn().Err(err).Str("session_id", sessionID).Str("sandbox_id", sbID).
				Msg("Golden build: failed to stop container (may have already exited)")
		}

		// Clear the debounce entry
		key := buildKey(project.ID, sbID)
		g.mu.Lock()
		delete(g.building, key)
		g.mu.Unlock()

		// Update status
		g.updateSandboxCacheStatus(ctx, project.ID, sbID, func(s *types.SandboxCacheState) {
			s.Status = "none"
			s.BuildSessionID = ""
			s.Error = ""
		})

		cancelled++
	}

	if cancelled == 0 {
		return fmt.Errorf("no active golden builds found")
	}

	log.Info().Str("project_id", project.ID).Int("cancelled", cancelled).
		Msg("Cancelled golden builds")
	return nil
}

// fanOutBuilds lists online sandboxes and launches a golden build goroutine for each.
func (g *GoldenBuildService) fanOutBuilds(ctx context.Context, project *types.Project) {
	sandboxes, err := g.store.ListSandboxes(ctx)
	if err != nil {
		log.Error().Err(err).Str("project_id", project.ID).Msg("Golden build: failed to list sandboxes")
		return
	}

	for _, sb := range sandboxes {
		if sb.Status != "online" {
			continue
		}
		key := buildKey(project.ID, sb.ID)

		g.mu.Lock()
		if startedAt, ok := g.building[key]; ok {
			if time.Since(startedAt) < 30*time.Minute {
				g.mu.Unlock()
				log.Info().Str("project_id", project.ID).Str("sandbox_id", sb.ID).
					Msg("Golden build already running on sandbox, skipping")
				continue
			}
			log.Warn().Str("project_id", project.ID).Str("sandbox_id", sb.ID).
				Msg("Golden build entry is stale (>30min), starting new build")
			delete(g.building, key)
		}
		g.building[key] = time.Now()
		g.mu.Unlock()

		log.Info().
			Str("project_id", project.ID).
			Str("project_name", project.Name).
			Str("sandbox_id", sb.ID).
			Msg("Triggering golden Docker cache build on sandbox")

		go g.runGoldenBuildOnSandbox(ctx, project, sb.ID)
	}
}

// runGoldenBuildOnSandbox runs the golden build on a specific sandbox.
func (g *GoldenBuildService) runGoldenBuildOnSandbox(parentCtx context.Context, project *types.Project, sandboxID string) {
	key := buildKey(project.ID, sandboxID)

	clearBuildingEntry := func() {
		g.mu.Lock()
		delete(g.building, key)
		g.mu.Unlock()
	}
	setFailed := func(errMsg string) {
		clearBuildingEntry()
		g.updateSandboxCacheStatus(context.Background(), project.ID, sandboxID, func(s *types.SandboxCacheState) {
			s.Status = "failed"
			s.Error = errMsg
			s.BuildSessionID = ""
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	// Get project repositories
	projectRepos, err := g.store.ListGitRepositories(ctx, &types.ListGitRepositoriesRequest{
		ProjectID: project.ID,
	})
	if err != nil {
		log.Error().Err(err).Str("project_id", project.ID).Str("sandbox_id", sandboxID).Msg("Golden build: failed to list project repos")
		setFailed(fmt.Sprintf("Failed to list repos: %v", err))
		return
	}

	if len(projectRepos) == 0 {
		log.Warn().Str("project_id", project.ID).Str("sandbox_id", sandboxID).Msg("Golden build: project has no repositories")
		setFailed("Project has no repositories")
		return
	}

	// Get repository IDs
	var repositoryIDs []string
	for _, repo := range projectRepos {
		if repo.ID != "" {
			repositoryIDs = append(repositoryIDs, repo.ID)
		}
	}

	// Determine primary repository
	primaryRepoID := project.DefaultRepoID
	if primaryRepoID == "" && len(projectRepos) > 0 {
		primaryRepoID = projectRepos[0].ID
	}

	// Get default branch from primary repo
	defaultBranch := "main"
	for _, repo := range projectRepos {
		if repo.ID == primaryRepoID && repo.DefaultBranch != "" {
			defaultBranch = repo.DefaultBranch
			break
		}
	}

	// Create a session for the golden build
	sessionID := system.GenerateSessionID()
	session := &types.Session{
		ID:             sessionID,
		Name:           fmt.Sprintf("Docker Cache Warm-up: %s (%s)", project.Name, sandboxID),
		Created:        time.Now(),
		Updated:        time.Now(),
		Mode:           types.SessionModeInference,
		Type:           types.SessionTypeText,
		Owner:          project.UserID,
		OrganizationID: project.OrganizationID,
		ProjectID:      project.ID,
		OwnerType:      types.OwnerTypeUser,
	}

	session, err = g.store.CreateSession(ctx, *session)
	if err != nil {
		log.Error().Err(err).Str("project_id", project.ID).Str("sandbox_id", sandboxID).Msg("Golden build: failed to create session")
		setFailed(fmt.Sprintf("Failed to create session: %v", err))
		return
	}

	log.Info().
		Str("project_id", project.ID).
		Str("session_id", session.ID).
		Str("sandbox_id", sandboxID).
		Msg("Golden build: session created")

	// Update sandbox status to "building"
	now := time.Now()
	g.updateSandboxCacheStatus(ctx, project.ID, sandboxID, func(s *types.SandboxCacheState) {
		s.Status = "building"
		s.BuildSessionID = session.ID
		s.LastBuildAt = &now
		s.Error = ""
	})

	// Get API key for the golden build session
	userAPIKey, err := g.specTaskService.GetOrCreateSessionAPIKey(ctx, &SessionAPIKeyRequest{
		OrganizationID: project.OrganizationID,
		UserID:         project.UserID,
		SessionID:      session.ID,
	})
	if err != nil {
		log.Error().Err(err).Str("project_id", project.ID).Str("sandbox_id", sandboxID).Msg("Golden build: failed to create API key")
		setFailed(fmt.Sprintf("Failed to create API key: %v", err))
		return
	}

	// Build env vars
	envVars := types.DesktopAgentAPIEnvVars(userAPIKey)
	envVars = append(envVars, "HELIX_GOLDEN_BUILD=true")

	// Create the desktop agent for the golden build, targeting specific sandbox
	agent := &types.DesktopAgent{
		OrganizationID:      project.OrganizationID,
		SessionID:           session.ID,
		UserID:              project.UserID,
		Input:               "Golden Docker cache build",
		ProjectID:           project.ID,
		RepositoryIDs:       repositoryIDs,
		PrimaryRepositoryID: primaryRepoID,
		DesktopType:         "ubuntu",
		Env:                 envVars,
		BranchMode:          "existing",
		WorkingBranch:       defaultBranch,
		DisplayWidth:       1920,
		DisplayHeight:      1080,
		DisplayRefreshRate: 60,
		Resolution:         "1080p",
		ZoomLevel:          200,
		GoldenBuild:        true,
		SandboxID:          sandboxID,
	}

	// Start the desktop container
	_, err = g.containerExecutor.StartDesktop(ctx, agent)
	if err != nil {
		log.Error().Err(err).
			Str("project_id", project.ID).
			Str("session_id", session.ID).
			Str("sandbox_id", sandboxID).
			Msg("Golden build: failed to start desktop")
		setFailed(fmt.Sprintf("Failed to start container: %v", err))
		return
	}

	log.Info().
		Str("project_id", project.ID).
		Str("session_id", session.ID).
		Str("sandbox_id", sandboxID).
		Msg("Golden build: container started, polling for completion")

	g.waitForGoldenBuildCompletion(ctx, project.ID, sandboxID, session.ID)
}

// waitForGoldenBuildCompletion polls the session until the golden build
// container exits, then updates the per-sandbox docker cache status.
func (g *GoldenBuildService) waitForGoldenBuildCompletion(ctx context.Context, projectID, sandboxID, sessionID string) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	key := buildKey(projectID, sandboxID)
	defer func() {
		g.mu.Lock()
		delete(g.building, key)
		g.mu.Unlock()
	}()

	for {
		select {
		case <-ctx.Done():
			log.Warn().Str("project_id", projectID).Str("sandbox_id", sandboxID).Str("session_id", sessionID).
				Msg("Golden build: timed out waiting for completion")
			g.updateSandboxCacheStatus(context.Background(), projectID, sandboxID, func(s *types.SandboxCacheState) {
				s.Status = "failed"
				s.Error = "Build timed out (30 min)"
				s.BuildSessionID = ""
			})
			return

		case <-ticker.C:
			session, err := g.store.GetSession(ctx, sessionID)
			if err != nil {
				// Session deleted or DB error — build is done
				log.Info().Str("project_id", projectID).Str("sandbox_id", sandboxID).Str("session_id", sessionID).
					Msg("Golden build: session no longer found, build complete")
				g.updateSandboxCacheStatus(context.Background(), projectID, sandboxID, func(s *types.SandboxCacheState) {
					now := time.Now()
					s.Status = "ready"
					s.LastReadyAt = &now
					s.BuildSessionID = ""
					s.Error = ""
				})
				return
			}

			if session.Metadata.DesiredState == types.DesiredStateStopped {
				log.Info().Str("project_id", projectID).Str("sandbox_id", sandboxID).Str("session_id", sessionID).
					Msg("Golden build: session stopped, build complete")
				g.updateSandboxCacheStatus(context.Background(), projectID, sandboxID, func(s *types.SandboxCacheState) {
					now := time.Now()
					s.Status = "ready"
					s.LastReadyAt = &now
					s.BuildSessionID = ""
					s.Error = ""
				})
				return
			}

			// Check if the container is still running. Hydra's monitorGoldenBuild
			// removes the container after promoting the golden cache, but doesn't
			// update the session record. Detect this by checking the executor.
			if !g.containerExecutor.HasRunningContainer(ctx, sessionID) {
				log.Info().Str("project_id", projectID).Str("sandbox_id", sandboxID).Str("session_id", sessionID).
					Msg("Golden build: container no longer running, build complete")
				g.updateSandboxCacheStatus(context.Background(), projectID, sandboxID, func(s *types.SandboxCacheState) {
					now := time.Now()
					s.Status = "ready"
					s.LastReadyAt = &now
					s.BuildSessionID = ""
					s.Error = ""
				})
				return
			}

			log.Debug().Str("project_id", projectID).Str("sandbox_id", sandboxID).Str("session_id", sessionID).
				Msg("Golden build: still running, polling again in 15s")
		}
	}
}
