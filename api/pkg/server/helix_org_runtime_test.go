package server

import "testing"

func TestWorkerRuntimeSupportsSubscription(t *testing.T) {
	tests := map[string]bool{
		"claude_code": true,
		"codex_cli":   true,
		"zed_agent":   false,
		"qwen_code":   false,
	}
	for runtime, want := range tests {
		t.Run(runtime, func(t *testing.T) {
			if got := workerRuntimeSupportsSubscription(runtime); got != want {
				t.Fatalf("workerRuntimeSupportsSubscription(%q) = %v, want %v", runtime, got, want)
			}
		})
	}
}
