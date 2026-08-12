package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/helixml/helix/api/pkg/connman"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestParseVHostConfig(t *testing.T) {
	cases := []struct {
		name           string
		devSubdomain   string
		serverURL      string
		wantBase       string
		wantEnabled    bool
		wantCanonical  string // single canonical we expect, "" if none
		canonicalCount int
	}{
		{
			name:           "both unset",
			wantBase:       "",
			wantEnabled:    false,
			canonicalCount: 0,
		},
		{
			name:           "server URL only",
			serverURL:      "https://helix.example.com",
			wantEnabled:    false,
			wantCanonical:  "helix.example.com",
			canonicalCount: 1,
		},
		{
			name:           "DEV_SUBDOMAIN as prefix",
			devSubdomain:   "dev",
			serverURL:      "https://helix.example.com",
			wantBase:       "dev.helix.example.com",
			wantEnabled:    true,
			wantCanonical:  "helix.example.com",
			canonicalCount: 1,
		},
		{
			name:           "DEV_SUBDOMAIN as full domain",
			devSubdomain:   "Apps.example.com",
			serverURL:      "https://helix.example.com",
			wantBase:       "apps.example.com",
			wantEnabled:    true,
			wantCanonical:  "helix.example.com",
			canonicalCount: 1,
		},
		{
			name:           "DEV_SUBDOMAIN prefix without server URL leaves base empty",
			devSubdomain:   "dev",
			wantEnabled:    false,
			canonicalCount: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := parseVHostConfig(tc.devSubdomain, tc.serverURL)
			if cfg.BaseDomain != tc.wantBase {
				t.Errorf("BaseDomain = %q want %q", cfg.BaseDomain, tc.wantBase)
			}
			if cfg.Enabled != tc.wantEnabled {
				t.Errorf("Enabled = %v want %v", cfg.Enabled, tc.wantEnabled)
			}
			if len(cfg.CanonicalHostnames) != tc.canonicalCount {
				t.Errorf("len(CanonicalHostnames) = %d want %d", len(cfg.CanonicalHostnames), tc.canonicalCount)
			}
			if tc.wantCanonical != "" {
				if _, ok := cfg.CanonicalHostnames[tc.wantCanonical]; !ok {
					t.Errorf("expected canonical %q in set, got %v", tc.wantCanonical, cfg.CanonicalHostnames)
				}
			}
		})
	}
}

func TestStripPort(t *testing.T) {
	cases := map[string]string{
		"host.example.com":      "host.example.com",
		"host.example.com:8080": "host.example.com",
		"localhost:80":          "localhost",
		"[::1]:8080":            "[::1]",
		"[2001:db8::1]:443":     "[2001:db8::1]",
		"":                      "",
	}
	for in, want := range cases {
		if got := stripPort(in); got != want {
			t.Errorf("stripPort(%q) = %q want %q", in, got, want)
		}
	}
}

func TestDispatchSandboxPreviewUsesSessionSandboxIDAsRevDialHost(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().GetSession(gomock.Any(), "ses_test").Return(&types.Session{
		ID:        "ses_test",
		SandboxID: "runner-a",
	}, nil)

	apiServer := &HelixAPIServer{
		Store:   mockStore,
		connman: connman.New(),
	}
	middleware := &VHostMiddleware{apiServer: apiServer}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	request := httptest.NewRequest(http.MethodGet, "http://share-test.dev.localhost/", nil).WithContext(ctx)
	response := httptest.NewRecorder()
	middleware.dispatchSandboxPreview(response, request, &types.VHostRoute{
		TargetKind: types.VHostTargetSandboxPreview,
		TargetID:   "ses_test",
		Port:       8080,
	})

	require.Equal(t, http.StatusServiceUnavailable, response.Code)
}
