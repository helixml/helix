package tools

import (
	"encoding/json"
	"testing"

	"github.com/helixml/helix-org/domain"
)

// createStreamTransport accepts both the canonical object form and a
// bare string shorthand so smaller chat models that collapse the
// object to its discriminator string still get a working call.
func TestCreateStreamTransportUnmarshal(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		input  string
		want   createStreamTransport
		hasErr bool
	}{
		{
			name:  "object form with kind only",
			input: `{"kind":"webhook"}`,
			want:  createStreamTransport{Kind: domain.TransportWebhook},
		},
		{
			name:  "object form with kind and config",
			input: `{"kind":"webhook","config":{"outbound_url":"http://x/in"}}`,
			want: createStreamTransport{
				Kind:   domain.TransportWebhook,
				Config: json.RawMessage(`{"outbound_url":"http://x/in"}`),
			},
		},
		{
			name:  "string shorthand webhook",
			input: `"webhook"`,
			want:  createStreamTransport{Kind: domain.TransportWebhook},
		},
		{
			name:  "string shorthand local",
			input: `"local"`,
			want:  createStreamTransport{Kind: domain.TransportLocal},
		},
		{
			name:   "malformed JSON",
			input:  `{not json`,
			hasErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var got createStreamTransport
			err := json.Unmarshal([]byte(tc.input), &got)
			if tc.hasErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if got.Kind != tc.want.Kind {
				t.Errorf("Kind = %q, want %q", got.Kind, tc.want.Kind)
			}
			if string(got.Config) != string(tc.want.Config) {
				t.Errorf("Config = %q, want %q", got.Config, tc.want.Config)
			}
		})
	}
}

// The schema must declare both shapes so strict-validating MCP
// clients accept either input form.
func TestCreateStreamSchemaTransportOneOf(t *testing.T) {
	t.Parallel()
	tr, ok := createStreamSchema.Properties["transport"]
	if !ok {
		t.Fatal("schema is missing the transport property")
	}
	if len(tr.OneOf) != 2 {
		t.Fatalf("transport.oneOf len = %d, want 2", len(tr.OneOf))
	}
	// One branch must be the bare-string enum, the other the object.
	var sawString, sawObject bool
	for _, b := range tr.OneOf {
		switch b.Type {
		case "string":
			sawString = true
			if len(b.Enum) == 0 {
				t.Errorf("string branch has no enum constraint")
			}
		case "object":
			sawObject = true
			if _, ok := b.Properties["kind"]; !ok {
				t.Errorf("object branch missing kind property")
			}
		}
	}
	if !sawString || !sawObject {
		t.Errorf("transport.oneOf must cover both string and object (got string=%v object=%v)", sawString, sawObject)
	}
}
