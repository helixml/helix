package mcptools

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	"github.com/helixml/helix/api/pkg/types"
)

const RequestSecretIntakeName tool.Name = "request_secret_intake"

// RequestSecretIntake gives a bot only the URL and safe metadata. Values
// submitted at that URL never travel through this MCP result.
type RequestSecretIntake struct{ deps Deps }

var requestSecretIntakeSchema = mustSchema[types.SecretIntakeCreateRequest]()

func (t *RequestSecretIntake) Name() tool.Name                 { return RequestSecretIntakeName }
func (t *RequestSecretIntake) InputSchema() *jsonschema.Schema { return requestSecretIntakeSchema }
func (t *RequestSecretIntake) Description() string {
	return "Create a secure, short-lived link for a person to enter requested usernames, passwords, API keys, or one-time codes outside chat. Return the link to the person. Never ask them to paste secret values into chat. The result contains no submitted values."
}
func (t *RequestSecretIntake) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	if inv.Caller == nil || inv.Caller.ID() == "" || inv.Caller.OrganizationID() == "" {
		return nil, errors.New("caller identity required")
	}
	if t.deps.SecretIntakeCreator == nil {
		return nil, errors.New("secret intake is not configured")
	}
	var input types.SecretIntakeCreateRequest
	if err := json.Unmarshal(inv.Args, &input); err != nil {
		return nil, errors.New("invalid intake request")
	}
	result, err := t.deps.SecretIntakeCreator(ctx, inv.Caller.OrganizationID(), inv.Caller.ID(), input)
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}
