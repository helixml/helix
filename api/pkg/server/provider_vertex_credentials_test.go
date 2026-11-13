package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// The Vertex region is interpolated into the request hostname, so it must not
// be able to name another host.
func TestValidateProviderEndpointShape_VertexRegion(t *testing.T) {
	for _, region := range []string{"us-east5", "europe-west1", "global"} {
		require.NoError(t, validateProviderEndpointShape(&types.ProviderEndpoint{
			Name: "vertex", VertexProjectID: "p", VertexRegion: region,
			VertexCredentialsJSON: `{"type":"service_account"}`,
		}), region)
	}
	for _, region := range []string{"evil.example/", "evil.example#", "a.b", "us-east5.evil.example?", "US-EAST5", "-x"} {
		require.Error(t, validateProviderEndpointShape(&types.ProviderEndpoint{
			Name: "vertex", VertexProjectID: "p", VertexRegion: region,
		}), region)
	}
}

// A non-admin Vertex endpoint must bring its own service account. Without
// one, the proxy authenticates with the server's credentials file or its
// Application Default Credentials.
func (s *ProviderHandlersSuite) TestCreateProviderEndpoint_NonAdminVertexNeedsOwnCredentials() {
	for name, body := range map[string]map[string]any{
		"server credentials file": {
			"name": "vertex", "vertex_project_id": "p", "vertex_region": "us-east5",
			"vertex_credentials_file": "/etc/helix/vertex.json",
		},
		"default credentials": {
			"name": "vertex", "vertex_project_id": "p", "vertex_region": "us-east5",
		},
	} {
		s.Run(name, func() {
			s.SetupTest()
			s.server.authMiddleware = &authMiddleware{store: s.store}
			s.store.EXPECT().GetUser(gomock.Any(), gomock.Any()).Return(&types.User{ID: "user_id"}, nil).AnyTimes()
			s.store.EXPECT().GetSystemSettings(gomock.Any()).Return(&types.SystemSettings{ProvidersManagementEnabled: true}, nil)
			s.store.EXPECT().CreateProviderEndpoint(gomock.Any(), gomock.Any()).Times(0)

			payload, err := json.Marshal(body)
			s.Require().NoError(err)
			req := httptest.NewRequest(http.MethodPost, "/v1/provider-endpoints", bytes.NewReader(payload)).WithContext(s.authCtx)
			rr := httptest.NewRecorder()
			s.server.createProviderEndpoint(rr, req)

			// Rejected by shape validation (400) or the credential-source
			// check (403); either way nothing is stored.
			s.Contains([]int{http.StatusBadRequest, http.StatusForbidden}, rr.Code, rr.Body.String())
		})
	}
}
