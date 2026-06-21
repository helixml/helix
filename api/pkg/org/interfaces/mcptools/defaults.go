package mcptools

import (
	"github.com/helixml/helix/api/pkg/org/application/roles"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

// BaseReadTools is the catalogue of read-only MCP tools available for
// operators to assign to Roles. A tool belongs here only if exposing it
// to a Worker is safe (it cannot mutate the org graph) and useful (the
// Worker needs it to introspect its reporting line, look up peers, or
// read streams).
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
// Used by OwnerRoleTools to build the owner bootstrap role's full tool set.
func MergeBaseReadTools(existing []tool.Name) []tool.Name {
	return roles.MergeTools(existing, BaseReadTools)
}
