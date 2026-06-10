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
}

// mergeBaseReadTools returns the union of `existing` and BaseReadTools.
// The order of `existing` is preserved; any BaseReadTools entries not
// already present are appended in BaseReadTools order. Duplicates within
// `existing` are also dropped, so the result is fully deduped.
//
// Used by create_role at hire time and by RoleReconciler at startup —
// keeping the merge in one place ensures both paths agree on order and
// dedup semantics.
func mergeBaseReadTools(existing []tool.Name) []tool.Name {
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
