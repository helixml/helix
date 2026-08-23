package nodes_test

import (
	"context"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/nodes"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	"github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
	"github.com/stretchr/testify/require"
)

const org = "org-tools"

func at() time.Time { return time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC) }

// live is the catalogue a post-cutover registry advertises — enough of it
// to exercise rename, prune and baseline.
var live = map[tool.Name]bool{
	"list_secrets": true, "get_secret": true,
	"create_trigger": true, "list_triggers": true, "attach_worker": true,
	"chat": true, "managers": true, "reports": true, "ask_human": true,
}

func seed(t *testing.T, st *store.Store, id string, tools []tool.Name) {
	t.Helper()
	n, err := orgchart.NewNode(orgchart.NodeID(id), "# "+id, tools, at(), org)
	require.NoError(t, err)
	require.NoError(t, st.Nodes.Create(context.Background(), n))
}

func toolsOf(t *testing.T, st *store.Store, id string) []tool.Name {
	t.Helper()
	n, err := st.Nodes.Get(context.Background(), org, orgchart.NodeID(id))
	require.NoError(t, err)
	return n.Tools
}

func svc(st *store.Store, base []tool.Name, known map[tool.Name]bool) *nodes.Nodes {
	return nodes.New(nodes.Deps{
		Nodes:     st.Nodes,
		Now:       at,
		BaseTools: base,
		KnownTools: func() map[tool.Name]bool {
			if known == nil {
				return nil
			}
			return known
		},
	})
}

// TestReconcile_RenamesRetiredToolsRatherThanDropping is the behaviour
// the upgrade turns on: a Bot that could mint a credential keeps an
// equivalent capability (discover + retrieve), and a Bot wired to the
// pre-cutover Topic primitives keeps the Trigger ones.
func TestReconcile_RenamesRetiredToolsRatherThanDropping(t *testing.T) {
	st := memory.New()
	seed(t, st, "b-sam", []tool.Name{"mint_credential", "subscribe", "publish", "list_topics"})

	require.NoError(t, svc(st, nil, live).Reconcile(context.Background(), org))

	require.Equal(t, []tool.Name{
		"list_secrets", "get_secret", "attach_worker", "chat", "list_triggers",
	}, toolsOf(t, st, "b-sam"))
}

// TestReconcile_PrunesNamesTheRegistryDoesNotKnow: a persisted name with
// no live tool behind it is silently ignored at invoke time, so leaving
// it makes the Bot's advertised capability a lie.
func TestReconcile_PrunesNamesTheRegistryDoesNotKnow(t *testing.T) {
	st := memory.New()
	seed(t, st, "b-sam", []tool.Name{"chat", "some_removed_tool", "managers"})

	require.NoError(t, svc(st, nil, live).Reconcile(context.Background(), org))

	require.Equal(t, []tool.Name{"chat", "managers"}, toolsOf(t, st, "b-sam"))
}

// TestReconcile_WithoutRegistryDoesNotPrune: a runtime that hasn't wired
// the registry must not guess a Bot's capability away. Renames still
// apply — those are a fixed mapping, not a guess.
func TestReconcile_WithoutRegistryDoesNotPrune(t *testing.T) {
	st := memory.New()
	seed(t, st, "b-sam", []tool.Name{"publish", "some_unknown_tool"})

	require.NoError(t, svc(st, nil, nil).Reconcile(context.Background(), org))

	require.Equal(t, []tool.Name{"chat", "some_unknown_tool"}, toolsOf(t, st, "b-sam"))
}

// TestReconcile_DeduplicatesAfterRename: `subscribe` renames to
// `attach_worker`, so a Bot that already had both ends up with one.
func TestReconcile_DeduplicatesAfterRename(t *testing.T) {
	st := memory.New()
	seed(t, st, "b-sam", []tool.Name{"subscribe", "attach_worker"})

	require.NoError(t, svc(st, nil, live).Reconcile(context.Background(), org))

	require.Equal(t, []tool.Name{"attach_worker"}, toolsOf(t, st, "b-sam"))
}

// TestReconcile_BackfillsBaselineAndIsIdempotent: the baseline is still
// unioned in, and a converged Bot is left untouched on a repeat run.
func TestReconcile_BackfillsBaselineAndIsIdempotent(t *testing.T) {
	st := memory.New()
	base := []tool.Name{"managers", "reports", "get_secret"}
	seed(t, st, "b-sam", []tool.Name{"publish"})

	require.NoError(t, svc(st, base, live).Reconcile(context.Background(), org))
	first := toolsOf(t, st, "b-sam")
	require.Equal(t, []tool.Name{"chat", "managers", "reports", "get_secret"}, first)

	require.NoError(t, svc(st, base, live).Reconcile(context.Background(), org))
	require.Equal(t, first, toolsOf(t, st, "b-sam"))
}
