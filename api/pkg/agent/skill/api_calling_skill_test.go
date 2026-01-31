package skill

import (
	"testing"

	"github.com/helixml/helix/api/pkg/tools"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/helix/api/pkg/util/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDirectAPICallingSkills(t *testing.T) {
	// Create a mock tool with multiple API actions
	tool := &types.Tool{
		ID:          "test-api-tool",
		Name:        "Test API",
		Description: "A test API tool",
		Config: types.ToolConfig{
			API: &types.ToolAPIConfig{
				Actions: []*types.ToolAPIAction{
					{
						Name:        "listJobVacancies",
						Description: "List job vacancies from the API",
					},
					{
						Name:        "getJobDetails",
						Description: "Get detailed information about a specific job",
					},
				},
				Schema: `{
					"paths": {
						"/vacancies": {
							"get": {
								"summary": "listJobVacancies",
								"description": "List job vacancies from the API",
								"parameters": []
							}
						},
						"/jobs/{id}": {
							"get": {
								"summary": "getJobDetails", 
								"description": "Get detailed information about a specific job",
								"parameters": []
							}
						}
					}
				}`,
			},
		},
	}

	// Test the new direct skills creation
	skills := NewDirectAPICallingSkills(nil, nil, tool)

	// Verify we get one skill per API action
	require.Len(t, skills, 2, "Should create one skill per API action")

	// Verify first skill (listJobVacancies)
	skill1 := skills[0]
	assert.Equal(t, "listJobVacancies", skill1.Name)
	assert.Equal(t, "List job vacancies from the API", skill1.Description)
	assert.True(t, skill1.Direct, "API skills should be direct to avoid inner skill loop")
	assert.Len(t, skill1.Tools, 1, "Each direct skill should have exactly one tool")

	// Verify parameters are exposed to outer agent loop (even if empty from mock schema)
	assert.NotNil(t, skill1.Parameters, "Direct skills should expose parameters to outer agent")

	// Verify second skill (getJobDetails)
	skill2 := skills[1]
	assert.Equal(t, "getJobDetails", skill2.Name)
	assert.Equal(t, "Get detailed information about a specific job", skill2.Description)
	assert.True(t, skill2.Direct, "API skills should be direct to avoid inner skill loop")
	assert.Len(t, skill2.Tools, 1, "Each direct skill should have exactly one tool")

	// Verify parameters are exposed to outer agent loop (even if empty from mock schema)
	assert.NotNil(t, skill2.Parameters, "Direct skills should expose parameters to outer agent")

	// Verify tool names match skill names
	assert.Equal(t, "listJobVacancies", skill1.Tools[0].Name())
	assert.Equal(t, "getJobDetails", skill2.Tools[0].Name())
}

func TestNewAPICallingSkill_WithPlanning(t *testing.T) {
	// Test the old function still works for backward compatibility
	tool := &types.Tool{
		ID:          "test-api-tool",
		Name:        "Test API",
		Description: "A test API tool",
		Config: types.ToolConfig{
			API: &types.ToolAPIConfig{
				Actions: []*types.ToolAPIAction{
					{
						Name:        "listJobVacancies",
						Description: "List job vacancies from the API",
					},
				},
				Schema: `{
					"paths": {
						"/vacancies": {
							"get": {
								"summary": "listJobVacancies",
								"description": "List job vacancies from the API",
								"parameters": []
							}
						}
					}
				}`,
			},
		},
	}

	// Test the old function
	skill := NewAPICallingSkillWithReasoning(nil, nil, tool)

	// Verify it creates a single skill with multiple tools (old behavior)
	assert.Equal(t, "Test_API", skill.Name)
	assert.Equal(t, "A test API tool", skill.Description)
	assert.False(t, skill.Direct, "Old API skills should not be direct (backward compatibility)")
	assert.Len(t, skill.Tools, 1, "Should have one tool per action")
}

func TestBuildParametersSchema(t *testing.T) {
	// This is a unit test for the helper function
	// Note: This function uses tools.Parameter which would require mocking
	// For now, testing that empty parameters return valid object schema
	emptyParams := []*tools.Parameter{}
	schema := buildParametersSchema(emptyParams)

	// Should return valid object schema even for no parameters (required for Anthropic API)
	expected := jsonschema.Definition{
		Type:       jsonschema.Object,
		Properties: map[string]jsonschema.Definition{},
		Required:   []string{},
	}
	assert.Equal(t, expected, schema, "Empty parameters should return valid object schema with type field")
}
