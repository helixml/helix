package activations

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/streamhub"
	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
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
	bc *streamhub.Hub,
	newID func() string,
	now func() time.Time,
	logger *slog.Logger,
	orgID string,
	workerID orgchart.WorkerID,
	body string,
) (streaming.EventID, error) {
	if st == nil || newID == nil || now == nil || strings.TrimSpace(body) == "" {
		return "", nil
	}
	streamID := activation.StreamID(workerID)
	event, err := streaming.NewMessageEvent(
		streaming.EventID("e-"+newID()),
		streamID,
		workerID,
		streaming.Message{From: string(workerID), Body: body},
		now(),
		orgID,
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
		bc.Notify(orgID, streamID)
	}
	return event.ID, nil
}
