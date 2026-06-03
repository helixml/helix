// Package lifecycle owns the cross-cutting orchestration that
// composes store + runtime + on-disk state when a Worker is created
// or destroyed.
//
// Hire is intentionally not yet a lifecycle method — the canonical
// hire path is api/pkg/org/tools.HireWorker (an MCP tool), which the
// REST layer drives via a synthetic Invocation. Fire has no MCP
// counterpart by design (the LLM should not be able to delete
// workers from chat), so it lives here as a plain Go service callable
// from REST handlers only.
package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/infrastructure/runtime/helix"
)

// HelixRuntime is the slice of runtime/helix.ProjectService that the
// Fire cascade needs to tear down a Worker's Helix-side project and
// agent app. Production wiring satisfies this with the in-process
// adapter used everywhere else; the interface exists so tests can
// stub.
type HelixRuntime interface {
	DeleteProject(ctx context.Context, id string) error
	DeleteApp(ctx context.Context, id string) error
}

// Service composes the worker-lifecycle operations the REST layer
// drives. All fields are required; pass nil HelixRuntime only in
// tests that don't need the Helix-side teardown.
type Service struct {
	Store   *store.Store
	Helix   HelixRuntime
	Logger  *slog.Logger
	EnvsDir string
	// Owner is the WorkerID of the embedded owner (typically "w-owner").
	// Fire refuses to delete the owner so the alpha can't be bricked by
	// a UI misclick.
	Owner orgchart.WorkerID
}

// ErrOwnerProtected is returned by Fire when the caller targets the
// embedded owner Worker. The REST layer maps it to 409 Conflict.
var ErrOwnerProtected = errors.New("cannot fire the owner worker")

// ErrRootPositionProtected is returned by DeletePosition when the
// caller targets the embedded root position (`p-root`). REST maps to
// 409.
var ErrRootPositionProtected = errors.New("cannot delete the root position")

// ErrOwnerRoleProtected is returned by DeleteRole when the caller
// targets the embedded owner role (`r-owner`). REST maps to 409.
var ErrOwnerRoleProtected = errors.New("cannot delete the owner role")

// Fire tears down a Worker end-to-end:
//
//  1. Read the Helix-runtime state (project + app IDs) before clearing.
//  2. DeleteProject on Helix — stops any active sessions.
//  3. DeleteApp on Helix — removes the auto-provisioned agent app.
//  4. Clear the WorkerRuntimeState sidecar.
//  5. Delete every Subscription + Grant for this worker.
//  6. Remove the env directory from disk and delete its row.
//  7. Delete the Worker row.
//
// Steps 2/3/5/6 are best-effort and logged on failure — a half-torn
// worker is better than refusing to clean up partial state. Step 7
// is the only step whose error propagates (the row is the user-
// visible source of truth).
//
// Activations are intentionally left behind as an audit trail.
func (s *Service) Fire(ctx context.Context, orgID string, id orgchart.WorkerID) error {
	if id == "" {
		return errors.New("worker id is empty")
	}
	if id == s.Owner {
		return ErrOwnerProtected
	}
	if s.Store == nil {
		return errors.New("lifecycle: store is nil")
	}
	if _, err := s.Store.Workers.Get(ctx, orgID, id); err != nil {
		return fmt.Errorf("get worker %q: %w", id, err)
	}

	state, _ := helix.LoadState(ctx, s.Store, orgID, id)

	if s.Helix != nil && state.ProjectID != "" {
		if err := s.Helix.DeleteProject(ctx, state.ProjectID); err != nil && !errors.Is(err, helix.ErrProjectNotFound) {
			s.logger().Warn("fire: delete helix project", "worker", id, "project", state.ProjectID, "err", err)
		}
	}
	if s.Helix != nil && state.AgentAppID != "" {
		if err := s.Helix.DeleteApp(ctx, state.AgentAppID); err != nil && !errors.Is(err, helix.ErrProjectNotFound) {
			s.logger().Warn("fire: delete helix app", "worker", id, "app", state.AgentAppID, "err", err)
		}
	}
	if s.Store.WorkerRuntimeState != nil {
		if err := s.Store.WorkerRuntimeState.Clear(ctx, orgID, id, helix.Backend); err != nil {
			s.logger().Warn("fire: clear runtime state", "worker", id, "err", err)
		}
	}

	if subs, err := s.Store.Subscriptions.ListForWorker(ctx, orgID, id); err == nil {
		for _, sub := range subs {
			if err := s.Store.Subscriptions.Delete(ctx, orgID, id, sub.StreamID); err != nil {
				s.logger().Warn("fire: delete subscription", "worker", id, "stream", sub.StreamID, "err", err)
			}
		}
	} else {
		s.logger().Warn("fire: list subscriptions", "worker", id, "err", err)
	}

	if grants, err := s.Store.Grants.ListByWorker(ctx, orgID, id); err == nil {
		for _, g := range grants {
			if err := s.Store.Grants.Delete(ctx, orgID, g.ID); err != nil {
				s.logger().Warn("fire: delete grant", "worker", id, "grant", g.ID, "err", err)
			}
		}
	} else {
		s.logger().Warn("fire: list grants", "worker", id, "err", err)
	}

	if env, err := s.Store.Environments.Get(ctx, orgID, id); err == nil {
		if env.Path != "" {
			if rmErr := os.RemoveAll(env.Path); rmErr != nil {
				s.logger().Warn("fire: remove env dir", "worker", id, "path", env.Path, "err", rmErr)
			}
		}
	}
	if err := s.Store.Environments.Delete(ctx, orgID, id); err != nil {
		s.logger().Warn("fire: delete environment row", "worker", id, "err", err)
	}

	if err := s.Store.Workers.Delete(ctx, orgID, id); err != nil {
		return fmt.Errorf("delete worker row %q: %w", id, err)
	}
	return nil
}

// DeletePosition tears down a Position end-to-end:
//
//  1. Refuse if id is the canonical root position ("p-root").
//  2. Fire every Worker whose Position() matches id (each Worker
//     gets the full Fire cascade — project teardown, env removal,
//     subscriptions, grants, the worker row).
//  3. Delete the Position row.
//
// The Worker fires are best-effort logged; the Position delete error
// propagates. Use this whenever a Position should disappear from the
// org chart — calling stores.Positions.Delete directly is a footgun
// because it leaves dangling workers whose Position() points at a
// deleted row.
func (s *Service) DeletePosition(ctx context.Context, orgID string, id orgchart.PositionID) error {
	if id == "" {
		return errors.New("position id is empty")
	}
	if id == orgchart.PositionID("p-root") {
		return ErrRootPositionProtected
	}
	if s.Store == nil {
		return errors.New("lifecycle: store is nil")
	}
	if _, err := s.Store.Positions.Get(ctx, orgID, id); err != nil {
		return fmt.Errorf("get position %q: %w", id, err)
	}
	if err := s.fireWorkersInPosition(ctx, orgID, id); err != nil {
		return err
	}
	if err := s.Store.Positions.Delete(ctx, orgID, id); err != nil {
		return fmt.Errorf("delete position %q: %w", id, err)
	}
	return nil
}

// DeleteRole tears down a Role end-to-end:
//
//  1. Refuse if id is the canonical owner role ("r-owner").
//  2. For each Position whose RoleID matches id, fire every Worker
//     in it and delete the Position (DeletePosition's body, inlined
//     so one bad position doesn't abort the cascade).
//  3. Delete the Role row.
func (s *Service) DeleteRole(ctx context.Context, orgID string, id orgchart.RoleID) error {
	if id == "" {
		return errors.New("role id is empty")
	}
	if id == orgchart.RoleID("r-owner") {
		return ErrOwnerRoleProtected
	}
	if s.Store == nil {
		return errors.New("lifecycle: store is nil")
	}
	if _, err := s.Store.Roles.Get(ctx, orgID, id); err != nil {
		return fmt.Errorf("get role %q: %w", id, err)
	}

	positions, err := s.Store.Positions.List(ctx, orgID)
	if err != nil {
		return fmt.Errorf("list positions: %w", err)
	}
	for _, pos := range positions {
		if pos.RoleID != id {
			continue
		}
		if pos.ID == orgchart.PositionID("p-root") {
			// Defensive — p-root shouldn't carry a deletable role,
			// but if a caller hand-pokes the DB, we still refuse.
			continue
		}
		if err := s.fireWorkersInPosition(ctx, orgID, pos.ID); err != nil {
			s.logger().Warn("delete role: fire workers", "role", id, "position", pos.ID, "err", err)
			continue
		}
		if err := s.Store.Positions.Delete(ctx, orgID, pos.ID); err != nil {
			s.logger().Warn("delete role: delete position", "role", id, "position", pos.ID, "err", err)
		}
	}

	if err := s.Store.Roles.Delete(ctx, orgID, id); err != nil {
		return fmt.Errorf("delete role %q: %w", id, err)
	}
	return nil
}

// fireWorkersInPosition fires every Worker filling the given
// Position. Honours the owner-protect rule: if the owner Worker
// occupies the Position we refuse (the caller meant to delete the
// surrounding Position/Role, but bringing the owner down with it is
// always wrong).
func (s *Service) fireWorkersInPosition(ctx context.Context, orgID string, posID orgchart.PositionID) error {
	workers, err := s.Store.Workers.List(ctx, orgID)
	if err != nil {
		return fmt.Errorf("list workers: %w", err)
	}
	for _, w := range workers {
		if w.Position() != posID {
			continue
		}
		if w.ID() == s.Owner {
			return ErrOwnerProtected
		}
		if err := s.Fire(ctx, orgID, w.ID()); err != nil {
			s.logger().Warn("fire worker for position teardown", "position", posID, "worker", w.ID(), "err", err)
		}
	}
	return nil
}

func (s *Service) logger() *slog.Logger {
	if s.Logger != nil {
		return s.Logger
	}
	return slog.Default()
}
