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

func TestChiefOfStaffRequiresBotNameAndPurposeBeforeCreation(t *testing.T) {
	for _, want := range []string{
		"a human-readable name or role title",
		"a concrete purpose",
		"then stop and wait",
		"Do not create a generic placeholder",
		"not permission to invent a generic assistant",
		"keel-hq/keel",
		"keel-maintainer",
		"proceed immediately",
		"full standard worker tool set",
		"attach that repository in the same turn",
		"register that exact external repository",
		"Never substitute a similarly named repository",
		"without asking a routine follow-up",
	} {
		if !strings.Contains(ChiefOfStaff, want) {
			t.Fatalf("ChiefOfStaff missing %q", want)
		}
	}
}
