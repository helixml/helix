// Package store defines the persistence contracts for the org-graph
// subsystem (workers, positions, roles, grants, streams, events,
// subscriptions, activations, environments, configs). The concrete
// implementation lives in the sibling gorm sub-package — dialect-
// portable GORM, wired against helix's Postgres connection.
package store

import (
	"context"
	"errors"

	"github.com/helixml/helix/api/pkg/org/activation"
	"github.com/helixml/helix/api/pkg/org/event"
	"github.com/helixml/helix/api/pkg/org/grant"
	"github.com/helixml/helix/api/pkg/org/position"
	"github.com/helixml/helix/api/pkg/org/role"
	"github.com/helixml/helix/api/pkg/org/stream"
	"github.com/helixml/helix/api/pkg/org/tool"
	"github.com/helixml/helix/api/pkg/org/worker"
	"github.com/helixml/helix/api/pkg/org/domain"
)

// ErrNotFound signals that the requested record does not exist.
// Repos wrap this with %w so callers can errors.Is it.
var ErrNotFound = errors.New("record not found")

// Every store method takes an explicit `orgID string` parameter
// (except Create/Update, where the org is carried by the domain
// aggregate). The composite (id, org_id) PK is what lets short
// readable IDs (`w-owner`, `p-root`, `r-owner`) repeat across helix
// tenants. ErrNotFound is returned when the (orgID, id) pair doesn't
// exist — even if the bare id exists under another org.

// Roles persists job descriptions.
type Roles interface {
	Create(ctx context.Context, role role.Role) error
	Get(ctx context.Context, orgID string, id role.ID) (role.Role, error)
	List(ctx context.Context, orgID string) ([]role.Role, error)
	Update(ctx context.Context, role role.Role) error
}

// Positions persists slots in the org chart.
type Positions interface {
	Create(ctx context.Context, pos domain.Position) error
	Get(ctx context.Context, orgID string, id position.ID) (domain.Position, error)
	List(ctx context.Context, orgID string) ([]domain.Position, error)
	ListChildren(ctx context.Context, orgID string, parent position.ID) ([]domain.Position, error)
	// Update mutates ParentID and RoleID; the (orgID, id) key cannot
	// change. Returns ErrNotFound when the (orgID, id) pair doesn't
	// exist.
	Update(ctx context.Context, pos domain.Position) error
}

// Workers persists humans and AIs. Update mutates fields the system
// allows changing in place — currently just IdentityContent (set at
// hire by the caller, replaced wholesale by update_identity). Identity
// is the per-Worker description; the system holds it in the domain
// rather than on disk so it survives any change in env layout.
//
// Delete removes the worker row. Callers are expected to have already
// torn down dependent rows (subscriptions, grants, environment,
// runtime state) — the store does not cascade. See the lifecycle
// service in api/pkg/org/lifecycle for the canonical cascade.
type Workers interface {
	Create(ctx context.Context, worker domain.Worker) error
	Get(ctx context.Context, orgID string, id worker.ID) (domain.Worker, error)
	List(ctx context.Context, orgID string) ([]domain.Worker, error)
	Update(ctx context.Context, worker domain.Worker) error
	Delete(ctx context.Context, orgID string, id worker.ID) error
}

// WorkerRuntimeState is a sidecar key/value store keyed by
// (orgID, workerID, backend). Runtime backends (the Helix integration
// today, future local containers, etc.) write whatever per-Worker
// pointers they need — Helix uses keys like "session_id", "project_id",
// "agent_app_id", "repo_id" — without forcing the domain to grow a
// field every time.
//
// The "backend" component is a free-form string the runtime owns
// (e.g. "helix"); helix-org core never reads or writes it.
type WorkerRuntimeState interface {
	Get(ctx context.Context, orgID string, workerID worker.ID, backend string) (map[string]string, error)
	Set(ctx context.Context, orgID string, workerID worker.ID, backend, key, value string) error
	SetMany(ctx context.Context, orgID string, workerID worker.ID, backend string, kv map[string]string) error
	Clear(ctx context.Context, orgID string, workerID worker.ID, backend string) error
}

// Grants persists tool grants.
type Grants interface {
	Create(ctx context.Context, g domain.ToolGrant) error
	Get(ctx context.Context, orgID string, id grant.ID) (domain.ToolGrant, error)
	ListByWorker(ctx context.Context, orgID string, workerID worker.ID) ([]domain.ToolGrant, error)
	FindForWorkerAndTool(ctx context.Context, orgID string, workerID worker.ID, toolName tool.Name) (domain.ToolGrant, error)
	Delete(ctx context.Context, orgID string, id grant.ID) error
}

// Streams persists named event sources. Streams are created explicitly
// via the create_stream tool. Every Stream carries a Transport — the
// default (TransportLocal) keeps events local and notifies the
// in-process broadcaster; other transports compose external I/O over
// the same local store.
type Streams interface {
	Create(ctx context.Context, s domain.Stream) error
	Get(ctx context.Context, orgID string, id stream.ID) (domain.Stream, error)
	List(ctx context.Context, orgID string) ([]domain.Stream, error)
}

// Subscriptions persists (Worker, Stream) links. The triple
// (orgID, workerID, streamID) is the key — there is no synthetic ID.
type Subscriptions interface {
	Create(ctx context.Context, sub domain.Subscription) error
	Delete(ctx context.Context, orgID string, workerID worker.ID, streamID stream.ID) error
	Find(ctx context.Context, orgID string, workerID worker.ID, streamID stream.ID) (domain.Subscription, error)
	ListForWorker(ctx context.Context, orgID string, workerID worker.ID) ([]domain.Subscription, error)
	ListForStream(ctx context.Context, orgID string, streamID stream.ID) ([]domain.Subscription, error)
}

// Events persists entries published on a Stream.
type Events interface {
	Append(ctx context.Context, e domain.Event) error
	ListForStream(ctx context.Context, orgID string, streamID stream.ID, limit int) ([]domain.Event, error)
	ListForWorker(ctx context.Context, orgID string, workerID worker.ID, limit int) ([]domain.Event, error)
	ListSince(ctx context.Context, orgID string, streamIDs []stream.ID, since event.ID, limit int) ([]domain.Event, error)
	// ListAll returns events across every Stream in the given org,
	// newest first. Powers the unified "All streams" activity feed in
	// the UI. If limit <= 0, no limit is applied — callers are
	// expected to pass a sane cap.
	ListAll(ctx context.Context, orgID string, limit int) ([]domain.Event, error)
}

// Environments persists the per-Worker directory handle. The manager
// populates the directory before hire; this table just tracks that a
// directory exists and which Worker owns it.
type Environments interface {
	Create(ctx context.Context, env domain.Environment) error
	Get(ctx context.Context, orgID string, workerID worker.ID) (domain.Environment, error)
	Delete(ctx context.Context, orgID string, workerID worker.ID) error
}

// Configs persists operational-config rows: transport credentials,
// model selection, runtime knobs, etc. Keyed by (orgID, key) so each
// helix tenant has its own settings.
type Configs interface {
	Set(ctx context.Context, cfg domain.Config) error
	Get(ctx context.Context, orgID, key string) (domain.Config, error)
	List(ctx context.Context, orgID, prefix string) ([]domain.Config, error)
	Delete(ctx context.Context, orgID, key string) error
}

// Store bundles all repositories a single concrete implementation provides.
// Handlers and tools depend on the narrower interfaces above; Store is the
// wiring point.
//
// Activations is the typed port defined in api/pkg/org/activation —
// the interface lives next to the aggregate it persists, so the
// storage boundary is part of the domain package, not a parallel
// declaration here. Lifted in B5.5.
type Store struct {
	Roles              Roles
	Positions          Positions
	Workers            Workers
	WorkerRuntimeState WorkerRuntimeState
	Grants             Grants
	Streams            Streams
	Subscriptions      Subscriptions
	Events             Events
	Environments       Environments
	Configs            Configs
	Activations        activation.Repository
}
