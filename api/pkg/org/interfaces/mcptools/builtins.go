package mcptools

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/helixml/helix/api/pkg/org/application/lifecycle"
	"github.com/helixml/helix/api/pkg/org/application/publishing"
	"github.com/helixml/helix/api/pkg/org/application/queries"
	"github.com/helixml/helix/api/pkg/org/application/reconcile"
	"github.com/helixml/helix/api/pkg/org/application/roles"
	"github.com/helixml/helix/api/pkg/org/application/streams"
	"github.com/helixml/helix/api/pkg/org/application/subscriptions"
	"github.com/helixml/helix/api/pkg/org/application/workers"
	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/credential"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	"github.com/helixml/helix/api/pkg/org/infrastructure/runtime"
	"github.com/helixml/helix/api/pkg/org/infrastructure/wakebus"
)

// Clock returns the current time. Tests override it.
type Clock func() time.Time

// IDGen generates new unique string IDs. Tests override it.
type IDGen func() string

// EventDispatcher fans a freshly-published Event out to every
// subscribed AI Worker as a separate Spawner activation. Tools call it
// after persisting an Event. The interface keeps tools.Deps free of a
// dependency on the dispatch package (avoiding an import cycle: the
// dispatcher itself imports tools).
type EventDispatcher interface {
	Dispatch(ctx context.Context, event streaming.Event)
	// DispatchHire fires a TriggerHire activation. activationID is the
	// pre-allocated audit-row ID hire_worker created before calling
	// DispatchHire — it travels through the trigger so the Spawner
	// reuses the existing row instead of writing a sibling. Empty
	// activationID is allowed for callers that don't pre-allocate
	// (legacy code paths, tests that don't wire activation.Repository).
	DispatchHire(ctx context.Context, orgID string, workerID orgchart.WorkerID, activationID activation.ID)
}

// Deps is the MCP tool surface — the pre-built application services and
// read facade every tool delegates to, the MCP-side mirror of the REST
// api.Deps. Tools never touch a store repository: reads go through
// Queries, writes through the aggregate services. Built once by
// Config.Build() at the composition root and handed to RegisterBuiltins.
type Deps struct {
	// Queries is the read facade every read tool projects from — the
	// same one the REST read handlers use, so the two surfaces can't
	// drift on read semantics.
	Queries       *queries.Queries
	Roles         *roles.Roles
	Streams       *streams.Streams
	Workers       *workers.Workers
	Subscriptions *subscriptions.Subscriptions
	Publishing    *publishing.Publishing
	// Lifecycle owns Hire (the MCP hire_worker tool delegates here, the
	// same service the REST POST /workers handler drives).
	Lifecycle *lifecycle.Service

	// Workspace is the per-runtime file-mirror port: update_role /
	// update_identity call MirrorFile after the service persists, so the
	// running session sees the change before the next activation.
	Workspace runtime.WorkspaceSync
	// ProjectConfig backs get_worker_project + configure_worker_project
	// (owner-only read/patch of a Worker's helix project config).
	ProjectConfig runtime.ProjectConfig
	// CredentialProviders is the registry mint_credential dispatches on.
	CredentialProviders map[string]credential.Provider
	// Hub lets the long-poll read tools (read_events, worker_log) block
	// on new events. It is a broadcaster, not a store.
	Hub *wakebus.Bus
}

// Config carries the construction seams the composition root supplies to
// assemble the tool Deps: the store + clock/id-gen + reconciler + the
// runtime collaborators. Build() turns it into a Deps. This is the only
// place store repositories are read — a composition convenience (the
// same shape as server.NewFromStore), never reached from a tool.
//
// EnvsDir is the directory under which each Worker's Environment lives.
// Hub/Dispatcher are optional (nil → publish skips notify/dispatch).
// Workspace defaults to a no-op for tests.
type Config struct {
	Store               *store.Store
	Queries             *queries.Queries
	Now                 Clock
	NewID               IDGen
	Hub                 *wakebus.Bus
	Dispatcher          EventDispatcher
	Workspace           runtime.WorkspaceSync
	HireHook            runtime.HireHook
	ProjectConfig       runtime.ProjectConfig
	Reconciler          *reconcile.Reconciler
	CredentialProviders map[string]credential.Provider
}

// Build assembles the application services from the config and returns
// the lean tool Deps. Reads from the store happen only here.
func (c Config) Build() Deps {
	return Deps{
		Queries:             c.Queries,
		Roles:               c.rolesService(),
		Streams:             c.streamsService(),
		Workers:             c.workersService(),
		Subscriptions:       c.subscriptionsService(),
		Publishing:          c.publishingService(),
		Lifecycle:           c.lifecycleService(),
		Workspace:           c.Workspace,
		ProjectConfig:       c.ProjectConfig,
		CredentialProviders: c.CredentialProviders,
		Hub:                 c.Hub,
	}
}

// subscriptionsService builds the subscription application service.
func (c Config) subscriptionsService() *subscriptions.Subscriptions {
	return subscriptions.New(subscriptions.Deps{
		Subscriptions: c.Store.Subscriptions,
		Streams:       c.Store.Streams,
		Workers:       c.Store.Workers,
		Now:           c.Now,
	})
}

// publishingService builds the publish application service. Hub/Dispatcher
// are assigned only when non-nil to avoid wrapping a typed-nil in the
// Notifier interface (which would make the nil check inside the service
// pass and then panic).
func (c Config) publishingService() *publishing.Publishing {
	pd := publishing.Deps{
		Streams: c.Store.Streams,
		Events:  c.Store.Events,
		Now:     c.Now,
		NewID:   c.NewID,
	}
	if c.Hub != nil {
		pd.Hub = c.Hub
	}
	if c.Dispatcher != nil {
		pd.Dispatcher = c.Dispatcher
	}
	return publishing.New(pd)
}

// workersService builds the worker-mutation application service. UpdateRole
// delegates to the roles service so the held-Role content rewrite preserves
// tools/streams.
func (c Config) workersService() *workers.Workers {
	return workers.New(workers.Deps{
		Workers:    c.Store.Workers,
		Roles:      c.rolesService(),
		Lines:      c.Store.ReportingLines,
		Reconciler: c.Reconciler,
	})
}

// lifecycleService builds the worker-lifecycle service (Hire) for the MCP
// surface. The hire semantics (env dir, reporting line, topology reconcile,
// hire dispatch) live in exactly one place — shared with the REST POST
// /workers handler. Only the Hire-relevant fields are wired (the MCP
// surface never fires Workers, so Helix/Mirror/Owner stay nil).
func (c Config) lifecycleService() *lifecycle.Service {
	svc := &lifecycle.Service{
		Store:      c.Store,
		Reconciler: c.Reconciler,
		HireHook:   c.HireHook,
		Now:        c.Now,
		NewID:      c.NewID,
	}
	// c.Dispatcher (EventDispatcher) satisfies lifecycle.HireDispatcher
	// (DispatchHire); guard the typed-nil-in-interface case.
	if c.Dispatcher != nil {
		svc.Dispatcher = c.Dispatcher
	}
	return svc
}

// rolesService builds the role-mutation application service, injecting
// BaseReadTools as the universal baseline so the MCP create_role tool and
// the REST role handlers union the same set.
func (c Config) rolesService() *roles.Roles {
	return roles.New(roles.Deps{
		Roles:     c.Store.Roles,
		Now:       c.Now,
		NewID:     c.NewID,
		BaseTools: BaseReadTools,
	})
}

// streamsService builds the stream-mutation application service.
func (c Config) streamsService() *streams.Streams {
	return streams.New(streams.Deps{
		Streams: c.Store.Streams,
		Now:     c.Now,
		NewID:   c.NewID,
	})
}

// DefaultDeps wires production defaults into a Config: real UUIDs and
// wall-clock time, a no-op WorkspaceSync that callers replace with the
// runtime-specific implementation, and the Queries facade + Reconciler
// built off the store. EnvsDir, Hub, and Dispatcher are left zero —
// composition callers wire them in before calling Build().
func DefaultDeps(s *store.Store) Config {
	c := Config{
		Store:               s,
		Now:                 func() time.Time { return time.Now().UTC() },
		NewID:               uuid.NewString,
		Workspace:           runtime.NoopWorkspaceSync{},
		HireHook:            runtime.NoopHireHook{},
		ProjectConfig:       runtime.NoopProjectConfig{},
		CredentialProviders: map[string]credential.Provider{},
	}
	c.Reconciler = reconcile.New(reconcile.Deps{
		Workers:        s.Workers,
		ReportingLines: s.ReportingLines,
		Streams:        s.Streams,
		Subscriptions:  s.Subscriptions,
		Now:            c.Now,
	})
	c.Queries = queries.New(queries.Deps{
		Roles: s.Roles, Workers: s.Workers, ReportingLines: s.ReportingLines,
		Streams: s.Streams, Subscriptions: s.Subscriptions, Events: s.Events,
		Activations: s.Activations,
	})
	return c
}

// RegisterBuiltins registers every built-in tool on the registry —
// mutations on the org graph plus the matching read tools. Test tools
// (like Ping) are not included.
func RegisterBuiltins(reg *Registry, deps Deps) error {
	if deps.Workspace == nil {
		return fmt.Errorf("tools.RegisterBuiltins: deps.Workspace is required (use runtime.NoopWorkspaceSync{} for tests)")
	}
	builtins := []tool.Tool{
		// Mutations.
		&CreateRole{deps: deps},
		&UpdateRole{deps: deps},
		&UpdateIdentity{deps: deps},
		&HireWorker{deps: deps},
		&CreateStream{deps: deps},
		&MintCredential{deps: deps, providers: deps.CredentialProviders},
		&StreamMembers{deps: deps},
		&Subscribe{deps: deps},
		&Unsubscribe{deps: deps},
		&InviteWorkers{deps: deps},
		&Publish{deps: deps},
		&DM{deps: deps},
		&ConfigureWorkerProject{deps: deps},
		// Reads. Each is a thin wrapper around a store call; together
		// they replace the jsonapi GET handlers the server used to expose.
		&ListRoles{deps: deps},
		&GetRole{deps: deps},
		&ListWorkers{deps: deps},
		&GetWorker{deps: deps},
		&Managers{deps: deps},
		&Reports{deps: deps},
		&GetWorkerProject{deps: deps},
		&ListStreams{deps: deps},
		&GetStream{deps: deps},
		&ListStreamEvents{deps: deps},
		&ReadEvents{deps: deps},
		&WorkerLog{deps: deps},
	}
	for _, tool := range builtins {
		if err := reg.Register(tool); err != nil {
			return fmt.Errorf("register %q: %w", tool.Name(), err)
		}
	}
	// Fail fast if BaseReadTools references a name that isn't registered
	// — a typo in defaults.go would otherwise produce silently-broken
	// Roles whose reconciled tool list is missing one of the baseline
	// entries.
	for _, name := range BaseReadTools {
		if _, err := reg.Get(name); err != nil {
			return fmt.Errorf("BaseReadTools references unregistered tool %q: %w", name, err)
		}
	}
	return nil
}
