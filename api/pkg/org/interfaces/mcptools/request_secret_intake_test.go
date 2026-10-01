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

type testSecretIntakeService struct{ t *testing.T }

func (s testSecretIntakeService) Create(_ context.Context, orgID, botID string, input types.SecretIntakeCreateRequest) (types.SecretIntakeCreateResult, error) {
	require.Equal(s.t, "org_1", orgID)
	require.Equal(s.t, "bot_1", botID)
	require.Equal(s.t, "api_key", input.Fields[0].Name)
	return types.SecretIntakeCreateResult{ID: "sci_1", Status: "pending", InviteURL: "https://helix.example/connect/intake#token"}, nil
}

func (s testSecretIntakeService) Status(_ context.Context, orgID, botID, intakeID string) (types.SecretIntakeStatusResult, error) {
	require.Equal(s.t, "org_1", orgID)
	require.Equal(s.t, "bot_1", botID)
	require.Equal(s.t, "sci_1", intakeID)
	return types.SecretIntakeStatusResult{ID: intakeID, Status: "submitted"}, nil
}

func (secretIntakeCaller) ID() string             { return "bot_1" }
func (secretIntakeCaller) OrganizationID() string { return "org_1" }

func TestRequestSecretIntakeReturnsLinkWithoutValues(t *testing.T) {
	args := []byte(`{"customer_id":"cust","conversation_id":"chat","title":"API key","fields":[{"name":"api_key","label":"API key","type":"password","required":true}]}`)
	out, err := (&RequestSecretIntake{deps: Deps{SecretIntakes: testSecretIntakeService{t}}}).Invoke(context.Background(), tool.Invocation{Caller: secretIntakeCaller{}, Args: json.RawMessage(args)})
	require.NoError(t, err)
	require.Contains(t, string(out), "invite_url")
	require.NotContains(t, string(out), "api_key")
}

func TestGetSecretIntakeStatusUsesCallerScope(t *testing.T) {
	out, err := (&GetSecretIntakeStatus{deps: Deps{SecretIntakes: testSecretIntakeService{t}}}).Invoke(context.Background(), tool.Invocation{Caller: secretIntakeCaller{}, Args: json.RawMessage(`{"intake_id":"sci_1"}`)})
	require.NoError(t, err)
	require.Contains(t, string(out), `"status":"submitted"`)
	require.NotContains(t, string(out), "values")
}
