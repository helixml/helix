package memory_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/processor"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
)

func mkProc(t *testing.T, id, name, input, tmpl, out, org string) processor.Processor {
	t.Helper()
	cfg, _ := json.Marshal(map[string]string{"template": tmpl})
	p, err := processor.NewProcessor(
		id, name, input, processor.KindTemplate, cfg,
		[]processor.Output{{TopicID: out}}, "w-owner", time.Now(), org,
	)
	if err != nil {
		t.Fatalf("NewProcessor: %v", err)
	}
	return p
}

func TestProcessorsRoundTrip(t *testing.T) {
	ctx := context.Background()
	s := memory.New()
	p := mkProc(t, "p-fmt", "Formatter", "s-in", "B:{{.Message.body}}", "s-out", "org-1")

	if err := s.Processors.Create(ctx, p); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := s.Processors.Get(ctx, "org-1", "p-fmt")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "Formatter" || got.InputTopicID != "s-in" || got.Outputs[0].TopicID != "s-out" {
		t.Fatalf("Get returned %+v", got)
	}

	list, err := s.Processors.List(ctx, "org-1")
	if err != nil || len(list) != 1 {
		t.Fatalf("List: %v len=%d", err, len(list))
	}

	// Update mutable fields.
	upd := mkProc(t, "p-fmt", "Renamed", "s-in", "X:{{.Message.subject}}", "s-out", "org-1")
	if err := s.Processors.Update(ctx, upd); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, _ = s.Processors.Get(ctx, "org-1", "p-fmt")
	if got.Name != "Renamed" {
		t.Errorf("after Update name = %q, want Renamed", got.Name)
	}

	if err := s.Processors.Delete(ctx, "org-1", "p-fmt"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Processors.Get(ctx, "org-1", "p-fmt"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("after Delete, Get err = %v, want ErrNotFound", err)
	}
}

func TestProcessorsListByInputTopic(t *testing.T) {
	ctx := context.Background()
	s := memory.New()
	_ = s.Processors.Create(ctx, mkProc(t, "p-a", "A", "s-in", "{{.Message.body}}", "s-a", "org-1"))
	_ = s.Processors.Create(ctx, mkProc(t, "p-b", "B", "s-in", "{{.Message.body}}", "s-b", "org-1"))
	_ = s.Processors.Create(ctx, mkProc(t, "p-c", "C", "s-other", "{{.Message.body}}", "s-c", "org-1"))
	// Another org sharing the input topic id must not leak in.
	_ = s.Processors.Create(ctx, mkProc(t, "p-a", "A", "s-in", "{{.Message.body}}", "s-z", "org-2"))

	got, err := s.Processors.ListByInputTopic(ctx, "org-1", "s-in")
	if err != nil {
		t.Fatalf("ListByInputTopic: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 processors on s-in in org-1, got %d", len(got))
	}
	if got[0].ID != "p-a" || got[1].ID != "p-b" {
		t.Errorf("want [p-a p-b], got [%s %s]", got[0].ID, got[1].ID)
	}
}

func TestProcessorsCrossTenantIsolation(t *testing.T) {
	ctx := context.Background()
	s := memory.New()
	_ = s.Processors.Create(ctx, mkProc(t, "p-x", "X", "s-in", "{{.Message.body}}", "s-out", "org-1"))
	if _, err := s.Processors.Get(ctx, "org-2", "p-x"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("cross-tenant Get err = %v, want ErrNotFound", err)
	}
}

func TestProcessorsDuplicateNameRejected(t *testing.T) {
	ctx := context.Background()
	s := memory.New()
	if err := s.Processors.Create(ctx, mkProc(t, "p-1", "Dup", "s-in", "{{.Message.body}}", "s-1", "org-1")); err != nil {
		t.Fatal(err)
	}
	if err := s.Processors.Create(ctx, mkProc(t, "p-2", "Dup", "s-in", "{{.Message.body}}", "s-2", "org-1")); err == nil {
		t.Error("want duplicate-name error, got nil")
	}
}
