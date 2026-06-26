package topics

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	"github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
)

// fixedClock returns a deterministic time so created/updated state is
// byte-comparable across adapters.
func fixedClock() time.Time { return time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC) }

func newService(st *store.Store) *Topics {
	seq := 0
	return New(Deps{
		Topics: st.Topics,
		Now:     fixedClock,
		NewID: func() string {
			seq++
			return "id" // deterministic so tests can assert the generated id
		},
	})
}

func TestTopicsCreate_HappyPath(t *testing.T) {
	t.Parallel()
	st := memory.New()
	svc := newService(st)
	ctx := context.Background()

	got, err := svc.Create(ctx, "org-test", CreateParams{
		ID:          "s-general",
		Name:        "general",
		Description: "the general channel",
		CreatedBy:   "w-owner",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got.ID != "s-general" || got.Name != "general" || got.Description != "the general channel" {
		t.Fatalf("unexpected topic: %+v", got)
	}
	if got.CreatedBy != "w-owner" {
		t.Fatalf("CreatedBy = %q, want w-owner", got.CreatedBy)
	}
	if got.OrganizationID != "org-test" {
		t.Fatalf("OrganizationID = %q, want org-test", got.OrganizationID)
	}
	// Empty transport defaults to local.
	if got.Transport.Kind != transport.KindLocal {
		t.Fatalf("Transport.Kind = %q, want local", got.Transport.Kind)
	}
	if !got.CreatedAt.Equal(fixedClock()) {
		t.Fatalf("CreatedAt = %v, want %v", got.CreatedAt, fixedClock())
	}

	// Persisted and retrievable.
	stored, err := st.Topics.Get(ctx, "org-test", "s-general")
	if err != nil {
		t.Fatalf("Get after create: %v", err)
	}
	if stored.Name != "general" {
		t.Fatalf("stored name = %q", stored.Name)
	}
}

func TestTopicsCreate_DuplicateNameRejected(t *testing.T) {
	t.Parallel()
	st := memory.New()
	svc := newService(st)
	ctx := context.Background()

	if _, err := svc.Create(ctx, "org-test", CreateParams{ID: "s-a", Name: "general"}); err != nil {
		t.Fatalf("first create: %v", err)
	}
	// Same name, different id, same org → conflict (mapped to 409 by adapters).
	_, err := svc.Create(ctx, "org-test", CreateParams{ID: "s-b", Name: "general"})
	if !errors.Is(err, store.ErrConflict) {
		t.Fatalf("want store.ErrConflict for duplicate name, got %v", err)
	}
	// The same name in a DIFFERENT org is fine (uniqueness is per-org).
	if _, err := svc.Create(ctx, "org-other", CreateParams{ID: "s-c", Name: "general"}); err != nil {
		t.Fatalf("same name in another org should be allowed: %v", err)
	}
}

func TestTopicsCreate_AutoID(t *testing.T) {
	t.Parallel()
	st := memory.New()
	svc := newService(st)
	ctx := context.Background()

	got, err := svc.Create(ctx, "org-test", CreateParams{Name: "n", CreatedBy: "w-owner"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got.ID != "s-id" {
		t.Fatalf("auto ID = %q, want s-id", got.ID)
	}
}

func TestTopicsCreate_WebhookTransport(t *testing.T) {
	t.Parallel()
	st := memory.New()
	svc := newService(st)
	ctx := context.Background()

	cfg := json.RawMessage(`{"outbound_url":"https://example.com/in"}`)
	got, err := svc.Create(ctx, "org-test", CreateParams{
		Name:      "hook",
		CreatedBy: "w-owner",
		Transport: transport.Transport{Kind: transport.KindWebhook, Config: cfg},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got.Transport.Kind != transport.KindWebhook {
		t.Fatalf("Transport.Kind = %q, want webhook", got.Transport.Kind)
	}
}

func TestTopicsCreate_OrgScoping(t *testing.T) {
	t.Parallel()
	st := memory.New()
	svc := newService(st)
	ctx := context.Background()

	if _, err := svc.Create(ctx, "org-a", CreateParams{ID: "s-x", Name: "x", CreatedBy: "w-owner"}); err != nil {
		t.Fatalf("Create org-a: %v", err)
	}
	// Same id under a different org is allowed (composite PK).
	if _, err := svc.Create(ctx, "org-b", CreateParams{ID: "s-x", Name: "x", CreatedBy: "w-owner"}); err != nil {
		t.Fatalf("Create org-b: %v", err)
	}
	// org-a cannot see org-b's via Get under org-a only sees its own.
	if _, err := st.Topics.Get(ctx, "org-a", "s-x"); err != nil {
		t.Fatalf("org-a should see its own topic: %v", err)
	}
}

func TestTopicsCreate_EmptyNameRejected(t *testing.T) {
	t.Parallel()
	st := memory.New()
	svc := newService(st)
	if _, err := svc.Create(context.Background(), "org-test", CreateParams{CreatedBy: "w-owner"}); err == nil {
		t.Fatal("Create with empty name: want error, got nil")
	}
}

func TestTopicsUpdate_HappyPath(t *testing.T) {
	t.Parallel()
	st := memory.New()
	svc := newService(st)
	ctx := context.Background()

	if _, err := svc.Create(ctx, "org-test", CreateParams{ID: "s-1", Name: "old", Description: "od", CreatedBy: "w-owner"}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := svc.Update(ctx, "org-test", "s-1", UpdateParams{Name: "new", Description: "nd"})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got.Name != "new" || got.Description != "nd" {
		t.Fatalf("unexpected update result: %+v", got)
	}
	// Immutable fields survive.
	if got.CreatedBy != "w-owner" {
		t.Fatalf("CreatedBy changed: %q", got.CreatedBy)
	}
	stored, _ := st.Topics.Get(ctx, "org-test", "s-1")
	if stored.Name != "new" {
		t.Fatalf("stored name = %q, want new", stored.Name)
	}
}

func TestTopicsUpdate_TransportPatch(t *testing.T) {
	t.Parallel()
	st := memory.New()
	svc := newService(st)
	ctx := context.Background()

	cfg := json.RawMessage(`{"repo":"helixml/helix","events":["issues"]}`)
	if _, err := svc.Create(ctx, "org-test", CreateParams{
		ID: "s-1", Name: "n", CreatedBy: "w-owner",
		Transport: transport.Transport{Kind: transport.KindGitHub, Config: cfg},
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Patch only the config, leave the kind.
	newCfg := json.RawMessage(`{"repo":"helixml/other","events":["pull_request"]}`)
	got, err := svc.Update(ctx, "org-test", "s-1", UpdateParams{
		Name:      "n",
		Transport: &TransportPatch{Config: newCfg},
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got.Transport.Kind != transport.KindGitHub {
		t.Fatalf("kind changed unexpectedly: %q", got.Transport.Kind)
	}
	if string(got.Transport.Config) != string(newCfg) {
		t.Fatalf("config = %s, want %s", got.Transport.Config, newCfg)
	}
}

func TestTopicsUpdate_NotFound(t *testing.T) {
	t.Parallel()
	st := memory.New()
	svc := newService(st)
	_, err := svc.Update(context.Background(), "org-test", "s-missing", UpdateParams{Name: "n"})
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("Update missing: err = %v, want ErrNotFound", err)
	}
}

func TestTopicsUpdate_OrgScoping(t *testing.T) {
	t.Parallel()
	st := memory.New()
	svc := newService(st)
	ctx := context.Background()
	if _, err := svc.Create(ctx, "org-a", CreateParams{ID: "s-1", Name: "n", CreatedBy: "w-owner"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	// Updating under the wrong org is a not-found, never a cross-tenant write.
	_, err := svc.Update(ctx, "org-b", "s-1", UpdateParams{Name: "hacked"})
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("cross-org update: err = %v, want ErrNotFound", err)
	}
}

func TestTopicsDelete_HappyPath(t *testing.T) {
	t.Parallel()
	st := memory.New()
	svc := newService(st)
	ctx := context.Background()
	if _, err := svc.Create(ctx, "org-test", CreateParams{ID: "s-1", Name: "n", CreatedBy: "w-owner"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := svc.Delete(ctx, "org-test", "s-1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := st.Topics.Get(ctx, "org-test", "s-1"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("after delete Get err = %v, want ErrNotFound", err)
	}
}

func TestTopicsDelete_NotFound(t *testing.T) {
	t.Parallel()
	st := memory.New()
	svc := newService(st)
	if err := svc.Delete(context.Background(), "org-test", "s-missing"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("Delete missing: err = %v, want ErrNotFound", err)
	}
}
