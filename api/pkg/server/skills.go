package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/agent"
	"github.com/helixml/helix/api/pkg/agent/skill/mcp"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// handleListSkills returns the list of available YAML skills
// listSkills godoc
// @Summary List YAML skills
// @Description List all available YAML-based skills
// @Tags    skills
// @Param   category query string false "Filter by category"
// @Param   provider query string false "Filter by OAuth provider"
// @Success 200 {object} types.SkillsListResponse
// @Router /api/v1/skills [get]
// @Security BearerAuth
func (s *HelixAPIServer) handleListSkills(_ http.ResponseWriter, r *http.Request) (*types.SkillsListResponse, error) {
	user := getRequestUser(r)
	if user == nil {
		return nil, fmt.Errorf("unauthorized")
	}

	// Get query parameters
	category := r.URL.Query().Get("category")
	provider := r.URL.Query().Get("provider")

	log.Debug().
		Str("user_id", user.ID).
		Str("category", category).
		Str("provider", provider).
		Msg("Listing skills")

	var skills []*types.SkillDefinition

	// Filter by category and provider if specified
	if category != "" && provider != "" {
		// Need to manually filter for both
		allSkills := s.skillManager.ListSkills()
		for _, skill := range allSkills {
			if skill.Category == category && skill.OAuthProvider == provider {
				skills = append(skills, skill)
			}
		}
	} else if category != "" {
		skills = s.skillManager.ListSkillsByCategory(category)
	} else if provider != "" {
		skills = s.skillManager.ListSkillsByProvider(provider)
	} else {
		skills = s.skillManager.ListSkills()
	}

	// Convert to response format
	skillDefinitions := make([]types.SkillDefinition, len(skills))
	for i, skill := range skills {
		skillDefinitions[i] = *skill
	}

	response := &types.SkillsListResponse{
		Skills: skillDefinitions,
		Count:  len(skillDefinitions),
	}

	log.Info().
		Int("count", len(skillDefinitions)).
		Str("user_id", user.ID).
		Msg("Retrieved skills")

	return response, nil
}

// handleGetSkill returns a specific skill by ID
// getSkill godoc
// @Summary Get a skill by ID
// @Description Get details of a specific YAML skill
// @Tags    skills
// @Param   id path string true "Skill ID"
// @Success 200 {object} types.SkillDefinition
// @Router /api/v1/skills/{id} [get]
// @Security BearerAuth
func (s *HelixAPIServer) handleGetSkill(_ http.ResponseWriter, r *http.Request) (*types.SkillDefinition, error) {
	user := getRequestUser(r)
	if user == nil {
		return nil, fmt.Errorf("unauthorized")
	}

	// Extract skill ID from URL
	vars := mux.Vars(r)
	skillID := vars["id"]

	log.Debug().
		Str("user_id", user.ID).
		Str("skill_id", skillID).
		Msg("Getting skill")

	skill, err := s.skillManager.GetSkill(skillID)
	if err != nil {
		log.Error().Err(err).
			Str("skill_id", skillID).
			Str("user_id", user.ID).
			Msg("Failed to get skill")
		return nil, fmt.Errorf("skill not found: %w", err)
	}

	log.Info().
		Str("skill_id", skillID).
		Str("user_id", user.ID).
		Msg("Retrieved skill")

	return skill, nil
}

// handleReloadSkills reloads all skills from the filesystem
// reloadSkills godoc
// @Summary Reload skills
// @Description Reload all YAML skills from the filesystem
// @Tags    skills
// @Success 200 {object} map[string]string
// @Router /api/v1/skills/reload [post]
// @Security BearerAuth
func (s *HelixAPIServer) handleReloadSkills(_ http.ResponseWriter, r *http.Request) (map[string]string, error) {
	user := getRequestUser(r)
	if user == nil {
		return nil, fmt.Errorf("unauthorized")
	}

	// Only admin users can reload skills
	if !user.Admin {
		return nil, fmt.Errorf("unauthorized: admin access required")
	}

	log.Info().
		Str("user_id", user.ID).
		Msg("Reloading skills")

	// Reload skills
	err := s.skillManager.ReloadSkills(r.Context())
	if err != nil {
		log.Error().Err(err).Msg("Failed to reload skills")
		return nil, fmt.Errorf("failed to reload skills: %w", err)
	}

	skillCount := s.skillManager.GetSkillCount()

	log.Info().
		Int("skill_count", skillCount).
		Str("user_id", user.ID).
		Msg("Skills reloaded successfully")

	return map[string]string{
		"status":  "success",
		"message": fmt.Sprintf("Successfully reloaded %d skills", skillCount),
	}, nil
}

// handleValidateMcpSkill - validates MCP skill configuration
// validateMcpSkill godoc
// @Summary Validate MCP skill configuration
// @Description Validate MCP skill configuration
// @Tags    skills
// @Param   request body types.AssistantMCP true "MCP skill configuration"
// @Success 200 {object} types.ToolMCPClientConfig
// @Router /api/v1/skills/validate [post]
// @Security BearerAuth
func (s *HelixAPIServer) handleValidateMcpSkill(_ http.ResponseWriter, r *http.Request) (*types.ToolMCPClientConfig, error) {
	user := getRequestUser(r)
	if user == nil {
		return nil, fmt.Errorf("unauthorized")
	}

	var config types.AssistantMCP
	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		return nil, fmt.Errorf("failed to decode request body: %w", err)
	}

	resp, err := mcp.InitializeMCPClientSkill(context.Background(), s.mcpClientGetter, agent.Meta{
		UserID: user.ID,
	}, s.oauthManager, &config)
	if err != nil {
		log.Error().Err(err).Msg("failed to initialize MCP client skill")
	}

	return resp, err
}
