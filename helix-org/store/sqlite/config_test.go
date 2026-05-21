package sqlite_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/helixml/helix/helix-org/domain"
	"github.com/helixml/helix/helix-org/store"
)

func TestConfigsSetGetUpsert(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	ctx := context.Background()
	now := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)

	cfg, err := domain.NewConfig("claude.bin", `"claude"`, now, "w-owner")
	if err != nil {
		t.Fatalf("NewConfig: %v", err)
	}
	if err := s.Configs.Set(ctx, cfg); err != nil {
		t.Fatalf("Set: %v", err)
	}

	got, err := s.Configs.Get(ctx, "claude.bin")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Value != `"claude"` {
		t.Fatalf("value = %q", got.Value)
	}

	// Upsert: change value, key stays the same.
	cfg2, _ := domain.NewConfig("claude.bin", `"/usr/local/bin/claude"`, now.Add(time.Hour), "w-owner")
	if err := s.Configs.Set(ctx, cfg2); err != nil {
		t.Fatalf("Set (update): %v", err)
	}
	got2, _ := s.Configs.Get(ctx, "claude.bin")
	if got2.Value != `"/usr/local/bin/claude"` {
		t.Fatalf("value after update = %q", got2.Value)
	}
}

func TestConfigsGetMissing(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	ctx := context.Background()

	_, err := s.Configs.Get(ctx, "nope")
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestConfigsListPrefix(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	ctx := context.Background()
	now := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)

	for _, kv := range []struct{ k, v string }{
		{"claude.bin", `"claude"`},
		{"claude.model", `"opus"`},
		{"transport.postmark", `{"token":"x"}`},
		{"dispatcher.timeout", `300`},
	} {
		c, _ := domain.NewConfig(kv.k, kv.v, now, "")
		if err := s.Configs.Set(ctx, c); err != nil {
			t.Fatalf("Set %q: %v", kv.k, err)
		}
	}

	all, _ := s.Configs.List(ctx, "")
	if len(all) != 4 {
		t.Fatalf("List() = %d, want 4", len(all))
	}

	claudeOnly, _ := s.Configs.List(ctx, "claude.")
	if len(claudeOnly) != 2 {
		t.Fatalf("List(claude.) = %d, want 2", len(claudeOnly))
	}

	none, _ := s.Configs.List(ctx, "missing.")
	if len(none) != 0 {
		t.Fatalf("List(missing.) = %d, want 0", len(none))
	}
}

func TestConfigsDelete(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	ctx := context.Background()
	now := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)

	c, _ := domain.NewConfig("temp.x", `1`, now, "")
	if err := s.Configs.Set(ctx, c); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := s.Configs.Delete(ctx, "temp.x"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Configs.Get(ctx, "temp.x"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("Get after Delete = %v, want ErrNotFound", err)
	}

	if err := s.Configs.Delete(ctx, "temp.x"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("Delete again = %v, want ErrNotFound", err)
	}
}
