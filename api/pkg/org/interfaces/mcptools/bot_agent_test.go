package mcptools

import (
	"encoding/json"
	"testing"

	"github.com/helixml/helix/api/pkg/org/application/activations"
)

func TestBotActivateViewExposesLegacyAppID(t *testing.T) {
	data, err := json.Marshal(toBotActivateView(activations.ActivateResult{AgentID: "app-legacy"}))
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	if fields["legacy_app_id"] != "app-legacy" {
		t.Fatalf("legacy_app_id = %v, want app-legacy", fields["legacy_app_id"])
	}
	for _, stale := range []string{"agent_id", "agent_app_id"} {
		if _, exists := fields[stale]; exists {
			t.Fatalf("Bot activation exposes stale %s: %s", stale, data)
		}
	}
}
