package tools

import (
	"testing"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
)

var alphaVantageAPI = `
openapi: 3.0.3
info:
  title: Alpha Vantage Market News & Sentiment API
  description: API for retrieving market news and sentiment data from Alpha Vantage.
  version: 1.0.0
servers:
  - url: https://www.alphavantage.co
paths:
  /query:
    get:
      summary: Get market news and sentiment data
      operationId: getMarketNewsAndSentimentData
      parameters:
        - name: function
          in: query
          required: true
          schema:
            type: string
            enum: [NEWS_SENTIMENT]
          description: The function to call, must be NEWS_SENTIMENT
        - name: tickers
          in: query
          required: false
          schema:
            type: array
            items:
              type: string
          style: form
          explode: false
          description: "Comma-separated list of stock/crypto/forex symbols to filter articles that mention these symbols. For example: tickers=IBM will filter for articles that mention the IBM ticker; tickers=COIN,CRYPTO:BTC,FOREX:USD will filter for articles that simultaneously mention Coinbase (COIN), Bitcoin (CRYPTO:BTC), and US Dollar (FOREX:USD) in their content."
        - name: topics
          in: query
          required: false
          schema:
            type: array
            items:
              type: string
          style: form
          explode: false
          description: "Comma-separated list of topics to filter articles that cover these topics. The news topics of your choice. For example: topics=technology will filter for articles that write about the technology sector; topics=technology,ipo will filter for articles that simultaneously cover technology and IPO in their content."
        - name: time_from
          in: query
          required: false
          schema:
            type: string
          description: Start time for filtering articles, in YYYYMMDDTHHMM format
        - name: time_to
          in: query
          required: false
          schema:
            type: string
          description: End time for filtering articles, in YYYYMMDDTHHMM format
        - name: sort
          in: query
          required: false
          schema:
            type: string
            enum: [LATEST, EARLIEST, RELEVANCE]
            default: LATEST
          description: Sort order of the articles
        - name: limit
          in: query
          required: false
          schema:
            type: integer
            maximum: 1000
            default: 50
          description: Maximum number of articles to return
        - name: apikey
          in: query
          required: true
          schema:
            type: string
          description: Your API key
      security:
        - ApiKeyAuth: []
      responses:
        '200':
          description: A list of news articles matching the criteria
          content:
            application/json:
              schema:
                type: object
                properties:
                  feed:
                    type: array
                    items:
                      $ref: '#/components/schemas/NewsItem'
components:
  schemas:
    NewsItem:
      type: object
      properties:
        title:
          type: string
        url:
          type: string
        time_published:
          type: string
  securitySchemes:
    ApiKeyAuth:
      type: apiKey
      in: query
      name: apikey
`

func TestValidateOperationIDs(t *testing.T) {
	c := &ChainStrategy{}

	tool := &types.Tool{
		ToolType: types.ToolTypeAPI,
		Config: types.ToolConfig{
			API: &types.ToolAPIConfig{
				Schema: alphaVantageAPI,
				URL:    "https://www.alphavantage.co",
			},
		},
	}

	err := ValidateTool("test-user-id", &types.AssistantConfig{}, tool, nil, c, false)
	require.NoError(t, err)

	// Check api actions
	require.Equal(t, 1, len(tool.Config.API.Actions))
	require.Equal(t, "getMarketNewsAndSentimentData", tool.Config.API.Actions[0].Name)
	require.Equal(t, "GET", tool.Config.API.Actions[0].Method)
	require.Equal(t, "/query", tool.Config.API.Actions[0].Path)
	require.Equal(t, "Get market news and sentiment data", tool.Config.API.Actions[0].Description)

}
