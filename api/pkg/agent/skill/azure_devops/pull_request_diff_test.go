package azuredevops

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/helixml/helix/api/pkg/agent"
	"github.com/helixml/helix/api/pkg/types"

	"github.com/stretchr/testify/require"
)

func Test_GetDiff(t *testing.T) {
	adoToken := os.Getenv("ADO_TOKEN")
	adoOrgURL := os.Getenv("ADO_ORG_URL")

	if os.Getenv("ADO_TOKEN") == "" {
		t.Skip("ADO_TOKEN is not set")
	}
	if os.Getenv("ADO_ORG_URL") == "" {
		t.Skip("ADO_ORG_URL is not set")
	}

	skill := NewPullRequestDiffSkill(adoOrgURL, adoToken)

	ctx := context.Background()

	ctx = types.SetAzureDevopsRepositoryContext(ctx, types.AzureDevopsRepositoryContext{
		RepositoryID: "73c763d4-bf41-49da-8481-896a4980b07c",
		ProjectID:    "4162255b-50ba-42af-a418-c088b814410a",
		RemoteURL:    "https://helixml@dev.azure.com/helixml/helix-agents/_git/helix-agents",

		LastMergeTargetCommitID: "b0f6a1d75557bf3deb83b305c5ee6d79312545a0", // Main branch
		// LastMergeTargetCommitID: "master", // Main branch
		LastMergeSourceCommitID: "30b66172254edd89648744f7421f703d78b7140a", // PR commit
		// LastMergeSourceCommitID: "feature/pr-2", // PR commit

		PullRequestID: 2,
	})

	response, err := skill.Tools[0].Execute(ctx, agent.Meta{}, map[string]interface{}{})
	require.NoError(t, err)

	fmt.Println(response)
}
