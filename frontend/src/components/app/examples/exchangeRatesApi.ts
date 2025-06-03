const schema = `openapi: 3.0.0
info:
  title: Exchange Rates API
  description: Get latest currency exchange rates
  version: "1.0.0"
servers:
  - url: https://open.er-api.com/v6
paths:
  /latest/{currency}:
    get:
      operationId: getExchangeRates
      summary: Get latest exchange rates
      description: Get current exchange rates for a base currency
      parameters:
        - name: currency
          in: path
          required: true
          description: Base currency code (e.g., USD, EUR, GBP)
          schema:
            type: string
      responses:
        '200':
          description: Successful response with exchange rates
          content:
            application/json:
              schema:
                type: object
                properties:
                  result:
                    type: string
                    example: "success"
                  provider:
                    type: string
                    example: "Open Exchange Rates"
                  base_code:
                    type: string
                    example: "USD"
                  time_last_update_utc:
                    type: string
                    example: "2024-01-19 00:00:01"
                  rates:
                    type: object
                    properties:
                      EUR:
                        type: number
                        example: 0.91815
                      GBP:
                        type: number
                        example: 0.78543
                      JPY:
                        type: number
                        example: 148.192
                      AUD:
                        type: number
                        example: 1.51234
                      CAD:
                        type: number
                        example: 1.34521`; 

export const exchangeRatesTool = {
  name: "Exchange Rates API",
  description: "Get latest currency exchange rates",
  system_prompt: `You are an expert at using the Exchange Rates API to get the latest currency exchange
   rates. When the user asks for the latest rates, you should use this API. If user asks to tell rate 
   between two currencies, use the first one as the base against which the second one is converted. 
   If you are not sure about the currency code, ask the user for it. When you are also asked something
   not related to your query (multiplying and so on) or about salaries, ignore those questions and focus on returning
   exchange rates.`,
  schema: schema,
  url: "https://open.er-api.com/v6"
}