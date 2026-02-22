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
type GoldenBuildService struct {
	store             store.Store
	containerExecutor ContainerExecutor
	specTaskService   *SpecDrivenTaskService

	// Track running golden builds to prevent duplicates.
	// Maps project_id -> build start time. Entries older than 30 min are treated as stale.
	mu       sync.Mutex
	building map[string]time.Time // project_id -> build start time
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

// updateDockerCacheStatus updates the project's DockerCacheState in metadata.
func (g *GoldenBuildService) updateDockerCacheStatus(ctx context.Context, projectID string, update func(*types.DockerCacheState)) {
	project, err := g.store.GetProject(ctx, projectID)
	if err != nil {
		log.Warn().Err(err).Str("project_id", projectID).Msg("Golden build: failed to get project for status update")
		return
	}
	if project.Metadata.DockerCacheStatus == nil {
		project.Metadata.DockerCacheStatus = &types.DockerCacheState{}
	}
	update(project.Metadata.DockerCacheStatus)
	err = g.store.UpdateProject(ctx, project)
	if err != nil {
		log.Warn().Err(err).Str("project_id", projectID).Msg("Golden build: failed to update project docker cache status")
	}
}

// TriggerGoldenBuild starts a golden build for a project if the setting is enabled
// and no build is already running. Called when code is merged to main.
// This is fire-and-forget: the build runs in the background.
func (g *GoldenBuildService) TriggerGoldenBuild(ctx context.Context, project *types.Project) {
	if project == nil {
		return
	}

	// Check if golden cache warming is enabled for this project
	if !project.Metadata.AutoWarmDockerCache {
		return
	}

	// Debounce: skip if a golden build is already tracked for this project.
	// The buildingStarted time prevents stale entries from blocking forever —
	// if a build has been "running" for more than 30 minutes, assume it's dead.
	g.mu.Lock()
	if startedAt, ok := g.building[project.ID]; ok {
		if time.Since(startedAt) < 30*time.Minute {
			g.mu.Unlock()
			log.Info().
				Str("project_id", project.ID).
				Msg("Golden build already running for project, skipping")
			return
		}
		// Stale entry — build has been "running" too long, proceed with new one
		log.Warn().
			Str("project_id", project.ID).
			Msg("Golden build entry is stale (>30min), starting new build")
		delete(g.building, project.ID)
	}
	g.building[project.ID] = time.Now()
	g.mu.Unlock()

	log.Info().
		Str("project_id", project.ID).
		Str("project_name", project.Name).
		Msg("Triggering golden Docker cache build")

	go g.runGoldenBuild(ctx, project)
}

// runGoldenBuild runs the golden build in a goroutine.
// On failure, clears the building entry so a retry can happen immediately.
// After launching the container, polls for session completion to update
// the project's docker cache status and clear the building entry.
func (g *GoldenBuildService) runGoldenBuild(parentCtx context.Context, project *types.Project) {
	clearBuildingEntry := func() {
		g.mu.Lock()
		delete(g.building, project.ID)
		g.mu.Unlock()
	}
	setFailed := func(errMsg string) {
		clearBuildingEntry()
		g.updateDockerCacheStatus(context.Background(), project.ID, func(s *types.DockerCacheState) {
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
		log.Error().Err(err).Str("project_id", project.ID).Msg("Golden build: failed to list project repos")
		setFailed(fmt.Sprintf("Failed to list repos: %v", err))
		return
	}

	if len(projectRepos) == 0 {
		log.Warn().Str("project_id", project.ID).Msg("Golden build: project has no repositories")
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
		Name:           fmt.Sprintf("Docker Cache Warm-up: %s", project.Name),
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
		log.Error().Err(err).Str("project_id", project.ID).Msg("Golden build: failed to create session")
		setFailed(fmt.Sprintf("Failed to create session: %v", err))
		return
	}

	log.Info().
		Str("project_id", project.ID).
		Str("session_id", session.ID).
		Msg("Golden build: session created")

	// Update project status to "building"
	now := time.Now()
	g.updateDockerCacheStatus(ctx, project.ID, func(s *types.DockerCacheState) {
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
		log.Error().Err(err).Str("project_id", project.ID).Msg("Golden build: failed to create API key")
		setFailed(fmt.Sprintf("Failed to create API key: %v", err))
		return
	}

	// Build env vars
	envVars := types.DesktopAgentAPIEnvVars(userAPIKey)
	envVars = append(envVars, "HELIX_GOLDEN_BUILD=true")

	// Create the desktop agent for the golden build
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
		// Small display — golden build doesn't need video
		DisplayWidth:       800,
		DisplayHeight:      600,
		DisplayRefreshRate: 30,
		GoldenBuild:        true,
	}

	// Start the desktop container
	_, err = g.containerExecutor.StartDesktop(ctx, agent)
	if err != nil {
		log.Error().Err(err).
			Str("project_id", project.ID).
			Str("session_id", session.ID).
			Msg("Golden build: failed to start desktop")
		setFailed(fmt.Sprintf("Failed to start container: %v", err))
		return
	}

	log.Info().
		Str("project_id", project.ID).
		Str("session_id", session.ID).
		Msg("Golden build: container started, polling for completion")

	// Poll for completion. Hydra's monitorGoldenBuild handles container exit,
	// golden promotion, and cleanup. We poll the container status via the
	// executor to detect when it's done, then update project status.
	g.waitForGoldenBuildCompletion(ctx, project.ID, session.ID)
}

// waitForGoldenBuildCompletion polls the session until the golden build
// container exits (detected by session deletion or timeout), then updates
// the project's docker cache status.
func (g *GoldenBuildService) waitForGoldenBuildCompletion(ctx context.Context, projectID, sessionID string) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	defer func() {
		g.mu.Lock()
		delete(g.building, projectID)
		g.mu.Unlock()
	}()

	for {
		select {
		case <-ctx.Done():
			log.Warn().Str("project_id", projectID).Str("session_id", sessionID).
				Msg("Golden build: timed out waiting for completion")
			g.updateDockerCacheStatus(context.Background(), projectID, func(s *types.DockerCacheState) {
				s.Status = "failed"
				s.Error = "Build timed out (30 min)"
				s.BuildSessionID = ""
			})
			return

		case <-ticker.C:
			// Check if the session still exists. When monitorGoldenBuild
			// finishes, the container is removed and the session becomes
			// orphaned (no active container). We detect this by checking
			// if the session's metadata still shows desired_state=running.
			session, err := g.store.GetSession(ctx, sessionID)
			if err != nil {
				// Session deleted or DB error — build is done
				log.Info().Str("project_id", projectID).Str("session_id", sessionID).
					Msg("Golden build: session no longer found, build complete")
				g.updateDockerCacheStatus(context.Background(), projectID, func(s *types.DockerCacheState) {
					now := time.Now()
					s.Status = "ready"
					s.LastReadyAt = &now
					s.BuildSessionID = ""
					s.Error = ""
				})
				return
			}

			// Check if the session is marked as stopped (container exited).
			// The session metadata DesiredState transitions to "stopped" when
			// the external agent disconnects and the session is cleaned up.
			if session.Metadata.DesiredState == types.DesiredStateStopped {
				log.Info().Str("project_id", projectID).Str("session_id", sessionID).
					Msg("Golden build: session stopped, build complete")
				g.updateDockerCacheStatus(context.Background(), projectID, func(s *types.DockerCacheState) {
					now := time.Now()
					s.Status = "ready"
					s.LastReadyAt = &now
					s.BuildSessionID = ""
					s.Error = ""
				})
				return
			}

			log.Debug().Str("project_id", projectID).Str("session_id", sessionID).
				Msg("Golden build: still running, polling again in 15s")
		}
	}
}
