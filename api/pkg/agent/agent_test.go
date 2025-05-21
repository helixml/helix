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

func Test_sanitizeToolName(t *testing.T) {
	type args struct {
		name string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "simple alphanumeric name",
			args: args{name: "getPetById"},
			want: "getPetById",
		},
		{
			name: "already with underscores",
			args: args{name: "get_pet_by_id"},
			want: "get_pet_by_id",
		},
		{
			name: "name with spaces",
			args: args{name: "get pet by id"},
			want: "get_pet_by_id",
		},
		{
			name: "name with special characters",
			args: args{name: "get-pet-by-id"},
			want: "get_pet_by_id",
		},
		{
			name: "name with multiple special characters",
			args: args{name: "get.pet@by#id"},
			want: "get_pet_by_id",
		},
		{
			name: "name with mixed case and special characters",
			args: args{name: "Get.Pet@By#Id"},
			want: "Get_Pet_By_Id",
		},
		{
			name: "name with numbers and special characters",
			args: args{name: "get-pet-123"},
			want: "get_pet_123",
		},
		{
			name: "name with consecutive special characters",
			args: args{name: "get--pet---by--id"},
			want: "get_pet_by_id",
		},
		{
			name: "name with leading/trailing special characters",
			args: args{name: "-get-pet-by-id-"},
			want: "_get_pet_by_id_",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SanitizeToolName(tt.args.name); got != tt.want {
				t.Errorf("SanitizeToolName() = %v, want %v", got, tt.want)
			}
		})
	}
}
