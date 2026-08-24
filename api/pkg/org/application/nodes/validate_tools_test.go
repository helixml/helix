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

// A typo in a tool name must fail the write, not persist a dead name.
// A Node carrying a name the registry doesn't know advertises a
// capability it does not have: the tool is silently ignored at invoke
// time, so the operator believes the Bot can do something it cannot.

func TestCreate_RejectsUnknownTool(t *testing.T) {
	st := memory.New()
	_, err := svc(st, nil, live).Create(context.Background(), org, nodes.CreateParams{
		ID:      "b-typo",
		Content: "# Typo",
		Tools:   []tool.Name{"chat", "not_a_real_tool_abc"},
	})
	require.ErrorIs(t, err, nodes.ErrUnknownTool)
	require.ErrorContains(t, err, "not_a_real_tool_abc")

	_, getErr := st.Nodes.Get(context.Background(), org, "b-typo")
	require.ErrorIs(t, getErr, store.ErrNotFound, "rejected create must leave no row")
}

func TestUpdate_RejectsUnknownTool(t *testing.T) {
	st := memory.New()
	seed(t, st, "b-sam", []tool.Name{"chat"})

	patch := []tool.Name{"chat", "totally_fake_tool_xyz"}
	_, err := svc(st, nil, live).Update(context.Background(), org, "b-sam", nodes.UpdateParams{Tools: &patch})
	require.ErrorIs(t, err, nodes.ErrUnknownTool)
	require.ErrorContains(t, err, "totally_fake_tool_xyz")

	require.Equal(t, []tool.Name{"chat"}, toolsOf(t, st, "b-sam"), "rejected patch must not mutate the row")
}

func TestAttachTools_RejectsUnknownTool(t *testing.T) {
	st := memory.New()
	seed(t, st, "b-sam", []tool.Name{"chat"})

	_, err := svc(st, nil, live).AttachTools(context.Background(), org, "b-sam", []tool.Name{"not_a_real_tool_abc"})
	require.ErrorIs(t, err, nodes.ErrUnknownTool)

	require.Equal(t, []tool.Name{"chat"}, toolsOf(t, st, "b-sam"))
}

// Known names still write. The baseline union is applied after
// validation, so a create with only known names is unaffected.
func TestCreate_AcceptsRegisteredTools(t *testing.T) {
	st := memory.New()
	node, err := svc(st, []tool.Name{"managers"}, live).Create(context.Background(), org, nodes.CreateParams{
		ID:      "b-ok",
		Content: "# OK",
		Tools:   []tool.Name{"chat", "attach_worker"},
	})
	require.NoError(t, err)
	require.Equal(t, []tool.Name{"chat", "attach_worker", "managers"}, node.Tools)
}

// Same rule the pruning path follows: a runtime that hasn't wired the
// registry must not guess. With no catalogue there is nothing to check
// against, so the write goes through rather than failing every create.
func TestCreate_UnwiredCatalogueSkipsValidation(t *testing.T) {
	st := memory.New()
	_, err := svc(st, nil, nil).Create(context.Background(), org, nodes.CreateParams{
		ID:      "b-nocat",
		Content: "# No catalogue",
		Tools:   []tool.Name{"whatever_tool"},
	})
	require.NoError(t, err)
	require.Equal(t, []tool.Name{"whatever_tool"}, toolsOf(t, st, "b-nocat"))
}

// A human placeholder never makes an MCP request and gets no tools —
// validation must not reject the empty list it is created with.
func TestCreate_HumanNodeUnaffected(t *testing.T) {
	st := memory.New()
	node, err := svc(st, []tool.Name{"managers"}, live).Create(context.Background(), org, nodes.CreateParams{
		ID:          "h-user",
		Content:     "Org member.",
		Kind:        orgchart.NodeKindHuman,
		HelixUserID: "usr-1",
	})
	require.NoError(t, err)
	require.Empty(t, node.Tools)
}
