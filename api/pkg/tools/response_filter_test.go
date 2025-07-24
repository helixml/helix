package tools

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	api_skill "github.com/helixml/helix/api/pkg/agent/skill/api_skills"
	"github.com/helixml/helix/api/pkg/types"

	golden "gotest.tools/v3/golden"
)

func TestFilterGithubPR(t *testing.T) {
	bts, err := os.ReadFile("./testdata/github_pull_request.json")
	require.NoError(t, err)

	manager := api_skill.NewManager()
	err = manager.LoadSkills(context.Background())
	require.NoError(t, err)

	skill, err := manager.GetSkill("github")
	require.NoError(t, err)

	tool := types.Tool{
		Config: types.ToolConfig{
			API: &types.ToolAPIConfig{
				Schema: skill.Schema,
			},
		},
	}

	filteredBody, err := removeUnknownKeys(&tool, "listPullRequests", 200, bts)
	require.NoError(t, err)

	// Should not contain head
	require.NotContains(t, string(filteredBody), "helixml:dependabot/npm_and_yarn/integration-test/frontend/npm_and_yarn-762e3aa393")

	golden.Assert(t, string(filteredBody), "github_pull_request_filtered.json")
}

func TestFilterGithubIssues(t *testing.T) {
	bts, err := os.ReadFile("./testdata/github_issues.json")
	require.NoError(t, err)

	manager := api_skill.NewManager()
	err = manager.LoadSkills(context.Background())
	require.NoError(t, err)

	skill, err := manager.GetSkill("github")
	require.NoError(t, err)

	tool := types.Tool{
		Config: types.ToolConfig{
			API: &types.ToolAPIConfig{
				Schema: skill.Schema,
			},
		},
	}

	filteredBody, err := removeUnknownKeys(&tool, "listRepoIssues", 200, bts)
	require.NoError(t, err)

	// Should have main fields:
	assert.Contains(t, string(filteredBody), `"user": {`)
	assert.Contains(t, string(filteredBody), `"state": "open`)
	assert.Contains(t, string(filteredBody), `"title": "Bump google.golang.org/api from 0.228.0 to 0.242.0",`)
	assert.Contains(t, string(filteredBody), `"number": 1160`)

	golden.Assert(t, string(filteredBody), "github_issues_filtered.json")
}
