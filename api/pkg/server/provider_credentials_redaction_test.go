package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"go.uber.org/mock/gomock"
)

const (
	testHeaderSecret = "sk-header-value-not-for-clients"
	testVertexSecret = `{"type":"service_account","private_key":"-----BEGIN PRIVATE KEY-----not-for-clients"}`
	testAPIKeySecret = "sk-api-key-not-for-clients"
)

func endpointWithCredentials(id string, endpointType types.ProviderEndpointType, owner string) *types.ProviderEndpoint {
	return &types.ProviderEndpoint{
		ID:                    id,
		Name:                  "shared-" + id,
		BaseURL:               "http://localhost:11434",
		EndpointType:          endpointType,
		Owner:                 owner,
		OwnerType:             types.OwnerTypeUser,
		APIKey:                testAPIKeySecret,
		Headers:               map[string]string{"Authorization": "Bearer " + testHeaderSecret, "anthropic-version": "2023-06-01"},
		VertexProjectID:       "project",
		VertexCredentialsJSON: testVertexSecret,
	}
}

func (s *ProviderHandlersSuite) assertNoProviderCredentials(body string) {
	s.NotContains(body, testHeaderSecret, "custom header values must be masked")
	s.NotContains(body, testVertexSecret, "Vertex service account JSON must be masked")
	s.NotContains(body, "BEGIN PRIVATE KEY", "Vertex service account JSON must be masked")
	s.NotContains(body, testAPIKeySecret, "API key must be masked")
}

// A global endpoint created by an admin is listed to every user. Its header
// values and Vertex credentials are as secret as its API key.
func (s *ProviderHandlersSuite) TestListProviderEndpoints_MasksGlobalEndpointCredentials() {
	s.store.EXPECT().ListProviderEndpoints(gomock.Any(), gomock.Any()).Return([]*types.ProviderEndpoint{
		endpointWithCredentials("pe_global", types.ProviderEndpointTypeGlobal, "admin_id"),
	}, nil)
	s.manager.EXPECT().ListProviders(gomock.Any(), "").Return([]types.Provider{}, nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/provider-endpoints", nil).WithContext(s.authCtx)
	rr := httptest.NewRecorder()
	s.server.listProviderEndpoints(rr, req)

	s.Require().Equal(http.StatusOK, rr.Code, rr.Body.String())
	s.assertNoProviderCredentials(rr.Body.String())

	var resp []*types.ProviderEndpoint
	s.Require().NoError(json.Unmarshal(rr.Body.Bytes(), &resp))
	s.Require().Len(resp, 1)
	s.Equal(maskedSecret, resp[0].Headers["Authorization"])
	s.Equal(maskedSecret, resp[0].VertexCredentialsJSON)
	s.Equal("project", resp[0].VertexProjectID, "non-secret Vertex fields stay visible")
}

// The list route accepts unauthenticated requests; it must not dereference a
// nil user.
func (s *ProviderHandlersSuite) TestListProviderEndpoints_Unauthenticated() {
	s.manager.EXPECT().ListProviders(gomock.Any(), "").Return([]types.Provider{}, nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/provider-endpoints", nil)
	rr := httptest.NewRecorder()
	s.NotPanics(func() { s.server.listProviderEndpoints(rr, req) })
	s.Equal(http.StatusOK, rr.Code, rr.Body.String())
}

func (s *ProviderHandlersSuite) TestCreateProviderEndpoint_MasksCredentials() {
	s.server.authMiddleware = &authMiddleware{store: s.store}
	s.store.EXPECT().GetUser(gomock.Any(), gomock.Any()).Return(&types.User{ID: "user_id"}, nil).AnyTimes()
	s.store.EXPECT().GetSystemSettings(gomock.Any()).Return(&types.SystemSettings{ProvidersManagementEnabled: true}, nil)
	s.store.EXPECT().ListProviderEndpoints(gomock.Any(), gomock.Any()).Return(nil, nil)
	s.store.EXPECT().CreateProviderEndpoint(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, ep *types.ProviderEndpoint) (*types.ProviderEndpoint, error) {
			s.Equal("Bearer "+testHeaderSecret, ep.Headers["Authorization"], "the stored value is the real one")
			created := *ep
			created.ID = "pe_new"
			return &created, nil
		})
	s.manager.EXPECT().GetClient(gomock.Any(), gomock.Any()).Return(nil, context.Canceled).AnyTimes()

	payload, err := json.Marshal(map[string]any{
		"name":     "custom",
		"base_url": "http://localhost:11434",
		"api_key":  testAPIKeySecret,
		"headers":  map[string]string{"Authorization": "Bearer " + testHeaderSecret},
	})
	s.Require().NoError(err)

	req := httptest.NewRequest(http.MethodPost, "/v1/provider-endpoints", bytes.NewReader(payload)).WithContext(s.authCtx)
	rr := httptest.NewRecorder()
	s.server.createProviderEndpoint(rr, req)

	s.Require().Equal(http.StatusOK, rr.Code, rr.Body.String())
	s.assertNoProviderCredentials(rr.Body.String())
}

// Clients edit an endpoint by round-tripping what they read, so masked values
// come back. They must keep the stored credentials, and the response must
// stay masked.
func (s *ProviderHandlersSuite) TestUpdateProviderEndpoint_MaskedValuesKeepStoredCredentials() {
	s.store.EXPECT().GetSystemSettings(gomock.Any()).Return(&types.SystemSettings{ProvidersManagementEnabled: true}, nil)
	stored := endpointWithCredentials("pe_user", types.ProviderEndpointTypeUser, "user_id")
	s.store.EXPECT().GetProviderEndpoint(gomock.Any(), &store.GetProviderEndpointsQuery{ID: "pe_user"}).Return(stored, nil)
	s.store.EXPECT().UpdateProviderEndpoint(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, ep *types.ProviderEndpoint) (*types.ProviderEndpoint, error) {
			s.Equal(map[string]string{
				"Authorization": "Bearer " + testHeaderSecret,
				"X-New":         "added",
			}, ep.Headers, "masked header keeps its value, removed header is dropped, new header is added")
			s.Equal(testVertexSecret, ep.VertexCredentialsJSON)
			s.Equal(testAPIKeySecret, ep.APIKey)
			return ep, nil
		})

	payload := `{"name":"shared-pe_user","base_url":"http://localhost:11434","api_key":"*****",` +
		`"vertex_credentials_json":"*****","headers":{"Authorization":"*****","X-New":"added"}}`
	req := httptest.NewRequest(http.MethodPut, "/v1/provider-endpoints/pe_user", strings.NewReader(payload)).WithContext(s.authCtx)
	req = mux.SetURLVars(req, map[string]string{"id": "pe_user"})
	rr := httptest.NewRecorder()
	s.server.updateProviderEndpoint(rr, req)

	s.Require().Equal(http.StatusOK, rr.Code, rr.Body.String())
	s.assertNoProviderCredentials(rr.Body.String())
}
