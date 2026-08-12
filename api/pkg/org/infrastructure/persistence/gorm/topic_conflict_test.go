package gorm_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
)

// TestTopicsCreate_DuplicateNameMapsToConflict verifies the gorm store maps
// the idx_topic_org_name unique violation to store.ErrConflict (the race
// backstop behind the topics service pre-check), against a real DB.
func TestTopicsCreate_DuplicateNameMapsToConflict(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	ctx := context.Background()
	now := time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)

	mk := func(id string) streaming.Topic {
		tp, err := streaming.NewTopic(streaming.TopicID(id), "general", "", "", now, transport.Transport{}, "org-1")
		if err != nil {
			t.Fatalf("NewTopic: %v", err)
		}
		return tp
	}
	if err := s.Topics.Create(ctx, mk("s-a")); err != nil {
		t.Fatalf("first create: %v", err)
	}
	if err := s.Topics.Create(ctx, mk("s-b")); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("duplicate name: want store.ErrConflict, got %v", err)
	}
	// Different org, same name is fine.
	tp, _ := streaming.NewTopic("s-c", "general", "", "", now, transport.Transport{}, "org-2")
	if err := s.Topics.Create(ctx, tp); err != nil {
		t.Fatalf("same name, other org should be allowed: %v", err)
	}
}
