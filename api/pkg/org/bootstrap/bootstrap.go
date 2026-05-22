// Package bootstrap creates the initial owner Worker and grants the
// structural tools. Runs exactly once — subsequent calls fail if any Worker
// already exists.
package bootstrap

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"

	"github.com/helixml/helix/api/pkg/org/grant"
	"github.com/helixml/helix/api/pkg/org/position"
	"github.com/helixml/helix/api/pkg/org/role"
	"github.com/helixml/helix/api/pkg/org/store"
	"github.com/helixml/helix/api/pkg/org/tool"
	"github.com/helixml/helix/api/pkg/org/worker"
	"github.com/helixml/helix/api/pkg/org/domain"
	"github.com/helixml/helix/api/pkg/org/tools"
)

// ownerRoleContent is the seed markdown for r-owner. Lives in a
// template file rather than a string literal so the prose can be
// edited like any other Role markdown — including the hiring
// playbook that teaches a fresh owner how to chain create_role →
// create_position → hire_worker → subscribe their streams.
//
//go:embed templates/owner_role.md
var ownerRoleContent string

// Params controls the bootstrap.
type Params struct {
	// EnvironmentPath is an absolute path to the owner's Environment. The
	// directory must already exist on disk — bootstrap does not create it.
	EnvironmentPath string

	// OrganizationID stamps the owner Worker (and, once H5.3 lands,
	// the rest of the seeded graph) with the helix.Organization the
	// bootstrap belongs to. Empty means single-tenant alpha — the
	// owner Worker has no OrgID. Multi-tenant deployments call Run
	// once per helix.Organization with the org's ID; the existing
	// "already initialised" check is filtered to that org so other
	// orgs can bootstrap independently.
	OrganizationID string
}

// Result summarises the newly-created owner.
type Result struct {
	WorkerID        worker.ID
	RoleID          role.ID
	PositionID      position.ID
	EnvironmentPath string
}

// ErrAlreadyInitialised is returned when at least one worker already exists.
var ErrAlreadyInitialised = errors.New("org is already initialised")

// Run performs the bootstrap: create the owner's Role, Position, Worker,
// Environment row, and grant every structural tool. Bootstrap is the root
// of trust — these are the only grants in the system not issued by a
// prior Worker — and the grants it issues stop at the structural set.
func Run(ctx context.Context, s *store.Store, params Params) (Result, error) {
	if params.EnvironmentPath == "" {
		return Result{}, fmt.Errorf("environmentPath is required")
	}
	if info, err := os.Stat(params.EnvironmentPath); err != nil {
		return Result{}, fmt.Errorf("environmentPath %q: %w", params.EnvironmentPath, err)
	} else if !info.IsDir() {
		return Result{}, fmt.Errorf("environmentPath %q is not a directory", params.EnvironmentPath)
	}

	existing, err := s.Workers.List(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("check existing workers: %w", err)
	}
	// "Already initialised" is per-org: a Worker carrying the
	// requested OrgID means bootstrap already ran for this tenant.
	// In single-tenant alpha (OrgID empty) the filter reduces to the
	// original "any worker exists" semantic.
	for _, w := range existing {
		if w.OrganizationID() == params.OrganizationID {
			return Result{}, ErrAlreadyInitialised
		}
	}

	now := time.Now().UTC()
	role, err := role.New("r-owner", ownerRoleContent, nil, nil, now)
	if err != nil {
		return Result{}, err
	}
	if err := s.Roles.Create(ctx, role); err != nil {
		return Result{}, fmt.Errorf("create owner role: %w", err)
	}

	rootPos, err := domain.NewPosition("p-root", role.ID, nil)
	if err != nil {
		return Result{}, err
	}
	if err := s.Positions.Create(ctx, rootPos); err != nil {
		return Result{}, fmt.Errorf("create root position: %w", err)
	}

	ownerIdentity := "# Owner\n\nThe person running this org. Edit this from /ui/org to " +
		"introduce yourself — your name, voice, and how you want subordinates to address you.\n"
	ownerWorker, err := domain.NewHumanWorker(worker.ID("w-owner"), rootPos.ID, ownerIdentity)
	if err != nil {
		return Result{}, err
	}
	var owner domain.Worker = ownerWorker
	if params.OrganizationID != "" {
		owner = owner.WithOrgID(params.OrganizationID)
	}
	if err := s.Workers.Create(ctx, owner); err != nil {
		return Result{}, fmt.Errorf("create owner worker: %w", err)
	}

	env, err := domain.NewEnvironment(owner.ID(), params.EnvironmentPath, now)
	if err != nil {
		return Result{}, err
	}
	if err := s.Environments.Create(ctx, env); err != nil {
		return Result{}, fmt.Errorf("create owner environment: %w", err)
	}

	// Every built-in tool — the owner is the root of trust and can do
	// anything. They issue subordinate Workers a narrower set via the
	// hire_worker / grant_tool tools.
	defaults := []tool.Name{
		// Mutations.
		tools.CreateRoleName,
		tools.UpdateRoleName,
		tools.UpdateIdentityName,
		tools.CreatePositionName,
		tools.HireWorkerName,
		tools.GrantToolName,
		tools.RevokeToolName,
		tools.CreateStreamName,
		tools.StreamMembersName,
		tools.SubscribeName,
		tools.UnsubscribeName,
		tools.InviteWorkersName,
		tools.PublishName,
		tools.DMName,
		// Reads.
		tools.ListRolesName,
		tools.GetRoleName,
		tools.ListPositionsName,
		tools.GetPositionName,
		tools.ListPositionChildrenName,
		tools.ListWorkersName,
		tools.GetWorkerName,
		tools.ListWorkerGrantsName,
		tools.GetWorkerEnvironmentName,
		tools.ListStreamsName,
		tools.GetStreamName,
		tools.ListStreamEventsName,
		tools.GetGrantName,
		tools.ReadEventsName,
		tools.WorkerLogName,
	}
	// Every Worker — including the owner — has an activations stream
	// (s-activations-<workerID>) where its turns are recorded. AI
	// Workers get theirs via hire_worker; the owner is bootstrapped
	// (not hired), so we mint here through the shared helper that
	// hire_worker also uses. observer = owner.ID() because /ui/streams
	// shows the owner subscribed to their own transcript.
	if err := tools.EnsureActivationStream(ctx, s, owner.ID(), owner.ID(), now); err != nil {
		return Result{}, fmt.Errorf("owner activation stream: %w", err)
	}

	for _, name := range defaults {
		g, err := domain.NewToolGrant(
			grant.ID("g-owner-"+uuid.NewString()),
			owner.ID(),
			name,
		)
		if err != nil {
			return Result{}, err
		}
		if err := s.Grants.Create(ctx, g); err != nil {
			return Result{}, fmt.Errorf("grant %q: %w", name, err)
		}
	}

	return Result{
		WorkerID:        owner.ID(),
		RoleID:          role.ID,
		PositionID:      rootPos.ID,
		EnvironmentPath: params.EnvironmentPath,
	}, nil
}
