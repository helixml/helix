const schema = `{
 "openapi": "3.0.1",
 "info": {
  "title": "Climate API",
  "description": "API to get historical climate data for a specific location.",
  "version": "1.0.0"
 },
 "servers": [
  {
   "url": "https://climate-api.open-meteo.com/v1"
  }
 ],
 "paths": {
  "/climate": {
   "get": {
    "summary": "Retrieve historical climate data for a specific location",
    "description": "This endpoint provides daily historical climate data including maximum temperature measurements from multiple climate models for a specified latitude and longitude.",
    "operationId": "getClimateData",
    "parameters": [
     {
      "name": "latitude",
      "in": "query",
      "required": true,
      "schema": {
       "type": "number",
       "example": 52.52
      },
      "description": "Latitude of the location for which climate data is to be retrieved."
     },
     {
      "name": "longitude",
      "in": "query",
      "required": true,
      "schema": {
       "type": "number",
       "example": 13.41
      },
      "description": "Longitude of the location for which climate data is to be retrieved."
     },
     {
      "name": "start_date",
      "in": "query",
      "required": true,
      "schema": {
       "type": "string",
       "format": "date",
       "example": "1999-01-01T00:00:00.000Z"
      },
      "description": "Start date for the climate data (YYYY-MM-DD)."
     },
     {
      "name": "end_date",
      "in": "query",
      "required": true,
      "schema": {
       "type": "string",
       "format": "date",
       "example": "2000-01-01T00:00:00.000Z"
      },
      "description": "End date for the climate data (YYYY-MM-DD)."
     },
     {
      "name": "models",
      "in": "query",
      "required": true,
      "schema": {
       "type": "string",
       "example": "CMCC_CM2_VHR4,FGOALS_f3_H,HiRAM_SIT_HR,MRI_AGCM3_2_S,EC_Earth3P_HR,MPI_ESM1_2_XR,NICAM16_8S"
      },
      "description": "Comma-separated list of climate models to be retrieved, options -  CMCC_CM2_VHR4,FGOALS_f3_H,HiRAM_SIT_HR,MRI_AGCM3_2_S,EC_Earth3P_HR,MPI_ESM1_2_XR,NICAM16_8S"
     },
     {
      "name": "daily",
      "in": "query",
      "required": true,
      "schema": {
       "type": "string",
       "example": "temperature_2m_max"
      },
      "description": "Comma-separated list of daily data to be retrieved (e.g., temperature_2m_max)."
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
      "description": "Successful response containing climate data.",
      "content": {
       "application/json": {
        "schema": {
         "type": "object",
         "properties": {
          "latitude": {
           "type": "number",
           "description": "Latitude of the location.",
           "example": 52.5
          },
          "longitude": {
           "type": "number",
           "description": "Longitude of the location.",
           "example": 13.400009
          },
          "generationtime_ms": {
           "type": "number",
           "description": "Time taken to generate the response in milliseconds.",
           "example": 0.20802021026611328
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
          "daily_units": {
           "type": "object",
           "description": "Units of the daily data.",
           "properties": {
            "time": {
             "type": "string",
             "description": "Unit of time format.",
             "example": "unixtime"
            },
            "temperature_2m_max_CMCC_CM2_VHR4": {
             "type": "string",
             "description": "Unit of maximum temperature for CMCC_CM2_VHR4 model.",
             "example": "°C"
            },
            "temperature_2m_max_FGOALS_f3_H": {
             "type": "string",
             "description": "Unit of maximum temperature for FGOALS_f3_H model.",
             "example": "°C"
            },
            "temperature_2m_max_HiRAM_SIT_HR": {
             "type": "string",
             "description": "Unit of maximum temperature for HiRAM_SIT_HR model.",
             "example": "°C"
            },
            "temperature_2m_max_MRI_AGCM3_2_S": {
             "type": "string",
             "description": "Unit of maximum temperature for MRI_AGCM3_2_S model.",
             "example": "°C"
            },
            "temperature_2m_max_EC_Earth3P_HR": {
             "type": "string",
             "description": "Unit of maximum temperature for EC_Earth3P_HR model.",
             "example": "°C"
            },
            "temperature_2m_max_MPI_ESM1_2_XR": {
             "type": "string",
             "description": "Unit of maximum temperature for MPI_ESM1_2_XR model.",
             "example": "°C"
            },
            "temperature_2m_max_NICAM16_8S": {
             "type": "string",
             "description": "Unit of maximum temperature for NICAM16_8S model.",
             "example": "°C"
            }
           }
          },
          "daily": {
           "type": "object",
           "description": "Daily climate data.",
           "properties": {
            "time": {
             "type": "array",
             "description": "List of times in unixtime format.",
             "items": {
              "type": "integer",
              "example": 915148800
             }
            },
            "temperature_2m_max_CMCC_CM2_VHR4": {
             "type": "array",
             "description": "List of maximum temperature measurements from CMCC_CM2_VHR4 model.",
             "items": {
              "type": "number",
              "nullable": true,
              "example": 11
             }
            },
            "temperature_2m_max_FGOALS_f3_H": {
             "type": "array",
             "description": "List of maximum temperature measurements from FGOALS_f3_H model.",
             "items": {
              "type": "number",
              "nullable": true,
              "example": -2.6
             }
            },
            "temperature_2m_max_HiRAM_SIT_HR": {
             "type": "array",
             "description": "List of maximum temperature measurements from HiRAM_SIT_HR model.",
             "items": {
              "type": "number",
              "nullable": true,
              "example": 7.6
             }
            },
            "temperature_2m_max_MRI_AGCM3_2_S": {
             "type": "array",
             "description": "List of maximum temperature measurements from MRI_AGCM3_2_S model.",
             "items": {
              "type": "number",
              "nullable": true,
              "example": -3.2
             }
            },
            "temperature_2m_max_EC_Earth3P_HR": {
             "type": "array",
             "description": "List of maximum temperature measurements from EC_Earth3P_HR model.",
             "items": {
              "type": "number",
              "nullable": true,
              "example": 3.8
             }
            },
            "temperature_2m_max_MPI_ESM1_2_XR": {
             "type": "array",
             "description": "List of maximum temperature measurements from MPI_ESM1_2_XR model.",
             "items": {
              "type": "number",
              "nullable": true,
              "example": 5.3
             }
            },
            "temperature_2m_max_NICAM16_8S": {
             "type": "array",
             "description": "List of maximum temperature measurements from NICAM16_8S model.",
             "items": {
              "type": "number",
              "nullable": true,
              "example": -0.4
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

export const climateTool = {
  name: "Climate API",
  description: "Get historical climate data for a specific location",
  system_prompt: "You are an expert at using the Climate API to get historical climate data for a specific location.",
  schema: schema,
  url: "https://climate-api.open-meteo.com/v1"
}