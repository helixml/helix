package mcptools

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/helixml/helix/api/pkg/org/application/bots"
	"github.com/helixml/helix/api/pkg/org/application/lifecycle"
	"github.com/helixml/helix/api/pkg/org/application/publishing"
	"github.com/helixml/helix/api/pkg/org/application/queries"
	"github.com/helixml/helix/api/pkg/org/application/reconcile"
	"github.com/helixml/helix/api/pkg/org/application/spectasks"
	"github.com/helixml/helix/api/pkg/org/application/subscriptions"
	"github.com/helixml/helix/api/pkg/org/application/topics"
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

// EventDispatcher fans a freshly-published Event out to every subscribed
// Bot as a separate Spawner activation. Tools call it after persisting
// an Event. The interface keeps tools.Deps free of a dependency on the
// dispatch package (avoiding an import cycle: the dispatcher itself
// imports tools).
type EventDispatcher interface {
	Dispatch(ctx context.Context, event streaming.Event)
	// DispatchHire fires a create activation. activationID is the
	// pre-allocated audit-row ID create_bot created before calling
	// DispatchHire — it travels through the trigger so the Spawner reuses
	// the existing row instead of writing a sibling. Empty activationID
	// is allowed for callers that don't pre-allocate (legacy code paths,
	// tests that don't wire activation.Repository).
	DispatchHire(ctx context.Context, orgID string, botID orgchart.BotID, activationID activation.ID)
}

// Deps is the MCP tool surface — the pre-built application services and
// read facade every tool delegates to, the MCP-side mirror of the REST
// api.Deps. Tools never touch a store repository: reads go through
// Queries, writes through the aggregate services. Built once by
// Config.Build() at the composition root and handed to RegisterBuiltins.
type Deps struct {
	// Queries is the read facade every read tool projects from — the same
	// one the REST read handlers use, so the two surfaces can't drift on
	// read semantics.
	Queries *queries.Queries
	// Bots is the bot-mutation service (the merge of the former roles +
	// workers services) — update_bot delegates here; create_bot goes
	// through Lifecycle, which itself drives Bots.
	Bots          *bots.Bots
	Topics        *topics.Topics
	Subscriptions *subscriptions.Subscriptions
	Publishing    *publishing.Publishing
	// Lifecycle owns Create (the MCP create_bot tool delegates here, the
	// same service the REST POST /bots handler drives).
	Lifecycle *lifecycle.Service

	// Workspace is the per-runtime file-mirror port: update_bot calls
	// MirrorFile after the service persists, so the running session sees
	// the change before the next activation.
	Workspace runtime.WorkspaceSync
	// ProjectConfig backs get_bot_project + configure_bot_project
	// (owner-only read/patch of a Bot's helix project config).
	ProjectConfig runtime.ProjectConfig
	// SpecTasks is the front-of-house application service backing the
	// spec-task tools (create/list/get/start/review/approve/request-changes/
	// create-PRs) scoped to the calling Worker's own project.
	SpecTasks *spectasks.Service
	// CredentialProviders is the registry mint_credential dispatches on.
	CredentialProviders map[string]credential.Provider
	// Hub lets the long-poll read tools (read_events, bot_log) block on
	// new events. It is a broadcaster, not a store.
	Hub *wakebus.Bus
}

// Config carries the construction seams the composition root supplies to
// assemble the tool Deps: the store + clock/id-gen + reconciler + the
// runtime collaborators. Build() turns it into a Deps. This is the only
// place store repositories are read — a composition convenience (the
// same shape as server.NewFromStore), never reached from a tool.
//
// Hub/Dispatcher are optional (nil → publish skips notify/dispatch).
// Workspace defaults to a no-op for tests.
type Config struct {
	Store         *store.Store
	Queries       *queries.Queries
	Now           Clock
	NewID         IDGen
	Hub           *wakebus.Bus
	Dispatcher    EventDispatcher
	Workspace     runtime.WorkspaceSync
	HireHook      runtime.HireHook
	ProjectConfig runtime.ProjectConfig
	// SpecTasks is the runtime port the spec-task tools dispatch on. nil
	// → Build defaults to runtime.NoopSpecTasks{} so the tools return a
	// clear "not wired" error instead of nil-derefing.
	SpecTasks           runtime.SpecTasks
	Reconciler          *reconcile.Reconciler
	CredentialProviders map[string]credential.Provider
}

// Build assembles the application services from the config and returns
// the lean tool Deps. Reads from the store happen only here.
func (c Config) Build() Deps {
	return Deps{
		Queries:             c.Queries,
		Bots:                c.botsService(),
		Topics:              c.topicsService(),
		Subscriptions:       c.subscriptionsService(),
		Publishing:          c.publishingService(),
		Lifecycle:           c.lifecycleService(),
		Workspace:           c.Workspace,
		ProjectConfig:       c.ProjectConfig,
		SpecTasks:           c.specTasksService(),
		CredentialProviders: c.CredentialProviders,
		Hub:                 c.Hub,
	}
}

// specTasksService builds the spec-task application service over the
// configured runtime port, defaulting to NoopSpecTasks when none is
// wired so the tools surface ErrSpecTasksUnsupported rather than
// nil-derefing on a typed-nil interface.
func (c Config) specTasksService() *spectasks.Service {
	port := c.SpecTasks
	if port == nil {
		port = runtime.NoopSpecTasks{}
	}
	return spectasks.New(port)
}

// subscriptionsService builds the subscription application service.
func (c Config) subscriptionsService() *subscriptions.Subscriptions {
	return subscriptions.New(subscriptions.Deps{
		Subscriptions: c.Store.Subscriptions,
		Topics:        c.Store.Topics,
		Bots:          c.Store.Bots,
		Now:           c.Now,
	})
}

// publishingService builds the publish application service. Hub/Dispatcher
// are assigned only when non-nil to avoid wrapping a typed-nil in the
// Notifier interface (which would make the nil check inside the service
// pass and then panic).
func (c Config) publishingService() *publishing.Publishing {
	pd := publishing.Deps{
		Topics: c.Store.Topics,
		Events: c.Store.Events,
		Now:    c.Now,
		NewID:  c.NewID,
	}
	if c.Hub != nil {
		pd.Hub = c.Hub
	}
	if c.Dispatcher != nil {
		pd.Dispatcher = c.Dispatcher
	}
	return publishing.New(pd)
}

// lifecycleService builds the bot-lifecycle service (Create/Delete) for
// the MCP surface. The create semantics (reporting line, topology
// reconcile, create dispatch) live in exactly one place — shared with
// the REST POST /bots handler. The bots service is wired so the row
// creation applies the base-read-tool union.
func (c Config) lifecycleService() *lifecycle.Service {
	svc := &lifecycle.Service{
		Store:          c.Store,
		Bots:           c.botsService(),
		BotReconcilers: []lifecycle.BotReconciler{c.Reconciler},
		HireHook:       c.HireHook,
		Now:            c.Now,
		NewID:          c.NewID,
	}
	// c.Dispatcher (EventDispatcher) satisfies lifecycle.CreateDispatcher
	// (DispatchHire); guard the typed-nil-in-interface case.
	if c.Dispatcher != nil {
		svc.Dispatcher = c.Dispatcher
	}
	return svc
}

// botsService builds the bot-mutation application service, injecting
// BaseReadTools as the universal baseline so the MCP create_bot tool and
// the REST bot handlers union the same set.
func (c Config) botsService() *bots.Bots {
	return bots.New(bots.Deps{
		Bots:       c.Store.Bots,
		Lines:      c.Store.ReportingLines,
		Reconciler: c.Reconciler,
		Now:        c.Now,
		NewID:      c.NewID,
		BaseTools:  BaseReadTools,
	})
}

// topicsService builds the topic-mutation application service.
func (c Config) topicsService() *topics.Topics {
	return topics.New(topics.Deps{
		Topics: c.Store.Topics,
		Now:    c.Now,
		NewID:  c.NewID,
	})
}

// DefaultDeps wires production defaults into a Config: real UUIDs and
// wall-clock time, a no-op WorkspaceSync that callers replace with the
// runtime-specific implementation, and the Queries facade + Reconciler
// built off the store. Hub and Dispatcher are left zero — composition
// callers wire them in before calling Build().
func DefaultDeps(s *store.Store) Config {
	c := Config{
		Store:               s,
		Now:                 func() time.Time { return time.Now().UTC() },
		NewID:               uuid.NewString,
		Workspace:           runtime.NoopWorkspaceSync{},
		HireHook:            runtime.NoopHireHook{},
		ProjectConfig:       runtime.NoopProjectConfig{},
		SpecTasks:           runtime.NoopSpecTasks{},
		CredentialProviders: map[string]credential.Provider{},
	}
	c.Reconciler = reconcile.New(reconcile.Deps{
		Bots:           s.Bots,
		ReportingLines: s.ReportingLines,
		Topics:         s.Topics,
		Subscriptions:  s.Subscriptions,
		Now:            c.Now,
	})
	c.Queries = queries.New(queries.Deps{
		Bots: s.Bots, ReportingLines: s.ReportingLines,
		Topics: s.Topics, Subscriptions: s.Subscriptions, Events: s.Events,
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
		&CreateBot{deps: deps},
		&UpdateBot{deps: deps},
		&CreateTopic{deps: deps},
		&MintCredential{deps: deps, providers: deps.CredentialProviders},
		&TopicMembers{deps: deps},
		&Subscribe{deps: deps},
		&Unsubscribe{deps: deps},
		&InviteBots{deps: deps},
		&Publish{deps: deps},
		&DM{deps: deps},
		&ConfigureBotProject{deps: deps},
		// Spec-task management — a Bot managing the spec tasks in its own
		// Helix project. Granted per-Role (not in BaseReadTools).
		NewCreateSpecTask(deps),
		NewStartSpecTaskPlanning(deps),
		NewApproveSpecTaskSpec(deps),
		NewRequestSpecTaskChanges(deps),
		NewCreateSpecTaskPRs(deps),
		// Reads. Each is a thin wrapper around a store call; together they
		// replace the jsonapi GET handlers the server used to expose.
		&ListBots{deps: deps},
		&GetBot{deps: deps},
		&Managers{deps: deps},
		&Reports{deps: deps},
		&GetBotProject{deps: deps},
		NewListSpecTasks(deps),
		NewGetSpecTask(deps),
		NewReviewSpecTaskSpec(deps),
		&ListTopics{deps: deps},
		&GetTopic{deps: deps},
		&ListTopicEvents{deps: deps},
		&ReadEvents{deps: deps},
		&BotLog{deps: deps},
	}
	for _, tool := range builtins {
		if err := reg.Register(tool); err != nil {
			return fmt.Errorf("register %q: %w", tool.Name(), err)
		}
	}
	// Fail fast if BaseReadTools references a name that isn't registered
	// — a typo in defaults.go would otherwise produce silently-broken
	// Bots whose reconciled tool list is missing one of the baseline
	// entries.
	for _, name := range BaseReadTools {
		if _, err := reg.Get(name); err != nil {
			return fmt.Errorf("BaseReadTools references unregistered tool %q: %w", name, err)
		}
	}
	return nil
}
