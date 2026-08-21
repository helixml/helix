package trigger

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/aggregate"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
)

type Trigger struct {
	aggregate.Aggregate
	Name        string
	Description string
	Kind        transport.Kind
	Config      json.RawMessage
}

func New(id, orgID, name, description string, kind transport.Kind, config json.RawMessage, createdBy string, createdAt time.Time) (Trigger, error) {
	a, err := aggregate.New(id, orgID, createdBy, createdAt)
	if err != nil {
		return Trigger{}, fmt.Errorf("trigger: %w", err)
	}
	if name == "" {
		return Trigger{}, errors.New("trigger name is empty")
	}
	t := transport.Transport{Kind: kind, Config: config}
	if err := t.Validate(); err != nil {
		return Trigger{}, fmt.Errorf("trigger transport: %w", err)
	}
	return Trigger{Aggregate: a, Name: name, Description: description, Kind: kind, Config: config}, nil
}
