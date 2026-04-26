// Package store defines the persistence contracts used by the server and
// tools. Concrete implementations live in sub-packages (e.g. sqlite).
package store

import (
	"context"
	"errors"

	"github.com/helixml/helix-org/domain"
)

// ErrNotFound signals that the requested record does not exist.
// Repos wrap this with %w so callers can errors.Is it.
var ErrNotFound = errors.New("record not found")

// Roles persists job descriptions.
type Roles interface {
	Create(ctx context.Context, role domain.Role) error
	Get(ctx context.Context, id domain.RoleID) (domain.Role, error)
	List(ctx context.Context) ([]domain.Role, error)
	Update(ctx context.Context, role domain.Role) error
}

// Positions persists slots in the org chart.
type Positions interface {
	Create(ctx context.Context, pos domain.Position) error
	Get(ctx context.Context, id domain.PositionID) (domain.Position, error)
	List(ctx context.Context) ([]domain.Position, error)
	ListChildren(ctx context.Context, parent domain.PositionID) ([]domain.Position, error)
}

// Workers persists humans and AIs.
type Workers interface {
	Create(ctx context.Context, worker domain.Worker) error
	Get(ctx context.Context, id domain.WorkerID) (domain.Worker, error)
	List(ctx context.Context) ([]domain.Worker, error)
}

// Grants persists tool grants.
type Grants interface {
	Create(ctx context.Context, g domain.ToolGrant) error
	Get(ctx context.Context, id domain.GrantID) (domain.ToolGrant, error)
	ListByWorker(ctx context.Context, workerID domain.WorkerID) ([]domain.ToolGrant, error)
	FindForWorkerAndTool(ctx context.Context, workerID domain.WorkerID, toolName domain.ToolName) (domain.ToolGrant, error)
	Delete(ctx context.Context, id domain.GrantID) error
}

// Channels persists shared event sources. Channels are created
// explicitly via the create_channel tool.
type Channels interface {
	Create(ctx context.Context, ch domain.Channel) error
	Get(ctx context.Context, id domain.ChannelID) (domain.Channel, error)
	List(ctx context.Context) ([]domain.Channel, error)
}

// Streams persists (Worker, Channel) subscriptions.
type Streams interface {
	Create(ctx context.Context, s domain.Stream) error
	Delete(ctx context.Context, id domain.StreamID) error
	FindForWorkerAndChannel(ctx context.Context, workerID domain.WorkerID, channelID domain.ChannelID) (domain.Stream, error)
	ListForWorker(ctx context.Context, workerID domain.WorkerID) ([]domain.Stream, error)
	ListForChannel(ctx context.Context, channelID domain.ChannelID) ([]domain.Stream, error)
}

// Events persists entries published on a Channel.
type Events interface {
	Append(ctx context.Context, e domain.Event) error
	ListForChannel(ctx context.Context, channelID domain.ChannelID, limit int) ([]domain.Event, error)
	// ListForWorker returns events reaching a Worker's Feed via their Streams,
	// newest first. If limit <= 0, no limit is applied.
	ListForWorker(ctx context.Context, workerID domain.WorkerID, limit int) ([]domain.Event, error)
	// ListSince returns events on the named Channels strictly newer than the
	// `since` event, oldest first. If channelIDs is empty, returns nothing
	// (caller's glob matched no channels). If `since` is empty, returns the
	// most recent `limit` events on the named channels in oldest-first order.
	// If `since` does not exist, returns the same as if it were empty. If
	// limit <= 0, no limit is applied.
	ListSince(ctx context.Context, channelIDs []domain.ChannelID, since domain.EventID, limit int) ([]domain.Event, error)
}

// Environments persists the per-Worker directory handle. The manager
// populates the directory before hire; this table just tracks that a
// directory exists and which Worker owns it.
type Environments interface {
	Create(ctx context.Context, env domain.Environment) error
	Get(ctx context.Context, workerID domain.WorkerID) (domain.Environment, error)
}

// Store bundles all repositories a single concrete implementation provides.
// Handlers and tools depend on the narrower interfaces above; Store is the
// wiring point.
type Store struct {
	Roles        Roles
	Positions    Positions
	Workers      Workers
	Grants       Grants
	Channels     Channels
	Streams      Streams
	Events       Events
	Environments Environments
}
