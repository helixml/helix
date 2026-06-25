package manager

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
)

func TestIsAnthropicAPIEndpoint(t *testing.T) {
	cases := []struct {
		name string
		ep   types.ProviderEndpoint
		want bool
	}{
		{"canonical name", types.ProviderEndpoint{Name: "anthropic"}, true},
		{"display name (EqualFold)", types.ProviderEndpoint{Name: "Anthropic"}, true},
		{"base url match", types.ProviderEndpoint{Name: "custom", BaseURL: "https://api.anthropic.com/v1"}, true},
		{"lookalike host not matched", types.ProviderEndpoint{Name: "custom", BaseURL: "https://api.anthropic.com.proxy.evil.com/v1"}, false},
		{"openai", types.ProviderEndpoint{Name: "openai", BaseURL: "https://api.openai.com/v1"}, false},
		{"renamed proxy (known gap)", types.ProviderEndpoint{Name: "claude-prod", BaseURL: "https://gw.internal/anthropic"}, false},
		{"vertex anthropic excluded", types.ProviderEndpoint{Name: "anthropic", VertexProjectID: "proj-1"}, false},
		{"vertex googleapis not matched", types.ProviderEndpoint{Name: "x", BaseURL: "https://anthropic.googleapis.com"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, isAnthropicAPIEndpoint(&tc.ep))
		})
	}
}

// Test_InitializeClient_AnthropicAuth covers the actual fix site: a DB/UI
// ProviderEndpoint run through initializeClient must send x-api-key (not Bearer)
// to /v1/models when it targets Anthropic, and Bearer otherwise.
func (suite *MultiClientManagerTestSuite) Test_InitializeClient_AnthropicAuth() {
	manager := NewProviderManager(suite.cfg, suite.store, nil, suite.modelInfoProvider)

	cases := []struct {
		name        string
		ep          types.ProviderEndpoint
		wantXAPIKey bool
	}{
		{"anthropic by name", types.ProviderEndpoint{Name: "anthropic", APIKey: "test-key"}, true},
		{"Anthropic display name", types.ProviderEndpoint{Name: "Anthropic", APIKey: "test-key"}, true},
		{"openai falls through to bearer", types.ProviderEndpoint{Name: "openai", APIKey: "test-key"}, false},
		{"vertex excluded -> bearer", types.ProviderEndpoint{Name: "anthropic", VertexProjectID: "proj-1", APIKey: "test-key"}, false},
	}

	for _, tc := range cases {
		suite.Run(tc.name, func() {
			var gotXAPIKey, gotAuth string
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotXAPIKey = r.Header.Get("x-api-key")
				gotAuth = r.Header.Get("Authorization")
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"claude-x","display_name":"x","type":"model"}]}`))
			}))
			defer ts.Close()

			tc.ep.BaseURL = ts.URL
			client, err := manager.initializeClient(&tc.ep)
			suite.Require().NoError(err)
			_, _ = client.ListModels(context.Background())

			if tc.wantXAPIKey {
				suite.Equal("test-key", gotXAPIKey, "must send x-api-key")
				suite.Empty(gotAuth, "must not send Authorization Bearer")
			} else {
				suite.Empty(gotXAPIKey, "must not send x-api-key")
				suite.NotEmpty(gotAuth, "must send Authorization Bearer")
			}
		})
	}
}
