package tools

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/helixml/helix/api/pkg/types"
)

func removeUnknownKeys(tool *types.Tool, action string, statusCode int, responseBody []byte) ([]byte, error) {
	loader := openapi3.NewLoader()

	schema, err := loader.LoadFromData([]byte(tool.Config.API.Schema))
	if err != nil {
		return nil, fmt.Errorf("failed to load openapi spec: %w", err)
	}

	var responseBodySchema *openapi3.SchemaRef

	responseStatus := strconv.Itoa(statusCode)

	for _, pathItem := range schema.Paths.Map() {
		for _, op := range pathItem.Operations() {
			if op.OperationID != action {
				continue
			}

			if op.Responses == nil {
				continue
			}

			// Get response schema
			if op.Responses != nil {
				for status, response := range op.Responses.Map() {
					if responseStatus == status {
						if response.Value.Content != nil {
							responseBodySchema = response.Value.Content["application/json"].Schema
						}
					}
				}
			}
		}
	}

	// If no schema found, return the original response body
	if responseBodySchema == nil {
		return responseBody, fmt.Errorf("no schema found for action %s", action)
	}

	// Decode the response body
	var responseData interface{}
	if err := json.Unmarshal(responseBody, &responseData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response body: %w", err)
	}

	// Filter the response data based on the schema
	filteredData, err := filterResponseData(responseData, responseBodySchema)
	if err != nil {
		return nil, fmt.Errorf("failed to filter response data: %w", err)
	}

	// Encode the filtered data back to JSON
	filteredBody, err := json.MarshalIndent(filteredData, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal filtered response: %w", err)
	}

	return filteredBody, nil
}

// filterResponseData recursively filters the response data based on the OpenAPI schema
func filterResponseData(data interface{}, schema *openapi3.SchemaRef) (interface{}, error) {
	if schema == nil || schema.Value == nil {
		return data, nil
	}

	switch v := data.(type) {
	case map[string]interface{}:
		return filterObject(v, schema)
	case []interface{}:
		return filterArray(v, schema)
	default:
		return data, nil
	}
}

// filterObject filters an object based on the schema properties
func filterObject(obj map[string]interface{}, schema *openapi3.SchemaRef) (map[string]interface{}, error) {
	if schema.Value == nil || schema.Value.Properties == nil {
		return obj, nil
	}

	filtered := make(map[string]interface{})

	for key, value := range obj {
		// Check if the key exists in the schema
		if propSchema, exists := schema.Value.Properties[key]; exists {
			// Recursively filter the value based on its schema
			filteredValue, err := filterResponseData(value, propSchema)
			if err != nil {
				return nil, fmt.Errorf("failed to filter property %s: %w", key, err)
			}
			filtered[key] = filteredValue
		}
		// If the key doesn't exist in the schema, it's discarded
	}

	return filtered, nil
}

// filterArray filters an array based on the schema items
func filterArray(arr []interface{}, schema *openapi3.SchemaRef) ([]interface{}, error) {
	if schema.Value == nil || schema.Value.Items == nil {
		return arr, nil
	}

	filtered := make([]interface{}, 0, len(arr))

	for i, item := range arr {
		// Filter each item based on the array's item schema
		filteredItem, err := filterResponseData(item, schema.Value.Items)
		if err != nil {
			return nil, fmt.Errorf("failed to filter array item %d: %w", i, err)
		}
		filtered = append(filtered, filteredItem)
	}

	return filtered, nil
}
