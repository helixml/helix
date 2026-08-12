package mcp

import (
	"encoding/json"
	"testing"

	m3 "github.com/mark3labs/mcp-go/mcp"

	"github.com/helixml/helix/api/pkg/util/jsonschema"
)

// TestBuildParametersNullableUnionType guards the create_bot "tools/topics
// published as string" bug: when an MCP tool declares an array parameter
// using the nullable-union form ("type": ["array","null"]) — the shape
// reflection-based schema generators emit for Go slice/pointer fields —
// buildParameters must resolve it to "array", not silently fall back to the
// "string" default. A spec-honouring harness obeys the resulting type, so a
// collapse to "string" makes the parameter uncallable with a real JSON array.
func TestBuildParametersNullableUnionType(t *testing.T) {
	raw := `{
      "name":"create_bot",
      "inputSchema":{
        "type":"object",
        "properties":{
          "tools":{"type":["array","null"],"items":{"type":"string"},"description":"MCP tools to grant"},
          "topics":{"type":["array","null"],"items":{"type":"string"},"description":"Existing Topic ids"}
        },
        "required":["tools","topics"]
      }
    }`
	var tl m3.Tool
	if err := json.Unmarshal([]byte(raw), &tl); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	def := buildParameters(tl.InputSchema)

	for _, name := range []string{"tools", "topics"} {
		prop, ok := def.Properties[name]
		if !ok {
			t.Fatalf("param %q missing from converted schema", name)
		}
		if prop.Type != jsonschema.Array {
			t.Errorf("param %q: got type %q, want %q", name, prop.Type, jsonschema.Array)
		}
		if prop.Items == nil || prop.Items.Type != jsonschema.String {
			t.Errorf("param %q: want items of type string, got %+v", name, prop.Items)
		}
	}
}

// TestConvertMapToDefinitionResolvesUnionType exercises the union resolution
// directly across the type members we care about, including "null"-first
// ordering, so the fix isn't accidentally order-dependent.
func TestConvertMapToDefinitionResolvesUnionType(t *testing.T) {
	cases := []struct {
		name string
		in   map[string]any
		want jsonschema.DataType
	}{
		{"array+null", map[string]any{"type": []any{"array", "null"}, "items": map[string]any{"type": "string"}}, jsonschema.Array},
		{"null+array", map[string]any{"type": []any{"null", "array"}, "items": map[string]any{"type": "string"}}, jsonschema.Array},
		{"string+null", map[string]any{"type": []any{"string", "null"}}, jsonschema.String},
		{"plain string", map[string]any{"type": "string"}, jsonschema.String},
		{"plain array", map[string]any{"type": "array", "items": map[string]any{"type": "string"}}, jsonschema.Array},
		{"missing type", map[string]any{"description": "x"}, jsonschema.String},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := convertMapToDefinition(tc.in).Type; got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
