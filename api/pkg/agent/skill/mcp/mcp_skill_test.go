package mcp

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/helixml/helix/api/pkg/util/jsonschema"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
)

func loadTools(t *testing.T, filename string) []mcp.Tool {
	toolsBts, err := os.ReadFile(filename)
	require.NoError(t, err)

	var tools []mcp.Tool

	err = json.Unmarshal(toolsBts, &tools)
	require.NoError(t, err)
	return tools
}

func Test_buildParameters(t *testing.T) {
	// Test the case with no properties
	t.Run("pg_list_schemas", func(t *testing.T) {
		pgListSchemas := loadTools(t, "testdata/pg_list_schemas.json")

		schema := buildParameters(pgListSchemas[0].InputSchema)

		// Verify the schema
		require.Equal(t, len(schema.Properties), 0)
	})

	t.Run("pg_list_objects", func(t *testing.T) {
		pgListObjects := loadTools(t, "testdata/pg_list_objects.json")

		schema := buildParameters(pgListObjects[0].InputSchema)

		// Verify the schema
		require.Equal(t, len(schema.Properties), 2)

		// Verify the properties
		require.Equal(t, schema.Properties["object_type"].Type, jsonschema.String)
		require.Equal(t, schema.Properties["schema_name"].Type, jsonschema.String)
	})
}
