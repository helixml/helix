package mcptools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/helixml/helix/api/pkg/org/application/bots"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	"github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
)

func TestSetHumanContactPatchesIdentity(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	cfg := DefaultDeps(st)
	person, err := cfg.botsService().Create(ctx, "org-test", bots.CreateParams{
		ID: "h-owner", Kind: orgchart.BotKindHuman, HelixUserID: "usr-owner",
		Content: "Owner", Identity: map[string]string{"email": "owner@example.com", "github": "owner"},
	})
	if err != nil {
		t.Fatal(err)
	}
	target := &SetHumanContact{deps: cfg.Build()}
	args, _ := json.Marshal(setHumanContactArgs{PersonID: string(person.ID), Contact: map[string]string{
		"preferred_contact": "slack", "slack_user_id": "U123", "github": "",
	}})
	if _, err := target.Invoke(ctx, tool.Invocation{Caller: botCaller{id: "chief-of-staff", orgID: "org-test"}, Args: args}); err != nil {
		t.Fatal(err)
	}
	updated, err := st.Bots.Get(ctx, "org-test", person.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Identity["email"] != "owner@example.com" || updated.Identity["slack_user_id"] != "U123" {
		t.Fatalf("identity patch lost or missed fields: %#v", updated.Identity)
	}
	if _, ok := updated.Identity["github"]; ok {
		t.Fatalf("empty patch did not remove github: %#v", updated.Identity)
	}
}

func TestOwnerBotToolsIncludesSetHumanContact(t *testing.T) {
	for _, name := range OwnerBotTools() {
		if name == SetHumanContactName {
			return
		}
	}
	t.Fatal("OwnerBotTools missing set_human_contact")
}

func TestSetHumanContactRejectsAgentAndIncompleteSlack(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	cfg := DefaultDeps(st)
	if _, err := cfg.botsService().Create(ctx, "org-test", bots.CreateParams{ID: "b-agent", Content: "Agent"}); err != nil {
		t.Fatal(err)
	}
	target := &SetHumanContact{deps: cfg.Build()}
	for _, tc := range []struct {
		person  string
		contact map[string]string
		want    string
	}{
		{"b-agent", map[string]string{"preferred_contact": "helix"}, "not a person"},
		{"h-owner", map[string]string{"preferred_contact": "slack"}, "slack_user_id is required"},
		{"h-owner", map[string]string{"preferred_contact": "sms"}, "preferred_contact must be helix or slack"},
	} {
		if tc.person == "h-owner" {
			_, _ = cfg.botsService().Create(ctx, "org-test", bots.CreateParams{ID: "h-owner", Kind: orgchart.BotKindHuman, Content: "Owner"})
		}
		args, _ := json.Marshal(setHumanContactArgs{PersonID: tc.person, Contact: tc.contact})
		_, err := target.Invoke(ctx, tool.Invocation{Caller: botCaller{id: "chief-of-staff", orgID: "org-test"}, Args: args})
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("error = %v, want %q", err, tc.want)
		}
	}
}
