package nodes_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/helixml/helix/api/pkg/org/application/nodes"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	"github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
	"github.com/helixml/helix/api/pkg/types"
)

// grantSvc treats chat as the standard worker set; every other tool needs
// authority to grant.
func grantSvc(st *store.Store) *nodes.Nodes {
	return nodes.New(nodes.Deps{Nodes: st.Nodes, Now: at, GrantableTools: []tool.Name{"chat"}})
}

func TestAuthorizeTools_ZeroGrantorMayGrantOnlyGrantableTools(t *testing.T) {
	st := memory.New()
	svc := grantSvc(st)

	_, err := svc.Create(context.Background(), org, nodes.CreateParams{ID: "b-std", Content: "# Std", Tools: []tool.Name{"chat"}})
	require.NoError(t, err)

	_, err = svc.Create(context.Background(), org, nodes.CreateParams{ID: "b-mgr", Content: "# Mgr", Tools: []tool.Name{"chat", "create_bot"}})
	require.ErrorIs(t, err, nodes.ErrToolGrantForbidden)
	_, getErr := st.Nodes.Get(context.Background(), org, "b-mgr")
	require.ErrorIs(t, getErr, store.ErrNotFound, "refused create must leave no row")
}

func TestAuthorizeTools_AdminHasNoCeiling(t *testing.T) {
	st := memory.New()
	_, err := grantSvc(st).Create(context.Background(), org, nodes.CreateParams{
		ID: "b-mgr", Content: "# Mgr", Tools: []tool.Name{"create_bot", "delete_bot"}, Grantor: nodes.Grantor{Admin: true},
	})
	require.NoError(t, err)
}

func TestAuthorizeTools_BotGrantsOnlyToolsItHolds(t *testing.T) {
	st := memory.New()
	seed(t, st, "b-lead", []tool.Name{"chat", "create_bot", "attach_tool"})
	seed(t, st, "b-worker", []tool.Name{"chat"})
	svc := grantSvc(st)
	lead := nodes.Grantor{Admin: true, Bot: "b-lead"}
	ctx := context.Background()

	// Within its own authority: allowed, on create and attach.
	_, err := svc.Create(ctx, org, nodes.CreateParams{ID: "b-hire", Content: "# Hire", Tools: []tool.Name{"chat", "attach_tool"}, Grantor: lead})
	require.NoError(t, err)
	_, err = svc.AttachTools(ctx, org, "b-worker", lead, []tool.Name{"create_bot"})
	require.NoError(t, err)

	// Beyond it: refused, including granting itself.
	_, err = svc.Create(ctx, org, nodes.CreateParams{ID: "b-admin", Content: "# Admin", Tools: []tool.Name{"delete_bot"}, Grantor: lead})
	require.ErrorIs(t, err, nodes.ErrToolGrantForbidden)
	_, err = svc.AttachTools(ctx, org, "b-lead", lead, []tool.Name{"delete_bot"})
	require.ErrorIs(t, err, nodes.ErrToolGrantForbidden)
	require.NotContains(t, toolsOf(t, st, "b-lead"), tool.Name("delete_bot"))
	_, err = svc.Update(ctx, org, "b-worker", nodes.UpdateParams{Tools: ptrTools([]tool.Name{"chat", "sandbox_ssh_access"}), Grantor: lead})
	require.ErrorIs(t, err, nodes.ErrToolGrantForbidden)
}

func TestAuthorizeTools_BotNeedsOwnerPrincipalForRestrictedTools(t *testing.T) {
	st := memory.New()
	seed(t, st, "b-lead", []tool.Name{"chat", "attach_tool", "delete_bot"})
	seed(t, st, "b-worker", []tool.Name{"chat"})

	_, err := grantSvc(st).AttachTools(context.Background(), org, "b-worker", nodes.Grantor{Bot: "b-lead"}, []tool.Name{"delete_bot"})
	require.ErrorIs(t, err, nodes.ErrToolGrantForbidden)
	require.NotContains(t, toolsOf(t, st, "b-worker"), tool.Name("delete_bot"))
}

func TestAuthorizeTools_BotCannotChangeAMorePrivilegedNode(t *testing.T) {
	st := memory.New()
	seed(t, st, "b-lead", []tool.Name{"chat", "detach_tool"})
	seed(t, st, "b-cos", []tool.Name{"chat", "detach_tool", "delete_bot"})
	svc := grantSvc(st)

	_, err := svc.DetachTools(context.Background(), org, "b-cos", nodes.Grantor{Admin: true, Bot: "b-lead"}, []tool.Name{"detach_tool"})
	require.ErrorIs(t, err, nodes.ErrToolGrantForbidden)
	require.Contains(t, toolsOf(t, st, "b-cos"), tool.Name("detach_tool"))

	// A zero Grantor (REST member) cannot even reshape the grantable part.
	_, err = svc.Update(context.Background(), org, "b-cos", nodes.UpdateParams{Tools: ptrTools([]tool.Name{"chat"})})
	require.ErrorIs(t, err, nodes.ErrToolGrantForbidden)
}

func TestAuthorizeTools_UnknownGrantingBotHasNoAuthority(t *testing.T) {
	st := memory.New()
	seed(t, st, "b-worker", []tool.Name{"chat"})
	svc := grantSvc(st)

	_, err := svc.AttachTools(context.Background(), org, "b-worker", nodes.Grantor{Admin: true, Bot: "b-ghost"}, []tool.Name{"create_bot"})
	require.ErrorIs(t, err, nodes.ErrToolGrantForbidden)
	_, err = svc.AttachTools(context.Background(), org, "b-worker", nodes.Grantor{Admin: true, Bot: "b-ghost"}, []tool.Name{"chat"})
	require.NoError(t, err, "grantable tools need no authority")
}

func TestAuthorizeTools_InstanceProfileTools(t *testing.T) {
	st := memory.New()
	seed(t, st, "b-broker", []tool.Name{"chat", "create_bot"})
	svc := grantSvc(st)

	profile := &types.BotInstanceProfile{Tools: []string{"create_bot"}}
	_, err := svc.Update(context.Background(), org, "b-broker", nodes.UpdateParams{InstanceProfile: profile})
	require.ErrorIs(t, err, nodes.ErrToolGrantForbidden)

	_, err = svc.Update(context.Background(), org, "b-broker", nodes.UpdateParams{InstanceProfile: profile, Grantor: nodes.Grantor{Admin: true}})
	require.NoError(t, err)
}
