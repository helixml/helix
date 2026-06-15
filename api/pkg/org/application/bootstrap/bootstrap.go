// Package bootstrap creates the per-org initial owner Role (with the
// structural tool list) and the owner Worker. The Worker's MCP surface
// is derived live from r-owner.Tools — there is no separate per-Worker
// tool-assignment step. Run once per helix.Organization; subsequent calls for
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

	"github.com/helixml/helix/api/pkg/org/application/reconcile"
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
	// OwnerRoleTools is the tool set the owner Role is seeded with —
	// injected by the composition root (tools.OwnerRoleTools()) so this
	// application package doesn't import the MCP-tool adapter package. The
	// owner Role's Tools is the single source of truth for what MCP tools
	// the owner Worker sees (the MCP handler reads Worker → Role.Tools
	// live on every request).
	OwnerRoleTools []tool.Name
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

	// The owner Role's Tools is the single source of truth for what MCP
	// tools the owner Worker sees: the MCP handler reads Worker →
	// Role.Tools live on every request. The list is injected
	// (tools.OwnerRoleTools()) so this package stays free of a dependency
	// on the MCP-tool adapter package.
	ownerRole, err := orgchart.NewRole("r-owner", ownerRoleContent, params.OwnerRoleTools, nil, now, params.OrganizationID)
	if err != nil {
		return Result{}, err
	}
	if err := s.Roles.Create(ctx, ownerRole); err != nil {
		return Result{}, fmt.Errorf("create owner role: %w", err)
	}

	ownerIdentity := "# Owner\n\nThe person running this org. Edit this from /helix-org to " +
		"introduce yourself — your name, voice, and how you want subordinates to address you.\n"
	owner, err := orgchart.NewHumanWorker(orgchart.WorkerID("w-owner"), ownerRole.ID, ownerIdentity, params.OrganizationID)
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

	// Mint the owner's activation Stream via the topology reconciler —
	// the same single owner of activation/team Stream lifecycle every
	// other mutation routes through. The owner is the manager-less root,
	// so the rule gives it a self-observed activation Stream (its chat
	// turns surface on the Streams page) and no team Stream yet (no
	// reports until it hires).
	rec := reconcile.New(reconcile.Deps{
		Workers:        s.Workers,
		ReportingLines: s.ReportingLines,
		Streams:        s.Streams,
		Subscriptions:  s.Subscriptions,
		Now:            func() time.Time { return now },
	})
	if err := rec.Reconcile(ctx, params.OrganizationID, owner.ID()); err != nil {
		return Result{}, fmt.Errorf("owner topology: %w", err)
	}

	return Result{
		WorkerID:        owner.ID(),
		RoleID:          ownerRole.ID,
		EnvironmentPath: params.EnvironmentPath,
	}, nil
}
