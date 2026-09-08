package seedprompts

import (
	"strings"
	"testing"
)

func TestChiefOfStaffOnboardingDeliversToNotificationAndChat(t *testing.T) {
	for _, want := range []string{"`ask_human`", "repeat the exact same message verbatim as your normal final response"} {
		if !strings.Contains(ChiefOfStaff, want) {
			t.Fatalf("ChiefOfStaff missing %q", want)
		}
	}
}
