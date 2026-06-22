package gorm_test

import (
	"context"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/streaming"
)

// TestEventsPageAndCountForTopic exercises the page-number pagination
// primitives (PageForTopic + CountForTopic) the REST messages
// endpoint composes over. Ordering is newest-first, matching
// ListForTopic.
func TestEventsPageAndCountForTopic(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	ctx := context.Background()
	base := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)

	// Five events on s-a, one on s-b (noise that must not be counted or
	// paged into s-a results).
	for _, e := range []struct {
		id, st string
		offset time.Duration
	}{
		{"e-1", "s-a", 1 * time.Second},
		{"e-2", "s-a", 2 * time.Second},
		{"e-3", "s-a", 3 * time.Second},
		{"e-4", "s-a", 4 * time.Second},
		{"e-5", "s-a", 5 * time.Second},
		{"e-other", "s-b", 6 * time.Second},
	} {
		ev, _ := streaming.NewEvent(streaming.EventID(e.id), streaming.TopicID(e.st), "w-owner", "body", base.Add(e.offset), "org-test")
		if err := s.Events.Append(ctx, ev); err != nil {
			t.Fatalf("Append %s: %v", e.id, err)
		}
	}

	// Count is per-topic and independent of any page window.
	total, err := s.Events.CountForTopic(ctx, "org-test", "s-a")
	if err != nil {
		t.Fatalf("CountForTopic: %v", err)
	}
	if total != 5 {
		t.Fatalf("count = %d, want 5", total)
	}

	// First page: newest two.
	page1, err := s.Events.PageForTopic(ctx, "org-test", "s-a", 2, 0)
	if err != nil {
		t.Fatalf("PageForTopic page1: %v", err)
	}
	if len(page1) != 2 || page1[0].ID != "e-5" || page1[1].ID != "e-4" {
		t.Fatalf("page1 = %v, want [e-5 e-4]", ids(page1))
	}

	// Second page: offset 2.
	page2, err := s.Events.PageForTopic(ctx, "org-test", "s-a", 2, 2)
	if err != nil {
		t.Fatalf("PageForTopic page2: %v", err)
	}
	if len(page2) != 2 || page2[0].ID != "e-3" || page2[1].ID != "e-2" {
		t.Fatalf("page2 = %v, want [e-3 e-2]", ids(page2))
	}

	// Last (partial) page.
	page3, err := s.Events.PageForTopic(ctx, "org-test", "s-a", 2, 4)
	if err != nil {
		t.Fatalf("PageForTopic page3: %v", err)
	}
	if len(page3) != 1 || page3[0].ID != "e-1" {
		t.Fatalf("page3 = %v, want [e-1]", ids(page3))
	}

	// Out-of-range offset yields an empty page, not an error.
	empty, err := s.Events.PageForTopic(ctx, "org-test", "s-a", 2, 10)
	if err != nil {
		t.Fatalf("PageForTopic out-of-range: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("out-of-range page = %v, want []", ids(empty))
	}

	// Unknown topic: zero count, empty page.
	if c, err := s.Events.CountForTopic(ctx, "org-test", "s-missing"); err != nil || c != 0 {
		t.Fatalf("CountForTopic(missing) = %d, %v, want 0, nil", c, err)
	}
}

func ids(evs []streaming.Event) []streaming.EventID {
	out := make([]streaming.EventID, len(evs))
	for i, e := range evs {
		out[i] = e.ID
	}
	return out
}
