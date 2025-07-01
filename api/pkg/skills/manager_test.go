package skills

import (
	"context"
	"testing"
)

func TestManagerLoadSkills(t *testing.T) {
	manager := NewManager()
	ctx := context.Background()

	// Test loading skills
	err := manager.LoadSkills(ctx)
	if err != nil {
		t.Fatalf("Failed to load skills: %v", err)
	}

	// Test that we have at least one skill loaded
	skills := manager.ListSkills()
	if len(skills) == 0 {
		t.Fatal("Expected at least one skill to be loaded")
	}

	// Test that we can find the GitHub skill
	githubSkill, err := manager.GetSkill("github")
	if err != nil {
		t.Fatalf("Failed to get GitHub skill: %v", err)
	}

	if githubSkill.Name != "github" {
		t.Errorf("Expected skill name 'github', got '%s'", githubSkill.Name)
	}

	if githubSkill.DisplayName != "GitHub" {
		t.Errorf("Expected display name 'GitHub', got '%s'", githubSkill.DisplayName)
	}

	if githubSkill.Provider != "github" {
		t.Errorf("Expected provider 'github', got '%s'", githubSkill.Provider)
	}

	if githubSkill.OAuthProvider != "github" {
		t.Errorf("Expected OAuth provider 'github', got '%s'", githubSkill.OAuthProvider)
	}

	// Test getting skills by provider
	githubSkills := manager.GetSkillsByProvider("github")
	if len(githubSkills) == 0 {
		t.Fatal("Expected at least one GitHub skill")
	}
}

func TestManagerReloadSkills(t *testing.T) {
	manager := NewManager()
	ctx := context.Background()

	// Initial load
	err := manager.LoadSkills(ctx)
	if err != nil {
		t.Fatalf("Failed to load skills: %v", err)
	}

	initialCount := len(manager.ListSkills())

	// Reload
	err = manager.ReloadSkills(ctx)
	if err != nil {
		t.Fatalf("Failed to reload skills: %v", err)
	}

	reloadCount := len(manager.ListSkills())
	if reloadCount != initialCount {
		t.Errorf("Expected %d skills after reload, got %d", initialCount, reloadCount)
	}
}
