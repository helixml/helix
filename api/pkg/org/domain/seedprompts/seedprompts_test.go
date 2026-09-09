package seedprompts

import (
	"strings"
	"testing"
)

func TestChiefOfStaffOnboardingOpening(t *testing.T) {
	for _, want := range []string{
		"replacing `<first name>` with it",
		"Hi <first name>, I'm your new Chief of Staff. 👋\n\nWhat would you like to accomplish?",
		"`ask_human`",
		"repeat the exact same message verbatim as your normal final response",
		"Wait for the owner's reply before asking about key people, their preferred delivery channel, repositories, servers, or workflows. Ask about those naturally in follow-up messages as they become relevant, not as a checklist.",
	} {
		if !strings.Contains(ChiefOfStaff, want) {
			t.Fatalf("ChiefOfStaff missing %q", want)
		}
	}
}
