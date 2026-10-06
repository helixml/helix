package toolcall

import (
	"encoding/json"
	"testing"
)

var editFile = Tool{
	Name: "edit_file",
	Schema: json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {"type": "string"},
			"line": {"type": "integer"}
		},
		"required": ["path"],
		"additionalProperties": false
	}`),
}

var noArgs = Tool{
	Name:   "list_files",
	Schema: json.RawMessage(`{"type": "object", "properties": {}}`),
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name   string
		tools  []Tool
		calls  []Call
		errors int
		kinds  string
	}{
		{
			name:  "valid call",
			tools: []Tool{editFile},
			calls: []Call{{Name: "edit_file", Arguments: `{"path": "a.go", "line": 3}`}},
		},
		{
			name:   "arguments are not json",
			tools:  []Tool{editFile},
			calls:  []Call{{Name: "edit_file", Arguments: `{"path": "a.go"`}},
			errors: 1,
			kinds:  KindInvalidJSON,
		},
		{
			name:   "tool was never offered",
			tools:  []Tool{editFile},
			calls:  []Call{{Name: "delete_file", Arguments: `{"path": "a.go"}`}},
			errors: 1,
			kinds:  KindUnknownName,
		},
		{
			name:   "required property missing",
			tools:  []Tool{editFile},
			calls:  []Call{{Name: "edit_file", Arguments: `{"line": 3}`}},
			errors: 1,
			kinds:  KindSchemaMismatch,
		},
		{
			name:   "wrong property type",
			tools:  []Tool{editFile},
			calls:  []Call{{Name: "edit_file", Arguments: `{"path": "a.go", "line": "three"}`}},
			errors: 1,
			kinds:  KindSchemaMismatch,
		},
		{
			name:   "undeclared property",
			tools:  []Tool{editFile},
			calls:  []Call{{Name: "edit_file", Arguments: `{"path": "a.go", "mode": "w"}`}},
			errors: 1,
			kinds:  KindSchemaMismatch,
		},
		{
			name:  "empty arguments on a no-argument tool are the empty object",
			tools: []Tool{noArgs},
			calls: []Call{{Name: "list_files", Arguments: ""}},
		},
		{
			name:   "empty arguments still fail a tool that requires them",
			tools:  []Tool{editFile},
			calls:  []Call{{Name: "edit_file", Arguments: ""}},
			errors: 1,
			kinds:  KindSchemaMismatch,
		},
		{
			name:  "tool without a schema is always valid",
			tools: []Tool{{Name: "anything"}},
			calls: []Call{{Name: "anything", Arguments: `{"whatever": true}`}},
		},
		{
			name:  "uncompilable schema is always valid",
			tools: []Tool{{Name: "broken", Schema: json.RawMessage(`{"type": 42}`)}},
			calls: []Call{{Name: "broken", Arguments: `{}`}},
		},
		{
			name:   "each bad call is counted once",
			tools:  []Tool{editFile},
			calls:  []Call{{Name: "nope", Arguments: `not json`}},
			errors: 1,
			kinds:  KindInvalidJSON,
		},
		{
			name:  "distinct kinds are recorded together",
			tools: []Tool{editFile},
			calls: []Call{
				{Name: "edit_file", Arguments: `{"path": "a.go"}`},
				{Name: "edit_file", Arguments: `{`},
				{Name: "nope", Arguments: `{}`},
			},
			errors: 2,
			kinds:  KindInvalidJSON + "," + KindUnknownName,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Validate(tc.tools, tc.calls)
			if got.Errors != tc.errors {
				t.Fatalf("errors = %d, want %d (kinds %q)", got.Errors, tc.errors, got.KindsString())
			}
			if got.KindsString() != tc.kinds {
				t.Fatalf("kinds = %q, want %q", got.KindsString(), tc.kinds)
			}
			if got.Calls != len(tc.calls) {
				t.Fatalf("calls = %d, want %d", got.Calls, len(tc.calls))
			}
			if got.Errored() != (tc.errors > 0) {
				t.Fatalf("errored = %v, want %v", got.Errored(), tc.errors > 0)
			}
		})
	}
}

func TestValidateNoCalls(t *testing.T) {
	got := Validate([]Tool{editFile}, nil)
	if got.Errored() || got.Calls != 0 || got.ToolsOffered != 1 {
		t.Fatalf("unexpected result: %+v", got)
	}
}

func TestCompileCaches(t *testing.T) {
	first := compile(editFile.Schema)
	second := compile(editFile.Schema)
	if first == nil || first != second {
		t.Fatalf("schema was recompiled instead of cached")
	}
}
