package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dgraph-io/ristretto/v2"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/model"
	"github.com/helixml/helix/api/pkg/openai/manager"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// Regression tests for TST-01-270: a tenant-controlled provider endpoint
// base_url must not make the API server issue requests to loopback,
// link-local/metadata or private-network addresses, neither when the endpoint
// is created nor when its catalogue is refreshed.

type ssrfFixture struct {
	server   *HelixAPIServer
	store    *store.MockStore
	upstream *httptest.Server
	hits     *atomic.Int32
}

// newSSRFFixture wires the real provider manager and OpenAI client (only the
// store and model-info lookups are mocked), plus a listener on 127.0.0.1 that
// stands in for the assessor's internal target and counts every request.
func newSSRFFixture(t *testing.T, allowPrivate bool) *ssrfFixture {
	t.Helper()
	ctrl := gomock.NewController(t)

	hits := &atomic.Int32{}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"internal-model","object":"model"}]}`))
	}))
	t.Cleanup(upstream.Close)

	cfg := &config.ServerConfig{}
	cfg.WebServer.ModelsCacheTTL = time.Minute
	cfg.Providers.EnableCustomUserProviders = true
	cfg.Providers.AllowPrivateNetworks = allowPrivate

	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().GetSystemSettings(gomock.Any()).Return(&types.SystemSettings{ProvidersManagementEnabled: true}, nil).AnyTimes()
	mockStore.EXPECT().GetUser(gomock.Any(), gomock.Any()).Return(&types.User{ID: "user_id"}, nil).AnyTimes()

	modelInfo := model.NewMockModelInfoProvider(ctrl)
	modelInfo.EXPECT().GetModelInfo(gomock.Any(), gomock.Any()).Return(nil, errors.New("not found")).AnyTimes()

	cache, err := ristretto.NewCache(&ristretto.Config[string, string]{NumCounters: 1e4, MaxCost: 1 << 20, BufferItems: 64})
	require.NoError(t, err)

	return &ssrfFixture{
		server: &HelixAPIServer{
			Cfg:               cfg,
			Store:             mockStore,
			providerManager:   manager.NewProviderManager(cfg, mockStore, nil, modelInfo),
			modelInfoProvider: modelInfo,
			cache:             cache,
			authMiddleware:    &authMiddleware{store: mockStore, cfg: authMiddlewareConfig{}},
		},
		store:    mockStore,
		upstream: upstream,
		hits:     hits,
	}
}

func ssrfUserCtx() context.Context {
	return setRequestUser(context.Background(), types.User{ID: "user_id"})
}

func (f *ssrfFixture) create(t *testing.T, baseURL string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(map[string]string{
		"name":          "ssrf-probe-b",
		"base_url":      baseURL,
		"api_key":       "dummy",
		"endpoint_type": "user",
		"owner_type":    "user",
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/provider-endpoints", bytes.NewReader(body)).WithContext(ssrfUserCtx())
	rr := httptest.NewRecorder()
	f.server.createProviderEndpoint(rr, req)
	return rr
}

func (f *ssrfFixture) refresh(t *testing.T, endpoint *types.ProviderEndpoint) *httptest.ResponseRecorder {
	t.Helper()
	f.store.EXPECT().GetProviderEndpoint(gomock.Any(), &store.GetProviderEndpointsQuery{ID: endpoint.ID}).Return(endpoint, nil)
	f.store.EXPECT().ListProviderEndpoints(gomock.Any(), gomock.Any()).Return([]*types.ProviderEndpoint{endpoint}, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/provider-endpoints/"+endpoint.ID+"/available-models?refresh=true", nil).WithContext(ssrfUserCtx())
	req = mux.SetURLVars(req, map[string]string{"id": endpoint.ID})
	rr := httptest.NewRecorder()
	f.server.listProviderEndpointModels(rr, req)
	return rr
}

// PoC step 1: POST /api/v1/provider-endpoints with an internal or non-HTTP
// base_url is rejected before anything is stored or fetched.
func TestProviderEndpointSSRF_CreateRejectsInternalBaseURL(t *testing.T) {
	f := newSSRFFixture(t, false)
	// Strict mock: any CreateProviderEndpoint / ListProviderEndpoints call fails the test.

	for _, baseURL := range []string{
		f.upstream.URL + "/v1",                     // loopback listener
		"http://localhost:11434/v1",                // loopback by name
		"http://169.254.169.254/latest/meta-data/", // cloud metadata
		"http://[::ffff:169.254.169.254]/",         // metadata, IPv4-mapped
		"http://[fe80::1]/v1",                      // IPv6 link-local
		"http://10.0.0.5:8000/v1",                  // RFC1918
		"http://192.168.1.10/v1",                   // RFC1918
		"http://[fd00::1]/v1",                      // unique-local
		"file:///etc/passwd",                       // non-HTTP scheme
		"gopher://8.8.8.8:70/",                     // non-HTTP scheme
	} {
		rr := f.create(t, baseURL)
		require.Equal(t, http.StatusBadRequest, rr.Code, "base_url %q: %s", baseURL, rr.Body.String())
		require.Contains(t, rr.Body.String(), "invalid base_url", baseURL)
	}

	// Give a (non-existent) warm goroutine a chance to run before asserting.
	time.Sleep(100 * time.Millisecond)
	require.Zero(t, f.hits.Load(), "the server must not contact the internal listener")
}

// PoC step 2: GET .../available-models?refresh=true on an endpoint whose
// base_url points at an internal address (e.g. stored before this fix, or a
// public hostname that later rebinds) fails without the request reaching it.
func TestProviderEndpointSSRF_RefreshDoesNotReachInternalAddress(t *testing.T) {
	f := newSSRFFixture(t, false)
	endpoint := &types.ProviderEndpoint{
		ID:           "pe_ssrf",
		Name:         "ssrf-probe-b",
		BaseURL:      f.upstream.URL + "/v1",
		APIKey:       "dummy",
		EndpointType: types.ProviderEndpointTypeUser,
		Owner:        "user_id",
		OwnerType:    types.OwnerTypeUser,
	}

	rr := f.refresh(t, endpoint)
	require.Equal(t, http.StatusBadGateway, rr.Code, rr.Body.String())
	require.Zero(t, f.hits.Load(), "catalogue refresh must not reach a loopback base_url")
}

// Control: with PROVIDER_ENDPOINTS_ALLOW_PRIVATE_NETWORKS a self-hosted
// deployment can still use a provider on the local network — proving the
// tests above fail because of the guard, not because the listener is
// unreachable.
func TestProviderEndpointSSRF_AllowPrivateNetworksReachesLocalProvider(t *testing.T) {
	f := newSSRFFixture(t, true)
	endpoint := &types.ProviderEndpoint{
		ID:           "pe_local",
		Name:         "local-vllm",
		BaseURL:      f.upstream.URL + "/v1",
		EndpointType: types.ProviderEndpointTypeUser,
		Owner:        "user_id",
		OwnerType:    types.OwnerTypeUser,
	}

	rr := f.refresh(t, endpoint)
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	require.Contains(t, rr.Body.String(), "internal-model")
	require.Positive(t, f.hits.Load())

	// Metadata stays blocked even when private networks are allowed.
	rr = f.create(t, "http://169.254.169.254/latest/meta-data/")
	require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())
}
