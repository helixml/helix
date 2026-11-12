package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"

	"github.com/helixml/helix/api/pkg/types"
	"go.uber.org/mock/gomock"
)

// The listing shows connection details only for endpoints the caller manages.
// Global and system endpoints are usable by name, but their base_url is
// backend infrastructure and stays hidden from tenants.
func (s *ProviderHandlersSuite) TestListProviderEndpoints_HidesBaseURLOfEndpointsCallerDoesNotOwn() {
	s.store.EXPECT().ListProviderEndpoints(gomock.Any(), gomock.Any()).Return([]*types.ProviderEndpoint{
		{ID: "pe_admin_global", Name: "admin-global", BaseURL: "http://node06.internal.example:8000/v1",
			EndpointType: types.ProviderEndpointTypeGlobal, Owner: "admin_id", OwnerType: types.OwnerTypeUser, APIKey: "sk-secret"},
		{ID: "pe_system", Name: "system-dynamic", BaseURL: "http://vllm.cluster.local/v1",
			EndpointType: types.ProviderEndpointTypeGlobal, Owner: string(types.OwnerTypeSystem), OwnerType: types.OwnerTypeSystem},
		{ID: "pe_mine", Name: "mine", BaseURL: "https://my-llm.example.com/v1",
			EndpointType: types.ProviderEndpointTypeUser, Owner: "user_id", OwnerType: types.OwnerTypeUser, APIKey: "sk-mine"},
	}, nil)
	s.manager.EXPECT().ListProviders(gomock.Any(), "").Return([]types.Provider{types.ProviderOpenAI}, nil)
	s.server.Cfg.Providers.OpenAI.BaseURL = "http://openai-proxy.internal.example/v1"

	req := httptest.NewRequest(http.MethodGet, "/v1/provider-endpoints", nil).WithContext(s.authCtx)
	rr := httptest.NewRecorder()
	s.server.listProviderEndpoints(rr, req)
	s.Require().Equal(http.StatusOK, rr.Code, rr.Body.String())

	s.NotContains(rr.Body.String(), "internal.example")
	s.NotContains(rr.Body.String(), "cluster.local")
	s.NotContains(rr.Body.String(), "sk-secret")
	s.NotContains(rr.Body.String(), "sk-mine", "the caller's own key is masked too")

	var resp []*types.ProviderEndpoint
	s.Require().NoError(json.Unmarshal(rr.Body.Bytes(), &resp))
	byID := map[string]*types.ProviderEndpoint{}
	for _, ep := range resp {
		byID[ep.ID] = ep
	}
	s.Equal("https://my-llm.example.com/v1", byID["pe_mine"].BaseURL, "the caller's own endpoint keeps its base_url")
	s.Empty(byID["pe_admin_global"].BaseURL)
	s.Empty(byID["pe_system"].BaseURL)
	s.Equal(maskedSecret, byID["pe_admin_global"].APIKey)
}

// A deployment admin manages every endpoint and sees where they point.
func (s *ProviderHandlersSuite) TestListProviderEndpoints_AdminSeesBaseURL() {
	s.store.EXPECT().ListProviderEndpoints(gomock.Any(), gomock.Any()).Return([]*types.ProviderEndpoint{
		{ID: "pe_admin_global", Name: "admin-global", BaseURL: "http://node06.internal.example:8000/v1",
			EndpointType: types.ProviderEndpointTypeGlobal, Owner: "other_admin", OwnerType: types.OwnerTypeUser},
	}, nil)
	s.manager.EXPECT().ListProviders(gomock.Any(), "").Return(nil, nil)

	ctx := setRequestUser(s.authCtx, types.User{ID: "admin_id", Admin: true})
	req := httptest.NewRequest(http.MethodGet, "/v1/provider-endpoints", nil).WithContext(ctx)
	rr := httptest.NewRecorder()
	s.server.listProviderEndpoints(rr, req)
	s.Require().Equal(http.StatusOK, rr.Code, rr.Body.String())
	s.Contains(rr.Body.String(), "node06.internal.example")
}
