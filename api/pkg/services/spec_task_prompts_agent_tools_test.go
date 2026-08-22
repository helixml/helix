package services

import (
	"strings"
	"testing"

	"github.com/helixml/helix/api/pkg/types"

	"github.com/stretchr/testify/require"
)

func TestBuildAgentToolsSectionEmptyWhenNoGrant(t *testing.T) {
	require.Equal(t, "", BuildAgentToolsSection(nil, nil))
	require.Equal(t, "", BuildAgentToolsSection([]string{}, []string{}))
}

func TestBuildAgentToolsSectionListsUnionAndRules(t *testing.T) {
	out := BuildAgentToolsSection([]string{"create_spectask"}, []string{"start_spectask_planning"})
	require.Contains(t, out, "- `create_spectask`")
	require.Contains(t, out, "- `start_spectask_planning`")
	// The two rules an agent cannot infer from tool descriptions alone.
	require.Contains(t, out, "backlog")
	require.Contains(t, out, "cannot see this conversation")
	require.True(t, strings.HasPrefix(out, "## Delegating to other spec tasks"))
}

func TestPlanningPromptOmitsDelegationWhenNoTools(t *testing.T) {
	require.NotContains(t, BuildPlanningPrompt(&types.SpecTask{ID: "spt_1", Name: "t", ProjectID: "prj_1"}, "", "", "", "", ""), "Delegating to other spec tasks")
}

func TestPlanningPromptIncludesDelegationWhenGranted(t *testing.T) {
	section := BuildAgentToolsSection([]string{"create_spectask"}, nil)
	require.Contains(t, BuildPlanningPrompt(&types.SpecTask{ID: "spt_1", Name: "t", ProjectID: "prj_1"}, "", "", "", "", section), "Delegating to other spec tasks")
}
