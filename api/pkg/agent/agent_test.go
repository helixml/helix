package agent

import (
	"strings"
	"testing"
)

func TestSkillValidation(t *testing.T) {
	// Test case 1: Missing Description
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("Expected panic due to missing Description, but no panic occurred")
		}
		msg, ok := r.(string)
		if !ok {
			t.Fatalf("Unexpected panic type: %T", r)
		}
		if !strings.Contains(msg, "missing a Description") {
			t.Fatalf("Unexpected panic message: %s", msg)
		}
	}()

	skill := Skill{
		Name: "TestSkill",
		// Description intentionally missing
		SystemPrompt: "Test system prompt",
	}
	_ = NewAgent("Test prompt", []Skill{skill})

	// This line should not be reached due to the panic
	t.Fatal("Test should have panicked before reaching this point")
}

func TestSkillValidationSystemPrompt(t *testing.T) {
	// Test case 2: Missing SystemPrompt
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("Expected panic due to missing SystemPrompt, but no panic occurred")
		}
		msg, ok := r.(string)
		if !ok {
			t.Fatalf("Unexpected panic type: %T", r)
		}
		if !strings.Contains(msg, "missing a SystemPrompt") {
			t.Fatalf("Unexpected panic message: %s", msg)
		}
	}()

	skill := Skill{
		Name:        "TestSkill",
		Description: "Test description",
		// SystemPrompt intentionally missing
	}
	_ = NewAgent("Test prompt", []Skill{skill})

	// This line should not be reached due to the panic
	t.Fatal("Test should have panicked before reaching this point")
}
