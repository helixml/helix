package types

import "testing"

// The default must follow the picker, so a newer model added at the top is
// what new tasks get, rather than a constant hardcoded somewhere else.
func TestDefaultClaudeSubscriptionModelIsPickerTop(t *testing.T) {
	if len(ClaudeSubscriptionModels) == 0 {
		t.Fatal("the Claude subscription model picker is empty")
	}
	if got, want := DefaultClaudeSubscriptionModel(), ClaudeSubscriptionModels[0].ID; got != want {
		t.Fatalf("default model = %q, want the picker's first entry %q", got, want)
	}
}
