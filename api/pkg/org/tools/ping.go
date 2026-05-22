package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/api/pkg/org/tool"
	"github.com/helixml/helix/api/pkg/org/domain"
)

// Ping is a trivial built-in tool used to exercise the invocation pipeline.
// It echoes the args back. Not part of the structural tool set.
type Ping struct{}

const PingName tool.Name = "ping"

type pingArgs struct {
	Message string `json:"message,omitempty"`
}

var pingSchema = mustSchema[pingArgs]()

func (Ping) Name() tool.Name                 { return PingName }
func (Ping) Description() string             { return "Echo a message back. Used to exercise the tool pipeline." }
func (Ping) InputSchema() *jsonschema.Schema { return pingSchema }

func (Ping) Invoke(_ context.Context, inv domain.Invocation) (json.RawMessage, error) {
	var args pingArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	out, err := json.Marshal(map[string]string{
		"echo":   args.Message,
		"caller": string(inv.Caller.ID()),
	})
	if err != nil {
		return nil, fmt.Errorf("marshal result: %w", err)
	}
	return out, nil
}
