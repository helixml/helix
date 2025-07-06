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
