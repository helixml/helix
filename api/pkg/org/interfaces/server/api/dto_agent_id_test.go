package api

import (
	"encoding/json"
	"testing"
)

func TestBotDTOAcceptsLegacyAgentID(t *testing.T) {
	var dto BotDTO
	if err := json.Unmarshal([]byte(`{"agent_app_id":"app_test"}`), &dto); err != nil {
		t.Fatal(err)
	}
	if dto.AgentID != "app_test" || dto.LegacyAgentID != "app_test" {
		t.Fatalf("agent ids = (%q, %q)", dto.AgentID, dto.LegacyAgentID)
	}
}

func TestBotDTOEmitsCanonicalAndLegacyAgentID(t *testing.T) {
	data, err := json.Marshal(BotDTO{AgentID: "app_test"})
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	if fields["agent_id"] != "app_test" || fields["agent_app_id"] != "app_test" {
		t.Fatalf("agent id fields = %s", data)
	}
}
