// Package store defines the persistence contracts for the org-graph
// subsystem (workers, positions, roles, grants, streams, events,
// subscriptions, activations, environments, configs). The concrete
// implementation lives in the sibling gorm sub-package — dialect-
// portable GORM, wired against helix's Postgres connection.
package store

import (
	"context"
	"errors"

	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/config"
	"github.com/helixml/helix/api/pkg/org/domain/environment"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
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
	Create(ctx context.Context, role orgchart.Role) error
	Get(ctx context.Context, orgID string, id orgchart.RoleID) (orgchart.Role, error)
	List(ctx context.Context, orgID string) ([]orgchart.Role, error)
	Update(ctx context.Context, role orgchart.Role) error
	// Delete removes the role row. Caller is expected to have torn
	// down dependent positions; the lifecycle service in
	// application/lifecycle owns the cascade (positions + workers).
	Delete(ctx context.Context, orgID string, id orgchart.RoleID) error
}

// Positions persists slots in the org chart.
type Positions interface {
	Create(ctx context.Context, pos orgchart.Position) error
	Get(ctx context.Context, orgID string, id orgchart.PositionID) (orgchart.Position, error)
	List(ctx context.Context, orgID string) ([]orgchart.Position, error)
	ListChildren(ctx context.Context, orgID string, parent orgchart.PositionID) ([]orgchart.Position, error)
	// Update mutates ParentID and RoleID; the (orgID, id) key cannot
	// change. Returns ErrNotFound when the (orgID, id) pair doesn't
	// exist.
	Update(ctx context.Context, pos orgchart.Position) error
	// Delete removes the position row. Caller is expected to have
	// already cleared any worker pointing at the position; the
	// lifecycle service owns the cascade.
	Delete(ctx context.Context, orgID string, id orgchart.PositionID) error
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
	Create(ctx context.Context, worker orgchart.Worker) error
	Get(ctx context.Context, orgID string, id orgchart.WorkerID) (orgchart.Worker, error)
	List(ctx context.Context, orgID string) ([]orgchart.Worker, error)
	Update(ctx context.Context, worker orgchart.Worker) error
	Delete(ctx context.Context, orgID string, id orgchart.WorkerID) error
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
	Get(ctx context.Context, orgID string, workerID orgchart.WorkerID, backend string) (map[string]string, error)
	Set(ctx context.Context, orgID string, workerID orgchart.WorkerID, backend, key, value string) error
	SetMany(ctx context.Context, orgID string, workerID orgchart.WorkerID, backend string, kv map[string]string) error
	Clear(ctx context.Context, orgID string, workerID orgchart.WorkerID, backend string) error
}

// Grants persists tool grants.
type Grants interface {
	Create(ctx context.Context, g orgchart.ToolGrant) error
	Get(ctx context.Context, orgID string, id orgchart.GrantID) (orgchart.ToolGrant, error)
	ListByWorker(ctx context.Context, orgID string, workerID orgchart.WorkerID) ([]orgchart.ToolGrant, error)
	FindForWorkerAndTool(ctx context.Context, orgID string, workerID orgchart.WorkerID, toolName tool.Name) (orgchart.ToolGrant, error)
	Delete(ctx context.Context, orgID string, id orgchart.GrantID) error
}

// Streams persists named event sources. Streams are created explicitly
// via the create_stream tool. Every Stream carries a Transport — the
// default (TransportLocal) keeps events local and notifies the
// in-process broadcaster; other transports compose external I/O over
// the same local store.
type Streams interface {
	Create(ctx context.Context, s streaming.Stream) error
	Get(ctx context.Context, orgID string, id streaming.StreamID) (streaming.Stream, error)
	List(ctx context.Context, orgID string) ([]streaming.Stream, error)
	// Update replaces the mutable fields on a Stream: name,
	// description, and the entire transport (kind + config). The
	// composite (id, orgID) identifies the row; ID, OrganizationID,
	// CreatedBy and CreatedAt are immutable and ignored. Returns
	// store.ErrNotFound when the row doesn't exist.
	Update(ctx context.Context, s streaming.Stream) error
	// Delete removes a stream row. Composite key (id, orgID). Callers
	// (REST handler, MCP delete_stream tool when added) are
	// responsible for any cascading subscription / role-manifest
	// cleanup — the Streams repo itself is intentionally narrow.
	Delete(ctx context.Context, orgID string, id streaming.StreamID) error
}

// Subscriptions persists (Position, Stream) links. The triple
// (orgID, positionID, streamID) is the key — there is no synthetic
// ID. Subscriptions are POSITION-anchored: "whoever fills this slot
// receives this stream", so hiring or firing a Worker into the
// position does NOT change which streams it consumes. Dispatch
// resolves stream → positions → current workers in those positions
// at delivery time.
type Subscriptions interface {
	Create(ctx context.Context, sub streaming.Subscription) error
	Delete(ctx context.Context, orgID string, positionID orgchart.PositionID, streamID streaming.StreamID) error
	Find(ctx context.Context, orgID string, positionID orgchart.PositionID, streamID streaming.StreamID) (streaming.Subscription, error)
	ListForPosition(ctx context.Context, orgID string, positionID orgchart.PositionID) ([]streaming.Subscription, error)
	ListForStream(ctx context.Context, orgID string, streamID streaming.StreamID) ([]streaming.Subscription, error)
}

// Events persists entries published on a Stream.
type Events interface {
	Append(ctx context.Context, e streaming.Event) error
	ListForStream(ctx context.Context, orgID string, streamID streaming.StreamID, limit int) ([]streaming.Event, error)
	ListForWorker(ctx context.Context, orgID string, workerID orgchart.WorkerID, limit int) ([]streaming.Event, error)
	ListSince(ctx context.Context, orgID string, streamIDs []streaming.StreamID, since streaming.EventID, limit int) ([]streaming.Event, error)
	// ListAll returns events across every Stream in the given org,
	// newest first. Powers the unified "All streams" activity feed in
	// the UI. If limit <= 0, no limit is applied — callers are
	// expected to pass a sane cap.
	ListAll(ctx context.Context, orgID string, limit int) ([]streaming.Event, error)
}

// Environments persists the per-Worker directory handle. The manager
// populates the directory before hire; this table just tracks that a
// directory exists and which Worker owns it.
type Environments interface {
	Create(ctx context.Context, env environment.Environment) error
	Get(ctx context.Context, orgID string, workerID orgchart.WorkerID) (environment.Environment, error)
	Delete(ctx context.Context, orgID string, workerID orgchart.WorkerID) error
}

// Configs persists operational-config rows: transport credentials,
// model selection, runtime knobs, etc. Keyed by (orgID, key) so each
// helix tenant has its own settings.
type Configs interface {
	Set(ctx context.Context, cfg config.Config) error
	Get(ctx context.Context, orgID, key string) (config.Config, error)
	List(ctx context.Context, orgID, prefix string) ([]config.Config, error)
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
