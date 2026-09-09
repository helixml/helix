package api

import (
	"encoding/json"
	"testing"
)

func TestBotDTOExposesBotVocabulary(t *testing.T) {
	data, err := json.Marshal(BotDTO{LegacyAppID: "app_test", Status: "running"})
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	if fields["legacy_app_id"] != "app_test" {
		t.Fatalf("legacy app field = %s", data)
	}
	if _, exists := fields["agent_id"]; exists {
		t.Fatalf("Bot DTO exposes agent_id: %s", data)
	}
	if _, exists := fields["agent_app_id"]; exists {
		t.Fatalf("Bot DTO exposes agent_app_id: %s", data)
	}
	if fields["status"] != "running" {
		t.Fatalf("status field = %s", data)
	}
	if _, exists := fields["agent_status"]; exists {
		t.Fatalf("Bot DTO exposes agent_status: %s", data)
	}
}

func TestBotDetailDTOPreservesProjectID(t *testing.T) {
	data, err := json.Marshal(BotDetailDTO{
		Bot: BotDTO{ID: "bot_test"}, LegacyAppID: "app_test", ProjectID: "project_test",
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
	if fields["legacy_app_id"] != "app_test" {
		t.Fatalf("legacy app field = %s", data)
	}
}
