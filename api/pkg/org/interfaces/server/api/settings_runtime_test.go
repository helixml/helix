package api

import "testing"

func TestRuntimeSupportsSubscription(t *testing.T) {
	tests := map[string]bool{
		"claude_code": true,
		"codex_cli":   true,
		"zed_agent":   false,
		"goose_code":  false,
	}
	for runtime, want := range tests {
		t.Run(runtime, func(t *testing.T) {
			if got := runtimeSupportsSubscription(runtime); got != want {
				t.Fatalf("runtimeSupportsSubscription(%q) = %v, want %v", runtime, got, want)
			}
		})
	}
}
