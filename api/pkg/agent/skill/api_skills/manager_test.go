package apiskill

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadSkills(t *testing.T) {
	manager := NewManager()
	err := manager.LoadSkills(context.Background())
	require.NoError(t, err)

	// Test that skills are loaded
	skills := manager.ListSkills()
	assert.NotEmpty(t, skills, "Should have loaded at least one skill")

	// Test getting a specific skill
	skill, err := manager.GetSkill("github")
	require.NoError(t, err)
	assert.Equal(t, "github", skill.ID)
	assert.Equal(t, "GitHub", skill.DisplayName)

	// Test our new Google Calendar skill
	calendarSkill, err := manager.GetSkill("google-calendar")
	require.NoError(t, err)
	assert.Equal(t, "google-calendar", calendarSkill.ID)
	assert.Equal(t, "Google Calendar", calendarSkill.DisplayName)
	assert.Equal(t, "google", calendarSkill.Provider)
	assert.Equal(t, "Productivity", calendarSkill.Category)
	assert.Equal(t, "google", calendarSkill.OAuthProvider)
	assert.Contains(t, calendarSkill.OAuthScopes, "https://www.googleapis.com/auth/calendar")
	assert.Contains(t, calendarSkill.OAuthScopes, "https://www.googleapis.com/auth/calendar.events")
}

func TestGetSkillsByProvider(t *testing.T) {
	manager := NewManager()
	err := manager.LoadSkills(context.Background())
	require.NoError(t, err)

	// Test getting Google skills (should include both Gmail and Google Calendar)
	googleSkills := manager.GetSkillsByProvider("google")
	assert.GreaterOrEqual(t, len(googleSkills), 2, "Should have at least Gmail and Google Calendar skills")

	// Verify both Gmail and Google Calendar are present
	skillNames := make(map[string]bool)
	for _, skill := range googleSkills {
		skillNames[skill.ID] = true
	}
	assert.True(t, skillNames["gmail"], "Should include Gmail skill")
	assert.True(t, skillNames["google-calendar"], "Should include Google Calendar skill")
}

func TestSkillSchemaValidation(t *testing.T) {
	manager := NewManager()
	err := manager.LoadSkills(context.Background())
	require.NoError(t, err)

	// Test Google Calendar skill schema
	skill, err := manager.GetSkill("google-calendar")
	require.NoError(t, err)

	// Check that schema contains expected operations
	assert.Contains(t, skill.Schema, "getPrimaryCalendar", "Schema should contain getPrimaryCalendar operation")
	assert.Contains(t, skill.Schema, "listCalendars", "Schema should contain listCalendars operation")
	assert.Contains(t, skill.Schema, "listCalendarEvents", "Schema should contain listCalendarEvents operation")
	assert.Contains(t, skill.Schema, "createCalendarEvent", "Schema should contain createCalendarEvent operation")
	assert.Contains(t, skill.Schema, "getCalendarEvent", "Schema should contain getCalendarEvent operation")
	assert.Contains(t, skill.Schema, "updateCalendarEvent", "Schema should contain updateCalendarEvent operation")
	assert.Contains(t, skill.Schema, "deleteCalendarEvent", "Schema should contain deleteCalendarEvent operation")

	// Check API base URL
	assert.Equal(t, "https://www.googleapis.com", skill.BaseURL)

	// Check system prompt
	assert.NotEmpty(t, skill.SystemPrompt)
	assert.Contains(t, skill.SystemPrompt, "calendar", "System prompt should mention calendar")
}
