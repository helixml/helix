package mcptools

import "testing"

func TestAskHumanToolIsNotRegistered(t *testing.T) {
	for _, name := range BaseReadTools {
		if name == "ask_human" {
			t.Fatal("ask_human must not be granted to org bots")
		}
	}
}
