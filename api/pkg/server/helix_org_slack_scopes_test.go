package server

import "testing"

func TestDefaultSlackBotScopesSupportHumanDelivery(t *testing.T) {
	scopes := make(map[string]bool, len(defaultSlackBotScopes))
	for _, scope := range defaultSlackBotScopes {
		scopes[scope] = true
	}
	for _, scope := range []string{"chat:write", "im:write", "users:read", "users:read.email"} {
		if !scopes[scope] {
			t.Fatalf("default Slack scopes missing %q", scope)
		}
	}
}
