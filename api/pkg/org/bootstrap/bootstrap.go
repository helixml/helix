// Package bootstrap creates the per-org initial owner Worker and grants
// the structural tools. Run once per helix.Organization — subsequent
// calls for the same org return ErrAlreadyInitialised, leaving other
// orgs free to bootstrap independently.
package bootstrap

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"

	"github.com/helixml/helix/api/pkg/org/domain"
	"github.com/helixml/helix/api/pkg/org/grant"
	"github.com/helixml/helix/api/pkg/org/position"
	"github.com/helixml/helix/api/pkg/org/role"
	"github.com/helixml/helix/api/pkg/org/store"
	"github.com/helixml/helix/api/pkg/org/tool"
	"github.com/helixml/helix/api/pkg/org/tools"
	"github.com/helixml/helix/api/pkg/org/worker"
)

//go:embed templates/owner_role.md
var ownerRoleContent string

// Params controls the bootstrap. OrganizationID is required (it gets
// stamped on every row the bootstrap creates and is the FK target for
// cascade-on-org-delete).
type Params struct {
	EnvironmentPath string
	OrganizationID  string
}

// Result summarises the newly-created owner.
type Result struct {
	WorkerID        worker.ID
	RoleID          role.ID
	PositionID      position.ID
	EnvironmentPath string
}

// ErrAlreadyInitialised is returned when the org already has an owner
// Worker.
var ErrAlreadyInitialised = errors.New("org is already initialised")

// Run performs the bootstrap for one helix.Organization: create the
// owner's Role, Position, Worker, Environment row, and grant every
// structural tool. Each row is stamped with params.OrganizationID, so
// the org-delete FK cascade reaps them automatically.
func Run(ctx context.Context, s *store.Store, params Params) (Result, error) {
	if params.EnvironmentPath == "" {
		return Result{}, fmt.Errorf("environmentPath is required")
	}
	if params.OrganizationID == "" {
		return Result{}, fmt.Errorf("organizationID is required")
	}
	if info, err := os.Stat(params.EnvironmentPath); err != nil {
		return Result{}, fmt.Errorf("environmentPath %q: %w", params.EnvironmentPath, err)
	} else if !info.IsDir() {
		return Result{}, fmt.Errorf("environmentPath %q is not a directory", params.EnvironmentPath)
	}

	existing, err := s.Workers.List(ctx, params.OrganizationID)
	if err != nil {
		return Result{}, fmt.Errorf("check existing workers: %w", err)
	}
	if len(existing) > 0 {
		return Result{}, ErrAlreadyInitialised
	}

	now := time.Now().UTC()
	ownerRole, err := role.New("r-owner", ownerRoleContent, nil, nil, now, params.OrganizationID)
	if err != nil {
		return Result{}, err
	}
	if err := s.Roles.Create(ctx, ownerRole); err != nil {
		return Result{}, fmt.Errorf("create owner role: %w", err)
	}

	rootPos, err := domain.NewPosition("p-root", ownerRole.ID, nil, params.OrganizationID)
	if err != nil {
		return Result{}, err
	}
	if err := s.Positions.Create(ctx, rootPos); err != nil {
		return Result{}, fmt.Errorf("create root position: %w", err)
	}

	ownerIdentity := "# Owner\n\nThe person running this org. Edit this from /helix-org to " +
		"introduce yourself — your name, voice, and how you want subordinates to address you.\n"
	owner, err := domain.NewHumanWorker(worker.ID("w-owner"), rootPos.ID, ownerIdentity, params.OrganizationID)
	if err != nil {
		return Result{}, err
	}
	if err := s.Workers.Create(ctx, owner); err != nil {
		return Result{}, fmt.Errorf("create owner worker: %w", err)
	}

	env, err := domain.NewEnvironment(owner.ID(), params.EnvironmentPath, now, params.OrganizationID)
	if err != nil {
		return Result{}, err
	}
	if err := s.Environments.Create(ctx, env); err != nil {
		return Result{}, fmt.Errorf("create owner environment: %w", err)
	}

	defaults := []tool.Name{
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
	if err := tools.EnsureActivationStream(ctx, s, params.OrganizationID, owner.ID(), owner.ID(), now); err != nil {
		return Result{}, fmt.Errorf("owner activation stream: %w", err)
	}

	for _, name := range defaults {
		g, err := domain.NewToolGrant(
			grant.ID("g-owner-"+uuid.NewString()),
			owner.ID(),
			name,
			params.OrganizationID,
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
		RoleID:          ownerRole.ID,
		PositionID:      rootPos.ID,
		EnvironmentPath: params.EnvironmentPath,
	}, nil
}
