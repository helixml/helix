package mcptools_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/processor"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	"github.com/helixml/helix/api/pkg/org/interfaces/mcptools"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
)

type procCaller struct{ id, orgID string }

func (c procCaller) ID() string             { return c.id }
func (c procCaller) OrganizationID() string { return c.orgID }

func seedTopic(t *testing.T, deps mcptools.Config, orgID, topicID string) {
	t.Helper()
	top, err := streaming.NewTopic(streaming.TopicID(topicID), "in", "", "w-owner",
		time.Now().UTC(), transport.Transport{Kind: transport.KindLocal}, orgID)
	if err != nil {
		t.Fatalf("NewTopic: %v", err)
	}
	if err := deps.Store.Topics.Create(context.Background(), top); err != nil {
		t.Fatalf("create topic: %v", err)
	}
}

func TestCreateListGetUpdateDeleteProcessor_JS(t *testing.T) {
	st := orggorm.GetOrgTestDB(t)
	deps := mcptools.DefaultDeps(st)
	deps.Now = func() time.Time { return time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC) }
	deps.NewID = func() string { return "proc-js-1" }
	built := deps.Build()
	caller := procCaller{id: "w-owner", orgID: "org-proc"}
	ctx := context.Background()

	seedTopic(t, deps, "org-proc", "s-in")

	code := `function process(event) { event.body = "JS:" + event.body; return event; }`
	cfg, _ := json.Marshal(map[string]string{"code": code})
	createArgs, _ := json.Marshal(map[string]any{
		"name":         "Enrich",
		"inputTopicId": "s-in",
		"kind":         "js",
		"config":       json.RawMessage(cfg),
	})

	reg := mcptools.NewRegistry()
	if err := mcptools.RegisterBuiltins(reg, built); err != nil {
		t.Fatalf("RegisterBuiltins: %v", err)
	}

	createTool, err := reg.Get(mcptools.CreateProcessorName)
	if err != nil {
		t.Fatalf("get create_processor: %v", err)
	}
	raw, err := createTool.Invoke(ctx, tool.Invocation{Caller: caller, Args: createArgs})
	if err != nil {
		t.Fatalf("create_processor: %v", err)
	}
	var created struct {
		ID      string `json:"id"`
		Kind    string `json:"kind"`
		Outputs []struct {
			TopicID string `json:"topicId"`
			Owned   bool   `json:"owned"`
		} `json:"outputs"`
	}
	if err := json.Unmarshal(raw, &created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	if created.ID == "" || created.Kind != "js" {
		t.Fatalf("created = %+v", created)
	}
	if len(created.Outputs) != 1 || !created.Outputs[0].Owned || created.Outputs[0].TopicID == "" {
		t.Fatalf("outputs = %+v", created.Outputs)
	}

	// list
	listTool, _ := reg.Get(mcptools.ListProcessorsName)
	raw, err = listTool.Invoke(ctx, tool.Invocation{Caller: caller, Args: []byte(`{}`)})
	if err != nil {
		t.Fatalf("list_processors: %v", err)
	}
	var listed struct {
		Processors []struct {
			ID string `json:"id"`
		} `json:"processors"`
	}
	if err := json.Unmarshal(raw, &listed); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(listed.Processors) != 1 || listed.Processors[0].ID != created.ID {
		t.Fatalf("list = %+v", listed)
	}

	// get
	getTool, _ := reg.Get(mcptools.GetProcessorName)
	getArgs, _ := json.Marshal(map[string]string{"id": created.ID})
	raw, err = getTool.Invoke(ctx, tool.Invocation{Caller: caller, Args: getArgs})
	if err != nil {
		t.Fatalf("get_processor: %v", err)
	}
	var got struct {
		Name         string `json:"name"`
		InputTopicID string `json:"inputTopicId"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("decode get: %v", err)
	}
	if got.Name != "Enrich" || got.InputTopicID != "s-in" {
		t.Fatalf("got = %+v", got)
	}

	// update code
	newCode := `function process(event) { event.body = "Y:" + event.body; return event; }`
	newCfg, _ := json.Marshal(map[string]string{"code": newCode})
	updArgs, _ := json.Marshal(map[string]any{
		"id":     created.ID,
		"name":   "Enrich v2",
		"kind":   "js",
		"config": json.RawMessage(newCfg),
	})
	updTool, _ := reg.Get(mcptools.UpdateProcessorName)
	raw, err = updTool.Invoke(ctx, tool.Invocation{Caller: caller, Args: updArgs})
	if err != nil {
		t.Fatalf("update_processor: %v", err)
	}
	var updated struct {
		Name string `json:"name"`
	}
	_ = json.Unmarshal(raw, &updated)
	if updated.Name != "Enrich v2" {
		t.Fatalf("updated name = %q", updated.Name)
	}

	// Execute the stored processor end-to-end through the domain.
	p, err := built.Processors.Get(ctx, "org-proc", processor.ProcessorID(created.ID))
	if err != nil {
		t.Fatalf("store get: %v", err)
	}
	res, err := p.Process(ctx, streaming.Message{Body: "hi"})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if len(res) != 1 || res[0].Message.Body != "Y:hi" {
		t.Fatalf("process results = %+v", res)
	}

	// delete
	delTool, _ := reg.Get(mcptools.DeleteProcessorName)
	delArgs, _ := json.Marshal(map[string]string{"id": created.ID})
	if _, err := delTool.Invoke(ctx, tool.Invocation{Caller: caller, Args: delArgs}); err != nil {
		t.Fatalf("delete_processor: %v", err)
	}
	if _, err := built.Processors.Get(ctx, "org-proc", processor.ProcessorID(created.ID)); err == nil {
		t.Fatal("processor still exists after delete")
	}
}

func TestOwnerBotToolsIncludeProcessorMutations(t *testing.T) {
	have := map[tool.Name]bool{}
	for _, n := range mcptools.OwnerBotTools() {
		have[n] = true
	}
	for _, n := range []tool.Name{
		mcptools.CreateProcessorName,
		mcptools.UpdateProcessorName,
		mcptools.DeleteProcessorName,
		mcptools.ListProcessorsName,
		mcptools.GetProcessorName,
	} {
		if !have[n] {
			t.Errorf("OwnerBotTools missing %q", n)
		}
	}
}

func TestProcessorToolsRegistered(t *testing.T) {
	st := orggorm.GetOrgTestDB(t)
	reg := mcptools.NewRegistry()
	if err := mcptools.RegisterBuiltins(reg, mcptools.DefaultDeps(st).Build()); err != nil {
		t.Fatalf("RegisterBuiltins: %v", err)
	}
	for _, n := range []tool.Name{
		mcptools.CreateProcessorName,
		mcptools.UpdateProcessorName,
		mcptools.DeleteProcessorName,
		mcptools.ListProcessorsName,
		mcptools.GetProcessorName,
	} {
		if _, err := reg.Get(n); err != nil {
			t.Errorf("tool %q not registered: %v", n, err)
		}
	}
}
