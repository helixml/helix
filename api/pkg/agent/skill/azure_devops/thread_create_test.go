package azuredevops

import (
	"context"
	"os"
	"testing"

	"github.com/helixml/helix/api/pkg/agent"
	"github.com/helixml/helix/api/pkg/types"

	"github.com/stretchr/testify/require"
)

func TestCreateThread_OnFile(t *testing.T) {
	adoToken := os.Getenv("ADO_TOKEN")
	adoOrgURL := os.Getenv("ADO_ORG_URL")

	if os.Getenv("ADO_TOKEN") == "" {
		t.Skip("ADO_TOKEN is not set")
	}
	if os.Getenv("ADO_ORG_URL") == "" {
		t.Skip("ADO_ORG_URL is not set")
	}

	skill := NewCreateThreadSkill(adoOrgURL, adoToken)

	ctx := context.Background()

	ctx = types.SetAzureDevopsRepositoryContext(ctx, types.AzureDevopsRepositoryContext{
		RepositoryID:  "73c763d4-bf41-49da-8481-896a4980b07c",
		ProjectID:     "4162255b-50ba-42af-a418-c088b814410a",
		RemoteURL:     "https://helixml@dev.azure.com/helixml/helix-agents/_git/helix-agents",
		PullRequestID: 2,
	})

	_, err := skill.Tools[0].Execute(ctx, agent.Meta{}, map[string]interface{}{
		"content":     "What is this?",
		"file_path":   "fib.py",
		"line_number": 19,
	})
	require.NoError(t, err)
}
