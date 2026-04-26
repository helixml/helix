package server

import (
	"context"
	"net/http"
	"path"
	"strconv"
	"time"

	"github.com/helixml/helix-org/domain"
)

// Defaults and caps for the tail endpoint.
const (
	tailDefaultLimit = 100
	tailMaxLimit     = 500
	tailMaxWaitSecs  = 60
)

// tail returns events from any Channel whose ID matches one of the
// supplied glob patterns, oldest-first.
//
// Query parameters:
//   - match=<glob> — repeatable. Matched against Channel ID via path.Match
//     ("c-*", "c-news*", "*"). Default: "*" (all channels). When more than
//     one is supplied the union is returned.
//   - since=<event-id> — return only events strictly newer than this
//     event. Stale or unknown IDs fall through to "no lower bound".
//   - limit=<n> — page size, 1..500, default 100.
//   - wait=<secs> — if the result is empty, block up to this many seconds
//     waiting for any event on any matching Channel. Capped at 60.
//
// Future work: the patterns currently always select Channels. When other
// stream sources land (per-Worker activation logs, system audit, etc.) the
// shape will gain a namespace prefix — e.g. "channel:c-*",
// "activation:w-*". Bare globs stay channel-scoped for back-compat.
func (s *Server) tail(w http.ResponseWriter, r *http.Request) {
	patterns := r.URL.Query()["match"]
	if len(patterns) == 0 {
		patterns = []string{"*"}
	}
	if err := validatePatterns(patterns); err != nil {
		writeError(w, http.StatusBadRequest, "invalid match", err.Error())
		return
	}
	limit, err := parseTailLimit(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid limit", err.Error())
		return
	}
	waitSecs, err := parseTailWait(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid wait", err.Error())
		return
	}
	since := domain.EventID(r.URL.Query().Get("since"))

	matching, err := s.matchChannels(r.Context(), patterns)
	if err != nil {
		writeStoreError(w, err, "tail")
		return
	}
	fresh, err := s.store.Events.ListSince(r.Context(), matching, since, limit)
	if err != nil {
		writeStoreError(w, err, "tail")
		return
	}
	if len(fresh) > 0 || waitSecs == 0 || s.broadcaster == nil {
		writeFeedEvents(w, fresh)
		return
	}

	// Long-poll. Use SubscribeAll so that channels created mid-tail
	// (e.g. by an Editor's hire trigger) still wake us; we re-resolve the
	// matching set after the wake.
	wake := s.broadcaster.SubscribeAll()
	defer s.broadcaster.UnsubscribeAll(wake)

	timer := time.NewTimer(time.Duration(waitSecs) * time.Second)
	defer timer.Stop()

	select {
	case <-wake:
	case <-timer.C:
	case <-r.Context().Done():
		return
	}

	matching, err = s.matchChannels(r.Context(), patterns)
	if err != nil {
		writeStoreError(w, err, "tail")
		return
	}
	fresh, err = s.store.Events.ListSince(r.Context(), matching, since, limit)
	if err != nil {
		writeStoreError(w, err, "tail")
		return
	}
	writeFeedEvents(w, fresh)
}

// matchChannels lists every Channel and returns the IDs whose name
// matches any of the supplied glob patterns.
func (s *Server) matchChannels(ctx context.Context, patterns []string) ([]domain.ChannelID, error) {
	all, err := s.store.Channels.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]domain.ChannelID, 0, len(all))
	for _, c := range all {
		for _, p := range patterns {
			ok, _ := path.Match(p, string(c.ID))
			if ok {
				out = append(out, c.ID)
				break
			}
		}
	}
	return out, nil
}

func validatePatterns(patterns []string) error {
	for _, p := range patterns {
		if _, err := path.Match(p, ""); err != nil {
			return &parseErr{"bad pattern " + strconv.Quote(p) + ": " + err.Error()}
		}
	}
	return nil
}

func parseTailLimit(r *http.Request) (int, error) {
	raw := r.URL.Query().Get("limit")
	if raw == "" {
		return tailDefaultLimit, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return 0, &parseErr{"limit must be a positive integer"}
	}
	if n > tailMaxLimit {
		n = tailMaxLimit
	}
	return n, nil
}

func parseTailWait(r *http.Request) (int, error) {
	raw := r.URL.Query().Get("wait")
	if raw == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 0, &parseErr{"wait must be a non-negative integer"}
	}
	if n > tailMaxWaitSecs {
		n = tailMaxWaitSecs
	}
	return n, nil
}
