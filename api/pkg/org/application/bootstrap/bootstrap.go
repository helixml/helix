// Package bootstrap creates the per-org initial owner Role (with the
// structural tool list) and the owner Worker. The Worker's MCP surface
// is derived live from r-owner.Tools — there is no separate per-Worker
// grants step. Run once per helix.Organization; subsequent calls for
// the same org return ErrAlreadyInitialised, leaving other orgs free
// to bootstrap independently.
package bootstrap

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/tools"
	"github.com/helixml/helix/api/pkg/org/domain/environment"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
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
	WorkerID        orgchart.WorkerID
	RoleID          orgchart.RoleID
	EnvironmentPath string
}

// ErrAlreadyInitialised is returned when the org already has an owner
// Worker.
var ErrAlreadyInitialised = errors.New("org is already initialised")

// Run performs the bootstrap for one helix.Organization: create the
// owner's Role, Worker, and Environment row. Each row is stamped with
// params.OrganizationID, so the org-delete FK cascade reaps them
// automatically.
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

	// The owner Role's Tools is the single source of truth for what
	// MCP tools the owner Worker sees: the MCP handler reads
	// Worker → Role.Tools live on every request.
	defaults := []tool.Name{
		tools.CreateRoleName,
		tools.UpdateRoleName,
		tools.UpdateIdentityName,
		tools.HireWorkerName,
		tools.CreateStreamName,
		tools.StreamMembersName,
		tools.SubscribeName,
		tools.UnsubscribeName,
		tools.InviteWorkersName,
		tools.PublishName,
		tools.DMName,
		tools.ListRolesName,
		tools.GetRoleName,
		tools.ListWorkersName,
		tools.GetWorkerName,
		tools.GetWorkerEnvironmentName,
		tools.ListStreamsName,
		tools.GetStreamName,
		tools.ListStreamEventsName,
		tools.ReadEventsName,
		tools.WorkerLogName,
	}

	ownerRole, err := orgchart.NewRole("r-owner", ownerRoleContent, defaults, nil, now, params.OrganizationID)
	if err != nil {
		return Result{}, err
	}
	if err := s.Roles.Create(ctx, ownerRole); err != nil {
		return Result{}, fmt.Errorf("create owner role: %w", err)
	}

	ownerIdentity := "# Owner\n\nThe person running this org. Edit this from /helix-org to " +
		"introduce yourself — your name, voice, and how you want subordinates to address you.\n"
	owner, err := orgchart.NewHumanWorker(orgchart.WorkerID("w-owner"), ownerRole.ID, nil, ownerIdentity, params.OrganizationID)
	if err != nil {
		return Result{}, err
	}
	if err := s.Workers.Create(ctx, owner); err != nil {
		return Result{}, fmt.Errorf("create owner worker: %w", err)
	}

	env, err := environment.New(owner.ID(), params.EnvironmentPath, now, params.OrganizationID)
	if err != nil {
		return Result{}, err
	}
	if err := s.Environments.Create(ctx, env); err != nil {
		return Result{}, fmt.Errorf("create owner environment: %w", err)
	}

	if err := tools.EnsureActivationStream(ctx, s, params.OrganizationID, owner.ID(), owner.ID(), now); err != nil {
		return Result{}, fmt.Errorf("owner activation stream: %w", err)
	}

	return Result{
		WorkerID:        owner.ID(),
		RoleID:          ownerRole.ID,
		EnvironmentPath: params.EnvironmentPath,
	}, nil
}
