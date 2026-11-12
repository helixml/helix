package anthropic

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/types"
)

func newVertexTestProxy() *Proxy {
	return &Proxy{
		cfg:                &config.ServerConfig{},
		vertexTokenSources: newTokenSourceCache(),
		defaultTokenSource: &staticTokenSource{token: "server-identity-token"},
	}
}

// A user endpoint with no credentials of its own must not authenticate as the
// server's Google identity (default token source / ADC).
func TestGetVertexTokenSource_TenantWithoutCredentialsRejected(t *testing.T) {
	p := newVertexTestProxy()
	endpoint := &types.ProviderEndpoint{
		EndpointType:    types.ProviderEndpointTypeUser,
		VertexProjectID: "some-project",
		VertexRegion:    "global",
	}

	ts, err := p.getVertexTokenSource(context.Background(), endpoint)
	require.Error(t, err)
	assert.Nil(t, ts)
}

// A user endpoint must not be able to point at a credentials file on the API
// server's filesystem.
func TestGetVertexTokenSource_TenantCredentialsFileIgnored(t *testing.T) {
	// Stands in for a key the operator mounted for the built-in provider.
	serverKey := filepath.Join(t.TempDir(), "vertex-sa.json")
	require.NoError(t, os.WriteFile(serverKey, []byte(`{"type":"service_account","client_email":"server@example.iam.gserviceaccount.com"}`), 0o600))

	p := newVertexTestProxy()
	endpoint := &types.ProviderEndpoint{
		EndpointType:          types.ProviderEndpointTypeOrg,
		VertexProjectID:       "some-project",
		VertexRegion:          "global",
		VertexCredentialsFile: serverKey,
	}

	_, err := p.getVertexTokenSource(context.Background(), endpoint)
	require.Error(t, err)
}

// An "external_account" credential names a local file and a token URL; the
// server would read the file and post its contents to that URL.
func TestGetVertexTokenSource_TenantExternalAccountRejected(t *testing.T) {
	var hits atomic.Int32
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer tokenServer.Close()

	secretFile := filepath.Join(t.TempDir(), "server-secret")
	require.NoError(t, os.WriteFile(secretFile, []byte("server-side-secret"), 0o600))

	creds, err := json.Marshal(map[string]any{
		"type":               "external_account",
		"audience":           "//iam.googleapis.com/projects/1/locations/global/workloadIdentityPools/p/providers/p",
		"subject_token_type": "urn:ietf:params:oauth:token-type:jwt",
		"token_url":          tokenServer.URL,
		"credential_source":  map[string]any{"file": secretFile},
	})
	require.NoError(t, err)

	p := newVertexTestProxy()
	endpoint := &types.ProviderEndpoint{
		EndpointType:          types.ProviderEndpointTypeUser,
		VertexProjectID:       "some-project",
		VertexRegion:          "global",
		VertexCredentialsJSON: string(creds),
	}

	ts, err := p.getVertexTokenSource(context.Background(), endpoint)
	if err == nil {
		_, _ = ts.Token()
	}
	require.Error(t, err)
	assert.Zero(t, hits.Load(), "token endpoint from user-supplied credentials was contacted")
}

func TestGetVertexTokenSource_GlobalUsesServerIdentity(t *testing.T) {
	p := newVertexTestProxy()
	endpoint := &types.ProviderEndpoint{
		EndpointType:    types.ProviderEndpointTypeGlobal,
		VertexProjectID: "some-project",
		VertexRegion:    "global",
	}

	ts, err := p.getVertexTokenSource(context.Background(), endpoint)
	require.NoError(t, err)
	tok, err := ts.Token()
	require.NoError(t, err)
	assert.Equal(t, "server-identity-token", tok.AccessToken)
}

func TestValidateTenantVertexCredentials(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		wantErr bool
	}{
		{"empty", "", true},
		{"not json", "nope", true},
		{"external account", `{"type":"external_account"}`, true},
		{"impersonated", `{"type":"impersonated_service_account"}`, true},
		{"authorized user", `{"type":"authorized_user"}`, true},
		{"service account custom token uri", `{"type":"service_account","token_uri":"http://127.0.0.1:8080/token"}`, true},
		{"service account default token uri", `{"type":"service_account","token_uri":"https://oauth2.googleapis.com/token"}`, false},
		{"service account no token uri", `{"type":"service_account"}`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTenantVertexCredentials(tt.json)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// The region is interpolated into the Vertex hostname; a crafted region must
// not redirect the request (and its bearer token) to another host.
func TestVertexTransformRequest_RejectsHostInjectionInRegion(t *testing.T) {
	for _, region := range []string{"attacker.example/", "attacker.example#", "x.attacker.example?", "us-east5.attacker.example:443/", ""} {
		req, err := http.NewRequest(http.MethodPost, "https://proxy.example.com/v1/messages", nil)
		require.NoError(t, err)
		err = vertexTransformRequest(req, "proj", region, &staticTokenSource{token: "tok"})
		assert.Error(t, err, "region %q", region)
		assert.NotEqual(t, "tok", stripBearer(req.Header.Get("Authorization")), "region %q", region)
	}
}

func stripBearer(v string) string {
	if len(v) > len("Bearer ") {
		return v[len("Bearer "):]
	}
	return ""
}
