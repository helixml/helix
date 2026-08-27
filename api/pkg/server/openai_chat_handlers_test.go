package server

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dgraph-io/ristretto/v2"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/extract"
	"github.com/helixml/helix/api/pkg/filestore"
	"github.com/helixml/helix/api/pkg/janitor"
	oai "github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/openai/manager"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	openai "github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

// FindProviderWithModelSuite covers the prefixed-model resolution path that
// makes Zed's settings.json round-trip work. Zed receives prefixed model ids
// from /v1/models (e.g. "user/openai/gpt-5.2") via mapHelixToZedProvider,
// stores them, and sends them back on /v1/chat/completions. The per-provider
// cache stores raw upstream ids ("gpt-5.2"), so without prefix-aware lookup
// the request fell through to the silent default-provider fence.
type FindProviderWithModelSuite struct {
	suite.Suite

	ctrl    *gomock.Controller
	store   *store.MockStore
	manager *manager.MockProviderManager

	server *HelixAPIServer
}

func TestFindProviderWithModelSuite(t *testing.T) {
	suite.Run(t, new(FindProviderWithModelSuite))
}

func (s *FindProviderWithModelSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.store = store.NewMockStore(s.ctrl)
	s.manager = manager.NewMockProviderManager(s.ctrl)

	cache, err := ristretto.NewCache(&ristretto.Config[string, string]{
		NumCounters: 1e4,
		MaxCost:     1 << 20,
		BufferItems: 64,
	})
	s.Require().NoError(err)

	cfg := &config.ServerConfig{}
	cfg.WebServer.ModelsCacheTTL = time.Minute

	s.server = &HelixAPIServer{
		Cfg:             cfg,
		Store:           s.store,
		providerManager: s.manager,
		cache:           cache,
	}
}

// Bare upstream id matches a global provider's cached model list. No prefix
// stripping needed; we return the bare id unchanged.
func (s *FindProviderWithModelSuite) TestGlobal_BareIDMatchesCachedModel() {
	s.manager.EXPECT().ListProviders(gomock.Any(), "").Return([]types.Provider{
		types.ProviderOpenAI,
	}, nil)
	s.store.EXPECT().ListProviderEndpoints(gomock.Any(), gomock.Any()).Return([]*types.ProviderEndpoint{}, nil).AnyTimes()

	cacheKey := "openai:" + string(types.OwnerTypeSystem)
	s.server.cache.SetWithTTL(cacheKey, `{"Models":[{"id":"gpt-5.2"}],"FetchedAt":"2030-01-01T00:00:00Z"}`, 1, time.Minute)
	s.server.cache.Wait()

	provider, bare := s.server.findProviderWithModel(context.Background(), "gpt-5.2", "user_x", "")
	s.Equal("openai", provider)
	s.Equal("gpt-5.2", bare)
}

// The bug we're fixing: Zed sends "user/openai/gpt-5.2" (prefixed by
// mapHelixToZedProvider). The DB-backed provider's name is "user/openai" so
// ParseProviderFromModel can't help (it splits on the first "/"). We must
// strip the literal provider prefix and match the residue against the cache.
func (s *FindProviderWithModelSuite) TestDBProvider_PrefixedIDMatchesViaResidue() {
	s.manager.EXPECT().ListProviders(gomock.Any(), "").Return([]types.Provider{}, nil)
	s.store.EXPECT().ListProviderEndpoints(gomock.Any(), gomock.Any()).Return([]*types.ProviderEndpoint{
		{Name: "user/openai", Owner: "org_test"},
	}, nil).AnyTimes()

	cacheKey := "user/openai:org_test"
	s.server.cache.SetWithTTL(cacheKey, `{"Models":[{"id":"gpt-5.2"}],"FetchedAt":"2030-01-01T00:00:00Z"}`, 1, time.Minute)
	s.server.cache.Wait()

	provider, bare := s.server.findProviderWithModel(context.Background(), "user/openai/gpt-5.2", "user_x", "org_test")
	s.Equal("user/openai", provider)
	s.Equal("gpt-5.2", bare, "bare model must have provider prefix stripped before forwarding to upstream")
}

// Static Models field on the provider record (DB-backed, no live cache yet)
// must also accept the prefixed form.
func (s *FindProviderWithModelSuite) TestDBProvider_StaticModelsAcceptPrefixedID() {
	s.manager.EXPECT().ListProviders(gomock.Any(), "").Return([]types.Provider{}, nil)
	s.store.EXPECT().ListProviderEndpoints(gomock.Any(), gomock.Any()).Return([]*types.ProviderEndpoint{
		{Name: "user/openai", Owner: "org_test", Models: []string{"gpt-5.2"}},
	}, nil).AnyTimes()

	provider, bare := s.server.findProviderWithModel(context.Background(), "user/openai/gpt-5.2", "user_x", "org_test")
	s.Equal("user/openai", provider)
	s.Equal("gpt-5.2", bare)
}

// Global provider with a slash-bearing name (theoretical but cheap to cover).
func (s *FindProviderWithModelSuite) TestGlobal_PrefixedIDMatchesViaResidue() {
	s.manager.EXPECT().ListProviders(gomock.Any(), "").Return([]types.Provider{
		types.ProviderOpenAI,
	}, nil)
	s.store.EXPECT().ListProviderEndpoints(gomock.Any(), gomock.Any()).Return([]*types.ProviderEndpoint{}, nil).AnyTimes()

	cacheKey := "openai:" + string(types.OwnerTypeSystem)
	s.server.cache.SetWithTTL(cacheKey, `{"Models":[{"id":"gpt-4o"}],"FetchedAt":"2030-01-01T00:00:00Z"}`, 1, time.Minute)
	s.server.cache.Wait()

	provider, bare := s.server.findProviderWithModel(context.Background(), "openai/gpt-4o", "user_x", "")
	s.Equal("openai", provider)
	s.Equal("gpt-4o", bare)
}

func (s *FindProviderWithModelSuite) TestExplicitEnvironmentGlobalRefRemainsScoped() {
	s.manager.EXPECT().ListProviders(gomock.Any(), "").Return([]types.Provider{types.ProviderOpenAI}, nil)

	provider, bare := s.server.findProviderWithModel(context.Background(), "global/openai/gpt-5.4", "user_x", "org_test")
	s.Equal("global/openai", provider)
	s.Equal("gpt-5.4", bare)
}

func (s *FindProviderWithModelSuite) TestExplicitOrganizationRefRemainsScopedWithSameVendorGlobal() {
	s.manager.EXPECT().ListProviders(gomock.Any(), "").Return([]types.Provider{types.ProviderOpenAI}, nil)
	s.store.EXPECT().ListProviderEndpoints(gomock.Any(), &store.ListProviderEndpointsQuery{
		Owner:      "user_x",
		OwnerType:  types.OwnerTypeUser,
		WithGlobal: true,
	}).Return([]*types.ProviderEndpoint{
		{ID: "pe_global_openai", Name: "openai", EndpointType: types.ProviderEndpointTypeGlobal},
	}, nil)
	s.store.EXPECT().ListProviderEndpoints(gomock.Any(), &store.ListProviderEndpointsQuery{
		Owner:     "org_test",
		OwnerType: types.OwnerTypeOrg,
	}).Return([]*types.ProviderEndpoint{
		{ID: "pe_org_openrouter", Name: "openrouter", Owner: "org_test", OwnerType: types.OwnerTypeOrg, EndpointType: types.ProviderEndpointTypeOrg},
	}, nil)

	provider, bare := s.server.findProviderWithModel(context.Background(), "pe_org_openrouter/cohere/north-mini-code:free", "user_x", "org_test")
	s.Equal("pe_org_openrouter", provider)
	s.Equal("cohere/north-mini-code:free", bare)
}

// Unknown model — no cache hit anywhere — returns the empty pair so the
// caller can fall through to ParseProviderFromModel and ultimately to
// getClient's default-provider fence.
func (s *FindProviderWithModelSuite) TestUnknownModel_ReturnsEmpty() {
	s.manager.EXPECT().ListProviders(gomock.Any(), "").Return([]types.Provider{}, nil)
	s.store.EXPECT().ListProviderEndpoints(gomock.Any(), gomock.Any()).Return([]*types.ProviderEndpoint{}, nil).AnyTimes()

	provider, bare := s.server.findProviderWithModel(context.Background(), "made-up-model", "user_x", "")
	s.Empty(provider)
	s.Empty(bare)
}

func (s *FindProviderWithModelSuite) TestChatCompletionWithoutAppPassesOrganizationContext() {
	client := oai.NewMockClient(s.ctrl)
	s.manager.EXPECT().GetClient(gomock.Any(), gomock.Any()).Return(client, nil).AnyTimes()
	s.manager.EXPECT().ListProviders(gomock.Any(), "").Return([]types.Provider{}, nil)
	s.store.EXPECT().ListProviderEndpoints(gomock.Any(), gomock.Any()).Return([]*types.ProviderEndpoint{}, nil).AnyTimes()
	s.store.EXPECT().GetUserMeta(gomock.Any(), "user_test").Return(&types.UserMeta{}, nil)
	client.EXPECT().BillingEnabled().Return(false)
	client.EXPECT().CreateChatCompletion(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, _ openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
			orgID, ok := oai.GetContextOrganizationID(ctx)
			s.True(ok)
			s.Equal("org_test", orgID)
			return openai.ChatCompletionResponse{}, nil
		},
	)

	controller, err := controller.NewController(context.Background(), controller.Options{
		Config:          s.server.Cfg,
		Store:           s.store,
		Janitor:         janitor.NewJanitor(config.Janitor{}),
		ProviderManager: s.manager,
		Filestore:       filestore.NewMockFileStore(s.ctrl),
		Extractor:       extract.NewMockExtractor(s.ctrl),
	})
	s.Require().NoError(err)
	s.server.Controller = controller

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(`{"model":"openai/test-model","messages":[{"role":"user","content":"hi"}]}`))
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: "user_test", OrganizationID: "org_test"}))
	rw := httptest.NewRecorder()

	s.server.createChatCompletion(rw, req)

	s.Equal(http.StatusOK, rw.Code)
}
