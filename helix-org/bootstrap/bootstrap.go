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

	"github.com/helixml/helix/api/pkg/org/transport"
	"github.com/helixml/helix/helix-org/agent"
	"github.com/helixml/helix/helix-org/domain"
	"github.com/helixml/helix/helix-org/store"
	"github.com/helixml/helix/helix-org/tools"
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
}

// Result summarises the newly-created owner.
type Result struct {
	WorkerID        domain.WorkerID
	RoleID          domain.RoleID
	PositionID      domain.PositionID
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
	if len(existing) > 0 {
		return Result{}, ErrAlreadyInitialised
	}

	now := time.Now().UTC()
	role, err := domain.NewRole("r-owner", ownerRoleContent, now)
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
	owner, err := domain.NewHumanWorker(domain.WorkerID("w-owner"), []domain.PositionID{rootPos.ID}, ownerIdentity)
	if err != nil {
		return Result{}, err
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
	defaults := []domain.ToolName{
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
	// (s-activations-<workerID>) where its turns are recorded. For AI
	// Workers this stream is created by hire_worker; w-owner is
	// bootstrapped (not hired), so we mint the stream and self-
	// subscription here so /ui/streams shows the owner alongside
	// every other Worker. The owner-chat bridge publishes activation
	// events to this stream via agent.PublishActivationEvent.
	ownerActStreamID := agent.ActivationStreamID(owner.ID())
	ownerActStream, err := domain.NewStream(
		ownerActStreamID,
		"Activations: "+string(owner.ID()),
		"Per-message activation transcript for "+string(owner.ID())+
			" — the owner's chat turns appear here, same as every AI Worker's.",
		owner.ID(),
		now,
		transport.Transport{},
	)
	if err != nil {
		return Result{}, fmt.Errorf("owner activation stream: %w", err)
	}
	if err := s.Streams.Create(ctx, ownerActStream); err != nil {
		return Result{}, fmt.Errorf("create owner activation stream: %w", err)
	}
	ownerActSub, err := domain.NewSubscription(owner.ID(), ownerActStreamID, now)
	if err != nil {
		return Result{}, fmt.Errorf("owner activation subscription: %w", err)
	}
	if err := s.Subscriptions.Create(ctx, ownerActSub); err != nil {
		return Result{}, fmt.Errorf("subscribe owner to activation stream: %w", err)
	}

	for _, name := range defaults {
		g, err := domain.NewToolGrant(
			domain.GrantID("g-owner-"+uuid.NewString()),
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
