package mcptools

import (
	"github.com/helixml/helix/api/pkg/org/application/roles"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

// BaseReadTools is the recommended baseline set of MCP tools for Roles.
// The principle: a tool belongs here only if exposing it to every Worker is
// safe (it cannot mutate the org graph) and useful (the Worker needs it
// to introspect its reporting line, look up peers, or read streams it
// has been subscribed to).
//
// New Roles are created with an empty tools list; operators add tools
// explicitly via the role detail page. MergeBaseReadTools is available
// for callers that want to seed a Role with this baseline (e.g. the
// owner-role bootstrap).
//
// Order matters: it is preserved when appending to a Role's tool list,
// so the merged output is deterministic.
//
// `mint_credential` is the sole non-read entry. It mints an external-
// provider credential (it does not mutate the org graph), and without
// it a Worker has nothing to authenticate gh/git/auth-curl with — there
// is no boot-time env-var fallback.
var BaseReadTools = []tool.Name{
	ManagersName,
	ReportsName,
	ListWorkersName,
	GetWorkerName,
	ListRolesName,
	GetRoleName,
	ListStreamsName,
	GetStreamName,
	ListStreamEventsName,
	ReadEventsName,
	WorkerLogName,
	MintCredentialName,
}

// OwnerRoleTools is the canonical tool set the bootstrap owner Role
// receives: every mutation in the system plus the universal base read
// set (via MergeBaseReadTools). It lives here — beside the tool name
// constants and BaseReadTools — so the owner-seed policy references the
// typed names directly and bootstrap (application) can be handed the
// list without importing this package. mint_credential arrives through
// BaseReadTools, so it is not repeated in the mutation list.
func OwnerRoleTools() []tool.Name {
	ownerMutations := []tool.Name{
		CreateRoleName,
		UpdateRoleName,
		UpdateIdentityName,
		HireWorkerName,
		CreateStreamName,
		StreamMembersName,
		SubscribeName,
		UnsubscribeName,
		InviteWorkersName,
		PublishName,
		DMName,
	}
	return MergeBaseReadTools(ownerMutations)
}

// MergeBaseReadTools returns the union of `existing` and BaseReadTools.
// The order of `existing` is preserved; any BaseReadTools entries not
// already present are appended in BaseReadTools order. Duplicates within
// `existing` are also dropped, so the result is fully deduped.
//
// Used by callers that want to seed a Role with the full baseline — e.g.
// the owner-role bootstrap and the RoleReconciler. New Roles created via
// the REST API or MCP create_role tool start with an empty tool list and
// are not passed through this helper.
func MergeBaseReadTools(existing []tool.Name) []tool.Name {
	return roles.MergeTools(existing, BaseReadTools)
}
