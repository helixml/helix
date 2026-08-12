package processors_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/processors"
	"github.com/helixml/helix/api/pkg/org/application/topics"
	"github.com/helixml/helix/api/pkg/org/domain/processor"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
)

const org = "org-1"

func tmplCfg(tmpl string) json.RawMessage {
	b, _ := json.Marshal(map[string]string{"template": tmpl})
	return b
}

func setup(t *testing.T) (*store.Store, *processors.Processors, *topics.Topics) {
	t.Helper()
	var n int
	id := func() string { n++; return time.Now().Format("150405.000") + "-" + string(rune('a'+n%26)) }
	s := memory.New()
	top := topics.New(topics.Deps{Topics: s.Topics, NewID: id})
	svc := processors.New(processors.Deps{Processors: s.Processors, Topics: top, NewID: id})
	return s, svc, top
}

func TestCreateAutoProvisionsOutputTopic(t *testing.T) {
	ctx := context.Background()
	s, svc, top := setup(t)
	if _, err := top.Create(ctx, org, topics.CreateParams{ID: "s-in", Name: "In"}); err != nil {
		t.Fatal(err)
	}
	p, err := svc.Create(ctx, org, processors.CreateParams{
		Name: "Fmt", InputTopicID: "s-in", Kind: processor.KindTemplate,
		Config: tmplCfg("{{ .Message.body }}"),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(p.Outputs) != 1 || p.Outputs[0].TopicID == "" || !p.Outputs[0].Owned {
		t.Fatalf("expected one owned auto-provisioned output, got %+v", p.Outputs)
	}
	// The output topic exists in the store.
	if _, err := s.Topics.Get(ctx, org, p.Outputs[0].TopicID); err != nil {
		t.Errorf("output topic not created: %v", err)
	}
	// Processor id minted with p- prefix.
	if len(p.ID) < 2 || p.ID[:2] != "p-" {
		t.Errorf("processor id = %q, want p- prefix", p.ID)
	}
}

func TestDeleteCascadesOwnedButKeepsExplicit(t *testing.T) {
	ctx := context.Background()
	s, svc, top := setup(t)
	_, _ = top.Create(ctx, org, topics.CreateParams{ID: "s-in", Name: "In"})
	shared, _ := top.Create(ctx, org, topics.CreateParams{ID: "s-shared", Name: "Shared"})

	// Two outputs: one auto-provisioned (owned), one explicit (shared).
	p, err := svc.Create(ctx, org, processors.CreateParams{
		Name: "Router", InputTopicID: "s-in", Kind: processor.KindTemplate,
		Config:  tmplCfg("{{ .Message.body }}"),
		Outputs: []processors.OutputSpec{{TopicID: shared.ID}},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if p.Outputs[0].Owned {
		t.Fatal("explicit output should not be owned")
	}

	if err := svc.Delete(ctx, org, p.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	// Processor gone.
	if _, err := svc.Get(ctx, org, p.ID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("processor still present: %v", err)
	}
	// Shared topic survived (not owned).
	if _, err := s.Topics.Get(ctx, org, shared.ID); err != nil {
		t.Errorf("shared explicit topic was wrongly deleted: %v", err)
	}
}

func TestDeleteRemovesAutoProvisionedTopic(t *testing.T) {
	ctx := context.Background()
	s, svc, top := setup(t)
	_, _ = top.Create(ctx, org, topics.CreateParams{ID: "s-in", Name: "In"})
	p, err := svc.Create(ctx, org, processors.CreateParams{
		Name: "Fmt", InputTopicID: "s-in", Kind: processor.KindTemplate,
		Config: tmplCfg("{{ .Message.body }}"),
	})
	if err != nil {
		t.Fatal(err)
	}
	outID := p.Outputs[0].TopicID
	if err := svc.Delete(ctx, org, p.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := s.Topics.Get(ctx, org, outID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("owned output topic %q should be deleted, err=%v", outID, err)
	}
}

func TestUpdateRevalidatesConfig(t *testing.T) {
	ctx := context.Background()
	_, svc, top := setup(t)
	_, _ = top.Create(ctx, org, topics.CreateParams{ID: "s-in", Name: "In"})
	p, _ := svc.Create(ctx, org, processors.CreateParams{
		Name: "Fmt", InputTopicID: "s-in", Kind: processor.KindTemplate,
		Config: tmplCfg("{{ .Message.body }}"),
	})
	// Malformed template rejected at update.
	_, err := svc.Update(ctx, org, p.ID, processors.UpdateParams{
		Name: "Fmt", Kind: processor.KindTemplate, Config: tmplCfg("{{ .Message.body "),
	})
	if err == nil {
		t.Error("want error updating to malformed template, got nil")
	}
}

// filterCfg builds an empty filter config blob.
func filterRouter(t *testing.T, ctx context.Context, svc *processors.Processors, top *topics.Topics) processor.Processor {
	t.Helper()
	_, _ = top.Create(ctx, org, topics.CreateParams{ID: "s-slack", Name: "Slack"})
	p, err := svc.Create(ctx, org, processors.CreateParams{
		Name: "Router", InputTopicID: "s-slack", Kind: processor.KindFilter,
		Outputs: []processors.OutputSpec{{Label: "default"}}, // unconditional default
	})
	if err != nil {
		t.Fatalf("create router: %v", err)
	}
	return p
}

func TestAddOutputProvisionsOwnedTopicAndPersists(t *testing.T) {
	ctx := context.Background()
	s, svc, top := setup(t)
	p := filterRouter(t, ctx, svc, top)

	out, err := svc.AddOutput(ctx, org, p.ID, processors.OutputSpec{
		Label: "alice", Match: `{{ mentions "alice" .Message.body }}`, ManagedFor: "w-alice",
	})
	if err != nil {
		t.Fatalf("add output: %v", err)
	}
	if out.TopicID == "" || !out.Owned || out.ManagedFor != "w-alice" {
		t.Fatalf("unexpected output: %+v", out)
	}
	// The owned topic exists.
	if _, err := s.Topics.Get(ctx, org, out.TopicID); err != nil {
		t.Errorf("owned output topic not created: %v", err)
	}
	// Persisted: the processor now has 2 outputs.
	got, _ := svc.Get(ctx, org, p.ID)
	if len(got.Outputs) != 2 {
		t.Fatalf("want 2 outputs after add, got %d", len(got.Outputs))
	}
}

func TestRemoveOutputDropsRouteAndOwnedTopic(t *testing.T) {
	ctx := context.Background()
	s, svc, top := setup(t)
	p := filterRouter(t, ctx, svc, top)
	out, _ := svc.AddOutput(ctx, org, p.ID, processors.OutputSpec{Label: "alice", Match: "x", ManagedFor: "w-alice"})

	if err := svc.RemoveOutput(ctx, org, p.ID, out.TopicID); err != nil {
		t.Fatalf("remove output: %v", err)
	}
	got, _ := svc.Get(ctx, org, p.ID)
	if len(got.Outputs) != 1 {
		t.Fatalf("want 1 output after remove, got %d", len(got.Outputs))
	}
	if _, err := s.Topics.Get(ctx, org, out.TopicID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("owned output topic %q should be deleted, err=%v", out.TopicID, err)
	}
}

func TestRemoveLastOutputRejected(t *testing.T) {
	ctx := context.Background()
	_, svc, top := setup(t)
	p := filterRouter(t, ctx, svc, top)
	// The router starts with exactly one (default) output; removing it would
	// leave the processor with zero outputs, which Validate forbids.
	if err := svc.RemoveOutput(ctx, org, p.ID, p.Outputs[0].TopicID); err == nil {
		t.Error("want error removing the last output, got nil")
	}
}

func TestDeleteAutomatedByInputRemovesRouterButNotManual(t *testing.T) {
	ctx := context.Background()
	s, svc, top := setup(t)
	_, _ = top.Create(ctx, org, topics.CreateParams{ID: "s-ws", Name: "Workspace"})

	// Automated router on s-ws (CreatedBy = SystemActor) with an owned output.
	auto, err := svc.Create(ctx, org, processors.CreateParams{
		Name: "Auto", InputTopicID: "s-ws", Kind: processor.KindFilter,
		Outputs: []processors.OutputSpec{{Label: "default"}}, CreatedBy: processor.SystemActor,
	})
	if err != nil {
		t.Fatal(err)
	}
	autoOut := auto.Outputs[0].TopicID
	// A human-authored filter on the SAME input topic — must be left alone.
	manual, err := svc.Create(ctx, org, processors.CreateParams{
		Name: "Manual", InputTopicID: "s-ws", Kind: processor.KindFilter,
		Outputs: []processors.OutputSpec{{Label: "default"}}, CreatedBy: "w-alice",
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.DeleteAutomatedByInput(ctx, org, "s-ws"); err != nil {
		t.Fatalf("DeleteAutomatedByInput: %v", err)
	}
	// Automated router gone + its owned output topic cascaded.
	if _, err := svc.Get(ctx, org, auto.ID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("automated router should be deleted, err=%v", err)
	}
	if _, err := s.Topics.Get(ctx, org, autoOut); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("automated router's owned output topic should be cascaded, err=%v", err)
	}
	// Manual processor untouched.
	if _, err := svc.Get(ctx, org, manual.ID); err != nil {
		t.Errorf("manual processor should survive, err=%v", err)
	}
	// Idempotent: a second call is a no-op.
	if err := svc.DeleteAutomatedByInput(ctx, org, "s-ws"); err != nil {
		t.Errorf("second DeleteAutomatedByInput should be no-op, got %v", err)
	}
}

var _ = streaming.TopicID("")
