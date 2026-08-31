package runtime

import (
	"context"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
)

// ProjectPrincipal is a non-Bot caller that already knows which project it
// acts in: a spec task's own coding agent, authenticated by its session-scoped
// api key. A Bot resolves its project from per-Worker runtime state; a spec
// task has none, so the resolved project and acting user ride the context
// instead — the same way this runtime already carries caller identity
// (see helix.WithHelixIdentity).
//
// Everything downstream still enforces org scope; the principal only answers
// "which project" and "attributed to whom", never "may I".
type ProjectPrincipal struct {
	ProjectID    string
	ActingUserID string
}

type projectPrincipalKey struct{}

// WithProjectPrincipal stashes p on ctx. A principal with no project is a
// no-op so callers can pass a lookup result without gating on it.
func WithProjectPrincipal(ctx context.Context, p ProjectPrincipal) context.Context {
	if p.ProjectID == "" {
		return ctx
	}
	return context.WithValue(ctx, projectPrincipalKey{}, p)
}

// ProjectPrincipalFromContext returns the stashed principal, if any.
func ProjectPrincipalFromContext(ctx context.Context) (ProjectPrincipal, bool) {
	p, ok := ctx.Value(projectPrincipalKey{}).(ProjectPrincipal)
	return p, ok
}

// boundWorkerKey carries the org Agent the calling principal's project is
// bound to — the live bond between a Helix project and the Agent whose
// runtime state names it as home project. The spec-task MCP backend
// resolves it once per request. Tools whose semantics are "as the calling
// Agent" (who posts to streams, whose reporting line reads managers, whose
// granted secrets get_secret returns) consult it instead of reading caller
// identity, while audit attribution deliberately stays on the caller (the
// task).
type boundWorkerKey struct{}

// WithBoundWorker stashes the bond on ctx. An empty id is a no-op so
// un-bonded requests stay indistinguishable from Bot requests.
func WithBoundWorker(ctx context.Context, nodeID orgchart.NodeID) context.Context {
	if nodeID == "" {
		return ctx
	}
	return context.WithValue(ctx, boundWorkerKey{}, nodeID)
}

// BoundWorkerFromContext returns the bound Agent, if the request carries one.
func BoundWorkerFromContext(ctx context.Context) (orgchart.NodeID, bool) {
	id, ok := ctx.Value(boundWorkerKey{}).(orgchart.NodeID)
	return id, ok && id != ""
}
