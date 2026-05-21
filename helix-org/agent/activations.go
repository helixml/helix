package agent

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/org/event"
	"github.com/helixml/helix/api/pkg/org/worker"
	"github.com/helixml/helix/helix-org/broadcast"
	"github.com/helixml/helix/helix-org/domain"
	"github.com/helixml/helix/helix-org/store"
)

// PublishActivationEvent appends one event to s-activations-<workerID>
// with the canonical Message envelope ({from: workerID, body}) and
// notifies the broadcaster so live UI subscribers see it. This is the
// single helper every Worker activation — whether driven by the
// event-dispatcher (AI Workers) or by the human chat surface (the
// owner) — uses to record its turn. Without one shared writer the
// data model splits: AI Workers have activation streams the operator
// can replay; the owner has a private SSE side-channel that vanishes
// when the browser tab closes.
//
// The owner is just-another-Worker. Its activations live on
// s-activations-w-owner alongside every AI Worker's.
//
// Returns (ID, nil) on success so callers can correlate transcript
// segments with the originating activation. A nil store / broadcaster
// / clock / id-source short-circuits to ("", nil) — useful in tests
// that don't wire the full org-graph dependencies.
func PublishActivationEvent(
	ctx context.Context,
	st *store.Store,
	bc *broadcast.Broadcaster,
	newID func() string,
	now func() time.Time,
	logger *slog.Logger,
	workerID worker.ID,
	body string,
) (event.ID, error) {
	if st == nil || newID == nil || now == nil || strings.TrimSpace(body) == "" {
		return "", nil
	}
	streamID := ActivationStreamID(workerID)
	event, err := domain.NewMessageEvent(
		event.ID("e-"+newID()),
		streamID,
		workerID,
		domain.Message{From: string(workerID), Body: body},
		now(),
	)
	if err != nil {
		if logger != nil {
			logger.Warn("publish activation event: build", "worker", workerID, "err", err)
		}
		return "", err
	}
	if err := st.Events.Append(ctx, event); err != nil {
		if logger != nil {
			logger.Warn("publish activation event: append", "worker", workerID, "err", err)
		}
		return "", err
	}
	if bc != nil {
		bc.Notify(streamID)
	}
	return event.ID, nil
}
