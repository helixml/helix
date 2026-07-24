package mcptools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/helixml/helix/api/pkg/org/application/bots"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	"github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
)

type fakeHumanDelivery struct {
	person orgchart.Bot
	route  string
}

func (f *fakeHumanDelivery) Deliver(_ context.Context, _ string, person orgchart.Bot, _, _, _ string, _ bool) (string, error) {
	f.person = person
	return f.route, nil
}

func TestAskHumanReturnsConfiguredDeliveryRoute(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	cfg := DefaultDeps(st)
	if _, err := cfg.botsService().Create(ctx, "org-test", bots.CreateParams{
		ID: "h-owner", Kind: orgchart.BotKindHuman, Content: "Owner",
		Identity: map[string]string{"preferred_contact": "slack", "slack_user_id": "U123"},
	}); err != nil {
		t.Fatal(err)
	}
	delivery := &fakeHumanDelivery{route: "slack_dm"}
	cfg.HumanDelivery = delivery
	target := &AskHuman{deps: cfg.Build()}
	args, _ := json.Marshal(askHumanArgs{PersonID: "h-owner", Message: "hello"})
	raw, err := target.Invoke(ctx, tool.Invocation{Caller: botCaller{id: "chief-of-staff", orgID: "org-test"}, Args: args})
	if err != nil {
		t.Fatal(err)
	}
	var response map[string]string
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatal(err)
	}
	if response["delivered"] != "slack_dm" || delivery.person.Identity["slack_user_id"] != "U123" {
		t.Fatalf("response/person = %#v/%#v", response, delivery.person.Identity)
	}
}
