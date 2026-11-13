package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/license"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/stripe"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func configTestServer(t *testing.T) (*HelixAPIServer, *store.MockStore) {
	m := store.NewMockStore(gomock.NewController(t))
	m.EXPECT().GetSystemSettings(gomock.Any()).Return(&types.SystemSettings{}, nil).AnyTimes()
	m.EXPECT().ListProviderEndpoints(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	cfg := &config.ServerConfig{}
	cfg.FileStore.LocalFSPath = t.TempDir()
	cfg.Janitor.SentryDsnFrontend = "https://public@sentry.example/1"
	cfg.Janitor.RudderStackWriteKey = "rs-write-key"
	return &HelixAPIServer{
		Cfg:            cfg,
		Store:          m,
		Stripe:         stripe.NewStripe(config.Stripe{}, nil),
		authMiddleware: &authMiddleware{store: m},
	}, m
}

func getConfigThroughRoute(t *testing.T, srv *HelixAPIServer, token string) types.ServerConfigForFrontend {
	handler := srv.authMiddleware.optionalUser(system.DefaultWrapperWithConfig(srv.config, system.WrapperConfig{SilenceErrors: true}))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/config", nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rr := httptest.NewRecorder()
	handler(rr, req)
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	var cfg types.ServerConfigForFrontend
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &cfg))
	return cfg
}

// Anonymous GET /config no longer fingerprints the deployment (exact build,
// deployment id); it keeps what the SPA needs before sign-in, including the
// browser-side Sentry and RudderStack ingestion keys.
func TestConfig_AnonymousOmitsBuildAndDeploymentIdentity(t *testing.T) {
	srv, _ := configTestServer(t)
	cfg := getConfigThroughRoute(t, srv, "")

	require.Empty(t, cfg.Version)
	require.Empty(t, cfg.LatestVersion)
	require.Empty(t, cfg.DeploymentID)
	require.Equal(t, "https://public@sentry.example/1", cfg.SentryDSNFrontend)
	require.Equal(t, "rs-write-key", cfg.RudderStackWriteKey)
}

// A bad or unknown token is served anonymously: /config must keep working for
// the login page even when the browser holds a stale credential.
func TestConfig_InvalidTokenServedAnonymously(t *testing.T) {
	srv, m := configTestServer(t)
	m.EXPECT().GetAPIKey(gomock.Any(), gomock.Any()).Return(nil, errors.New("not found"))

	cfg := getConfigThroughRoute(t, srv, types.APIKeyPrefix+"stale")
	require.Empty(t, cfg.Version)
	require.Empty(t, cfg.DeploymentID)
}

func TestConfig_SignedInCallerGetsBuildAndDeploymentIdentity(t *testing.T) {
	srv, _ := configTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/config", nil)
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: "usr_a"}))

	cfg, err := srv.config(httptest.NewRecorder(), req)
	require.NoError(t, err)
	require.NotEmpty(t, cfg.Version)
	require.NotEmpty(t, cfg.DeploymentID)
}

// /status tells non-admins only that the deployment is unlicensed; a deployment
// admin sees who holds the licence and what it allows.
func TestFrontendLicenseInfo_DetailsOnlyForAdmins(t *testing.T) {
	decoded := &license.License{
		Organization: "Acme Corp",
		Valid:        true,
		ValidUntil:   time.Now().Add(24 * time.Hour),
		Features:     license.Features{Users: true},
		Limits:       license.Limits{Users: 50, Machines: 5},
	}

	require.Nil(t, frontendLicenseInfo(decoded, false), "a valid licence is not described to non-admins at all")

	admin := frontendLicenseInfo(decoded, true)
	require.True(t, admin.Valid)
	require.Equal(t, "Acme Corp", admin.Organization)
	require.Equal(t, int64(50), admin.Limits.Users)

	expired := *decoded
	expired.ValidUntil = time.Now().Add(-time.Hour)
	tenant := frontendLicenseInfo(&expired, false)
	require.NotNil(t, tenant, "non-admins must still learn the deployment is unlicensed")
	require.False(t, tenant.Valid)
	require.Empty(t, tenant.Organization)
	require.True(t, tenant.ValidUntil.IsZero())
	require.Zero(t, tenant.Limits.Users)
}
