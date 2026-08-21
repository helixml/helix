package mcptools

import (
	"github.com/helixml/helix/api/pkg/org/application/nodes"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

// BaseReadTools is the set of MCP tools that every Bot must expose. The
// principle: a tool belongs here only if exposing it to every Bot is
// safe (it cannot mutate the org graph) and useful (the Bot needs it to
// introspect its reporting line, look up peers, or read topics it has
// been subscribed to).
//
// `create_bot` unions this list into the caller-supplied tools so that
// new Nodes can never miss the baseline. The bots Reconcile backfill
// unions this list into every existing Bot's tools so that pre-existing
// Nodes get backfilled at API server start.
//
// Order matters: it is preserved when appending to a Bot's tool list, so
// the reconciled output is deterministic.
//
// The baseline also includes two safe actions: get_secret obtains an
// org-scoped external-provider credential, and ask_human contacts a human
// node through their configured route. Neither mutates the org graph. Without
// get_secret,
// a Bot has nothing to authenticate gh/git/auth-curl with — there is no
// boot-time env-var fallback. Every Bot needs this, so it sits in the
// baseline.
var BaseReadTools = []tool.Name{
	ManagersName,
	ReportsName,
	ListBotsName,
	GetBotName,
	ListTopicsName,
	GetTopicName,
	ListTopicEventsName,
	ReadEventsName,
	BotLogName,
	GetSecretName,
	AskHumanName,
	// Every bot can discover metadata for credentials explicitly granted to it.
	// Values remain behind the separately audited get_secret call.
	ListSecretsName,
	// Processor introspection — safe reads so any bot can discover the
	// transform/filter/js nodes feeding topics it may subscribe to.
	ListProcessorsName,
	GetProcessorName,
}

// AssetManagementTools is the org-wide asset inventory and mutation surface.
// It is granted to the owner Bot, not to every Bot. Link-derived operational
// asset tools live in assets.ServerTools and remain a separate capability.
var AssetManagementTools = []tool.Name{
	ListOrgAssetsName,
	GetOrgAssetName,
	CreateServerAssetName,
	UpdateServerAssetName,
	DeleteAssetName,
	ListAssetLinksName,
	LinkAssetName,
	UnlinkAssetName,
	GetAssetHealthName,
}

// OwnerBotTools is the canonical tool set the bootstrap owner Bot
// (Chief of Staff) receives: every mutation in the system plus the
// universal base read set (via MergeBaseReadTools). It lives here —
// beside the tool name constants and BaseReadTools — so the owner-seed
// policy references the typed names directly and bootstrap can be handed
// the list without importing this package. get_secret arrives
// through BaseReadTools, so it is not repeated in the mutation list.
//
// Repository tools (list/attach/detach) are here so CoS can equip the
// bots it creates with the codebases they need to do real work.
func OwnerBotTools() []tool.Name {
	ownerMutations := []tool.Name{
		CreateBotName,
		SetBotContentName,
		AttachToolName,
		DetachToolName,
		DeleteBotName,
		CreateTopicName,
		TopicMembersName,
		SubscribeName,
		UnsubscribeName,
		PublishName,
		DMName,
		SetHumanContactName,
		// Processors: define topic transforms/filters/js and rewire them.
		CreateProcessorName,
		UpdateProcessorName,
		DeleteProcessorName,
		// Git repositories: discover org repos and wire them onto bot projects.
		ListRepositoriesName,
		ListBotRepositoriesName,
		AttachRepositoryName,
		DetachRepositoryName,
		// Agent desktop lifecycle (same as chart Start / Stop / Restart).
		StartBotName,
		StopBotName,
		RestartBotName,
		// Standalone Sandboxes API lifecycle for the organization.
		ListSandboxRuntimesName,
		ListSandboxesName,
		GetSandboxName,
		CreateSandboxName,
		UpdateSandboxName,
		DeleteSandboxName,
		SandboxSSHAccessName,
	}
	// Org assets: create/configure inventory and grant/revoke Bot access.
	ownerMutations = append(ownerMutations, AssetManagementTools...)
	return MergeBaseReadTools(ownerMutations)
}

// MergeBaseReadTools returns the union of `existing` and BaseReadTools.
// The order of `existing` is preserved; any BaseReadTools entries not
// already present are appended in BaseReadTools order. Duplicates within
// `existing` are also dropped, so the result is fully deduped.
//
// Used by every entry point that creates a Bot — the MCP create_bot
// tool, the REST POST /orgs/{org}/bots handler, and the bots Reconcile
// backfill. Keeping the merge in one place ensures all paths agree on
// order and dedup semantics, and adding a new entry point only requires
// a single call to this helper.
func MergeBaseReadTools(existing []tool.Name) []tool.Name {
	return nodes.MergeTools(existing, BaseReadTools)
}
