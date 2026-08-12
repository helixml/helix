package mcptools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

// TestSchemaWireArrayParams guards against the "create_bot tools/topics
// published as string" bug at the WIRE level: it serves each bulk tool over a
// real go-sdk tools/list round-trip and asserts the array parameters arrive
// as {type:"array", items:{type:"string"}}, never as a scalar "string" and
// never as a nullable union like ["array","null"].
//
// The in-Go assertArrayProp check (bulk_tools_test.go) stayed green while the
// reported bug was live because it inspects the *jsonschema.Schema directly;
// this test inspects the bytes a client actually receives.
func TestSchemaWireArrayParams(t *testing.T) {
	names := func() []tool.Name {
		return []tool.Name{"publish", "subscribe", "create_bot", "attach_tool", "detach_tool"}
	}
	deps := Deps{ToolNames: names}

	// tool -> array params expected to publish as array-of-string.
	specs := []struct {
		tl     tool.Tool
		params []string
	}{
		{NewCreateBot(deps), []string{"tools", "topics"}},
		{&AttachTool{deps: deps}, []string{"tools"}},
		{&DetachTool{deps: deps}, []string{"tools"}},
		{&Subscribe{deps: deps}, []string{"topicIds"}},
		{&Unsubscribe{deps: deps}, []string{"topicIds"}},
	}

	srv := mcp.NewServer(&mcp.Implementation{Name: "wire-test", Version: "0"}, nil)
	for _, s := range specs {
		s := s
		srv.AddTool(&mcp.Tool{
			Name:        string(s.tl.Name()),
			Description: s.tl.Description(),
			InputSchema: s.tl.InputSchema(),
		}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{}, nil
		})
	}

	ct, st := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := srv.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	cli := mcp.NewClient(&mcp.Implementation{Name: "wire-test-client", Version: "0"}, nil)
	sess, err := cli.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	res, err := sess.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}

	wantParams := map[string][]string{}
	for _, s := range specs {
		wantParams[string(s.tl.Name())] = s.params
	}

	for _, tl := range res.Tools {
		params, ok := wantParams[tl.Name]
		if !ok {
			continue
		}
		// Re-marshal the served schema to inspect it exactly as a client sees it.
		raw, err := json.Marshal(tl.InputSchema)
		if err != nil {
			t.Fatalf("%s: marshal schema: %v", tl.Name, err)
		}
		var schema struct {
			Properties map[string]struct {
				Type  json.RawMessage `json:"type"`
				Items *struct {
					Type string `json:"type"`
				} `json:"items"`
			} `json:"properties"`
		}
		if err := json.Unmarshal(raw, &schema); err != nil {
			t.Fatalf("%s: unmarshal schema: %v", tl.Name, err)
		}
		for _, p := range params {
			prop, ok := schema.Properties[p]
			if !ok {
				t.Errorf("%s.%s: missing from served schema", tl.Name, p)
				continue
			}
			// type must be the scalar JSON string "array" — not "string",
			// and not a union array like ["array","null"].
			var typeStr string
			if err := json.Unmarshal(prop.Type, &typeStr); err != nil {
				t.Errorf("%s.%s: type is not a scalar string (got %s) — harnesses that expect a scalar type would fall back to string", tl.Name, p, prop.Type)
				continue
			}
			if typeStr != "array" {
				t.Errorf("%s.%s: got type %q, want \"array\"", tl.Name, p, typeStr)
			}
			if prop.Items == nil || prop.Items.Type != "string" {
				t.Errorf("%s.%s: want items.type \"string\", got %+v", tl.Name, p, prop.Items)
			}
		}
	}
}
