package mcptools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/api/pkg/org/application/bots"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

type SetHumanContact struct {
	deps Deps
}

const SetHumanContactName tool.Name = "set_human_contact"

var setHumanContactSchema = mustSchema[setHumanContactArgs]()

type setHumanContactArgs struct {
	PersonID string            `json:"personId"`
	Contact  map[string]string `json:"contact"`
}

func (t *SetHumanContact) Name() tool.Name                 { return SetHumanContactName }
func (t *SetHumanContact) InputSchema() *jsonschema.Schema { return setHumanContactSchema }
func (t *SetHumanContact) Description() string {
	return "Patch a person's contact identity without replacing unrelated contact fields. " +
		"Use preferred_contact=helix or slack. Slack delivery uses slack_user_id and optional " +
		"slack_channel_id and slack_team_id. Empty values remove fields. Owner-only."
}

func (t *SetHumanContact) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args setHumanContactArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	if args.PersonID == "" || len(args.Contact) == 0 {
		return nil, fmt.Errorf("personId and contact are required")
	}
	orgID := inv.Caller.OrganizationID()
	if orgID == "" {
		return nil, fmt.Errorf("set_human_contact: caller has no OrgID")
	}
	person, err := t.deps.Queries.GetBot(ctx, orgID, orgchart.BotID(args.PersonID))
	if err != nil {
		return nil, fmt.Errorf("person %q: %w", args.PersonID, err)
	}
	if !person.IsHuman() {
		return nil, fmt.Errorf("%q is not a person (kind=human)", args.PersonID)
	}
	identity := make(map[string]string, len(person.Identity)+len(args.Contact))
	for key, value := range person.Identity {
		identity[key] = value
	}
	for key, value := range args.Contact {
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if key == "" {
			return nil, fmt.Errorf("contact keys cannot be empty")
		}
		if value == "" {
			delete(identity, key)
		} else {
			identity[key] = value
		}
	}
	if preferred := identity["preferred_contact"]; preferred != "" && preferred != "helix" && preferred != "slack" {
		return nil, fmt.Errorf("preferred_contact must be helix or slack")
	}
	if identity["preferred_contact"] == "slack" && identity["slack_user_id"] == "" {
		return nil, fmt.Errorf("slack_user_id is required when preferred_contact is slack")
	}
	if _, err := t.deps.Bots.Update(ctx, orgID, person.ID, bots.UpdateParams{Identity: &identity}); err != nil {
		return nil, fmt.Errorf("set human contact: %w", err)
	}
	return json.Marshal(map[string]string{"id": args.PersonID})
}
