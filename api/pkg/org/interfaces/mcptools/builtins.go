package mcptools

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/helixml/helix/api/pkg/org/application/activations"
	"github.com/helixml/helix/api/pkg/org/application/assets"
	"github.com/helixml/helix/api/pkg/org/application/attachments"
	"github.com/helixml/helix/api/pkg/org/application/lifecycle"
	"github.com/helixml/helix/api/pkg/org/application/nodes"
	"github.com/helixml/helix/api/pkg/org/application/processors"
	"github.com/helixml/helix/api/pkg/org/application/projects"
	"github.com/helixml/helix/api/pkg/org/application/publishing"
	"github.com/helixml/helix/api/pkg/org/application/queries"
	"github.com/helixml/helix/api/pkg/org/application/reconcile"
	orgsandboxes "github.com/helixml/helix/api/pkg/org/application/sandboxes"
	"github.com/helixml/helix/api/pkg/org/application/spectasks"
	"github.com/helixml/helix/api/pkg/org/application/triggers"
	"github.com/helixml/helix/api/pkg/org/application/workersecrets"
	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/eventsource"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	"github.com/helixml/helix/api/pkg/org/infrastructure/assetssh"
	"github.com/helixml/helix/api/pkg/org/infrastructure/runtime"
	"github.com/helixml/helix/api/pkg/org/infrastructure/wakebus"
)

// Clock returns the current time. Tests override it.
type Clock func() time.Time

// IDGen generates new unique string IDs. Tests override it.
type IDGen func() string

type AgentContentUpdater interface {
	UpdateAgentContent(ctx context.Context, appID, content string) error
}

type AgentProfileReader interface {
	AgentProfile(ctx context.Context, appID string) (name, instructions string, err error)
}

// EventDispatcher fans a freshly-published event out to every attached
// Bot as a separate Spawner activation. The interface keeps tools.Deps
// free of a dependency on the dispatch package (avoiding an import
// cycle: the dispatcher itself imports tools).
type EventDispatcher interface {
	Route(ctx context.Context, e eventsource.Event) error
	// DispatchHire fires a create activation. activationID is the
	// pre-allocated audit-row ID create_bot created before calling
	// DispatchHire — it travels through the trigger so the Spawner reuses
	// the existing row instead of writing a sibling. Empty activationID
	// is allowed for callers that don't pre-allocate (legacy code paths,
	// tests that don't wire activation.Repository).
	DispatchHire(ctx context.Context, orgID string, botID orgchart.NodeID, activationID activation.ID)
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
	// Nodes is the bot-mutation service (the merge of the former roles +
	// workers services) — set_bot_content and attach_tool/detach_tool
	// delegate here; create_bot goes through Lifecycle, which itself drives
	// Nodes.
	Nodes *nodes.Nodes
	// Triggers owns Trigger create/update/delete plus inbound-hook
	// provisioning — the same service the REST /triggers handlers drive.
	Triggers *triggers.Service
	// Attachments owns attach/detach of a Worker to a source. Same
	// service as the REST attachment endpoints.
	Attachments *attachments.Service
	Publishing  *publishing.Publishing
	// Lifecycle owns Create (the MCP create_bot tool delegates here, the
	// same service the REST POST /bots handler drives).
	Lifecycle *lifecycle.Service
	// Activations owns start/stop/restart of a Bot's agent desktop
	// (start_bot / stop_bot / restart_bot). Same service as the REST
	// activate / stop-agent / restart-agent endpoints.
	Activations *activations.Activations
	// Processors owns create/update/delete/list of Processors
	// (template, truncate, filter, js). Same service as the REST
	// /processors handlers. nil → processor tools report "not wired".
	Processors           *processors.Processors
	Assets               *assets.Service
	AssetSSH             ServerAssetRuntime
	AssetSSHIssuer       AssetSSHIdentityIssuer
	SandboxSSHIssuer     SandboxSSHIdentityIssuer
	AssetSSHProxyAddress string
	AssetHealth          func(ctx context.Context, orgID, assetRef string) assetssh.Health

	AgentContentUpdater AgentContentUpdater
	AgentProfileReader  AgentProfileReader
	// ProjectConfig backs get_bot_project + configure_bot_project
	// (owner-only read/patch of a Bot's helix project config).
	ProjectConfig runtime.ProjectConfig
	// SpecTasks is the front-of-house application service backing the
	// spec-task tools (CRUD, agent chat/lifecycle, review/approval, and PR
	// creation) scoped to the calling Worker's permitted projects.
	SpecTasks *spectasks.Service
	// Projects is the front-of-house application service backing the
	// project-discovery tools (list_projects/get_project), scoped to the
	// caller's org.
	Projects *projects.Service
	// Sandboxes is the front-of-house application service backing standalone
	// sandbox discovery and CRUD, scoped to the caller's org.
	Sandboxes *orgsandboxes.Service
	// Repositories backs list_repositories / list_bot_repositories /
	// attach_repository / detach_repository — org git repos attached to
	// Bot projects so sandboxes can clone the code.
	Repositories  runtime.Repositories
	WorkerSecrets *workersecrets.Service
	// Hub lets the long-poll read tools (read_events, bot_log) block on
	// new events. It is a broadcaster, not a store.
	Hub *wakebus.Bus

	// ToolNames returns the catalogue of registered tool names, used by
	// create_bot/attach_tool/detach_tool to build their `tools` enum
	// dynamically so new tools appear automatically. Wired by
	// RegisterBuiltins to read the live registry; nil → the tools fall
	// back to an unconstrained string array (still valid, just no enum).
	ToolNames func() []tool.Name

	HumanDelivery HumanDelivery
}

// Config carries the construction seams the composition root supplies to
// assemble the tool Deps: the store + clock/id-gen + reconciler + the
// runtime collaborators. Build() turns it into a Deps. This is the only
// place store repositories are read — a composition convenience (the
// same shape as server.NewFromStore), never reached from a tool.
//
// Hub/Dispatcher are optional (nil → publish skips notify/dispatch).
type Config struct {
	Store                   *store.Store
	Queries                 *queries.Queries
	Now                     Clock
	NewID                   IDGen
	Hub                     *wakebus.Bus
	Dispatcher              EventDispatcher
	AgentContentUpdater     AgentContentUpdater
	AgentProfileReader      AgentProfileReader
	ToolChangeNotifier      func(context.Context, string)
	RestartRequiredNotifier func(context.Context, string, orgchart.NodeID)
	// KnownTools reports the live tool catalogue so the bots service can
	// prune persisted names the registry no longer knows. nil disables
	// pruning.
	KnownTools    func() map[tool.Name]bool
	HireHook      runtime.HireHook
	ProjectConfig runtime.ProjectConfig
	WorkerSecrets *workersecrets.Service
	// SpecTasks is the runtime port the spec-task tools dispatch on. nil
	// → Build defaults to runtime.NoopSpecTasks{} so the tools return a
	// clear "not wired" error instead of nil-derefing.
	SpecTasks runtime.SpecTasks
	// Projects is the runtime port the project-discovery tools dispatch on.
	// nil → Build defaults to runtime.NoopProjects{} so the tools return a
	// clear "not wired" error instead of nil-derefing.
	Projects runtime.Projects
	// Sandboxes is the runtime port for standalone org sandbox management.
	// nil → Build defaults to runtime.NoopSandboxes{}.
	Sandboxes runtime.Sandboxes
	// ProjectAccess resolves the caller Bot's own runtime project so project
	// discovery can combine it with the explicit per-Bot allowlist.
	ProjectAccess projects.OwnProjectResolver
	// Repositories is the runtime port for org git-repo list/attach/detach.
	// nil → Build defaults to runtime.NoopRepositories{}.
	Repositories runtime.Repositories
	Reconciler   *reconcile.Reconciler
	// Lifecycle, when set, is used verbatim instead of building a fresh one,
	// so the MCP tools share the composition root's reconciler-complete
	// service. nil → lifecycleService() builds a standalone service.
	Lifecycle *lifecycle.Service
	// Activations, when set, is used by start_bot / stop_bot / restart_bot.
	// Built at the composition root (needs project ensurer + stop/reset
	// ports). nil → those tools report "not wired".
	Activations *activations.Activations
	// Processors, when set, is used by create/list/get/update/delete
	// processor tools. nil → Build() constructs one from Store when
	// possible.
	Processors *processors.Processors
	// Triggers, when set, is used verbatim so the MCP surface shares the
	// composition root's provisioner-complete service. nil → Build()
	// constructs one from Store.
	Triggers             *triggers.Service
	Attachments          *attachments.Service
	Assets               *assets.Service
	AssetSSH             ServerAssetRuntime
	AssetSSHIssuer       AssetSSHIdentityIssuer
	SandboxSSHIssuer     SandboxSSHIdentityIssuer
	AssetSSHProxyAddress string
	AssetHealth          func(ctx context.Context, orgID, assetRef string) assetssh.Health
	Publishing           *publishing.Publishing
	// HumanDelivery sends ask_human messages through the person's configured route.
	HumanDelivery HumanDelivery
}

// Build assembles the application services from the config and returns
// the lean tool Deps. Reads from the store happen only here.
func (c Config) Build() Deps {
	return Deps{
		Queries:              c.Queries,
		Nodes:                c.botsService(),
		Triggers:             c.triggersService(),
		Attachments:          c.attachmentsService(),
		Publishing:           c.Publishing,
		Lifecycle:            c.lifecycleService(),
		Activations:          c.Activations,
		Processors:           c.processorsService(),
		Assets:               c.Assets,
		AssetSSH:             c.AssetSSH,
		AssetSSHIssuer:       c.AssetSSHIssuer,
		SandboxSSHIssuer:     c.SandboxSSHIssuer,
		AssetSSHProxyAddress: c.AssetSSHProxyAddress,
		AssetHealth:          c.AssetHealth,
		AgentContentUpdater:  c.AgentContentUpdater,
		AgentProfileReader:   c.AgentProfileReader,
		ProjectConfig:        c.ProjectConfig,
		WorkerSecrets:        c.WorkerSecrets,
		SpecTasks:            c.specTasksService(),
		Projects:             c.projectsService(),
		Sandboxes:            c.sandboxesService(),
		Repositories:         c.repositoriesPort(),
		Hub:                  c.Hub,
		HumanDelivery:        c.HumanDelivery,
	}
}

func (c Config) sandboxesService() *orgsandboxes.Service {
	port := c.Sandboxes
	if port == nil {
		port = runtime.NoopSandboxes{}
	}
	var members orgsandboxes.MemberVerifier
	if c.Queries != nil {
		members = c.Queries
	}
	return orgsandboxes.New(port, members)
}

// processorsService returns the pre-built Processors service when the
// composition root supplied one; otherwise builds a standalone service
// over the store (tests / DefaultDeps).
func (c Config) processorsService() *processors.Processors {
	if c.Processors != nil {
		return c.Processors
	}
	if c.Store == nil {
		return nil
	}
	return processors.New(processors.Deps{
		Processors:  c.Store.Processors,
		Triggers:    c.Store.Triggers,
		Attachments: c.Store.WorkerAttachments,
		Now:         c.Now,
		NewID:       c.NewID,
	})
}

// repositoriesPort returns the configured Repositories port, defaulting to
// NoopRepositories when none is wired.
func (c Config) repositoriesPort() runtime.Repositories {
	if c.Repositories == nil {
		return runtime.NoopRepositories{}
	}
	return c.Repositories
}

// projectsService builds the project-discovery application service over the
// configured runtime port, defaulting to NoopProjects when none is wired.
// Queries satisfies projects.MemberVerifier (GetBot); pass it so every
// project read verifies the caller Bot is a member of its org.
func (c Config) projectsService() *projects.Service {
	port := c.Projects
	if port == nil {
		port = runtime.NoopProjects{}
	}
	var members projects.MemberVerifier
	if c.Queries != nil {
		members = c.Queries
	}
	return projects.New(port, members, c.ProjectAccess)
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
	// Queries satisfies spectasks.MemberVerifier (GetBot); pass it so every
	// spec-task call verifies the caller Bot is a member of its org. nil is
	// tolerated (the check is then skipped — the MCP mount already enforces it).
	var members spectasks.MemberVerifier
	if c.Queries != nil {
		members = c.Queries
	}
	return spectasks.New(port, members)
}

// attachmentsService returns the pre-built attachment service when the
// composition root supplied one; otherwise builds one over the store.
func (c Config) attachmentsService() *attachments.Service {
	if c.Attachments != nil {
		return c.Attachments
	}
	if c.Store == nil {
		return nil
	}
	return attachments.New(attachments.Deps{Store: c.Store, Now: c.Now, NewID: c.NewID})
}

// lifecycleService builds the bot-lifecycle service (Create/Delete) for
// the MCP surface. The create semantics (reporting line, topology
// reconcile, create dispatch) live in exactly one place — shared with
// the REST POST /bots handler. The bots service is wired so the row
// creation applies the base-read-tool union.
func (c Config) lifecycleService() *lifecycle.Service {
	if c.Lifecycle != nil {
		return c.Lifecycle
	}
	svc := &lifecycle.Service{
		Store:           c.Store,
		Nodes:           c.botsService(),
		Attacher:        c.attachmentsService(),
		NodeReconcilers: []lifecycle.NodeReconciler{c.Reconciler},
		HireHook:        c.HireHook,
		Now:             c.Now,
		NewID:           c.NewID,
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
func (c Config) botsService() *nodes.Nodes {
	return nodes.New(nodes.Deps{
		Nodes:             c.Store.Nodes,
		Lines:             c.Store.ReportingLines,
		Reconciler:        c.Reconciler,
		Now:               c.Now,
		NewID:             c.NewID,
		BaseTools:         BaseReadTools,
		KnownTools:        c.KnownTools,
		OnToolsChanged:    c.ToolChangeNotifier,
		OnRestartRequired: c.RestartRequiredNotifier,
	})
}

// triggersService returns the pre-built Trigger service when the
// composition root supplied one (with its inbound provisioners);
// otherwise builds a provisioner-less one over the store.
func (c Config) triggersService() *triggers.Service {
	if c.Triggers != nil {
		return c.Triggers
	}
	if c.Store == nil {
		return nil
	}
	return triggers.New(triggers.Deps{
		Triggers:    c.Store.Triggers,
		Attachments: c.Store.WorkerAttachments,
		Events:      c.Store.Events,
		Now:         c.Now,
		NewID:       c.NewID,
	})
}

// DefaultDeps wires production defaults into a Config: real UUIDs,
// wall-clock time, and the Queries facade + Reconciler built off the store.
// Hub and Dispatcher are left zero — composition callers wire them in before
// calling Build().
func DefaultDeps(s *store.Store) Config {
	c := Config{
		Store:         s,
		Now:           func() time.Time { return time.Now().UTC() },
		NewID:         uuid.NewString,
		HireHook:      runtime.NoopHireHook{},
		ProjectConfig: runtime.NoopProjectConfig{},
		SpecTasks:     runtime.NoopSpecTasks{},
		Projects:      runtime.NoopProjects{},
		Sandboxes:     runtime.NoopSandboxes{},
		Repositories:  runtime.NoopRepositories{},
	}
	c.Reconciler = reconcile.New(reconcile.Deps{
		Nodes:          s.Nodes,
		ReportingLines: s.ReportingLines,
		Triggers:       s.Triggers,
		Attachments:    s.WorkerAttachments,
		Now:            c.Now,
	})
	c.Queries = queries.New(queries.Deps{
		Nodes: s.Nodes, ReportingLines: s.ReportingLines,
		Triggers: s.Triggers, Attachments: s.WorkerAttachments,
		Processors: s.Processors, Events: s.Events,
		Activations: s.Activations,
	})
	return c
}

// RegisterBuiltins registers every built-in tool on the registry —
// mutations on the org graph plus the matching read tools. Test tools
// (like Ping) are not included.
func RegisterBuiltins(reg *Registry, deps Deps) error {
	if deps.Publishing == nil {
		return fmt.Errorf("tools.RegisterBuiltins: deps.Publishing is required")
	}
	// Wire the tool-name catalogue from the live registry so the
	// create_bot/attach_tool/detach_tool `tools` enums always reflect the
	// registered set. The closure reads reg lazily (at InputSchema time),
	// so it sees every tool registered below regardless of order.
	if deps.ToolNames == nil {
		deps.ToolNames = func() []tool.Name {
			all := reg.List()
			names := make([]tool.Name, len(all))
			for i, t := range all {
				names[i] = t.Name()
			}
			return names
		}
	}
	builtins := []tool.Tool{
		// Mutations.
		&CreateBot{deps: deps},
		&SetBotContent{deps: deps},
		&AttachTool{deps: deps},
		&DetachTool{deps: deps},
		&DeleteBot{deps: deps},
		&CreateTrigger{deps: deps},
		&GetSecret{deps: deps},
		&TriggerMembers{deps: deps},
		&AttachWorker{deps: deps},
		&DetachWorker{deps: deps},
		&Chat{deps: deps},
		&DM{deps: deps},
		&AskHuman{deps: deps},
		&SetHumanContact{deps: deps},
		&ConfigureBotProject{deps: deps},
		// Processors — topic transforms/filters/js. Mutations are
		// OwnerBotTools; list/get are BaseReadTools.
		&CreateProcessor{deps: deps},
		&UpdateProcessor{deps: deps},
		&DeleteProcessor{deps: deps},
		// Agent desktop lifecycle — same as REST activate / stop-agent /
		// restart-agent. OwnerBotTools grants these to Chief of Staff.
		NewStartBot(deps),
		NewStopBot(deps),
		NewRestartBot(deps),
		// Spec-task management — a Bot managing tasks in its permitted Helix
		// projects. Granted per-Role (not in BaseReadTools).
		NewCreateSpecTask(deps),
		NewUpdateSpecTask(deps),
		NewStartSpecTaskPlanning(deps),
		NewSendSpecTaskAgentMessage(deps),
		NewListSpecTaskAgentMessages(deps),
		NewStartSpecTaskAgent(deps),
		NewStopSpecTaskAgent(deps),
		NewRestartSpecTaskAgent(deps),
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
		&ListSecrets{deps: deps},
		// Project discovery — an org-wide PM Bot lists/reads the projects in
		// its org before deciding which to manage. Org-scoped reads; granted
		// per-Role (not in BaseReadTools).
		NewListProjects(deps),
		NewGetProject(deps),
		// Standalone org sandboxes — discovery and CRUD. Chief of Staff gets
		// these through OwnerBotTools; other Bots may receive them explicitly.
		NewListSandboxRuntimes(deps),
		NewListSandboxes(deps),
		NewGetSandbox(deps),
		NewCreateSandbox(deps),
		NewUpdateSandbox(deps),
		NewDeleteSandbox(deps),
		NewSandboxSSHAccess(deps),
		// Git repositories — list org repos and attach/detach them on Bot
		// projects so sandboxes clone the code. OwnerBotTools grants these
		// to Chief of Staff by default.
		NewListRepositories(deps),
		NewListBotRepositories(deps),
		NewAttachRepository(deps),
		NewDetachRepository(deps),
		NewListSpecTasks(deps),
		NewGetSpecTask(deps),
		NewReviewSpecTaskSpec(deps),
		&ListTriggers{deps: deps},
		&GetTrigger{deps: deps},
		&ListTriggerEvents{deps: deps},
		&ReadEvents{deps: deps},
		&BotLog{deps: deps},
		&ListProcessors{deps: deps},
		&GetProcessor{deps: deps},
		&ListOrgAssets{deps: deps},
		&GetOrgAsset{deps: deps},
		&CreateServerAsset{deps: deps},
		&UpdateServerAsset{deps: deps},
		&DeleteAsset{deps: deps},
		&ListAssetLinks{deps: deps},
		&LinkAsset{deps: deps},
		&UnlinkAsset{deps: deps},
		&GetAssetHealth{deps: deps},
		&ListAssets{deps: deps},
		&GetAsset{deps: deps},
		&ServerRunCommand{deps: deps},
		&ServerListCommands{deps: deps},
		&ServerGetCommand{deps: deps},
		&ServerKillCommand{deps: deps},
		&ServerListFiles{deps: deps},
		&ServerReadFile{deps: deps},
		&ServerWriteFile{deps: deps},
		&ServerSSHAccess{deps: deps},
	}
	for _, tool := range builtins {
		if err := reg.Register(tool); err != nil {
			return fmt.Errorf("register %q: %w", tool.Name(), err)
		}
	}
	// Fail fast if BaseReadTools references a name that isn't registered
	// — a typo in defaults.go would otherwise produce silently-broken
	// Nodes whose reconciled tool list is missing one of the baseline
	// entries.
	for _, name := range BaseReadTools {
		if _, err := reg.Get(name); err != nil {
			return fmt.Errorf("BaseReadTools references unregistered tool %q: %w", name, err)
		}
	}
	return nil
}
