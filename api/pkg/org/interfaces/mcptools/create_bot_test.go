package mcptools

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
)

// newCreateBotCaller sets up the minimal env create_bot needs: a
// store-backed Config, a deterministic clock + ID generator, and a
// caller Bot whose OrganizationID create_bot reads. The tool only checks
// Caller.OrganizationID, so we don't have to pre-seed a manager bot.
func newCreateBotCaller(t *testing.T, orgID string) (Config, orgchart.Bot) {
	t.Helper()
	st := orggorm.GetOrgTestDB(t)
	deps := DefaultDeps(st)
	deps.Now = func() time.Time { return time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC) }
	deps.NewID = func() string { return "id-create-bot-test" }
	caller, err := orgchart.NewBot("b-owner", "# Owner", nil, deps.Now(), orgID)
	if err != nil {
		t.Fatalf("new caller: %v", err)
	}
	return deps, caller
}

// invokeCreateBot runs the tool and reads back the created Bot from the
// store so tests can assert on Bot.Tools directly.
func invokeCreateBot(t *testing.T, deps Config, caller orgchart.Bot, args string) orgchart.Bot {
	t.Helper()
	ctx := context.Background()
	out, err := (&CreateBot{deps: deps.Build()}).Invoke(ctx, tool.Invocation{
		Caller: botCaller{id: string(caller.ID), orgID: caller.OrganizationID},
		Args:   json.RawMessage(args),
	})
	if err != nil {
		t.Fatalf("create_bot invoke: %v", err)
	}
	var resp struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	bot, err := deps.Store.Bots.Get(ctx, caller.OrganizationID, orgchart.BotID(resp.ID))
	if err != nil {
		t.Fatalf("get back bot: %v", err)
	}
	return bot
}

// botCaller adapts a Bot identity to tool.Caller for direct-Invoke
// unit tests (the MCP server builds the real adapter at the boundary).
type botCaller struct{ id, orgID string }

func (c botCaller) ID() string             { return c.id }
func (c botCaller) OrganizationID() string { return c.orgID }

// TestCreateBotEmptyToolsGetsFullBaseline simulates a caller that
// forgets the `tools` field entirely (or passes []). The created Bot
// must still expose the full read baseline — otherwise it would have no
// MCP surface at all and §13-style introspection would silently fail.
func TestCreateBotEmptyToolsGetsFullBaseline(t *testing.T) {
	t.Parallel()
	deps, caller := newCreateBotCaller(t, "org-test")
	bot := invokeCreateBot(t, deps, caller, `{"id":"b-empty","content":"# Empty bot"}`)
	if !reflect.DeepEqual(bot.Tools, BaseReadTools) {
		t.Fatalf("empty-tools bot drifted from BaseReadTools.\n got: %v\nwant: %v", bot.Tools, BaseReadTools)
	}
}

// TestCreateBotUnionWithCallerTools is the headline behaviour: a
// caller-supplied tools list is preserved (order + custom tools) and
// the baseline is appended. The duplicate `managers` in the caller
// input — already part of the baseline — must not appear twice.
func TestCreateBotUnionWithCallerTools(t *testing.T) {
	t.Parallel()
	deps, caller := newCreateBotCaller(t, "org-test")
	bot := invokeCreateBot(t, deps, caller,
		`{"id":"b-qa","content":"# QA","tools":["publish","managers","subscribe"]}`)
	want := []tool.Name{
		// Caller's order preserved, deduped (managers comes from the
		// caller, not from the baseline appendage).
		PublishName,
		ManagersName,
		SubscribeName,
		// Baseline tail in BaseReadTools order, minus the already-present `managers`.
		ReportsName,
		ListBotsName,
		GetBotName,
		ListTopicsName,
		GetTopicName,
		ListTopicEventsName,
		ReadEventsName,
		BotLogName,
		MintCredentialName,
	}
	if !reflect.DeepEqual(bot.Tools, want) {
		t.Fatalf("create_bot union drifted.\n got: %v\nwant: %v", bot.Tools, want)
	}
}
