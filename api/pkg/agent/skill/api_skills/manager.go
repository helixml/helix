package apiskill

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"strings"
	"sync"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"
)

//go:embed *.yaml
var skillFiles embed.FS

// Manager handles loading and managing YAML-based skills
type Manager struct {
	skills map[string]*types.SkillDefinition
	mutex  sync.RWMutex
}

// NewManager creates a new skill manager
func NewManager() *Manager {
	return &Manager{
		skills: make(map[string]*types.SkillDefinition),
	}
}

// LoadSkills loads all embedded YAML skills
func (m *Manager) LoadSkills(_ context.Context) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Read all YAML files from embedded filesystem
	entries, err := fs.ReadDir(skillFiles, ".")
	if err != nil {
		return fmt.Errorf("failed to read embedded skills directory: %w", err)
	}

	loadedCount := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}

		log.Debug().Str("file", entry.Name()).Msg("Loading skill file")

		data, err := fs.ReadFile(skillFiles, entry.Name())
		if err != nil {
			log.Error().Err(err).Str("file", entry.Name()).Msg("Failed to read skill file")
			continue
		}

		var yamlSkill types.YAMLSkill
		if err := yaml.Unmarshal(data, &yamlSkill); err != nil {
			log.Error().Err(err).Str("file", entry.Name()).Msg("Failed to parse skill YAML")
			continue
		}

		// Convert YAML skill to internal skill definition
		skillDef := &types.SkillDefinition{
			ID:                 yamlSkill.Metadata.Name,
			Name:               yamlSkill.Metadata.Name,
			DisplayName:        yamlSkill.Metadata.DisplayName,
			Description:        yamlSkill.Spec.Description,
			SystemPrompt:       yamlSkill.Spec.SystemPrompt,
			Category:           yamlSkill.Metadata.Category,
			Provider:           yamlSkill.Metadata.Provider,
			Icon:               yamlSkill.Spec.Icon,
			OAuthProvider:      yamlSkill.Spec.OAuth.Provider,
			OAuthScopes:        yamlSkill.Spec.OAuth.Scopes,
			BaseURL:            yamlSkill.Spec.API.BaseURL,
			Headers:            yamlSkill.Spec.API.Headers,
			Schema:             yamlSkill.Spec.API.Schema,
			RequiredParameters: yamlSkill.Spec.API.RequiredParameters,
			Configurable:       yamlSkill.Spec.Configurable,
			SkipUnknownKeys:    yamlSkill.Spec.SkipUnknownKeys,
			TransformOutput:    yamlSkill.Spec.TransformOutput,
			LoadedAt:           time.Now(),
			FilePath:           entry.Name(),
		}

		// Store the skill
		m.skills[skillDef.ID] = skillDef
		loadedCount++
	}

	return nil
}

// GetSkill returns a skill by ID
func (m *Manager) GetSkill(id string) (*types.SkillDefinition, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	skill, exists := m.skills[id]
	if !exists {
		return nil, fmt.Errorf("skill with id '%s' not found", id)
	}

	return skill, nil
}

// ListSkills returns all loaded skills
func (m *Manager) ListSkills() []*types.SkillDefinition {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	skills := make([]*types.SkillDefinition, 0, len(m.skills))
	for _, skill := range m.skills {
		skills = append(skills, skill)
	}

	return skills
}

// GetSkillsByProvider returns all skills for a specific provider
func (m *Manager) GetSkillsByProvider(provider string) []*types.SkillDefinition {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	var skills []*types.SkillDefinition
	for _, skill := range m.skills {
		if skill.Provider == provider {
			skills = append(skills, skill)
		}
	}

	return skills
}

// ReloadSkills reloads all skills (useful for development)
func (m *Manager) ReloadSkills(ctx context.Context) error {
	log.Info().Msg("Reloading YAML skills")

	m.mutex.Lock()
	// Clear existing skills
	m.skills = make(map[string]*types.SkillDefinition)
	m.mutex.Unlock()

	return m.LoadSkills(ctx)
}

// GetSkillCount returns the total number of loaded skills
func (m *Manager) GetSkillCount() int {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return len(m.skills)
}

// ListSkillsByCategory returns all skills for a specific category
func (m *Manager) ListSkillsByCategory(category string) []*types.SkillDefinition {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	var skills []*types.SkillDefinition
	for _, skill := range m.skills {
		if skill.Category == category {
			skills = append(skills, skill)
		}
	}

	return skills
}

// ListSkillsByProvider returns all skills for a specific provider (alias for GetSkillsByProvider)
func (m *Manager) ListSkillsByProvider(provider string) []*types.SkillDefinition {
	return m.GetSkillsByProvider(provider)
}
