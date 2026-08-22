package server

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/helixml/helix/api/pkg/types"
)

func TestSanitizeAgentToolsRejectsOrgGraphTools(t *testing.T) {
	// delete_bot and publish are real registry tools, but they act on the org
	// graph and would not resolve for a project-scoped caller.
	require.Equal(t,
		[]string{"create_spectask"},
		sanitizeAgentTools([]string{"create_spectask", "delete_bot", "publish", "not_a_tool"}))
	require.Empty(t, sanitizeAgentTools(nil))
}

func TestSanitizeAgentToolsDedupes(t *testing.T) {
	require.Equal(t,
		[]string{"get_spectask", "create_spectask"},
		sanitizeAgentTools([]string{"get_spectask", "create_spectask", "get_spectask"}))
}

func TestEligibleSpecTaskToolsUnionsProjectAndTask(t *testing.T) {
	project := &types.Project{AgentTools: []string{"create_spectask", "list_spectasks"}}
	task := &types.SpecTask{AgentTools: []string{"get_spectask", "create_spectask"}}
	require.Equal(t,
		[]string{"create_spectask", "get_spectask", "list_spectasks"},
		eligibleSpecTaskTools(project, task))
}

func TestEligibleSpecTaskToolsFiltersIneligibleStoredNames(t *testing.T) {
	// A name that predates the catalogue (or was written by an older build)
	// must not widen the surface at call time.
	project := &types.Project{AgentTools: []string{"create_spectask", "delete_bot"}}
	task := &types.SpecTask{AgentTools: []string{"publish"}}
	require.Equal(t, []string{"create_spectask"}, eligibleSpecTaskTools(project, task))
}

func TestEligibleSpecTaskToolsEmptyMeansNoSurface(t *testing.T) {
	require.Empty(t, eligibleSpecTaskTools(&types.Project{}, &types.SpecTask{}))
}
