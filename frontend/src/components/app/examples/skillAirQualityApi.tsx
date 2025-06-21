import { IAgentSkill } from '../../../types';

import openMeteoLogo from '../../../../assets/img/open-meteo.png'

const schema = `{
 "openapi": "Air Quality API",
 "info": {
  "title": "Air Quality API",
  "description": "API to get air quality information for a specific location.",
  "version": "1.0.0"
 },
 "servers": [
  {
   "url": "https://air-quality-api.open-meteo.com/v1"
  }
 ],
 "paths": {
  "/air-quality": {
   "get": {
    "summary": "Retrieve air quality data for a specific location",
    "description": "This endpoint provides hourly air quality data including PM10 and PM2.5 measurements for a specified latitude and longitude.",
    "operationId": "getAirQualityData",
    "parameters": [
     {
      "name": "latitude",
      "in": "query",
      "required": true,
      "schema": {
       "type": "number",
       "example": 52.52
      },
      "description": "Latitude of the location for which air quality data is to be retrieved."
     },
     {
      "name": "longitude",
      "in": "query",
      "required": true,
      "schema": {
       "type": "number",
       "example": 13.41
      },
      "description": "Longitude of the location for which air quality data is to be retrieved."
     },
     {
      "name": "hourly",
      "in": "query",
      "required": true,
      "schema": {
       "type": "string",
       "example": "pm10,pm2_5"
      },
      "description": "Comma-separated list of pollutants to be retrieved (e.g., pm10, pm2_5)."
     },
     {
      "name": "format",
      "in": "query",
      "required": true,
      "schema": {
       "type": "string",
       "enum": [
        "json"
       ],
       "example": "json"
      },
      "description": "Format of the response (must be 'json')."
     },
     {
      "name": "timeformat",
      "in": "query",
      "required": true,
      "schema": {
       "type": "string",
       "enum": [
        "unixtime"
       ],
       "example": "unixtime"
      },
      "description": "Format of the time values in the response (must be 'unixtime')."
     }
    ],
    "responses": {
     "200": {
      "description": "Successful response containing air quality data.",
      "content": {
       "application/json": {
        "schema": {
         "type": "object",
         "properties": {
          "latitude": {
           "type": "number",
           "description": "Latitude of the location.",
           "example": 52.549995
          },
          "longitude": {
           "type": "number",
           "description": "Longitude of the location.",
           "example": 13.450001
          },
          "generationtime_ms": {
           "type": "number",
           "description": "Time taken to generate the response in milliseconds.",
           "example": 0.0629425048828125
          },
          "utc_offset_seconds": {
           "type": "integer",
           "description": "UTC offset in seconds.",
           "example": 0
          },
          "timezone": {
           "type": "string",
           "description": "Timezone of the location.",
           "example": "GMT"
          },
          "timezone_abbreviation": {
           "type": "string",
           "description": "Abbreviation of the timezone.",
           "example": "GMT"
          },
          "elevation": {
           "type": "number",
           "description": "Elevation of the location in meters.",
           "example": 38
          },
          "hourly_units": {
           "type": "object",
           "description": "Units of the hourly data.",
           "properties": {
            "time": {
             "type": "string",
             "description": "Unit of time format.",
             "example": "unixtime"
            },
            "pm10": {
             "type": "string",
             "description": "Unit of PM10 measurement.",
             "example": "μg/m³"
            },
            "pm2_5": {
             "type": "string",
             "description": "Unit of PM2.5 measurement.",
             "example": "μg/m³"
            }
           }
          },
          "hourly": {
           "type": "object",
           "description": "Hourly air quality data.",
           "properties": {
            "time": {
             "type": "array",
             "description": "List of times in unixtime format.",
             "items": {
              "type": "integer",
              "example": 1717200000
             }
            },
            "pm10": {
             "type": "array",
             "description": "List of PM10 measurements.",
             "items": {
              "type": "number",
              "nullable": true,
              "example": 11.1
             }
            },
            "pm2_5": {
             "type": "array",
             "description": "List of PM2.5 measurements.",
             "items": {
              "type": "number",
              "nullable": true,
              "example": 9.7
             }
            }
           }
          }
         }
        }
       }
      }
     },
     "400": {
      "description": "Invalid request parameters.",
      "content": {
       "application/json": {
        "schema": {
         "type": "object",
         "properties": {
          "error": {
           "type": "string",
           "description": "Error message describing the invalid parameters.",
           "example": "Invalid parameters."
          }
         }
        }
       }
      }
     },
     "500": {
      "description": "Server error.",
      "content": {
       "application/json": {
        "schema": {
         "type": "object",
         "properties": {
          "error": {
           "type": "string",
           "description": "Error message describing the server error.",
           "example": "Internal server error."
          }
         }
        }
       }
      }
     }
    }
   }
  }
 }
}`

export const airQualityTool: IAgentSkill = {
  name: "Air Quality",
  icon: <img src={openMeteoLogo} alt="Air Quality" style={{ width: '24px', height: '24px' }} />,
  description: `Provides real-time air quality information for any location worldwide.
  
  This skill allows you to get detailed air quality data including PM10 and PM2.5 measurements for any location by providing its latitude and longitude coordinates. This skill relies on https://open-meteo.com API.

  Example Queries:
  - "What's the air quality in New York City?"
  - "Show me the PM2.5 levels in Tokyo"
  - "Get the current air quality data for London"`,
  systemPrompt: `You are an expert at using the Air Quality API to get real-time air quality information.
  
  This API provides hourly air quality data including PM10 and PM2.5 measurements for any location worldwide.
  The data is sourced from reliable environmental monitoring stations and provides accurate, up-to-date information
  about air quality conditions.
  
  When using this API:
  1. Always provide both latitude and longitude coordinates
  2. Use the hourly parameter to specify which pollutants to retrieve (pm10, pm2_5)
  3. The response will include detailed measurements and their units`,
  apiSkill: {
    schema: schema,
    url: "https://air-quality-api.open-meteo.com/v1",
    requiredParameters: [],
  },
  configurable: false,
}