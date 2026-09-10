package mcptools

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/lifecycle"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
)

type testAgentCreator struct{}

func (testAgentCreator) CreateAgent(context.Context, string, string, string, lifecycle.AgentConfig) (lifecycle.CreatedAgent, error) {
	return lifecycle.CreatedAgent{LegacyAppID: "app-test"}, nil
}

// newCreateBotCaller sets up the minimal env create_bot needs: a
// store-backed Config, a deterministic clock + ID generator, and a
// caller Bot whose OrganizationID create_bot reads. The tool only checks
// Caller.OrganizationID, so we don't have to pre-seed a manager bot.
func newCreateBotCaller(t *testing.T, orgID string) (Config, orgchart.Node) {
	t.Helper()
	st := orggorm.GetOrgTestDB(t)
	deps := DefaultDeps(st)
	deps.AgentCreator = testAgentCreator{}
	deps.Now = func() time.Time { return time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC) }
	deps.NewID = func() string { return "id-create-bot-test" }
	caller, err := orgchart.NewNode("b-owner", "# Owner", nil, deps.Now(), orgID)
	if err != nil {
		t.Fatalf("new caller: %v", err)
	}
	return deps, caller
}

// invokeCreateBot runs the tool and reads back the created Bot from the
// store so tests can assert on Bot.Tools directly.
func invokeCreateBot(t *testing.T, deps Config, caller orgchart.Node, args string) orgchart.Node {
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
	bot, err := deps.Store.Nodes.Get(ctx, caller.OrganizationID, orgchart.NodeID(resp.ID))
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

func TestCreateBotDescriptionRequiresConfirmedNameAndPurpose(t *testing.T) {
	t.Parallel()
	description := (&CreateBot{}).Description()
	for _, want := range []string{
		"human-readable name or role title",
		"concrete purpose",
		"ask for it and wait",
		"never create a generic placeholder",
		"b-keel-maintainer",
		"full standard worker set",
		"same turn",
		"exact owner/repository match",
		"register the exact external repository",
		"substitute a similarly named repository",
	} {
		if !strings.Contains(description, want) {
			t.Errorf("create_bot description missing %q", want)
		}
	}
}

// TestCreateBotEmptyToolsGetsDefaultWorkerSet simulates a caller that
// forgets the `tools` field entirely (or passes []). The created Bot
// must still expose the full standard worker capability set.
func TestCreateBotEmptyToolsGetsDefaultWorkerSet(t *testing.T) {
	t.Parallel()
	deps, caller := newCreateBotCaller(t, "org-test")
	bot := invokeCreateBot(t, deps, caller, `{"id":"b-empty","content":"# Empty bot"}`)
	want := DefaultBotTools()
	if !reflect.DeepEqual(bot.Tools, want) {
		t.Fatalf("empty-tools bot drifted from DefaultBotTools.\n got: %v\nwant: %v", bot.Tools, want)
	}
}

// TestCreateBotUnionWithCallerTools is the headline behaviour: a
// caller-supplied tools list is preserved (order + custom tools) and
// the standard worker set is appended. Default entries in the caller input
// must not appear twice.
func TestCreateBotUnionWithCallerTools(t *testing.T) {
	t.Parallel()
	deps, caller := newCreateBotCaller(t, "org-test")
	bot := invokeCreateBot(t, deps, caller,
		`{"id":"b-qa","content":"# QA","tools":["chat","managers","attach_worker"]}`)
	want := MergeDefaultBotTools([]tool.Name{ChatName, ManagersName, AttachWorkerName})
	if !reflect.DeepEqual(bot.Tools, want) {
		t.Fatalf("create_bot union drifted.\n got: %v\nwant: %v", bot.Tools, want)
	}
}
