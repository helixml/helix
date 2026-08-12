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

func TestBotDTOCanonicalAgentIDWinsConflict(t *testing.T) {
	var dto BotDTO
	if err := json.Unmarshal([]byte(`{"agent_id":"agent_new","agent_app_id":"agent_old"}`), &dto); err != nil {
		t.Fatal(err)
	}
	if dto.AgentID != "agent_new" || dto.LegacyAgentID != "agent_new" {
		t.Fatalf("agent ids = (%q, %q)", dto.AgentID, dto.LegacyAgentID)
	}

	data, err := json.Marshal(BotDTO{AgentID: "agent_new", LegacyAgentID: "agent_old"})
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	if fields["agent_id"] != "agent_new" || fields["agent_app_id"] != "agent_new" {
		t.Fatalf("agent id fields = %s", data)
	}
}

func TestAgentDetailDTOPreservesProjectID(t *testing.T) {
	data, err := json.Marshal(AgentDetailDTO{
		BotDTO: BotDTO{
			ID:            "bot_test",
			AgentID:       "agent_new",
			LegacyAgentID: "agent_old",
		},
		ProjectID: "project_test",
	})
	if err != nil {
		t.Fatal(err)
	}

	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	if fields["project_id"] != "project_test" {
		t.Fatalf("project_id = %v, body = %s", fields["project_id"], data)
	}
	if fields["agent_id"] != "agent_new" || fields["agent_app_id"] != "agent_new" {
		t.Fatalf("agent id fields = %s", data)
	}

	var decoded AgentDetailDTO
	if err := json.Unmarshal([]byte(`{
		"id":"bot_test",
		"agent_id":"agent_new",
		"agent_app_id":"agent_old",
		"project_id":"project_test"
	}`), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.ProjectID != "project_test" {
		t.Fatalf("project_id = %q", decoded.ProjectID)
	}
	if decoded.AgentID != "agent_new" || decoded.LegacyAgentID != "agent_new" {
		t.Fatalf("agent ids = (%q, %q)", decoded.AgentID, decoded.LegacyAgentID)
	}
}
