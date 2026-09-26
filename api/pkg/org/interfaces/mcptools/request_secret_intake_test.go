package mcptools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/helixml/helix/api/pkg/org/domain/tool"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
)

type secretIntakeCaller struct{}

func (secretIntakeCaller) ID() string             { return "bot_1" }
func (secretIntakeCaller) OrganizationID() string { return "org_1" }

func TestRequestSecretIntakeReturnsLinkWithoutValues(t *testing.T) {
	creator := func(_ context.Context, orgID, botID string, input types.SecretIntakeCreateRequest) (types.SecretIntakeCreateResult, error) {
		require.Equal(t, "org_1", orgID)
		require.Equal(t, "bot_1", botID)
		require.Equal(t, "api_key", input.Fields[0].Name)
		return types.SecretIntakeCreateResult{ID: "sci_1", Status: "pending", InviteURL: "https://helix.example/connect/intake#token"}, nil
	}
	args := []byte(`{"customer_id":"cust","conversation_id":"chat","title":"API key","fields":[{"name":"api_key","label":"API key","type":"password","required":true}]}`)
	out, err := (&RequestSecretIntake{deps: Deps{SecretIntakeCreator: creator}}).Invoke(context.Background(), tool.Invocation{Caller: secretIntakeCaller{}, Args: json.RawMessage(args)})
	require.NoError(t, err)
	require.Contains(t, string(out), "invite_url")
	require.NotContains(t, string(out), "api_key")
}

func TestGetSecretIntakeStatusUsesCallerScope(t *testing.T) {
	status := func(_ context.Context, orgID, botID, intakeID string) (types.SecretIntakeStatusResult, error) {
		require.Equal(t, "org_1", orgID)
		require.Equal(t, "bot_1", botID)
		require.Equal(t, "sci_1", intakeID)
		return types.SecretIntakeStatusResult{ID: intakeID, Status: "submitted"}, nil
	}
	out, err := (&GetSecretIntakeStatus{deps: Deps{SecretIntakeStatus: status}}).Invoke(context.Background(), tool.Invocation{Caller: secretIntakeCaller{}, Args: json.RawMessage(`{"intake_id":"sci_1"}`)})
	require.NoError(t, err)
	require.Contains(t, string(out), `"status":"submitted"`)
	require.NotContains(t, string(out), "values")
}
