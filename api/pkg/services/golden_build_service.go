package services

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/helixml/helix/api/pkg/hydra"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// goldenBuildTimeout bounds a single golden build's wall-clock duration (see
// types.GoldenBuildTimeout). It is passed to Hydra in the create request so the
// sandbox-side result monitor uses the same deadline. On timeout the build is
// marked failed AND its container is stopped, so a blown build doesn't keep
// compiling and burning CPU.
const goldenBuildTimeout = types.GoldenBuildTimeout

// goldenBuildResultPolls is how many extra polls waitForGoldenBuildCompletion
// waits for hydra to record a result after the build container has gone.
const goldenBuildResultPolls = 4

// goldenBuildPollInterval is how often waitForGoldenBuildCompletion checks the
// build container. Var so tests can shorten it.
var goldenBuildPollInterval = 15 * time.Second

// goldenBuildClaimGrace is how long a claimed build may go without a build
// session before reconciliation treats the claim as abandoned (the API died
// between claiming the build and creating its session). Var for tests.
var goldenBuildClaimGrace = 2 * time.Minute

// goldenBuildRetryBackoff is how long to wait before retrying an interrupted
// attempt: 1m, 2m, 4m, ... Var so tests can retry immediately.
var goldenBuildRetryBackoff = func(attempt int) time.Duration {
	return time.Minute << (attempt - 1)
}

// buildTrigger says why a build is being claimed.
type buildTrigger int

const (
	// triggerNew is a merge to main or a manual build: it starts a fresh retry
	// budget, or queues a rebuild if a build is already running.
	triggerNew buildTrigger = iota
	// triggerRetry retries an interrupted attempt of the current trigger.
	triggerRetry
)

// GoldenBuildService manages golden Docker cache builds for projects.
// When a merge to main happens and the project has AutoWarmDockerCache enabled,
// it triggers a golden build session that runs the startup script to populate
// the Docker cache, then promotes the result to the project's golden snapshot.
//
// Builds are fanned out to ALL online sandboxes so every sandbox has a warm
// cache. All build state (status, attempt, pending rebuild, interruption) lives
// in the golden_builds table, so an API restart loses nothing: ReconcileSandbox
// resumes monitoring running builds, retries interrupted ones and starts
// pending rebuilds when the sandbox's Hydra (re)connects.
//
// A build that ends without a result because its container, sandbox or Hydra
// went away is an interruption and is retried up to GoldenBuildMaxAttempts
// times per trigger. A build whose startup script exits non-zero, times out or
// fails to promote is a real failure and is not retried.
type GoldenBuildService struct {
	store             store.Store
	containerExecutor ContainerExecutor
	specTaskService   *SpecDrivenTaskService

	// monitoring holds the build session IDs a goroutine in THIS process is
	// running or polling. Goroutine liveness is inherently per-process; this
	// only stops reconciliation starting a second monitor for the same build.
	// Whether a build is running is always read from golden_builds.
	mu         sync.Mutex
	monitoring map[string]bool

	// ctx parents every background build and monitor; cancelling it (process
	// shutdown) ends them without recording an outcome. wg tracks them.
	ctx context.Context
	wg  sync.WaitGroup
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
		monitoring:        make(map[string]bool),
		ctx:               context.Background(),
	}
}

// goBackground runs fn in a goroutine tracked by g.wg.
func (g *GoldenBuildService) goBackground(fn func()) {
	g.wg.Add(1)
	go func() {
		defer g.wg.Done()
		fn()
	}()
}

// startMonitoring marks sessionID as owned by a goroutine in this process.
// Returns false if one already owns it.
func (g *GoldenBuildService) startMonitoring(sessionID string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.monitoring[sessionID] {
		return false
	}
	g.monitoring[sessionID] = true
	return true
}

func (g *GoldenBuildService) stopMonitoring(sessionID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.monitoring, sessionID)
}

func (g *GoldenBuildService) isMonitoring(sessionID string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.monitoring[sessionID]
}

// staleBuilding reports whether a "building" row has outlived any build that
// could still be running (its monitor's deadline passed without recording an
// outcome, e.g. the sandbox never came back).
func staleBuilding(s *types.SandboxCacheState) bool {
	return s.LastBuildAt != nil && time.Since(*s.LastBuildAt) > goldenBuildTimeout+5*time.Minute
}

// claimBuild atomically moves the project's golden build on sandboxID to
// "building". If a build is already running, a new trigger is recorded as a
// pending rebuild instead (queued=true). A retry only claims a build that is
// still waiting to be retried.
func (g *GoldenBuildService) claimBuild(ctx context.Context, projectID, sandboxID string, trigger buildTrigger) (claimed, queued bool, err error) {
	_, err = g.store.UpdateGoldenBuild(ctx, projectID, sandboxID, func(s *types.SandboxCacheState) bool {
		claimed, queued = false, false
		if s.Status == types.GoldenBuildStatusBuilding && !staleBuilding(s) {
			if trigger == triggerNew && !s.PendingRebuild {
				s.PendingRebuild = true
				queued = true
				return true
			}
			queued = trigger == triggerNew
			return false
		}
		now := time.Now()
		switch trigger {
		case triggerRetry:
			if s.Status != types.GoldenBuildStatusRetrying {
				return false
			}
			s.Attempt++
		default:
			s.Attempt = 1
			s.TriggeredAt = &now
		}
		s.Status = types.GoldenBuildStatusBuilding
		s.BuildSessionID = ""
		s.LastBuildAt = &now
		s.Error = ""
		s.NextRetryAt = nil
		s.PendingRebuild = false
		if trigger == triggerNew {
			s.InterruptReason = ""
		}
		claimed = true
		return true
	})
	return claimed, queued, err
}

// startBuild claims the build on sandboxID and, if claimed, runs it in the
// background. This is the only way a build starts: new triggers, pending
// rebuilds and retries all come through here.
func (g *GoldenBuildService) startBuild(ctx context.Context, project *types.Project, sandboxID string, trigger buildTrigger) (claimed, queued bool, err error) {
	claimed, queued, err = g.claimBuild(ctx, project.ID, sandboxID, trigger)
	if err != nil {
		return false, false, err
	}
	if queued {
		log.Info().Str("project_id", project.ID).Str("sandbox_id", sandboxID).
			Msg("Golden build already running on sandbox, queued rebuild for when it finishes")
	}
	if !claimed {
		return false, queued, nil
	}
	log.Info().
		Str("project_id", project.ID).
		Str("project_name", project.Name).
		Str("sandbox_id", sandboxID).
		Bool("retry", trigger == triggerRetry).
		Msg("Triggering golden Docker cache build on sandbox")
	g.goBackground(func() { g.runGoldenBuildOnSandbox(project, sandboxID) })
	return true, false, nil
}

// ReconcileSandbox drives every golden build on sandboxID forward from its
// persisted state: resume monitoring builds no goroutine in this process is
// polling (API restart), retry interrupted builds whose backoff has passed,
// and start pending rebuilds. Called when the sandbox's Hydra connects and
// periodically while it is online. Safe to call concurrently and repeatedly.
func (g *GoldenBuildService) ReconcileSandbox(ctx context.Context, sandboxID string) {
	builds, err := g.store.ListGoldenBuilds(ctx, &store.ListGoldenBuildsQuery{SandboxID: sandboxID})
	if err != nil {
		log.Error().Err(err).Str("sandbox_id", sandboxID).Msg("Golden build reconcile: failed to list builds")
		return
	}
	for _, b := range builds {
		g.reconcileBuild(ctx, b)
	}
}

// reconcileBuild decides and starts the next step for one build.
func (g *GoldenBuildService) reconcileBuild(ctx context.Context, b *types.SandboxCacheState) {
	logger := log.With().Str("project_id", b.ProjectID).Str("sandbox_id", b.SandboxID).Logger()
	switch {
	case b.Status == types.GoldenBuildStatusBuilding && b.BuildSessionID == "":
		if b.LastBuildAt != nil && time.Since(*b.LastBuildAt) < goldenBuildClaimGrace {
			return // being started right now
		}
		g.finishInterrupted(ctx, b.ProjectID, b.SandboxID, "", "the API restarted before the build container was created")

	case b.Status == types.GoldenBuildStatusBuilding:
		if !g.startMonitoring(b.BuildSessionID) {
			return
		}
		logger.Info().Str("session_id", b.BuildSessionID).Int("attempt", b.Attempt).
			Msg("Golden build reconcile: resuming monitor for persisted build")
		deadline := time.Now().Add(goldenBuildTimeout)
		if b.LastBuildAt != nil {
			deadline = b.LastBuildAt.Add(goldenBuildTimeout)
		}
		g.goBackground(func() {
			defer g.stopMonitoring(b.BuildSessionID)
			ctx, cancel := context.WithDeadline(g.ctx, deadline)
			defer cancel()
			g.waitForGoldenBuildCompletion(ctx, b.ProjectID, b.SandboxID, b.BuildSessionID)
		})

	case b.Status == types.GoldenBuildStatusRetrying || (!b.Active() && b.PendingRebuild):
		trigger := triggerNew
		if !b.PendingRebuild {
			if b.NextRetryAt != nil && time.Now().Before(*b.NextRetryAt) {
				return
			}
			trigger = triggerRetry
		}
		project, err := g.store.GetProject(ctx, b.ProjectID)
		if err != nil {
			logger.Error().Err(err).Msg("Golden build reconcile: failed to get project")
			return
		}
		if trigger == triggerRetry && !project.Metadata.AutoWarmDockerCache {
			g.setFailed(ctx, b.ProjectID, b.SandboxID, "", fmt.Sprintf("Interrupted (%s); not retried because auto-warm is off", b.InterruptReason))
			return
		}
		if _, _, err := g.startBuild(ctx, project, b.SandboxID, trigger); err != nil {
			logger.Error().Err(err).Msg("Golden build reconcile: failed to start build")
		}
	}
}

// TriggerGoldenBuild starts golden builds on all online sandboxes if the setting is enabled
// and no build is already running on each sandbox. Called when code is merged to main.
func (g *GoldenBuildService) TriggerGoldenBuild(ctx context.Context, project *types.Project) {
	if project == nil {
		return
	}

	if !project.Metadata.AutoWarmDockerCache {
		log.Info().
			Str("project_id", project.ID).
			Msg("Skipping golden build on merge to main: auto_warm_docker_cache is disabled for project")
		return
	}

	if _, _, _, err := g.fanOutBuilds(ctx, project); err != nil {
		log.Error().Err(err).Str("project_id", project.ID).Msg("Golden build: failed to fan out builds")
	}
}

// TriggerManualGoldenBuild starts golden builds on all online sandboxes regardless of the
// AutoWarmDockerCache setting. Used by the "Prime Cache" button in the UI.
func (g *GoldenBuildService) TriggerManualGoldenBuild(ctx context.Context, project *types.Project) error {
	if project == nil {
		return fmt.Errorf("project is nil")
	}

	started, queued, online, err := g.fanOutBuilds(ctx, project)
	if err != nil {
		return err
	}
	if started == 0 {
		if online == 0 {
			return fmt.Errorf("no online sandboxes available for golden build")
		}
		if queued > 0 {
			return fmt.Errorf("golden build already running on all %d sandbox(es)", queued)
		}
		return fmt.Errorf("no sandboxes could start a golden build")
	}
	return nil
}

// CancelGoldenBuilds stops all running golden builds for a project and drops
// any pending rebuild or scheduled retry.
func (g *GoldenBuildService) CancelGoldenBuilds(ctx context.Context, project *types.Project) error {
	if project == nil {
		return fmt.Errorf("project is nil")
	}

	builds, err := g.store.ListGoldenBuilds(ctx, &store.ListGoldenBuildsQuery{ProjectID: project.ID, ActiveOnly: true})
	if err != nil {
		return fmt.Errorf("list golden builds: %w", err)
	}

	cancelled := 0
	for _, b := range builds {
		if b.BuildSessionID != "" {
			if err := g.containerExecutor.StopDesktop(ctx, b.BuildSessionID); err != nil {
				return fmt.Errorf("stop golden build %s on sandbox %s: %w", b.BuildSessionID, b.SandboxID, err)
			}
		}
		// The monitor notices the build is no longer its own and exits quietly.
		if _, err := g.store.UpdateGoldenBuild(ctx, project.ID, b.SandboxID, func(s *types.SandboxCacheState) bool {
			s.Status = types.GoldenBuildStatusNone
			s.BuildSessionID = ""
			s.Error = ""
			s.InterruptReason = ""
			s.NextRetryAt = nil
			s.PendingRebuild = false
			return true
		}); err != nil {
			return fmt.Errorf("cancel golden build on sandbox %s: %w", b.SandboxID, err)
		}
		cancelled++
	}

	if cancelled == 0 {
		return fmt.Errorf("no active golden builds found")
	}

	log.Info().Str("project_id", project.ID).Int("cancelled", cancelled).
		Msg("Cancelled golden builds")
	return nil
}

// fanOutBuilds starts (or queues) a new-trigger build on every online sandbox.
func (g *GoldenBuildService) fanOutBuilds(ctx context.Context, project *types.Project) (started, queued, online int, err error) {
	sandboxes, err := g.store.ListSandboxInstances(ctx)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("failed to list sandboxes: %w", err)
	}

	for _, sb := range sandboxes {
		if sb.Status != "online" {
			continue
		}
		online++
		claimed, wasQueued, err := g.startBuild(ctx, project, sb.ID, triggerNew)
		if err != nil {
			log.Error().Err(err).Str("project_id", project.ID).Str("sandbox_id", sb.ID).
				Msg("Golden build: failed to claim build")
			continue
		}
		if claimed {
			started++
		}
		if wasQueued {
			queued++
		}
	}
	return started, queued, online, nil
}

// runGoldenBuildOnSandbox runs a claimed golden build on a specific sandbox.
func (g *GoldenBuildService) runGoldenBuildOnSandbox(project *types.Project, sandboxID string) {
	ctx, cancel := context.WithTimeout(g.ctx, goldenBuildTimeout)
	defer cancel()

	// Get project repositories
	projectRepos, err := g.store.ListGitRepositories(ctx, &types.ListGitRepositoriesRequest{
		ProjectID: project.ID,
	})
	if err != nil {
		log.Error().Err(err).Str("project_id", project.ID).Str("sandbox_id", sandboxID).Msg("Golden build: failed to list project repos")
		g.setFailed(ctx, project.ID, sandboxID, "", fmt.Sprintf("Failed to list repos: %v", err))
		return
	}

	if len(projectRepos) == 0 {
		log.Warn().Str("project_id", project.ID).Str("sandbox_id", sandboxID).Msg("Golden build: project has no repositories")
		g.setFailed(ctx, project.ID, sandboxID, "", "Project has no repositories")
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
		// Excludes the session from the desktop idle checker: a golden build
		// never has interactions, so it would look idle after an hour.
		Metadata: types.SessionMetadata{GoldenBuild: true},
	}

	session, err = g.store.CreateSession(ctx, *session)
	if err != nil {
		log.Error().Err(err).Str("project_id", project.ID).Str("sandbox_id", sandboxID).Msg("Golden build: failed to create session")
		g.setFailed(ctx, project.ID, sandboxID, "", fmt.Sprintf("Failed to create session: %v", err))
		return
	}

	// Own the session before persisting it, so reconciliation never starts a
	// second monitor for it.
	g.startMonitoring(session.ID)
	defer g.stopMonitoring(session.ID)

	state, err := g.store.UpdateGoldenBuild(ctx, project.ID, sandboxID, func(s *types.SandboxCacheState) bool {
		if s.Status != types.GoldenBuildStatusBuilding || s.BuildSessionID != "" {
			return false // cancelled, or superseded, since the claim
		}
		s.BuildSessionID = session.ID
		return true
	})
	if err != nil {
		log.Error().Err(err).Str("project_id", project.ID).Str("sandbox_id", sandboxID).Msg("Golden build: failed to record build session")
		return
	}
	if state.BuildSessionID != session.ID {
		log.Info().Str("project_id", project.ID).Str("sandbox_id", sandboxID).
			Msg("Golden build: cancelled before the container was started")
		return
	}

	log.Info().
		Str("project_id", project.ID).
		Str("session_id", session.ID).
		Str("sandbox_id", sandboxID).
		Int("attempt", state.Attempt).
		Msg("Golden build: session created")

	// Get API key for the golden build session
	if g.specTaskService == nil {
		g.setFailed(ctx, project.ID, sandboxID, session.ID, "specTaskService not available")
		return
	}
	userAPIKey, err := g.specTaskService.GetOrCreateSessionAPIKey(ctx, &SessionAPIKeyRequest{
		OrganizationID: project.OrganizationID,
		UserID:         project.UserID,
		SessionID:      session.ID,
	})
	if err != nil {
		log.Error().Err(err).Str("project_id", project.ID).Str("sandbox_id", sandboxID).Msg("Golden build: failed to create API key")
		g.setFailed(ctx, project.ID, sandboxID, session.ID, fmt.Sprintf("Failed to create API key: %v", err))
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
		DisplayWidth:        1920,
		DisplayHeight:       1080,
		DisplayRefreshRate:  60,
		Resolution:          "1080p",
		ZoomLevel:           200,
		GoldenBuild:         true,
		// Hydra must not give up on the build before we do.
		GoldenBuildTimeoutSeconds: int(goldenBuildTimeout / time.Second),
		SandboxID:                 sandboxID,
	}

	// Start the desktop container. A start failure is environmental (sandbox
	// restarting, disk pressure, image pull) rather than the startup script,
	// so it is an interruption, bounded by the retry budget.
	_, err = g.containerExecutor.StartDesktop(ctx, agent)
	if err != nil {
		log.Error().Err(err).
			Str("project_id", project.ID).
			Str("session_id", session.ID).
			Str("sandbox_id", sandboxID).
			Msg("Golden build: failed to start desktop")
		g.finishInterrupted(ctx, project.ID, sandboxID, session.ID, fmt.Sprintf("failed to start build container: %v", err))
		return
	}

	log.Info().
		Str("project_id", project.ID).
		Str("session_id", session.ID).
		Str("sandbox_id", sandboxID).
		Msg("Golden build: container started, polling for completion")

	g.waitForGoldenBuildCompletion(ctx, project.ID, sandboxID, session.ID)
}

// stoppedByIdleChecker reports whether the desktop idle checker stopped the
// session. It should never stop a golden build (ListIdleDesktops excludes
// them), but say so if it did rather than blame the startup script.
func (g *GoldenBuildService) stoppedByIdleChecker(sessionID string) bool {
	session, err := g.store.GetSession(context.Background(), sessionID)
	return err == nil && session.Metadata.ExternalAgentStatus == "terminated_idle"
}

// stillOurs reports whether sessionID is still the project's current build on
// sandboxID. A cancelled or superseded build's monitor must stop quietly.
func (g *GoldenBuildService) stillOurs(ctx context.Context, projectID, sandboxID, sessionID string) bool {
	state, err := g.store.GetGoldenBuild(ctx, projectID, sandboxID)
	if errors.Is(err, store.ErrNotFound) {
		return false
	}
	if err != nil {
		return true // can't tell; keep monitoring
	}
	return state.Status == types.GoldenBuildStatusBuilding && state.BuildSessionID == sessionID
}

// waitForGoldenBuildCompletion polls until the golden build container exits,
// then queries Hydra for the build result and records the outcome. While the
// sandbox's Hydra is unreachable (Hydra restart, sandbox recreate) it keeps
// waiting: once Hydra is back it either still has the container (the build
// carries on) or doesn't (an interruption, retried).
func (g *GoldenBuildService) waitForGoldenBuildCompletion(ctx context.Context, projectID, sandboxID, sessionID string) {
	ticker := time.NewTicker(goldenBuildPollInterval)
	defer ticker.Stop()

	logger := log.With().Str("project_id", projectID).Str("sandbox_id", sandboxID).Str("session_id", sessionID).Logger()
	missingResultPolls := 0
	var unreachable error
	for {
		select {
		case <-ctx.Done():
			if g.ctx.Err() != nil {
				return // shutting down; reconciliation resumes the build
			}
			bg := context.Background()
			if unreachable != nil {
				logger.Warn().Err(unreachable).Msg("Golden build: deadline passed while the sandbox was unreachable")
				g.finishInterrupted(bg, projectID, sandboxID, sessionID, fmt.Sprintf("sandbox unreachable until the build deadline: %v", unreachable))
				return
			}
			logger.Warn().Dur("timeout", goldenBuildTimeout).
				Msg("Golden build: timed out waiting for completion, stopping container")
			// Stop the container so a blown build doesn't keep compiling and
			// burning CPU. ctx is already cancelled, so use a fresh context.
			stopCtx, stopCancel := context.WithTimeout(bg, 30*time.Second)
			stopErr := g.containerExecutor.StopDesktop(stopCtx, sessionID)
			stopCancel()
			if stopErr != nil {
				logger.Warn().Err(stopErr).Msg("Golden build: failed to stop timed-out container")
			}
			g.setFailed(bg, projectID, sandboxID, sessionID, fmt.Sprintf("Build timed out (%s)", goldenBuildTimeout))
			return

		case <-ticker.C:
			if !g.stillOurs(ctx, projectID, sandboxID, sessionID) {
				logger.Info().Msg("Golden build: no longer the current build (cancelled or superseded), stopping monitor")
				return
			}

			running, err := g.containerExecutor.GoldenBuildContainerRunning(ctx, sandboxID, sessionID)
			if err != nil {
				if unreachable == nil {
					logger.Warn().Err(err).Msg("Golden build: sandbox unreachable, waiting for it to come back")
				}
				unreachable = err
				continue
			}
			if unreachable != nil {
				logger.Info().Bool("container_running", running).Msg("Golden build: sandbox reachable again")
				unreachable = nil
			}
			if running {
				missingResultPolls = 0
				logger.Debug().Dur("poll_interval", goldenBuildPollInterval).Msg("Golden build: still running, polling again")
				continue
			}

			result, err := g.containerExecutor.GetGoldenBuildResult(ctx, sandboxID, projectID)
			if err != nil {
				logger.Warn().Err(err).Msg("Golden build: failed to get build result from sandbox")
				unreachable = err
				continue
			}
			// Results are keyed by project: one from an earlier build is not ours.
			if result != nil && result.SessionID != "" && result.SessionID != sessionID {
				result = nil
			}
			// Hydra records the outcome a few seconds after the container goes
			// away (e.g. when it was stopped from outside); give it a few polls.
			if result == nil && missingResultPolls < goldenBuildResultPolls {
				missingResultPolls++
				continue
			}

			g.recordOutcome(ctx, projectID, sandboxID, sessionID, result)
			return
		}
	}
}

// recordOutcome classifies a finished build's result and records it.
func (g *GoldenBuildService) recordOutcome(ctx context.Context, projectID, sandboxID, sessionID string, result *hydra.GoldenBuildResult) {
	logger := log.With().Str("project_id", projectID).Str("sandbox_id", sandboxID).Str("session_id", sessionID).Logger()

	if result != nil && result.Success {
		logger.Info().Int64("cache_size_bytes", result.CacheSizeBytes).Msg("Golden build: completed successfully")
		g.finish(ctx, projectID, sandboxID, sessionID, func(s *types.SandboxCacheState) {
			now := time.Now()
			s.Status = types.GoldenBuildStatusReady
			s.LastReadyAt = &now
			s.SizeBytes = result.CacheSizeBytes
		})
		return
	}

	var interruption string
	switch {
	case result == nil:
		// Hydra no longer knows the container and never recorded a result:
		// the sandbox was recreated or Hydra restarted after it went away.
		interruption = "build container disappeared without a result (sandbox or Hydra restarted)"
	case result.ExitCode == "exited":
		// The container stopped without the startup script finishing.
		interruption = result.Error
	}
	if interruption != "" {
		if g.stoppedByIdleChecker(sessionID) {
			interruption = "stopped by the desktop idle checker: " + interruption
		}
		logger.Warn().Str("reason", interruption).Msg("Golden build: interrupted")
		g.finishInterrupted(ctx, projectID, sandboxID, sessionID, interruption)
		return
	}

	// The startup script ran and failed, the build timed out, or promoting
	// the cache failed: a real failure, not retried.
	errMsg := fmt.Sprintf("Startup script exited with code %s", result.ExitCode)
	if result.Error != "" {
		errMsg = result.Error
	}
	logger.Warn().Str("error", errMsg).Msg("Golden build: failed")
	g.setFailed(ctx, projectID, sandboxID, sessionID, errMsg)
}

// setFailed records a real failure for the build owned by sessionID ("" for
// a build that failed before its session existed).
func (g *GoldenBuildService) setFailed(ctx context.Context, projectID, sandboxID, sessionID, errMsg string) {
	g.finish(ctx, projectID, sandboxID, sessionID, func(s *types.SandboxCacheState) {
		s.Status = types.GoldenBuildStatusFailed
		s.Error = errMsg
	})
}

// finishInterrupted records that the build owned by sessionID ended without a
// result. Auto-warm projects retry after a backoff until the trigger's attempt
// budget is spent; the next attempt is started by reconcileBuild.
func (g *GoldenBuildService) finishInterrupted(ctx context.Context, projectID, sandboxID, sessionID, reason string) {
	if sessionID != "" {
		// The dead attempt's container is gone, but its Docker data (a session
		// zvol of tens of GB on ZFS hosts) and workspace would otherwise sit
		// until the orphan reaper's grace period. A promoted build's data has
		// already moved into the golden, so destroying is always safe here.
		// Best-effort: the orphan reaper is the backstop.
		if err := g.containerExecutor.DestroyDesktop(context.WithoutCancel(ctx), sessionID, ""); err != nil {
			log.Warn().Err(err).Str("project_id", projectID).Str("sandbox_id", sandboxID).Str("session_id", sessionID).
				Msg("Golden build: failed to destroy interrupted build's resources")
		}
	}
	autoWarm := false
	if project, err := g.store.GetProject(ctx, projectID); err == nil {
		autoWarm = project.Metadata.AutoWarmDockerCache
	} else {
		log.Warn().Err(err).Str("project_id", projectID).Msg("Golden build: failed to get project, not retrying interrupted build")
	}
	g.finish(ctx, projectID, sandboxID, sessionID, func(s *types.SandboxCacheState) {
		s.InterruptReason = reason
		switch {
		case s.PendingRebuild:
			// A newer trigger is waiting; it supersedes the retry.
			s.Status = types.GoldenBuildStatusRetrying
		case !autoWarm:
			s.Status = types.GoldenBuildStatusFailed
			s.Error = fmt.Sprintf("Interrupted (%s); not retried because auto-warm is off", reason)
		case s.Attempt >= types.GoldenBuildMaxAttempts:
			s.Status = types.GoldenBuildStatusFailed
			s.Error = fmt.Sprintf("Interrupted %d times, giving up: %s", s.Attempt, reason)
		default:
			next := time.Now().Add(goldenBuildRetryBackoff(s.Attempt))
			s.Status = types.GoldenBuildStatusRetrying
			s.NextRetryAt = &next
		}
	})
}

// finish applies a terminal transition to the build owned by sessionID, then
// starts whatever comes next (a pending rebuild or a due retry). The
// transition is skipped if the build was cancelled or superseded meanwhile.
func (g *GoldenBuildService) finish(ctx context.Context, projectID, sandboxID, sessionID string, apply func(*types.SandboxCacheState)) {
	ctx = context.WithoutCancel(ctx)
	applied := false
	state, err := g.store.UpdateGoldenBuild(ctx, projectID, sandboxID, func(s *types.SandboxCacheState) bool {
		if s.Status != types.GoldenBuildStatusBuilding || s.BuildSessionID != sessionID {
			return false
		}
		s.BuildSessionID = ""
		s.Error = ""
		s.NextRetryAt = nil
		apply(s)
		applied = true
		return true
	})
	if err != nil {
		log.Error().Err(err).Str("project_id", projectID).Str("sandbox_id", sandboxID).Msg("Golden build: failed to record outcome")
		return
	}
	if !applied {
		return
	}
	log.Info().
		Str("project_id", projectID).
		Str("sandbox_id", sandboxID).
		Str("session_id", sessionID).
		Str("status", state.Status).
		Int("attempt", state.Attempt).
		Bool("pending_rebuild", state.PendingRebuild).
		Str("error", state.Error).
		Str("interrupt_reason", state.InterruptReason).
		Msg("Golden build: outcome recorded")
	g.stopMonitoring(sessionID)
	g.reconcileBuild(ctx, state)
}
