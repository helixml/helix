import { IAgentSkill } from '../../../types';

import alphavantageLogo from '../../../../assets/img/alphavantage.png'

const schema = `openapi: 3.0.3
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
`

export const alphaVantageTool: IAgentSkill = {
  name: "Market News",
  icon: <img src={alphavantageLogo} alt="Alpha Vantage" style={{ width: '24px', height: '24px' }} />,
  description: `Provides up-to-date financial information from Alpha Vantage.
  
  This skill empowers you to stay on top of the latest market trends by fetching live and historical market news and sentiment data directly within your workspace.

  Example Queries:
  - "Give me market news about Apple stock"
  - "Show me the latest news about Bitcoin"
  - "Give me news about the IPO and earnings in the finance sector from last week"`,
  systemPrompt: `You are an expert at using the Alpha Vantage API to get the latest market news and sentiment data.
  
  This API returns live and historical market news & sentiment data from a large & growing selection of premier news 
  outlets around the world, covering stocks, cryptocurrencies, forex, and a wide range of topics such as fiscal policy, 
  mergers & acquisitions, IPOs, etc. This API, combined with our core stock API, fundamental data, and technical indicator APIs, 
  can provide you with a 360-degree view of the financial market and the broader economy.
  
  function to be used: "NEWS_SENTIMENT", then you can either use the tickers or topics to search. DO NOT USE BOTH AT THE SAME TIME.
  Be super careful when using the tickers parameter, it's very easy to get the wrong ticker.`,
  apiSkill: {
    schema: schema,
    url: "https://www.alphavantage.co",
    requiredParameters: [{
      name: 'apikey',
      description: 'Your free API key from https://www.alphavantage.co/support/#api-key',
      type: 'query',
      required: true,
    }],
  },
  configurable: false, // Only API key is required from the user, description and name are not configurable  
}