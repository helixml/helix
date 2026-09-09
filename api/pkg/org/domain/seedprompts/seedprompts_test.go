package seedprompts

import (
	"strings"
	"testing"
)

func TestChiefOfStaffOnboardingOpening(t *testing.T) {
	for _, want := range []string{
		"Ask the owner in your normal response",
		"Hi, I'm your new Chief of Staff. 👋\n\nWhat would you like to accomplish?",
		"Wait for the owner's reply before asking about key people, repositories, servers, or workflows. Ask about those naturally in follow-up messages as they become relevant, not as a checklist.",
	} {
		if !strings.Contains(ChiefOfStaff, want) {
			t.Fatalf("ChiefOfStaff missing %q", want)
		}
	}
	if strings.Contains(ChiefOfStaff, "ask_human") || strings.Contains(ChiefOfStaff, "set_human_contact") {
		t.Fatal("ChiefOfStaff must not refer to removed human-placeholder tools")
	}
}
