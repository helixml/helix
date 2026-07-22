package memory_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
)

func TestEventsAppendRejectsDuplicateID(t *testing.T) {
	repo := memory.New().Events
	event, err := streaming.NewEvent("e-1", "s-1", "", "body", time.Now(), "org-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Append(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	if err := repo.Append(context.Background(), event); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("duplicate error=%v", err)
	}
}
