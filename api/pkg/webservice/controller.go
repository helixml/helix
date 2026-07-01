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
	"runtime/debug"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/hydra"
	"github.com/helixml/helix/api/pkg/sandbox"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// recoverGoroutine converts a panic in a detached goroutine into a logged error
// instead of a process-wide crash. A panic in ANY goroutine takes down the
// whole API binary — which would drop EVERY hosted web service and the control
// plane, not just the one project. Every detached deploy/recovery goroutine
// must guard itself with this. The optional onPanic runs cleanup (e.g. mark the
// deploy failed) so state doesn't wedge.
func recoverGoroutine(what string, onPanic func(recovered any)) {
	if r := recover(); r != nil {
		log.Error().
			Interface("panic", r).
			Str("goroutine", what).
			Bytes("stack", debug.Stack()).
			Msg("recovered panic in detached goroutine (would otherwise have crashed the API process)")
		if onPanic != nil {
			onPanic(r)
		}
	}
}

// ProjectSecretsGetter returns prod-scoped project secrets as `KEY=value`
// env-var strings to inject into the web service container.
type ProjectSecretsGetter func(ctx context.Context, projectID string) ([]string, error)

// Controller orchestrates project web service deploys.
type Controller struct {
	store         store.Store
	sandboxes     *sandbox.Controller
	provisionWait time.Duration // upper bound for waiting for sandbox status=running
	provisionPoll time.Duration // poll interval while waiting
	bootstrapWait time.Duration // upper bound for the in-place deploy exec (clone/fetch/launch)
	readinessWait time.Duration // upper bound for waiting for the app to bind to its port
	readinessPoll time.Duration // poll interval while waiting

	// getProjectSecrets, when set, supplies prod-scoped project secrets to
	// inject into the web service container env. Optional — nil means no
	// secrets are injected.
	getProjectSecrets ProjectSecretsGetter
}

// New constructs a Controller. The defaults are sized for Docker-capable
// web-service sandboxes that boot docker-compose stacks: a few minutes of
// cold-start plus a generous window for the app to bind its port.
// (Web-service deploys source .helix/startup.sh from the helix-specs branch.)
func New(s store.Store, sc *sandbox.Controller) *Controller {
	return &Controller{
		store:         s,
		sandboxes:     sc,
		provisionWait: 3 * time.Minute,
		provisionPoll: 2 * time.Second,
		bootstrapWait: 5 * time.Minute,
		// Web services boot whole docker-compose stacks, so a cold first
		// deploy can take several minutes (image builds + pulls + dependency
		// installs + per-service healthcheck start_periods). Give the app
		// plenty of time to bind its port before we call the deploy failed
		// and roll back.
		readinessWait: 10 * time.Minute,
		readinessPoll: 3 * time.Second,
	}
}

// SetProjectSecretsGetter wires the prod-scoped secret injection callback.
func (c *Controller) SetProjectSecretsGetter(getter ProjectSecretsGetter) {
	c.getProjectSecrets = getter
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
	// A panic here must not crash the whole API (and every other hosted
	// service). Recover and mark this deploy failed so it doesn't wedge in
	// "building" — the health monitor will then retry cleanly.
	defer recoverGoroutine("runDeploy project="+req.ProjectID, func(any) {
		c.markFailed(context.Background(), deployID, "", "internal error during deploy (panic recovered)")
	})

	ctx, cancel := context.WithTimeout(context.Background(), c.provisionWait+c.bootstrapWait+c.readinessWait+2*time.Minute)
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

	// Mint a short-lived API key so the in-container `git clone` can
	// authenticate to Helix's git server (the repo is private; an
	// unauthenticated clone fails with "could not read Username"). Revoked
	// once the deploy finishes.
	gitToken, revokeToken, err := c.mintDeployGitToken(ctx, project, req.Owner)
	if err != nil {
		c.markFailed(ctx, deployID, sb.ID, fmt.Sprintf("mint git token: %s", err))
		return
	}
	defer revokeToken()

	// Deploy the requested code in place (stops the old app first).
	if err := c.deployInPlace(ctx, sb, repo, req.ProjectID, req.CommitSHA, state.ContainerPort, gitToken); err != nil {
		c.markFailed(ctx, deployID, sb.ID, fmt.Sprintf("deploy: %s", err))
		return
	}

	// Poll the container port until the app responds. We treat any HTTP
	// response (even 4xx/5xx) as "the app is bound" — what matters here
	// is that the listener exists, not that the app likes the request.
	if err := c.waitForReady(ctx, sb.ID, state.ContainerPort); err != nil {
		// Roll back to the last-known-good commit so the site comes back up
		// against the same intact /data. The data is never touched either way.
		c.rollback(ctx, sb, repo, req.ProjectID, previousSHA, state.ContainerPort, gitToken)
		c.markFailed(ctx, deployID, sb.ID, fmt.Sprintf("readiness: %s", err))
		return
	}

	// Make the hosted containers self-heal on crash. The compose file is
	// user-controlled, so we enforce the restart policy from our side once the
	// app is up. Best-effort — never fails the deploy.
	c.applyRestartPolicy(ctx, sb)

	// Same sandbox across deploys, but keep active_sandbox_id authoritative.
	if err := c.store.SetActiveWebServiceSandbox(ctx, req.ProjectID, sb.ID); err != nil {
		c.markFailed(ctx, deployID, sb.ID, fmt.Sprintf("set active sandbox: %s", err))
		return
	}

	// Mark this deploy live; supersede the previous live row if any.
	c.markLive(ctx, deployID, sb.ID)
	c.markSupersededPrevious(ctx, deployID, req.ProjectID)
}

// applyRestartPolicy sets restart=unless-stopped on every container in the
// web-service sandbox so a crashed app container is brought back automatically
// by the inner dockerd. The compose file is user-controlled, so we enforce
// this from our side. Best-effort: logs and swallows errors.
func (c *Controller) applyRestartPolicy(ctx context.Context, sb *types.Sandbox) {
	hydraClient, err := c.sandboxes.HydraClient(sb)
	if err != nil {
		log.Warn().Err(err).Str("sandbox_id", sb.ID).Msg("web service: restart-policy: no hydra client")
		return
	}
	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	_, err = hydraClient.RunSandboxCommand(cctx, sb.ID, &hydra.ExecRequest{
		SandboxID:      sb.ID,
		Cmd:            "/bin/sh",
		Args:           []string{"-c", `ids=$(docker ps -q); [ -n "$ids" ] && docker update --restart unless-stopped $ids >/dev/null || true`},
		Cwd:            "/",
		TimeoutSeconds: 25,
	})
	if err != nil {
		log.Warn().Err(err).Str("sandbox_id", sb.ID).Msg("web service: failed to apply restart policy")
	}
}

// sandboxDockerAlive returns true if the sandbox's inner dockerd answers a
// quick `docker info`. Used by recovery to decide restart-in-place vs recreate.
func (c *Controller) sandboxDockerAlive(ctx context.Context, sb *types.Sandbox) bool {
	hydraClient, err := c.sandboxes.HydraClient(sb)
	if err != nil {
		return false
	}
	cctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	resp, err := hydraClient.RunSandboxCommand(cctx, sb.ID, &hydra.ExecRequest{
		SandboxID:      sb.ID,
		Cmd:            "/bin/sh",
		Args:           []string{"-c", "docker info >/dev/null 2>&1"},
		Cwd:            "/",
		TimeoutSeconds: 20,
	})
	if err != nil {
		return false
	}
	if resp != nil && resp.ExitCode != nil && *resp.ExitCode != 0 {
		return false
	}
	return true
}

// DeployBuildTimeout bounds how long a web-service deploy may sit
// pending/building before it is treated as an interrupted (orphaned) build.
// Shared by the health monitor (recovery) and the API (health reporting) so
// they agree on what "still deploying" means.
const DeployBuildTimeout = 15 * time.Minute

// Probe reports whether the project's web service answers on its container port
// through the hydra proxy right now. It is the single source of truth for "is
// this web service actually live", used by both the health monitor and the API
// so the UI reflects real health rather than the last deploy row.
func (c *Controller) Probe(ctx context.Context, st *types.ProjectWebServiceState, timeout time.Duration) bool {
	if st == nil || !st.Enabled || st.ActiveSandboxID == "" {
		return false
	}
	sb, err := c.sandboxes.Get(ctx, st.ActiveSandboxID)
	if err != nil || sb == nil || sb.Status != types.SandboxStatusRunning {
		return false
	}
	hc, err := c.sandboxes.HydraClient(sb)
	if err != nil {
		return false
	}
	pctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return hc.ProbeDevContainerPort(pctx, sb.ID, st.ContainerPort) == nil
}

// Health returns the real, probe-based status of a project's web service:
// "disabled", "deploying", "live" or "unhealthy". The API surfaces this so the
// UI reflects actual health instead of trusting the last deploy row (a deploy
// stays "live" long after its container dies).
func (c *Controller) Health(ctx context.Context, projectID string) string {
	st, err := c.store.GetProjectWebServiceState(ctx, projectID)
	if err != nil || st == nil || !st.Enabled || st.ActiveSandboxID == "" {
		return "disabled"
	}
	if deploys, _ := c.store.ListWebServiceDeploys(ctx, projectID, 1); len(deploys) > 0 {
		d := deploys[0]
		inflight := d.Status == types.WebServiceDeployStatusPending || d.Status == types.WebServiceDeployStatusBuilding
		if inflight && time.Since(d.StartedAt) < DeployBuildTimeout {
			return "deploying"
		}
	}
	if c.Probe(ctx, st, 5*time.Second) {
		return "live"
	}
	return "unhealthy"
}

// RecoverWebService brings a project's web service back to a serving state
// after the health-monitor detects it down. It escalates:
//  1. active sandbox row gone → Redeploy (provisions a fresh sandbox).
//  2. sandbox not running, or its inner dockerd is unresponsive (the hung-
//     dockerd failure that caused our outage) → delete the sandbox, then
//     Redeploy. The broken container is removed via the OUTER docker, so a hung
//     inner dockerd doesn't block it; the project-keyed /data (incl. Postgres)
//     reattaches to the fresh sandbox.
//  3. sandbox running + dockerd alive → Redeploy in place (re-runs
//     .helix/startup.sh → docker compose up against the same sandbox).
//
// Redeploy is async; this returns once recovery has been kicked off.
func (c *Controller) RecoverWebService(ctx context.Context, projectID string) error {
	state, err := c.store.GetProjectWebServiceState(ctx, projectID)
	if err != nil {
		return fmt.Errorf("get web service state: %w", err)
	}
	if !state.Enabled || state.ActiveSandboxID == "" {
		return nil // nothing to recover
	}
	project, err := c.store.GetProject(ctx, projectID)
	if err != nil {
		return fmt.Errorf("get project: %w", err)
	}

	recreate := false
	reason := "restart in place"
	sb, err := c.sandboxes.Get(ctx, state.ActiveSandboxID)
	switch {
	case err != nil || sb == nil:
		reason = "active sandbox row is gone"
		// Redeploy provisions a fresh sandbox; /data reattaches by project id.
	case sb.Status != types.SandboxStatusRunning:
		recreate, reason = true, fmt.Sprintf("sandbox status=%s", sb.Status)
	case !c.sandboxDockerAlive(ctx, sb):
		recreate, reason = true, "sandbox dockerd unresponsive"
	}

	if recreate {
		log.Warn().Str("project_id", projectID).Str("sandbox_id", state.ActiveSandboxID).
			Str("reason", reason).Msg("health-monitor: recreating web-service sandbox")
		if delErr := c.sandboxes.Delete(context.Background(), state.ActiveSandboxID); delErr != nil {
			log.Warn().Err(delErr).Str("sandbox_id", state.ActiveSandboxID).
				Msg("health-monitor: failed to delete broken sandbox (continuing to redeploy)")
		}
	} else {
		log.Info().Str("project_id", projectID).Str("reason", reason).
			Msg("health-monitor: recovering web service")
	}

	if _, err := c.Redeploy(ctx, DeployRequest{ProjectID: projectID, Owner: project.UserID}); err != nil {
		return fmt.Errorf("redeploy: %w", err)
	}
	return nil
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
		Name: fmt.Sprintf("web-service-%s", project.ID),
		// The web service hosts whole docker-compose stacks, so it needs a
		// Docker daemon. The desktop runtime is the Docker-capable image
		// (privileged, ships dockerd + docker compose, gets the
		// /var/lib/docker volume); the agent/GUI stack is disabled for
		// sandbox-API containers so it runs effectively headless.
		Runtime:        types.SandboxRuntimeUbuntuDesktop,
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
//
// Prod-scoped project secrets are injected via the exec environment (see
// projectSecretEnv) so they reach startup.sh without being written into the
// command line or logs.
func (c *Controller) deployInPlace(ctx context.Context, sb *types.Sandbox, repo *types.GitRepository, projectID, sha string, containerPort int, gitToken string) error {
	hydraClient, err := c.sandboxes.HydraClient(sb)
	if err != nil {
		return err
	}

	script := deployScript(repo.CloneURL, sha, repoDirName(repo), containerPort)

	// Inject prod-scoped project secrets via the exec environment (NOT inlined
	// into the shell script) so the values don't leak into command logs. The
	// `setsid env HELIX_WEB_SERVICE_PORT=... bash .helix/startup.sh` in the
	// deploy script inherits this exec process environment, so secrets propagate
	// through to startup.sh.
	env := c.projectSecretEnv(ctx, projectID, sb.ID)
	// The clone token also rides in via the env (not the script string) so it
	// stays out of command logs; deployScript reads it to authenticate clone/fetch.
	if gitToken != "" {
		env = append(env, "HELIX_GIT_TOKEN="+gitToken)
	}

	resp, execErr := hydraClient.RunSandboxCommand(ctx, sb.ID, &hydra.ExecRequest{
		SandboxID:      sb.ID,
		Cmd:            "/bin/bash",
		Args:           []string{"-c", script},
		Cwd:            "/",
		Env:            env,
		TimeoutSeconds: int(c.bootstrapWait.Seconds()),
	})
	if execErr != nil {
		return execErr
	}
	// The bootstrap backgrounds the app and exits 0 on success. A non-zero exit
	// means clone/fetch/launch failed — fail the deploy now rather than waiting
	// out the full readiness window against an app that will never bind.
	if resp != nil && resp.ExitCode != nil && *resp.ExitCode != 0 {
		return fmt.Errorf("bootstrap exited %d: %s", *resp.ExitCode, strings.TrimSpace(resp.Stderr+resp.Stdout))
	}
	return nil
}

// mintDeployGitToken creates a short-lived API key owned by the deploying user
// so the in-container git clone can authenticate to Helix's private git server.
// The returned revoke func deletes the key; call it once the deploy finishes.
func (c *Controller) mintDeployGitToken(ctx context.Context, project *types.Project, owner string) (string, func(), error) {
	key, err := system.GenerateAPIKey()
	if err != nil {
		return "", func() {}, err
	}
	if _, err := c.store.CreateAPIKey(ctx, &types.ApiKey{
		Owner:          owner,
		OwnerType:      types.OwnerTypeUser,
		Key:            key,
		Name:           "web-service-deploy",
		Type:           types.APIKeyType("api"),
		OrganizationID: project.OrganizationID,
		ProjectID:      project.ID,
	}); err != nil {
		return "", func() {}, err
	}
	revoke := func() {
		if err := c.store.DeleteAPIKey(context.Background(), key); err != nil {
			log.Warn().Err(err).Str("project_id", project.ID).Msg("failed to revoke web-service deploy git token")
		}
	}
	return key, revoke, nil
}

// projectSecretEnv returns the prod-scoped project secrets as `KEY=value`
// env-var strings to inject into the web service container. Returns an empty
// slice (never nil-panics) when no getter is wired, no project is set, or the
// getter fails — a secret-load failure must not block a deploy.
func (c *Controller) projectSecretEnv(ctx context.Context, projectID, sandboxID string) []string {
	env := []string{}
	if c.getProjectSecrets == nil || projectID == "" {
		return env
	}
	secretEnv, err := c.getProjectSecrets(ctx, projectID)
	if err != nil {
		log.Warn().Err(err).Str("project_id", projectID).Str("sandbox_id", sandboxID).Msg("failed to load prod project secrets, continuing without them")
		return env
	}
	if len(secretEnv) > 0 {
		env = append(env, secretEnv...)
		log.Info().Int("secret_count", len(secretEnv)).Str("project_id", projectID).Str("sandbox_id", sandboxID).Msg("injected prod project secrets into web service env")
	}
	return env
}

// rollback re-deploys a previously-live commit in place after a failed
// deploy, so the site returns to last-known-good. Best-effort: failures are
// logged, not surfaced (the deploy is already being marked failed). When there
// is no previous commit (first-ever deploy), there is nothing to roll back to;
// the broken app is left stopped.
func (c *Controller) rollback(ctx context.Context, sb *types.Sandbox, repo *types.GitRepository, projectID, previousSHA string, containerPort int, gitToken string) {
	if previousSHA == "" {
		log.Warn().Str("sandbox_id", sb.ID).Msg("deploy failed and no previous commit to roll back to; app left stopped")
		return
	}
	if err := c.deployInPlace(ctx, sb, repo, projectID, previousSHA, containerPort, gitToken); err != nil {
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

// repoDirName returns a filesystem-safe single path component for the app
// code checkout, mirroring the spec-task convention of cloning the primary
// repo into a dir named after it (helix-run-startup-script.sh uses
// $HOME/work/$HELIX_PRIMARY_REPO_NAME). Falls back to "app" when the name is
// empty or unusable.
func repoDirName(repo *types.GitRepository) string {
	safe := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '_', r == '-':
			return r
		default:
			return '-'
		}
	}, strings.TrimSpace(repo.Name))
	safe = strings.Trim(safe, "-.")
	if safe == "" {
		return "app"
	}
	return safe
}

// deployScript builds the in-place deploy shell script. It reproduces the
// spec-task agent workspace layout EXACTLY so a project's .helix/startup.sh
// behaves identically in both contexts — no script can work in one and fail in
// the other:
//   - the app code is cloned into its own dir ($CODE = /workspace/<repo>),
//   - the helix-specs branch is checked out as a SIBLING git worktree
//     ($SPECS = /workspace/helix-specs) — never a subdir of the code, so
//     `dirname "$0"/..` is the helix-specs worktree in both contexts (matching
//     helix-run-startup-script.sh), not the code dir,
//   - startup.sh is invoked with CWD = the code dir and
//     $0 = $SPECS/.helix/startup.sh.
//
// It stops any previously-launched app (tracked via a pidfile under /data)
// BEFORE starting the new one, guaranteeing a single writer of the durable
// /data dir. The app is launched in its own session (setsid) so we can stop its
// whole process group on the next deploy. It receives HELIX_WEB_SERVICE_PORT
// and HELIX_WEB_SERVICE_DATA_DIR=/data — apps put their database/uploads under
// the latter so state survives redeploys and reboots.
func deployScript(cloneURL, sha, codeDir string, containerPort int) string {
	code := "/workspace/" + codeDir
	checkout := ""
	if sha != "" {
		checkout = "git -C " + shellEscape(code) + " checkout " + shellEscape(sha)
	}
	return strings.Join([]string{
		"set -e",
		"mkdir -p /data /workspace",
		"PIDFILE=/data/.helix-webservice.pid",
		"LOGFILE=/data/.helix-webservice.log",
		"CODE=" + shellEscape(code),
		"SPECS=/workspace/helix-specs",
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
		// Embed the deploy token (passed via env, not the script string) into
		// the clone URL so git can authenticate to Helix's private git server.
		"CLONE_URL=" + shellEscape(cloneURL),
		`if [ -n "${HELIX_GIT_TOKEN:-}" ]; then CLONE_URL=$(printf '%s' "$CLONE_URL" | sed -E "s#^(https?://)#\1api:${HELIX_GIT_TOKEN}@#"); fi`,
		// Clone the app code into its own dir on first run; refresh origin (the
		// previous deploy's token was revoked) otherwise.
		`if [ ! -d "$CODE/.git" ]; then`,
		`  git clone --depth 50 "$CLONE_URL" "$CODE"`,
		`else`,
		`  git -C "$CODE" remote set-url origin "$CLONE_URL"`,
		`fi`,
		`git -C "$CODE" fetch --all`,
		checkout,
		// The startup script is sourced exclusively from the project's
		// helix-specs branch (its canonical Helix-metadata branch) — never from
		// the deployed app branch — so there is a single definition of how the
		// project boots. Check it out as a SIBLING worktree of the code,
		// detached at the fetched tip so redeploys never hit a branch-checkout
		// conflict; remove any stale worktree first. Shallow clone implies
		// --single-branch, so helix-specs must be fetched explicitly.
		`git -C "$CODE" worktree remove --force "$SPECS" 2>/dev/null || true`,
		`rm -rf "$SPECS"`,
		`git -C "$CODE" fetch --depth 1 origin helix-specs`,
		`git -C "$CODE" worktree add --force --detach "$SPECS" FETCH_HEAD`,
		`if [ ! -f "$SPECS/.helix/startup.sh" ]; then`,
		`  echo 'No .helix/startup.sh found on the helix-specs branch' >&2; exit 2`,
		`fi`,
		`chmod +x "$SPECS/.helix/startup.sh" 2>/dev/null || true`,
		// Invoke exactly like the spec-task runner (helix-run-startup-script.sh):
		// CWD = code root, $0 = the sibling helix-specs worktree. setsid → own
		// process group for a clean stop on the next deploy.
		`cd "$CODE"`,
		fmt.Sprintf(`setsid env HELIX_WEB_SERVICE_PORT=%d HELIX_WEB_SERVICE_DATA_DIR=/data bash "$SPECS/.helix/startup.sh" >"$LOGFILE" 2>&1 &`, containerPort),
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
