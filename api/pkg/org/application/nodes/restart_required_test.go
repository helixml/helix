package nodes_test

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/org/application/nodes"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	"github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
	"github.com/stretchr/testify/require"
)

// restartSpy records every (orgID, nodeID) the service reports as needing
// a sandbox restart.
type restartSpy struct{ calls []orgchart.NodeID }

func (r *restartSpy) hook() func(context.Context, string, orgchart.NodeID) {
	return func(_ context.Context, _ string, id orgchart.NodeID) {
		r.calls = append(r.calls, id)
	}
}

func restartSvc(st *store.Store, spy *restartSpy) *nodes.Nodes {
	return nodes.New(nodes.Deps{
		Nodes:             st.Nodes,
		Now:               at,
		BaseTools:         []tool.Name{"chat"},
		KnownTools:        func() map[tool.Name]bool { return live },
		OnRestartRequired: spy.hook(),
	})
}

func ptrTools(t []tool.Name) *[]tool.Name { return &t }
func ptrStr(s string) *string             { return &s }
func ptrBool(b bool) *bool                { return &b }

func TestUpdate_FiresRestartRequiredOnToolChange(t *testing.T) {
	st := memory.New()
	seed(t, st, "b-one", []tool.Name{"chat"})
	spy := &restartSpy{}

	_, err := restartSvc(st, spy).Update(context.Background(), org, "b-one", nodes.UpdateParams{
		Tools: ptrTools([]tool.Name{"chat", "get_secret"}),
	})
	require.NoError(t, err)

	require.Equal(t, []orgchart.NodeID{"b-one"}, spy.calls)
}

func TestUpdate_FiresRestartRequiredOnContentChange(t *testing.T) {
	st := memory.New()
	seed(t, st, "b-two", []tool.Name{"chat"})
	spy := &restartSpy{}

	_, err := restartSvc(st, spy).Update(context.Background(), org, "b-two", nodes.UpdateParams{
		Content: ptrStr("# rewritten instructions"),
	})
	require.NoError(t, err)

	require.Equal(t, []orgchart.NodeID{"b-two"}, spy.calls)
}

// The whole point of the narrow fingerprint: edits that already reach a
// running sandbox must not nag.
func TestUpdate_SilentOnHotApplyingFields(t *testing.T) {
	st := memory.New()
	seed(t, st, "b-three", []tool.Name{"chat"})
	spy := &restartSpy{}
	svc := restartSvc(st, spy)

	_, err := svc.Update(context.Background(), org, "b-three", nodes.UpdateParams{
		Name: ptrStr("Chief of Staff"),
	})
	require.NoError(t, err)

	_, err = svc.Update(context.Background(), org, "b-three", nodes.UpdateParams{
		PreserveContext: ptrBool(true),
	})
	require.NoError(t, err)

	_, err = svc.Update(context.Background(), org, "b-three", nodes.UpdateParams{
		ProjectIDs: &[]string{"prj_01"},
	})
	require.NoError(t, err)

	require.Empty(t, spy.calls)
}

// Re-submitting the same tool list (the UI sends the whole array on every
// save) is not a change.
func TestUpdate_SilentOnNoOpToolResubmit(t *testing.T) {
	st := memory.New()
	seed(t, st, "b-four", []tool.Name{"chat", "get_secret"})
	spy := &restartSpy{}

	_, err := restartSvc(st, spy).Update(context.Background(), org, "b-four", nodes.UpdateParams{
		Tools: ptrTools([]tool.Name{"get_secret", "chat"}),
	})
	require.NoError(t, err)

	require.Empty(t, spy.calls)
}

func TestAttachTools_FiresRestartRequired(t *testing.T) {
	st := memory.New()
	seed(t, st, "b-five", []tool.Name{"chat"})
	spy := &restartSpy{}

	_, err := restartSvc(st, spy).AttachTools(context.Background(), org, "b-five", []tool.Name{"get_secret"})
	require.NoError(t, err)

	require.Equal(t, []orgchart.NodeID{"b-five"}, spy.calls)
}

func TestAttachTools_SilentWhenNothingAdded(t *testing.T) {
	st := memory.New()
	seed(t, st, "b-six", []tool.Name{"chat", "get_secret"})
	spy := &restartSpy{}

	_, err := restartSvc(st, spy).AttachTools(context.Background(), org, "b-six", []tool.Name{"get_secret"})
	require.NoError(t, err)

	require.Empty(t, spy.calls)
}

func TestDetachTools_FiresRestartRequired(t *testing.T) {
	st := memory.New()
	seed(t, st, "b-seven", []tool.Name{"chat", "get_secret"})
	spy := &restartSpy{}

	_, err := restartSvc(st, spy).DetachTools(context.Background(), org, "b-seven", []tool.Name{"get_secret"})
	require.NoError(t, err)

	require.Equal(t, []orgchart.NodeID{"b-seven"}, spy.calls)
}

// A nil hook is the standalone/test wiring. It must not panic.
func TestUpdate_NilHookIsSafe(t *testing.T) {
	st := memory.New()
	seed(t, st, "b-eight", []tool.Name{"chat"})
	svc := nodes.New(nodes.Deps{Nodes: st.Nodes, Now: at, BaseTools: []tool.Name{"chat"}})

	_, err := svc.Update(context.Background(), org, "b-eight", nodes.UpdateParams{
		Tools: ptrTools([]tool.Name{"chat", "get_secret"}),
	})
	require.NoError(t, err)
}
