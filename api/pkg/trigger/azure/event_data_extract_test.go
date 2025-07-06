package azure

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRenderPullRequestCommentedEvent(t *testing.T) {
	// Load the test data
	bts, err := os.ReadFile("testdata/pr_commented.json")
	if err != nil {
		t.Fatalf("failed to read test data: %v", err)
	}

	// Unmarshal the test data
	var prc PullRequestComment
	err = json.Unmarshal(bts, &prc)
	if err != nil {
		t.Fatalf("failed to unmarshal test data: %v", err)
	}

	rendered, err := renderPullRequestCommentedEvent(prc)
	require.NoError(t, err)

	// Looking for specific pieces of information
	expected := []string{
		"Azure DevOps Pull Request Comment Event",
		"- Content: who wrote this code?",
		"- Project Name: helix-agents",
		"- PR ID: 1",
	}

	for _, expected := range expected {
		require.Contains(t, rendered, expected)
	}
}

func TestRenderPullRequestCreatedUpdatedEvent(t *testing.T) {
	// Test with created event
	bts, err := os.ReadFile("testdata/pr_created.json")
	require.NoError(t, err)

	var pr PullRequest
	err = json.Unmarshal(bts, &pr)
	require.NoError(t, err)

	rendered, err := renderPullRequestCreatedUpdatedEvent(pr)
	require.NoError(t, err)

	// Looking for specific pieces of information
	expected := []string{
		"Azure DevOps Pull Request Created Event",
		"- PR ID: 1",
		"- Title: content",
		"- Description: content",
		"- Project Name: helix-agents",
		"- Repository Name: helix-agents",
		"- Source Branch: refs/heads/feature/pr_1",
		"- Target Branch: refs/heads/master",
	}

	for _, expected := range expected {
		require.Contains(t, rendered, expected)
	}

	// Test with updated event
	bts, err = os.ReadFile("testdata/pr_updated.json")
	require.NoError(t, err)

	err = json.Unmarshal(bts, &pr)
	require.NoError(t, err)

	rendered, err = renderPullRequestCreatedUpdatedEvent(pr)
	require.NoError(t, err)

	// Looking for specific pieces of information
	expected = []string{
		"Azure DevOps Pull Request Updated Event",
		"- PR ID: 1",
		"- Title: content",
		"- Description: content",
		"- Project Name: helix-agents",
		"- Repository Name: helix-agents",
		"- Source Branch: refs/heads/feature/pr_1",
		"- Target Branch: refs/heads/master",
	}

	for _, expected := range expected {
		require.Contains(t, rendered, expected)
	}
}
