package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

// A web service whose active_sandbox_id points at a deleted row must serve the
// branded holding page, not a raw 502 carrying internal error text.
//
// This window is entirely normal — recovery deletes the old sandbox before
// provisioning its replacement, and only repoints active_sandbox_id at the end
// of a successful deploy — and on 2026-09-27 it lasted ~28h, during which
// we-find.ai served "active sandbox not found: not found" to the public.
func TestDispatchProjectWebServiceMissingSandboxServesHoldingPage(t *testing.T) {
	cases := []struct {
		name       string
		deploys    []*types.WebServiceDeploy
		wantInBody string
	}{
		{
			name:       "no deploy in flight gets temporarily unavailable",
			deploys:    []*types.WebServiceDeploy{{ID: "wsd_1", Status: types.WebServiceDeployStatusFailed}},
			wantInBody: "temporarily unavailable",
		},
		{
			name:       "deploy in flight gets starting up",
			deploys:    []*types.WebServiceDeploy{{ID: "wsd_2", Status: types.WebServiceDeployStatusBuilding, StartedAt: time.Now()}},
			wantInBody: "starting up",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockStore := store.NewMockStore(ctrl)
			mockStore.EXPECT().GetProjectWebServiceState(gomock.Any(), "prj_web").Return(
				&types.ProjectWebServiceState{
					ProjectID:       "prj_web",
					Enabled:         true,
					ActiveSandboxID: "sbx_deleted",
					ContainerPort:   8080,
				}, nil)
			mockStore.EXPECT().GetSandbox(gomock.Any(), "sbx_deleted").Return(nil, store.ErrNotFound)
			mockStore.EXPECT().ListWebServiceDeploys(gomock.Any(), "prj_web", 1).Return(c.deploys, nil)

			middleware := &VHostMiddleware{apiServer: &HelixAPIServer{Store: mockStore, connman: connman.New()}}

			request := httptest.NewRequest(http.MethodGet, "http://we-find.ai/", nil)
			response := httptest.NewRecorder()
			middleware.dispatchProjectWebService(response, request, &types.VHostRoute{
				TargetKind: types.VHostTargetProjectWebService,
				TargetID:   "prj_web",
				Hostname:   "we-find.ai",
				Port:       8080,
			})

			require.Equal(t, http.StatusServiceUnavailable, response.Code,
				"must be a 503 holding page, not a 502")
			body := response.Body.String()
			require.NotContains(t, body, "active sandbox not found",
				"internal error text must never reach the customer")
			require.NotContains(t, body, "sbx_deleted", "must not leak the sandbox id")
			require.Contains(t, strings.ToLower(body), c.wantInBody)
			require.Contains(t, response.Header().Get("Content-Type"), "text/html")
		})
	}
}

// Nothing deployed yet on a customer hostname is also a holding page, not the
// plain-text "project web service has no active deployment".
func TestDispatchProjectWebServiceNoDeploymentServesHoldingPage(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().GetProjectWebServiceState(gomock.Any(), "prj_web").Return(
		&types.ProjectWebServiceState{ProjectID: "prj_web", Enabled: true}, nil)
	mockStore.EXPECT().ListWebServiceDeploys(gomock.Any(), "prj_web", 1).Return(nil, nil)

	middleware := &VHostMiddleware{apiServer: &HelixAPIServer{Store: mockStore, connman: connman.New()}}

	request := httptest.NewRequest(http.MethodGet, "http://we-find.ai/", nil)
	response := httptest.NewRecorder()
	middleware.dispatchProjectWebService(response, request, &types.VHostRoute{
		TargetKind: types.VHostTargetProjectWebService,
		TargetID:   "prj_web",
		Hostname:   "we-find.ai",
		Port:       8080,
	})

	require.Equal(t, http.StatusServiceUnavailable, response.Code)
	require.NotContains(t, response.Body.String(), "no active deployment")
	require.Contains(t, response.Header().Get("Content-Type"), "text/html")
}
