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
	if _, _, err := top.Create(ctx, org, topics.CreateParams{ID: "s-in", Name: "In"}); err != nil {
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
	_, _, _ = top.Create(ctx, org, topics.CreateParams{ID: "s-in", Name: "In"})
	shared, _, _ := top.Create(ctx, org, topics.CreateParams{ID: "s-shared", Name: "Shared"})

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
	_, _, _ = top.Create(ctx, org, topics.CreateParams{ID: "s-in", Name: "In"})
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
	_, _, _ = top.Create(ctx, org, topics.CreateParams{ID: "s-in", Name: "In"})
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

var _ = streaming.TopicID("")
