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

	"github.com/helixml/helix/api/pkg/org/domain/activation"
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

// ErrOwnerRoleProtected is returned by DeleteRole when the caller
// targets the embedded owner role (`r-owner`). REST maps to 409.
var ErrOwnerRoleProtected = errors.New("cannot delete the owner role")

// Fire tears down a Worker end-to-end:
//
//  1. Read the Helix-runtime state (project + app IDs) before clearing.
//  2. DeleteProject on Helix — stops any active sessions.
//  3. DeleteApp on Helix — removes the auto-provisioned agent app.
//  4. Clear the WorkerRuntimeState sidecar.
//  5. Delete every Subscription for this worker.
//  6. Remove the env directory from disk and delete its row.
//  7. Delete the per-Worker activation Stream
//     (`s-activations-<workerID>`) so it stops surfacing as an
//     active channel on the Streams page.
//  8. Delete the Worker row.
//
// Steps 2/3/5/6/7 are best-effort and logged on failure — a half-
// torn worker is better than refusing to clean up partial state.
// Step 8 is the only step whose error propagates (the row is the
// user-visible source of truth).
//
// Subscriptions are worker-anchored, so they die with the worker. A
// new hire into the same Role does not automatically inherit them;
// the hiring playbook explicitly re-subscribes.
//
// Tool capability is derived from Role.Tools, so there is no
// per-Worker grants cascade.
//
// Activation events themselves are intentionally left behind as an
// audit trail; only the Stream row is dropped.
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

	// The worker's subscriptions and every reporting line that
	// references it are cascaded structurally when the worker row is
	// deleted below (Workers.Delete drops the subs; the
	// org_reporting_lines ON DELETE CASCADE foreign keys drop the
	// lines). Nothing to drop explicitly here.

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

	// Drop the per-Worker activation Stream so it no longer shows up
	// on the Streams page. Best-effort: log on failure so a half-torn
	// worker still has its row deleted below. Events on the stream
	// survive (audit trail) because the Events table isn't keyed on
	// Streams.
	if s.Store.Streams != nil {
		streamID := activation.StreamID(id)
		if err := s.Store.Streams.Delete(ctx, orgID, streamID); err != nil {
			s.logger().Warn("fire: delete activation stream", "worker", id, "stream", streamID, "err", err)
		}
	}

	if err := s.Store.Workers.Delete(ctx, orgID, id); err != nil {
		return fmt.Errorf("delete worker row %q: %w", id, err)
	}
	return nil
}

// DeleteRole tears down a Role end-to-end:
//
//  1. Refuse if id is the canonical owner role ("r-owner").
//  2. Fire every Worker whose RoleID matches id (each Worker gets
//     the full Fire cascade — project teardown, env removal,
//     subscriptions, the worker row). Refuses if the owner Worker
//     holds this role.
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

	if err := s.fireWorkersWithRole(ctx, orgID, id); err != nil {
		return err
	}

	if err := s.Store.Roles.Delete(ctx, orgID, id); err != nil {
		return fmt.Errorf("delete role %q: %w", id, err)
	}
	return nil
}

// fireWorkersWithRole fires every Worker holding the given Role.
// Honours the owner-protect rule: if the owner Worker holds the
// Role, refuse the whole operation — bringing the owner down with
// a role deletion is always wrong.
func (s *Service) fireWorkersWithRole(ctx context.Context, orgID string, roleID orgchart.RoleID) error {
	workers, err := s.Store.Workers.List(ctx, orgID)
	if err != nil {
		return fmt.Errorf("list workers: %w", err)
	}
	for _, w := range workers {
		if w.RoleID() != roleID {
			continue
		}
		if w.ID() == s.Owner {
			return ErrOwnerProtected
		}
		if err := s.Fire(ctx, orgID, w.ID()); err != nil {
			s.logger().Warn("fire worker for role teardown", "role", roleID, "worker", w.ID(), "err", err)
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
