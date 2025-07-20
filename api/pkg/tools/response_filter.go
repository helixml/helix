package tools

import (
	"fmt"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/helixml/helix/api/pkg/types"
)

func removeUnknownKeys(tool *types.Tool, action string, responseStatus string, responseBody []byte) ([]byte, error) {
	loader := openapi3.NewLoader()

	schema, err := loader.LoadFromData([]byte(tool.Config.API.Schema))
	if err != nil {
		return nil, fmt.Errorf("failed to load openapi spec: %w", err)
	}

	var responseBodySchema *openapi3.SchemaRef

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

}
