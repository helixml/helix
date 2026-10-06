package trigger

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/aggregate"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
)

// Trigger is one inbound event source: a named transport that delivers
// events into the org, plus the event stream those events land on (a
// Trigger's stream is its own id). Workers attach directly to a Trigger;
// there is nothing in between.
//
// A Trigger of transport.KindLocal is an internal channel — a DM, a team
// chat, a transcript — carrying only messages the org itself produces.
type Trigger struct {
	aggregate.Aggregate
	Name        string
	Description string
	Kind        transport.Kind
	Config      json.RawMessage
}

// Transport returns the Trigger's inbound transport as the value type the
// transport package's typed config accessors read.
func (t Trigger) Transport() transport.Transport {
	return transport.Transport{Kind: t.Kind, Config: t.Config}
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
