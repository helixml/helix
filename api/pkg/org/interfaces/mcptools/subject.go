package mcptools

import (
	"context"
	"errors"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	"github.com/helixml/helix/api/pkg/org/infrastructure/runtime"
)

// ErrNoBoundWorker is returned when a spec-task caller asks for the
// Agent-derived authority of a project with no bound Agent: the bond is
// gone (Agent fired, project re-homed) or never existed. Loud, never
// silent — a half-authorized delegated call is worse than none.
var ErrNoBoundWorker = errors.New("this task's project is not bound to a helix-org agent; the agent's identity, tools, and granted credentials are unavailable to it")

// SubjectForCaller answers "whose identity does this tool act as?" for
// every org tool reachable to a delegated (spec-task) caller. Two callers
// share one registry:
//
//   - Bot caller (no project principal on ctx): acts as itself — the
//     historical behavior, unchanged.
//   - Spec-task caller (ProjectPrincipal on ctx, set by the spec-task MCP
//     backend): acts on behalf of the Agent its project is bound to
//     (BoundWorkerFromContext), so streams read as the Agent posts,
//     managers/reports walk its line, and granted secrets resolve from its
//     bindings. The task cannot supply an id from args; the bond is the
//     only source.
//   - Spec-task caller with no bond: ErrNoBoundWorker — the blast radius of
//     delegation must never exceed "what the bound Agent could do".
//
// Every tool is classified against this decision (see the spec-task tool
// audit): (a) on-behalf-of — resolve caller-identity semantics through this
// helper; (b) task-scoped — key on caller.ID(), which is already the task;
// (c) incompatible with delegation — fail with ErrNoBoundWorker (or an
// equivalent clean error) and zero side effects. Audit attribution is
// separate: the org audit log and the worker-secret recorder keep logging
// the raw caller (the task id), not the subject.
func SubjectForCaller(ctx context.Context, caller tool.Caller) (orgchart.NodeID, error) {
	if _, ok := runtime.ProjectPrincipalFromContext(ctx); !ok {
		// Bot path. A missing identity is a wiring bug; fail closed.
		if caller == nil || caller.ID() == "" {
			return "", errors.New("caller identity is missing")
		}
		return orgchart.NodeID(caller.ID()), nil
	}
	if bound, ok := runtime.BoundWorkerFromContext(ctx); ok {
		return bound, nil
	}
	return "", ErrNoBoundWorker
}
