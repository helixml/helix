package tools

import (
	"bytes"

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
