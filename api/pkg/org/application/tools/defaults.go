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
