// Package webservice orchestrates the deploy lifecycle for a project's
// hosted web application.
//
// Deploys are IN-PLACE on a single long-lived, runner-pinned sandbox per
// project — NOT blue/green. A web app that owns a database keeps that DB on
// disk under /data, and two processes must never open the same on-disk
// database concurrently (Postgres refuses; SQLite corrupts). So a deploy
// stops the running app BEFORE starting the new one — at most one instance
// ever touches /data. This costs a brief restart window of downtime, which is
// the correct trade for data safety. Operators who need zero-downtime
// blue/green or horizontal scaling point Helix at an external Kubernetes
// cluster instead (see design doc 002107).
//
// The durable data dir /data is bind-mounted by the sandbox provisioner,
// keyed by project (not sandbox id), so it survives redeploys and reboots.
// The sandbox is Persistent=true, so the scheduler's sticky guard pins it to
// its runner and refuses to relocate it (which would orphan local-disk data).
//
// The Controller wires together pre-existing primitives: sandbox.Controller
// for provisioning, hydra.RevDialClient for in-container exec, and
// store.Store for persistence.
package webservice

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/hydra"
	"github.com/helixml/helix/api/pkg/sandbox"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// Controller orchestrates project web service deploys.
type Controller struct {
	store         store.Store
	sandboxes     *sandbox.Controller
	provisionWait time.Duration // upper bound for waiting for sandbox status=running
	provisionPoll time.Duration // poll interval while waiting
	bootstrapWait time.Duration // upper bound for the in-place deploy exec (clone/fetch/launch)
	readinessWait time.Duration // upper bound for waiting for the app to bind to its port
	readinessPoll time.Duration // poll interval while waiting
}

// New constructs a Controller. The defaults are sized for typical
// headless containers (~30s cold-start, up to 90s for the app to bind).
func New(s store.Store, sc *sandbox.Controller) *Controller {
	return &Controller{
		store:         s,
		sandboxes:     sc,
		provisionWait: 3 * time.Minute,
		provisionPoll: 2 * time.Second,
		bootstrapWait: 5 * time.Minute,
		readinessWait: 90 * time.Second,
		readinessPoll: 2 * time.Second,
	}
}

// DeployRequest is the input to Redeploy.
type DeployRequest struct {
	ProjectID string
	Owner     string // user id of the actor — recorded for audit, owner of the new sandbox row
	CommitSHA string // optional; empty means "current HEAD of the primary repo"
}

// Redeploy is the manual / auto-deploy primitive: provision a new
// sandbox, bootstrap the project into it, and cut routing over to the
// new container. Records a WebServiceDeploy row at every state
// transition so the UI shows a coherent history.
//
// Returns the deploy row immediately (status=pending or building);
// the heavy work — provisioning, exec, cutover — runs asynchronously
// in a goroutine. Callers can poll WebServiceDeploy.Status to observe
// progress.
func (c *Controller) Redeploy(ctx context.Context, req DeployRequest) (*types.WebServiceDeploy, error) {
	if req.ProjectID == "" {
		return nil, errors.New("project_id is required")
	}
	if req.Owner == "" {
		return nil, errors.New("owner is required")
	}

	project, err := c.store.GetProject(ctx, req.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("get project: %w", err)
	}

	state, err := c.store.GetProjectWebServiceState(ctx, req.ProjectID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, fmt.Errorf("web service is not enabled for this project — call PUT /web-service first")
		}
		return nil, fmt.Errorf("get web service state: %w", err)
	}
	if !state.Enabled {
		return nil, errors.New("web service is disabled for this project")
	}

	repo, err := c.resolvePrimaryRepo(ctx, project)
	if err != nil {
		return nil, fmt.Errorf("resolve primary repo: %w", err)
	}

	deploy := &types.WebServiceDeploy{
		ProjectID: req.ProjectID,
		CommitSHA: req.CommitSHA,
		Status:    types.WebServiceDeployStatusPending,
	}
	if err := c.store.CreateWebServiceDeploy(ctx, deploy); err != nil {
		return nil, fmt.Errorf("create deploy row: %w", err)
	}

	go c.runDeploy(deploy.ID, req, project, repo, state)

	return deploy, nil
}

// runDeploy executes the long-running ensure-sandbox → in-place deploy →
// readiness flow. Detached so the API request returns quickly. Every state
// transition is persisted so the UI can reflect progress.
//
// In-place model: the project has ONE web-service sandbox. A deploy stops the
// running app, fetches/checks out the new code, and restarts the app — so the
// durable /data is only ever opened by one process (single writer). On
// readiness failure we roll back to the previously-live commit so the site
// returns to last-known-good against the same intact /data.
func (c *Controller) runDeploy(
	deployID string,
	req DeployRequest,
	project *types.Project,
	repo *types.GitRepository,
	state *types.ProjectWebServiceState,
) {
	ctx, cancel := context.WithTimeout(context.Background(), c.provisionWait+c.bootstrapWait+2*time.Minute)
	defer cancel()

	if err := c.markBuilding(ctx, deployID); err != nil {
		log.Warn().Err(err).Str("deploy_id", deployID).Msg("failed to mark deploy building")
	}

	// Capture the commit that is currently live so we can roll back to it if
	// this deploy fails its readiness check.
	previousSHA := c.lastLiveSHA(ctx, req.ProjectID)

	// Get-or-create the project's single, runner-pinned web-service sandbox.
	sb, err := c.ensureSandbox(ctx, req, project, state)
	if err != nil {
		c.markFailed(ctx, deployID, "", fmt.Sprintf("ensure sandbox: %s", err))
		return
	}

	// Deploy the requested code in place (stops the old app first).
	if err := c.deployInPlace(ctx, sb, repo, req.CommitSHA, state.ContainerPort); err != nil {
		c.markFailed(ctx, deployID, sb.ID, fmt.Sprintf("deploy: %s", err))
		return
	}

	// Poll the container port until the app responds. We treat any HTTP
	// response (even 4xx/5xx) as "the app is bound" — what matters here
	// is that the listener exists, not that the app likes the request.
	if err := c.waitForReady(ctx, sb.ID, state.ContainerPort); err != nil {
		// Roll back to the last-known-good commit so the site comes back up
		// against the same intact /data. The data is never touched either way.
		c.rollback(ctx, sb, repo, previousSHA, state.ContainerPort)
		c.markFailed(ctx, deployID, sb.ID, fmt.Sprintf("readiness: %s", err))
		return
	}

	// Same sandbox across deploys, but keep active_sandbox_id authoritative.
	if err := c.store.SetActiveWebServiceSandbox(ctx, req.ProjectID, sb.ID); err != nil {
		c.markFailed(ctx, deployID, sb.ID, fmt.Sprintf("set active sandbox: %s", err))
		return
	}

	// Mark this deploy live; supersede the previous live row if any.
	c.markLive(ctx, deployID, sb.ID)
	c.markSupersededPrevious(ctx, deployID, req.ProjectID)
}

// ensureSandbox returns the project's single web-service sandbox, creating it
// on the first deploy. The sandbox is Persistent=true (so the scheduler pins
// it to its runner and refuses to relocate it) and never auto-expires. On
// first creation we record the bound runner onto the project state for
// visibility. If an existing sandbox is not running, we fail loudly rather
// than silently moving the service off its data — the pinned runner is likely
// offline.
func (c *Controller) ensureSandbox(ctx context.Context, req DeployRequest, project *types.Project, state *types.ProjectWebServiceState) (*types.Sandbox, error) {
	if state.ActiveSandboxID != "" {
		sb, err := c.sandboxes.Get(ctx, state.ActiveSandboxID)
		if err == nil && sb != nil {
			if sb.Status != types.SandboxStatusRunning {
				return nil, fmt.Errorf("web service sandbox %s is not running (status=%s); its pinned runner may be offline — refusing to relocate (data is on the original runner)", sb.ID, sb.Status)
			}
			return sb, nil
		}
		// Row gone (deleted): fall through and provision a fresh one. The
		// durable /data dir is keyed by project, so it is reattached.
		log.Warn().Err(err).Str("project_id", project.ID).Str("sandbox_id", state.ActiveSandboxID).
			Msg("recorded web-service sandbox is gone; provisioning a fresh one (durable /data is project-keyed and reattaches)")
	}

	createReq := &types.CreateSandboxRequest{
		Name:           fmt.Sprintf("web-service-%s", project.ID),
		Runtime:        types.SandboxRuntimeHeadlessUbuntu,
		ProjectID:      project.ID,
		Purpose:        types.SandboxPurposeWebService,
		Persistent:     true,
		TimeoutSeconds: -1, // never auto-expire — web services are long-lived
	}
	sb, err := c.sandboxes.Create(ctx, project.OrganizationID, req.Owner, createReq)
	if err != nil {
		return nil, err
	}

	// Poll until running or fail.
	deadline := time.Now().Add(c.provisionWait)
	for time.Now().Before(deadline) {
		latest, err := c.sandboxes.Get(ctx, sb.ID)
		if err != nil {
			return nil, fmt.Errorf("poll sandbox: %w", err)
		}
		switch latest.Status {
		case types.SandboxStatusRunning:
			// Record this sandbox as active and pin the project to its runner.
			if err := c.store.SetActiveWebServiceSandbox(ctx, project.ID, latest.ID); err != nil {
				log.Warn().Err(err).Str("project_id", project.ID).Msg("failed to record active web-service sandbox")
			}
			if latest.HostDeviceID != "" {
				if err := c.store.SetWebServiceHostDeviceID(ctx, project.ID, latest.HostDeviceID); err != nil {
					log.Warn().Err(err).Str("project_id", project.ID).Msg("failed to record web-service runner pin")
				}
			}
			return latest, nil
		case types.SandboxStatusFailed, types.SandboxStatusStopped:
			return nil, fmt.Errorf("sandbox %s reached terminal status %s before running", latest.ID, latest.Status)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(c.provisionPoll):
		}
	}
	return nil, fmt.Errorf("sandbox %s did not reach running state within %s", sb.ID, c.provisionWait)
}

// deployInPlace execs a shell inside the existing web-service sandbox that:
//  1. stops the previously-launched app (so /data has a single writer),
//  2. clones the repo on first run / fetches + checks out the requested SHA,
//  3. launches `.helix/startup.sh` in its own session, recording its PID.
//
// The exec runs to completion once the app is LAUNCHED (not necessarily
// ready) — the app is backgrounded via setsid. Readiness is polled separately.
func (c *Controller) deployInPlace(ctx context.Context, sb *types.Sandbox, repo *types.GitRepository, sha string, containerPort int) error {
	hydraClient, err := c.sandboxes.HydraClient(sb)
	if err != nil {
		return err
	}

	script := deployScript(repo.CloneURL, sha, containerPort)

	_, execErr := hydraClient.RunSandboxCommand(ctx, sb.ID, &hydra.ExecRequest{
		SandboxID:      sb.ID,
		Cmd:            "/bin/bash",
		Args:           []string{"-c", script},
		Cwd:            "/",
		TimeoutSeconds: int(c.bootstrapWait.Seconds()),
	})
	return execErr
}

// rollback re-deploys a previously-live commit in place after a failed
// deploy, so the site returns to last-known-good. Best-effort: failures are
// logged, not surfaced (the deploy is already being marked failed). When there
// is no previous commit (first-ever deploy), there is nothing to roll back to;
// the broken app is left stopped.
func (c *Controller) rollback(ctx context.Context, sb *types.Sandbox, repo *types.GitRepository, previousSHA string, containerPort int) {
	if previousSHA == "" {
		log.Warn().Str("sandbox_id", sb.ID).Msg("deploy failed and no previous commit to roll back to; app left stopped")
		return
	}
	if err := c.deployInPlace(ctx, sb, repo, previousSHA, containerPort); err != nil {
		log.Error().Err(err).Str("sandbox_id", sb.ID).Str("rollback_sha", previousSHA).Msg("rollback to previous commit failed")
		return
	}
	log.Info().Str("sandbox_id", sb.ID).Str("rollback_sha", previousSHA).Msg("rolled back web service to previous commit after failed deploy")
}

// lastLiveSHA returns the commit SHA of the project's most recent live (or
// superseded) deploy, or "" if there is none. Used to roll back on failure.
func (c *Controller) lastLiveSHA(ctx context.Context, projectID string) string {
	deploys, err := c.store.ListWebServiceDeploys(ctx, projectID, 50)
	if err != nil {
		log.Warn().Err(err).Str("project_id", projectID).Msg("failed to list deploys for rollback baseline")
		return ""
	}
	for _, d := range deploys {
		if (d.Status == types.WebServiceDeployStatusLive || d.Status == types.WebServiceDeployStatusSuperseded) && d.CommitSHA != "" {
			return d.CommitSHA
		}
	}
	return ""
}

// deployScript builds the in-place deploy shell script. It stops any
// previously-launched app (tracked via a pidfile under /data) BEFORE starting
// the new one, guaranteeing a single writer of the durable /data dir. The app
// is launched in its own session (setsid) so we can stop its whole process
// group on the next deploy. It receives HELIX_WEB_SERVICE_PORT and
// HELIX_WEB_SERVICE_DATA_DIR=/data — apps put their database/uploads under the
// latter so state survives redeploys and reboots.
func deployScript(cloneURL, sha string, containerPort int) string {
	checkout := ""
	if sha != "" {
		checkout = "git checkout " + shellEscape(sha)
	}
	return strings.Join([]string{
		"set -e",
		"mkdir -p /data /workspace",
		"PIDFILE=/data/.helix-webservice.pid",
		"LOGFILE=/data/.helix-webservice.log",
		// Stop the previous app instance (single-writer guarantee). setsid made
		// it a group leader, so PID == PGID and `kill -- -PID` stops the group.
		`if [ -f "$PIDFILE" ]; then`,
		`  OLDPID=$(cat "$PIDFILE" 2>/dev/null || true)`,
		`  if [ -n "$OLDPID" ]; then`,
		`    kill -TERM -"$OLDPID" 2>/dev/null || kill -TERM "$OLDPID" 2>/dev/null || true`,
		`    for _ in $(seq 1 30); do kill -0 "$OLDPID" 2>/dev/null || break; sleep 1; done`,
		`    kill -KILL -"$OLDPID" 2>/dev/null || true`,
		`  fi`,
		`  rm -f "$PIDFILE"`,
		"fi",
		"cd /workspace",
		"if [ ! -d .git ]; then",
		"  git clone --depth 50 " + shellEscape(cloneURL) + " .",
		"fi",
		"git fetch --all",
		checkout,
		"chmod +x .helix/startup.sh 2>/dev/null || true",
		"if [ ! -f .helix/startup.sh ]; then",
		"  echo 'No .helix/startup.sh found in repository' >&2; exit 2",
		"fi",
		fmt.Sprintf(`setsid env HELIX_WEB_SERVICE_PORT=%d HELIX_WEB_SERVICE_DATA_DIR=/data bash .helix/startup.sh >"$LOGFILE" 2>&1 &`, containerPort),
		`echo $! > "$PIDFILE"`,
	}, "\n")
}

// waitForReady polls the container's configured port via the existing
// hydra proxy path until any HTTP response comes back, or until the
// deadline. "Listener present" is the readiness contract — we don't
// care whether the app returns 200 or 503, only that something is
// bound to the port and able to answer.
func (c *Controller) waitForReady(ctx context.Context, sandboxID string, port int) error {
	sb, err := c.sandboxes.Get(ctx, sandboxID)
	if err != nil {
		return fmt.Errorf("get sandbox: %w", err)
	}
	hydraClient, err := c.sandboxes.HydraClient(sb)
	if err != nil {
		return err
	}

	deadline := time.Now().Add(c.readinessWait)
	var lastErr error
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := hydraClient.ProbeDevContainerPort(probeCtx, sandboxID, port)
		cancel()
		if err == nil {
			return nil
		}
		lastErr = err

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(c.readinessPoll):
		}
	}
	if lastErr != nil {
		return fmt.Errorf("app did not bind to port %d within %s: %w", port, c.readinessWait, lastErr)
	}
	return fmt.Errorf("app did not bind to port %d within %s", port, c.readinessWait)
}

// resolvePrimaryRepo finds the project's primary git repository.
func (c *Controller) resolvePrimaryRepo(ctx context.Context, project *types.Project) (*types.GitRepository, error) {
	if project.DefaultRepoID == "" {
		return nil, errors.New("project has no primary repository configured")
	}
	return c.store.GetGitRepository(ctx, project.DefaultRepoID)
}

func (c *Controller) markBuilding(ctx context.Context, deployID string) error {
	return c.store.UpdateWebServiceDeploy(ctx, deployID, map[string]interface{}{
		"status": types.WebServiceDeployStatusBuilding,
	})
}

func (c *Controller) markFailed(ctx context.Context, deployID, sandboxID, errMsg string) {
	now := time.Now()
	updates := map[string]interface{}{
		"status":      types.WebServiceDeployStatusFailed,
		"error":       errMsg,
		"finished_at": &now,
	}
	if sandboxID != "" {
		updates["sandbox_id"] = sandboxID
	}
	if err := c.store.UpdateWebServiceDeploy(ctx, deployID, updates); err != nil {
		log.Warn().Err(err).Str("deploy_id", deployID).Msg("failed to mark deploy failed")
	}
	log.Warn().Str("deploy_id", deployID).Str("error", errMsg).Msg("web service deploy failed")
}

func (c *Controller) markLive(ctx context.Context, deployID, sandboxID string) {
	now := time.Now()
	if err := c.store.UpdateWebServiceDeploy(ctx, deployID, map[string]interface{}{
		"status":      types.WebServiceDeployStatusLive,
		"sandbox_id":  sandboxID,
		"finished_at": &now,
	}); err != nil {
		log.Warn().Err(err).Str("deploy_id", deployID).Msg("failed to mark deploy live")
	}
}

// markSupersededPrevious flips any prior live deploys for this project
// to status=superseded so the deploys list shows exactly one live row.
func (c *Controller) markSupersededPrevious(ctx context.Context, currentDeployID, projectID string) {
	deploys, err := c.store.ListWebServiceDeploys(ctx, projectID, 50)
	if err != nil {
		log.Warn().Err(err).Msg("failed to list deploys for supersede pass")
		return
	}
	for _, d := range deploys {
		if d.ID == currentDeployID || d.Status != types.WebServiceDeployStatusLive {
			continue
		}
		if err := c.store.UpdateWebServiceDeploy(ctx, d.ID, map[string]interface{}{
			"status": types.WebServiceDeployStatusSuperseded,
		}); err != nil {
			log.Warn().Err(err).Str("deploy_id", d.ID).Msg("failed to mark prior deploy superseded")
		}
	}
}

// shellEscape returns a single-quoted string safe for /bin/sh.
func shellEscape(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
