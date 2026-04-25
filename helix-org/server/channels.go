package server

import (
	"context"
	"net/http"
	"time"

	"github.com/helixml/helix-org/domain"
)

type channelAttributes struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	CreatedBy   domain.WorkerID `json:"createdBy"`
	CreatedAt   time.Time       `json:"createdAt"`
}

func channelResource(c domain.Channel) Resource {
	return Resource{
		Type: "channels",
		ID:   string(c.ID),
		Attributes: mustAttributes(channelAttributes{
			Name:        c.Name,
			Description: c.Description,
			CreatedBy:   c.CreatedBy,
			CreatedAt:   c.CreatedAt,
		}),
	}
}

// listChannels returns every Channel in the org. Useful for a human
// operator to discover what's worth tailing.
func (s *Server) listChannels(w http.ResponseWriter, r *http.Request) {
	channels, err := s.store.Channels.List(r.Context())
	if err != nil {
		writeStoreError(w, err, "list channels")
		return
	}
	out := make([]Resource, 0, len(channels))
	for _, c := range channels {
		out = append(out, channelResource(c))
	}
	writeCollection(w, http.StatusOK, out)
}

func (s *Server) getChannel(w http.ResponseWriter, r *http.Request) {
	id := domain.ChannelID(r.PathValue("id"))
	ch, err := s.store.Channels.Get(r.Context(), id)
	if err != nil {
		writeStoreError(w, err, "get channel")
		return
	}
	writeResource(w, http.StatusOK, channelResource(ch))
}

// listChannelEvents long-polls events on a single Channel. Shares the
// ?since, ?wait, ?limit semantics of the feed endpoint — which makes it
// trivial for curl-based observers to `tail -f` a channel.
func (s *Server) listChannelEvents(w http.ResponseWriter, r *http.Request) {
	channelID := domain.ChannelID(r.PathValue("id"))
	if _, err := s.store.Channels.Get(r.Context(), channelID); err != nil {
		writeStoreError(w, err, "list channel events")
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

	fresh, err := s.readChannelFresh(r.Context(), channelID, limit, since)
	if err != nil {
		writeStoreError(w, err, "list channel events")
		return
	}
	if len(fresh) > 0 || waitSecs == 0 || s.broadcaster == nil {
		writeFeedEvents(w, fresh)
		return
	}

	wake := s.broadcaster.Subscribe([]domain.ChannelID{channelID})
	defer s.broadcaster.Unsubscribe([]domain.ChannelID{channelID}, wake)

	timer := time.NewTimer(time.Duration(waitSecs) * time.Second)
	defer timer.Stop()

	select {
	case <-wake:
	case <-timer.C:
	case <-r.Context().Done():
		return
	}

	fresh, err = s.readChannelFresh(r.Context(), channelID, limit, since)
	if err != nil {
		writeStoreError(w, err, "list channel events")
		return
	}
	writeFeedEvents(w, fresh)
}

// readChannelFresh returns events on the channel newer than `since`
// (exclusive), newest-first, up to `limit`.
func (s *Server) readChannelFresh(ctx context.Context, channelID domain.ChannelID, limit int, since domain.EventID) ([]domain.Event, error) {
	events, err := s.store.Events.ListForChannel(ctx, channelID, limit)
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
