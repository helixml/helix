export const coindeskSchema = `
openapi: 3.0.0
info:
  title: CoinDesk Bitcoin Price Index API
  description: This service provides current price indexes for Bitcoin in various currencies.
  version: "1.0.0"
servers:
  - url: https://api.coindesk.com/v1
paths:
  /bpi/currentprice.json:
    get:
      operationId: coindeskGetBitcoinCurrentPrice
      summary: Get current Bitcoin price index
      description: Retrieves the current Bitcoin price index in various currencies without requiring any parameters.
      responses:
        '200':
          description: A successful response providing the current Bitcoin prices.
          content:
            application/json:
              schema:
                type: object
                properties:
                  time:
                    type: object
                    properties:
                      updated:
                        type: string
                        example: "May 14, 2024 10:36:11 UTC"
                      updatedISO:
                        type: string
                        format: date-time
                        example: "2024-05-14T10:36:11+00:00"
                      updateduk:
                        type: string
                        example: "May 14, 2024 at 11:36 BST"
                  disclaimer:
                    type: string
                    example: "This data was produced from the CoinDesk Bitcoin Price Index (USD). Non-USD currency data converted using hourly conversion rate from openexchangerates.org"
                  chartName:
                    type: string
                    example: "Bitcoin"
                  bpi:
                    type: object
                    properties:
                      USD:
                        $ref: '#/components/schemas/Currency'
                      GBP:
                        $ref: '#/components/schemas/Currency'
                      EUR:
                        $ref: '#/components/schemas/Currency'
components:
  schemas:
    Currency:
      type: object
      properties:
        code:
          type: string
          example: "USD"
        symbol:
          type: string
          example: "&#36;"
        rate:
          type: string
          example: "61,655.335"
        description:
          type: string
          example: "United States Dollar"
        rate_float:
          type: number
          format: float
          example: 61655.3349
`;