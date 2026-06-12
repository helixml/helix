// Package webservice orchestrates the deploy lifecycle for a project's
// hosted web application: provisioning a sandbox, cloning the primary
// repo, running .helix/startup.sh, and cutting traffic over to the new
// container by updating project_web_service_state.ActiveSandboxID.
//
// The Controller wires together pre-existing primitives: sandbox.Controller
// for provisioning, hydra.RevDialClient for in-container exec, and
// store.Store for persistence. There is no new runner-side workload type
// — the bootstrap script runs as a detached exec inside a plain headless
// sandbox.
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

// runDeploy executes the long-running provision → bootstrap → cutover
// flow. Detached so the API request returns quickly. Every state
// transition is persisted so the UI can reflect progress.
func (c *Controller) runDeploy(
	deployID string,
	req DeployRequest,
	project *types.Project,
	repo *types.GitRepository,
	state *types.ProjectWebServiceState,
) {
	ctx, cancel := context.WithTimeout(context.Background(), c.provisionWait+2*time.Minute)
	defer cancel()

	if err := c.markBuilding(ctx, deployID); err != nil {
		log.Warn().Err(err).Str("deploy_id", deployID).Msg("failed to mark deploy building")
	}

	sb, err := c.provisionSandbox(ctx, req, project)
	if err != nil {
		c.markFailed(ctx, deployID, "", fmt.Sprintf("provision sandbox: %s", err))
		return
	}

	if err := c.runBootstrap(ctx, sb, repo, req.CommitSHA, state.ContainerPort); err != nil {
		c.markFailed(ctx, deployID, sb.ID, fmt.Sprintf("bootstrap: %s", err))
		// Leave the sandbox running so the operator can exec in and debug.
		return
	}

	// Poll the container port until the app responds. We treat any HTTP
	// response (even 4xx/5xx) as "the app is bound" — what matters here
	// is that the listener exists, not that the app likes the request.
	if err := c.waitForReady(ctx, sb.ID, state.ContainerPort); err != nil {
		c.markFailed(ctx, deployID, sb.ID, fmt.Sprintf("readiness: %s", err))
		// Sandbox stays up; operator can inspect logs / exec.
		return
	}

	// Cut over routing.
	previousSandboxID := state.ActiveSandboxID
	if err := c.store.SetActiveWebServiceSandbox(ctx, req.ProjectID, sb.ID); err != nil {
		c.markFailed(ctx, deployID, sb.ID, fmt.Sprintf("cutover: %s", err))
		return
	}

	// Mark this deploy live; supersede the previous live row if any.
	c.markLive(ctx, deployID, sb.ID)
	c.markSupersededPrevious(ctx, deployID, req.ProjectID)

	// Stop the previous sandbox to free resources. Best-effort.
	if previousSandboxID != "" && previousSandboxID != sb.ID {
		if err := c.sandboxes.Delete(ctx, previousSandboxID); err != nil {
			log.Warn().Err(err).
				Str("previous_sandbox_id", previousSandboxID).
				Str("new_sandbox_id", sb.ID).
				Msg("failed to stop previous web service sandbox after cutover")
		}
	}
}

// provisionSandbox creates a new web-service sandbox for the project
// and polls until it reports status=running.
func (c *Controller) provisionSandbox(ctx context.Context, req DeployRequest, project *types.Project) (*types.Sandbox, error) {
	createReq := &types.CreateSandboxRequest{
		Name:           fmt.Sprintf("web-service-%s", project.ID),
		Runtime:        types.SandboxRuntimeHeadlessUbuntu,
		ProjectID:      project.ID,
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

// runBootstrap execs a detached shell inside the freshly-provisioned
// sandbox that clones the repo, optionally checks out the requested
// SHA, and runs `.helix/startup.sh`. The exec returns immediately —
// `startup.sh` typically becomes the long-running web server process.
func (c *Controller) runBootstrap(ctx context.Context, sb *types.Sandbox, repo *types.GitRepository, sha string, containerPort int) error {
	hydraClient, err := c.sandboxes.HydraClient(sb)
	if err != nil {
		return err
	}

	checkout := ""
	if sha != "" {
		checkout = fmt.Sprintf(" && git checkout %s", shellEscape(sha))
	}

	script := strings.Join([]string{
		"set -e",
		"mkdir -p /workspace",
		"cd /workspace",
		"if [ ! -d .git ]; then",
		"  git clone --depth 50 " + shellEscape(repo.CloneURL) + " .",
		"fi",
		"git fetch --all" + checkout,
		"chmod +x .helix/startup.sh 2>/dev/null || true",
		"if [ -f .helix/startup.sh ]; then",
		fmt.Sprintf("  exec env HELIX_WEB_SERVICE_PORT=%d bash .helix/startup.sh", containerPort),
		"else",
		"  echo 'No .helix/startup.sh found in repository' >&2; exit 2",
		"fi",
	}, "\n")

	_, execErr := hydraClient.RunSandboxCommand(ctx, sb.ID, &hydra.ExecRequest{
		SandboxID: sb.ID,
		Cmd:       "/bin/bash",
		Args:      []string{"-c", script},
		Cwd:       "/",
		Detached:  true,
	})
	return execErr
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
