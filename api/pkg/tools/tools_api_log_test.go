package tools

import (
	"bytes"
	"net/http"
	"net/http/httptest"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// Tool header values carry the tool's credentials (secrets are substituted
// into them); preparing a request must not log them at any level.
func (suite *ActionTestSuite) Test_prepareRequest_DoesNotLogHeaderValues() {
	var buf bytes.Buffer
	previous := log.Logger
	log.Logger = zerolog.New(&buf).Level(zerolog.TraceLevel)
	defer func() { log.Logger = previous }()

	for _, headers := range []map[string]string{
		{"X-Api-Key": "tool-secret-header-value"},
		{"X-Api-Key": "tool-secret-header-value", "Authorization": "Bearer tool-secret-bearer"},
	} {
		tool := &types.Tool{
			Name:     "getPetDetail",
			ToolType: types.ToolTypeAPI,
			Config: types.ToolConfig{API: &types.ToolAPIConfig{
				URL:     "https://example.com",
				Schema:  petStoreAPISpec,
				Headers: headers,
			}},
		}
		_, err := suite.strategy.prepareRequest(suite.ctx, tool, "showPetById", map[string]interface{}{"petId": "1"})
		suite.Require().NoError(err)
	}

	suite.NotContains(buf.String(), "tool-secret-header-value")
	suite.NotContains(buf.String(), "tool-secret-bearer")
}

// OAuth tokens shorter than the old log prefix length used to panic, and the
// prepared request's Authorization value was logged.
func (suite *ActionTestSuite) Test_RunAPIActionWithParameters_ShortOAuthTokenNotLogged() {
	var buf bytes.Buffer
	previous := log.Logger
	log.Logger = zerolog.New(&buf).Level(zerolog.TraceLevel)
	defer func() { log.Logger = previous }()

	var gotAuth string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"id": 1, "name": "rex"}`))
	}))
	defer ts.Close()

	tool := &types.Tool{
		Name:     "pets",
		ToolType: types.ToolTypeAPI,
		Config: types.ToolConfig{API: &types.ToolAPIConfig{
			URL:           ts.URL,
			Schema:        petStoreAPISpec,
			OAuthProvider: "github",
		}},
	}
	_, err := suite.strategy.RunAPIActionWithParameters(suite.ctx, &types.RunAPIActionRequest{
		Tool:        tool,
		Action:      "showPetById",
		Parameters:  map[string]interface{}{"petId": "1"},
		OAuthTokens: map[string]string{"github": "tk1"},
	})
	suite.Require().NoError(err)
	suite.strategy.wg.Wait()
	suite.Equal("Bearer tk1", gotAuth)
	suite.NotContains(buf.String(), "Bearer tk1")
}
