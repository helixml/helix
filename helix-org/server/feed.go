package server

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/helixml/helix-org/domain"
)

// Defaults and caps for feed pagination and long-polling.
const (
	feedDefaultLimit = 50
	feedMaxLimit     = 200
	feedMaxWaitSecs  = 60
)

type feedEventAttributes struct {
	ChannelID domain.ChannelID `json:"channelId"`
	Source    domain.WorkerID  `json:"source"`
	Body      string           `json:"body"`
	CreatedAt time.Time        `json:"createdAt"`
}

// listFeed returns the events reaching the Worker's Feed, newest first.
//
// Query parameters:
//   - limit=<n>    — page size, 1..200, default 50.
//   - since=<id>   — return only events newer than this one (present in the
//     current feed page). Walks the newest-first result and returns
//     everything up to but not including the named event.
//   - wait=<secs>  — if, after filtering by `since`, the result is empty,
//     block up to this many seconds waiting for a new Event on any
//     Channel the Worker subscribes to, then return whatever is newer
//     than `since` at that moment (which may still be empty on timeout).
//     Capped at feedMaxWaitSecs (60).
//
// The Worker must exist.
func (s *Server) listFeed(w http.ResponseWriter, r *http.Request) {
	workerID := domain.WorkerID(r.PathValue("id"))
	if _, err := s.store.Workers.Get(r.Context(), workerID); err != nil {
		writeStoreError(w, err, "list feed")
		return
	}

	limit, err := parseFeedLimit(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid limit", err.Error())
		return
	}
	waitSecs, err := parseFeedWait(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid wait", err.Error())
		return
	}
	since := domain.EventID(r.URL.Query().Get("since"))

	fresh, err := s.readFresh(r.Context(), workerID, limit, since)
	if err != nil {
		writeStoreError(w, err, "list feed")
		return
	}
	if len(fresh) > 0 || waitSecs == 0 || s.broadcaster == nil {
		writeFeedEvents(w, fresh)
		return
	}

	// Long-poll: subscribe to the Worker's Channels, then wait.
	streams, err := s.store.Streams.ListForWorker(r.Context(), workerID)
	if err != nil {
		writeStoreError(w, err, "list feed")
		return
	}
	channelIDs := make([]domain.ChannelID, 0, len(streams))
	for _, st := range streams {
		channelIDs = append(channelIDs, st.ChannelID)
	}
	wake := s.broadcaster.Subscribe(channelIDs)
	defer s.broadcaster.Unsubscribe(channelIDs, wake)

	timer := time.NewTimer(time.Duration(waitSecs) * time.Second)
	defer timer.Stop()

	select {
	case <-wake:
	case <-timer.C:
	case <-r.Context().Done():
		return
	}

	// Re-query — the wake-up may have coalesced multiple events.
	fresh, err = s.readFresh(r.Context(), workerID, limit, since)
	if err != nil {
		writeStoreError(w, err, "list feed")
		return
	}
	writeFeedEvents(w, fresh)
}

// readFresh returns events newer than `since` (exclusive), newest-first,
// up to `limit`. An empty `since` means "return everything".
func (s *Server) readFresh(ctx context.Context, workerID domain.WorkerID, limit int, since domain.EventID) ([]domain.Event, error) {
	events, err := s.store.Events.ListForWorker(ctx, workerID, limit)
	if err != nil {
		return nil, err
	}
	if since == "" {
		return events, nil
	}
	for i, e := range events {
		if e.ID == since {
			return events[:i], nil
		}
	}
	return events, nil
}

func parseFeedLimit(r *http.Request) (int, error) {
	raw := r.URL.Query().Get("limit")
	if raw == "" {
		return feedDefaultLimit, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return 0, &parseErr{"limit must be a positive integer"}
	}
	if n > feedMaxLimit {
		n = feedMaxLimit
	}
	return n, nil
}

func parseFeedWait(r *http.Request) (int, error) {
	raw := r.URL.Query().Get("wait")
	if raw == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 0, &parseErr{"wait must be a non-negative integer"}
	}
	if n > feedMaxWaitSecs {
		n = feedMaxWaitSecs
	}
	return n, nil
}

type parseErr struct{ msg string }

func (e *parseErr) Error() string { return e.msg }

func writeFeedEvents(w http.ResponseWriter, events []domain.Event) {
	out := make([]Resource, 0, len(events))
	for _, e := range events {
		out = append(out, Resource{
			Type: "events",
			ID:   string(e.ID),
			Attributes: mustAttributes(feedEventAttributes{
				ChannelID: e.ChannelID,
				Source:    e.Source,
				Body:      e.Body,
				CreatedAt: e.CreatedAt,
			}),
		})
	}
	writeCollection(w, http.StatusOK, out)
}
