package mcptools

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

const GetSecretIntakeStatusName tool.Name = "get_secret_intake_status"

type GetSecretIntakeStatus struct{ deps Deps }
type getSecretIntakeStatusArgs struct {
	IntakeID string `json:"intake_id"`
}

var getSecretIntakeStatusSchema = mustSchema[getSecretIntakeStatusArgs]()

func (t *GetSecretIntakeStatus) Name() tool.Name                 { return GetSecretIntakeStatusName }
func (t *GetSecretIntakeStatus) InputSchema() *jsonschema.Schema { return getSecretIntakeStatusSchema }
func (t *GetSecretIntakeStatus) Description() string {
	return "Check whether a secure secret intake was submitted, expired, or revoked. Returns status only, never values."
}
func (t *GetSecretIntakeStatus) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	if inv.Caller == nil || inv.Caller.ID() == "" || inv.Caller.OrganizationID() == "" {
		return nil, errors.New("caller identity required")
	}
	if t.deps.SecretIntakeStatus == nil {
		return nil, errors.New("secret intake is not configured")
	}
	var args getSecretIntakeStatusArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil || args.IntakeID == "" {
		return nil, errors.New("intake_id is required")
	}
	result, err := t.deps.SecretIntakeStatus(ctx, inv.Caller.OrganizationID(), inv.Caller.ID(), args.IntakeID)
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}
