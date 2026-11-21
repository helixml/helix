package secret

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
)

func resetCreateCommand(t *testing.T) {
	t.Helper()
	for name, value := range map[string]string{
		"name": "", "value": "", "app-id": "", "project": "", "scope": string(types.SecretScopeDev),
	} {
		flag := createCmd.Flags().Lookup(name)
		require.NoError(t, flag.Value.Set(value))
		flag.Changed = false
	}
	rootCmd.SetArgs(nil)
}

func TestCreateProjectSecretScope(t *testing.T) {
	for _, test := range []struct {
		name       string
		scopeArgs  []string
		wantScope  string
		wantTarget string
	}{
		{name: "default dev", wantScope: string(types.SecretScopeDev), wantTarget: "project sessions"},
		{name: "explicit dev", scopeArgs: []string{"--scope", "dev"}, wantScope: string(types.SecretScopeDev), wantTarget: "project sessions"},
		{name: "prod", scopeArgs: []string{"--scope", "prod"}, wantScope: string(types.SecretScopeProd), wantTarget: "web services"},
		{name: "both", scopeArgs: []string{"--scope", "both"}, wantScope: string(types.SecretScopeBoth), wantTarget: "project sessions and web services"},
	} {
		t.Run(test.name, func(t *testing.T) {
			resetCreateCommand(t)
			t.Cleanup(func() { resetCreateCommand(t) })
			var output bytes.Buffer
			rootCmd.SetOut(&output)
			t.Cleanup(func() { rootCmd.SetOut(nil) })

			var created types.CreateSecretRequest
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/api/v1/projects/prj_test":
					_ = json.NewEncoder(w).Encode(types.Project{ID: "prj_test"})
				case r.Method == http.MethodPost && r.URL.Path == "/api/v1/projects/prj_test/secrets":
					if err := json.NewDecoder(r.Body).Decode(&created); err != nil {
						t.Errorf("decode request: %v", err)
					}
					_ = json.NewEncoder(w).Encode(types.Secret{ID: "sec_test"})
				default:
					http.NotFound(w, r)
				}
			}))
			t.Cleanup(server.Close)
			t.Setenv("HELIX_URL", server.URL)
			t.Setenv("HELIX_API_KEY", "test-key")

			args := []string{"create", "--name", "AUTH_CLIENT_SECRET", "--value", "secret", "--project", "prj_test"}
			rootCmd.SetArgs(append(args, test.scopeArgs...))
			require.NoError(t, rootCmd.Execute())
			require.Equal(t, "AUTH_CLIENT_SECRET", created.Name)
			require.Equal(t, "secret", created.Value)
			require.Equal(t, "prj_test", created.ProjectID)
			require.Equal(t, test.wantScope, created.Scope)
			require.Contains(t, output.String(), test.wantScope+" scope (injected into "+test.wantTarget+")")
		})
	}
}

func TestCreateProjectSecretRejectsInvalidScopeBeforePromptOrRequest(t *testing.T) {
	for _, test := range []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "invalid scope",
			args:    []string{"create", "--name", "AUTH_CLIENT_SECRET", "--project", "prj_test", "--scope", "production"},
			wantErr: `invalid secret scope "production", must be one of: dev, prod, both`,
		},
		{
			name:    "scope without project",
			args:    []string{"create", "--name", "AUTH_CLIENT_SECRET", "--scope", "prod"},
			wantErr: "--scope requires --project",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			resetCreateCommand(t)
			t.Cleanup(func() { resetCreateCommand(t) })

			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				requests.Add(1)
			}))
			t.Cleanup(server.Close)
			t.Setenv("HELIX_URL", server.URL)
			t.Setenv("HELIX_API_KEY", "test-key")

			rootCmd.SetArgs(test.args)
			err := rootCmd.Execute()
			require.EqualError(t, err, test.wantErr)
			require.Zero(t, requests.Load())
		})
	}
}
