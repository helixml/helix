package tools

import "github.com/helixml/helix/api/pkg/org/domain/tool"

// BaseReadTools is the set of MCP tools that every Role must expose. The
// principle: a tool belongs here only if exposing it to every Worker is
// safe (it cannot mutate the org graph) and useful (the Worker needs it
// to introspect its reporting line, look up peers, or read streams it
// has been subscribed to).
//
// `create_role` unions this list into the caller-supplied tools so that
// new Roles can never miss the baseline. RoleReconciler unions this list
// into every existing Role's tools so that pre-existing Roles get
// backfilled at API server start.
//
// Order matters: it is preserved when appending to a Role's tool list,
// so the reconciled output is deterministic.
//
// `mint_credential` is the sole non-read entry. It mints an external-
// provider credential (it does not mutate the org graph), and without
// it a Worker has nothing to authenticate gh/git/auth-curl with — there
// is no boot-time env-var fallback. Every Worker needs this, so it sits
// in the baseline.
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
	GetWorkerEnvironmentName,
	MintCredentialName,
}

// MergeBaseReadTools returns the union of `existing` and BaseReadTools.
// The order of `existing` is preserved; any BaseReadTools entries not
// already present are appended in BaseReadTools order. Duplicates within
// `existing` are also dropped, so the result is fully deduped.
//
// Used by every entry point that creates a Role — the MCP create_role
// tool, the REST POST /orgs/{org}/roles handler, and the RoleReconciler.
// Keeping the merge in one place ensures all paths agree on order and
// dedup semantics, and adding a new entry point only requires a single
// call to this helper.
func MergeBaseReadTools(existing []tool.Name) []tool.Name {
	seen := make(map[tool.Name]struct{}, len(existing)+len(BaseReadTools))
	out := make([]tool.Name, 0, len(existing)+len(BaseReadTools))
	for _, name := range existing {
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	for _, name := range BaseReadTools {
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out
}
