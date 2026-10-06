package runtime

import "context"

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
