import { IAgentSkill } from '../../../types';

import exchangeRatesLogo from '../../../../assets/img/exchange-rates-api.png'

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

export const exchangeRatesSkill: IAgentSkill = {
  name: "Currency Exchange Rates",
  icon: <img src={exchangeRatesLogo} alt="Exchange Rates API" style={{ width: '24px', height: '24px' }} />,
  description: `Get latest currency exchange rates.
  
  Example Queries:
  - "What is the exchange rate for EUR to USD?"
  - "What is the exchange rate for EUR to GBP?"
  - "What is the exchange rate for EUR to JPY?"
  - "What is the exchange rate for EUR to AUD?"
  `,
  systemPrompt: `You are an expert at using the Exchange Rates API to get the latest currency exchange
   rates. When the user asks for the latest rates, you should use this API. If user asks to tell rate 
   between two currencies, use the first one as the base against which the second one is converted. 
   If you are not sure about the currency code, ask the user for it. When you are also asked something
   not related to your query (multiplying and so on) or about salaries, ignore those questions and focus on returning
   exchange rates
  `,
  apiSkill: {
    schema: schema,
    url: "https://open.er-api.com/v6",
    requiredParameters: [],
  },
  configurable: false, // Nothing needs to be configured for this API
}