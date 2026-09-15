package mcptools

import "testing"

func TestHumanContactToolIsNotGranted(t *testing.T) {
	for _, name := range OwnerBotTools() {
		if name == "set_human_contact" {
			t.Fatal("set_human_contact must not be granted to org bots")
		}
	}
}
