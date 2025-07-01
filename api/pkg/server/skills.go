package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// setupSkillRoutes configures the skill-related routes
func (s *HelixAPIServer) setupSkillRoutes(r *mux.Router) {
	// Skill management routes
	r.HandleFunc("/skills", system.DefaultWrapper(s.handleListSkills)).Methods("GET")
	r.HandleFunc("/skills/{id}", system.DefaultWrapper(s.handleGetSkill)).Methods("GET")
	r.HandleFunc("/skills/{id}/test", system.DefaultWrapper(s.handleTestSkill)).Methods("POST")
	r.HandleFunc("/skills/reload", system.DefaultWrapper(s.handleReloadSkills)).Methods("POST")
}

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

// handleTestSkill tests a skill by making an API call
// testSkill godoc
// @Summary Test a skill
// @Description Test a skill by executing one of its operations
// @Tags    skills
// @Param   id path string true "Skill ID"
// @Param   request body types.SkillTestRequest true "Test request"
// @Success 200 {object} types.SkillTestResponse
// @Router /api/v1/skills/{id}/test [post]
// @Security BearerAuth
func (s *HelixAPIServer) handleTestSkill(_ http.ResponseWriter, r *http.Request) (*types.SkillTestResponse, error) {
	user := getRequestUser(r)
	if user == nil {
		return nil, fmt.Errorf("unauthorized")
	}

	// Only admin users can test skills
	if !user.Admin {
		return nil, fmt.Errorf("unauthorized: admin access required")
	}

	// Extract skill ID from URL
	vars := mux.Vars(r)
	skillID := vars["id"]

	// Parse test request
	var testRequest types.SkillTestRequest
	if err := json.NewDecoder(r.Body).Decode(&testRequest); err != nil {
		log.Error().Err(err).Msg("Failed to decode test request")
		return nil, fmt.Errorf("error decoding request: %w", err)
	}

	testRequest.SkillID = skillID

	log.Info().
		Str("skill_id", skillID).
		Str("operation", testRequest.Operation).
		Str("user_id", user.ID).
		Msg("Testing skill")

	// Get the skill
	_, err := s.skillManager.GetSkill(skillID)
	if err != nil {
		return nil, fmt.Errorf("skill not found: %w", err)
	}

	// TODO: Implement skill testing logic here
	// This would involve:
	// 1. Parsing the OpenAPI schema
	// 2. Finding the specified operation
	// 3. Getting OAuth token for the user
	// 4. Making the API call
	// 5. Returning the response

	// For now, return a placeholder response
	response := &types.SkillTestResponse{
		Success:    false,
		StatusCode: 501,
		Response:   map[string]interface{}{"message": "Skill testing not yet implemented"},
		Error:      "Testing functionality is not yet implemented",
		Duration:   0,
	}

	log.Info().
		Str("skill_id", skillID).
		Str("operation", testRequest.Operation).
		Bool("success", response.Success).
		Msg("Skill test completed")

	return response, nil
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

	err := s.skillManager.ReloadSkills(r.Context())
	if err != nil {
		log.Error().Err(err).
			Str("user_id", user.ID).
			Msg("Failed to reload skills")
		return nil, fmt.Errorf("failed to reload skills: %w", err)
	}

	skillCount := s.skillManager.GetSkillCount()

	log.Info().
		Int("skill_count", skillCount).
		Str("user_id", user.ID).
		Msg("Successfully reloaded skills")

	return map[string]string{
		"message": "Skills reloaded successfully",
		"count":   strconv.Itoa(skillCount),
	}, nil
}
