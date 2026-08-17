package project

import (
	"encoding/json"
	"testing"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
)

func TestBuildForkPayloadUsesCodeAgentConfig(t *testing.T) {
	config := &types.CodeAgentExecutionConfig{
		Runtime:         types.CodeAgentRuntimeZedAgent,
		CredentialType:  types.CodeAgentCredentialTypeAPIKey,
		ProviderRef:     "openai",
		Model:           "gpt-5.6-sol",
		ReasoningEffort: "high",
	}

	payload := buildForkPayload("sample", "org_123", "project", "description", config)
	encoded, err := json.Marshal(payload)
	require.NoError(t, err)

	var body map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(encoded, &body))
	require.Contains(t, body, "code_agent_config")
	require.NotContains(t, body, "app_id")
	require.NotContains(t, body, "helix_app_id")
	require.NotContains(t, body, "code_agent_overrides")
}
