package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/publishing"
	"github.com/helixml/helix/api/pkg/org/application/streams"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
)

// ---- Streams ------------------------------------------------------------

// listStreams returns every stream + a unified recent-events firehose.
//
// @Summary Helix-org: list streams
// @Tags HelixOrg
// @Produce json
// @Success 200 {object} api.StreamsResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/streams [get]
func (a *apiHandler) listStreams(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	streams, err := a.deps.Queries.ListStreams(ctx, orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("list streams: %w", err))
		return
	}
	sort.SliceStable(streams, func(i, j int) bool { return streams[i].CreatedAt.Before(streams[j].CreatedAt) })

	resp := StreamsResponse{Streams: make([]StreamDTO, 0, len(streams))}
	for _, s := range streams {
		dto := StreamDTO{
			ID:          string(s.ID),
			Name:        s.Name,
			Description: s.Description,
			Kind:        string(s.Transport.Kind),
			CreatedBy:   string(s.CreatedBy),
			CreatedAt:   s.CreatedAt.Format(time.RFC3339),
		}
		dto.CanPublish = s.Transport.Kind != transport.KindGitHub
		if !dto.CanPublish {
			dto.DisableReason = "github transport is inbound only — act on the repo with `gh` from the worker's environment"
			dto.EffectivePublicURL = a.resolveEffectivePublicURL(ctx, orgID)
		}
		if cfg, err := transportConfigMap(s.Transport); err == nil {
			dto.Config = cfg
		}
		subs, err := a.deps.Queries.StreamSubscribers(ctx, orgID, s.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Errorf("list subscriptions for %s: %w", s.ID, err))
			return
		}
		for _, sub := range subs {
			// Subscriptions are worker-anchored — return worker ids
			// directly. The Streams page renders them as chips.
			dto.Subscribers = append(dto.Subscribers, string(sub.WorkerID))
		}
		events, err := a.deps.Queries.StreamEvents(ctx, orgID, s.ID, 50)
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Errorf("list events for %s: %w", s.ID, err))
			return
		}
		for _, ev := range events {
			dto.RecentEvents = append(dto.RecentEvents, eventCard(ev))
		}
		resp.Streams = append(resp.Streams, dto)
	}

	recent, err := a.deps.Queries.AllEvents(ctx, orgID, 50)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("list all events: %w", err))
		return
	}
	for _, ev := range recent {
		resp.Recent = append(resp.Recent, eventCard(ev))
	}
	writeJSON(w, http.StatusOK, resp)
}

// createStream creates a new Stream. Mirrors the MCP create_stream
// tool — same Transport shape, same "id auto-falls-back-to-s-<uuid>"
// behaviour. CreatedBy is set to the embedded owner worker so REST
// + chat creations are attributable.
//
// @Summary Helix-org: create a stream
// @Tags HelixOrg
// @Accept json
// @Produce json
// @Param payload body api.CreateStreamRequest true "Stream spec"
// @Success 201 {object} api.StreamDTO
// @Failure 400 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/streams [post]
func (a *apiHandler) createStream(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var req CreateStreamRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, errors.New("name is required"))
		return
	}
	tr := transport.Transport{}
	if req.Transport != nil {
		tr.Kind = transport.Kind(req.Transport.Kind)
		if len(req.Transport.Config) > 0 {
			raw, err := json.Marshal(req.Transport.Config)
			if err != nil {
				writeError(w, http.StatusBadRequest, fmt.Errorf("encode transport config: %w", err))
				return
			}
			tr.Config = raw
		}
	}
	s, err := a.deps.Streams.Create(ctx, orgID, streams.CreateParams{
		ID:          strings.TrimSpace(req.ID),
		Name:        req.Name,
		Description: req.Description,
		CreatedBy:   strings.TrimSpace(req.As),
		Transport:   tr,
	})
	if err != nil {
		writeError(w, errStatus(err), fmt.Errorf("create stream: %w", err))
		return
	}
	writeJSON(w, http.StatusCreated, StreamDTO{
		ID:          string(s.ID),
		Name:        s.Name,
		Description: s.Description,
		Kind:        string(s.Transport.Kind),
		CreatedBy:   string(s.CreatedBy),
		CreatedAt:   s.CreatedAt.Format(time.RFC3339),
	})
}

// getStream returns a single stream + its current subscribers and
// recent events. Powers the stream detail page.
//
// @Summary Helix-org: get a stream
// @Tags HelixOrg
// @Produce json
// @Param id path string true "Stream ID"
// @Success 200 {object} api.StreamDTO
// @Failure 404 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/streams/{id} [get]
func (a *apiHandler) getStream(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	id := streaming.StreamID(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, errors.New("stream id is required"))
		return
	}
	s, err := a.deps.Queries.GetStream(ctx, orgID, id)
	if err != nil {
		writeError(w, errStatus(err), fmt.Errorf("get stream %s: %w", id, err))
		return
	}
	dto := StreamDTO{
		ID:          string(s.ID),
		Name:        s.Name,
		Description: s.Description,
		Kind:        string(s.Transport.Kind),
		CreatedBy:   string(s.CreatedBy),
		CreatedAt:   s.CreatedAt.Format(time.RFC3339),
	}
	dto.CanPublish = s.Transport.Kind != transport.KindGitHub
	if !dto.CanPublish {
		dto.DisableReason = "github transport is inbound only — act on the repo with `gh` from the worker's environment"
		dto.EffectivePublicURL = a.resolveEffectivePublicURL(ctx, orgID)
	}
	if cfg, err := transportConfigMap(s.Transport); err == nil {
		dto.Config = cfg
	}
	if subs, err := a.deps.Queries.StreamSubscribers(ctx, orgID, s.ID); err == nil {
		for _, sub := range subs {
			// Subscriptions are worker-anchored — return worker ids
			// directly. The Streams page renders them as chips.
			dto.Subscribers = append(dto.Subscribers, string(sub.WorkerID))
		}
	}
	if events, err := a.deps.Queries.StreamEvents(ctx, orgID, s.ID, 50); err == nil {
		for _, ev := range events {
			dto.RecentEvents = append(dto.RecentEvents, eventCard(ev))
		}
	}
	writeJSON(w, http.StatusOK, dto)
}

// resolveEffectivePublicURL returns the base URL helix uses for
// github webhook payload URLs, in the same priority order as
// installGitHubWebhook: `streams.public_url` org config wins,
// SERVER_URL (env) is the fallback. Returns "" when neither is
// set. Surfaced in StreamDTO so the detail page can evaluate the
// loopback warning without re-implementing the priority logic.
func (a *apiHandler) resolveEffectivePublicURL(ctx context.Context, orgID string) string {
	envURL := strings.TrimSpace(a.deps.PublicServerURL)
	if a.deps.Configs != nil {
		if override, err := a.deps.Configs.GetString(ctx, orgID, "streams.public_url"); err == nil {
			if v := strings.TrimSpace(override); v != "" {
				return v
			}
		}
	}
	return envURL
}

// transportConfigMap unmarshals a Transport.Config raw JSON blob
// into a typed map for the StreamDTO `config` field. Returns an
// empty map when the transport has no config (local kind).
func transportConfigMap(t transport.Transport) (map[string]interface{}, error) {
	if len(t.Config) == 0 {
		return nil, nil
	}
	var out map[string]interface{}
	if err := json.Unmarshal(t.Config, &out); err != nil {
		return nil, fmt.Errorf("decode transport config: %w", err)
	}
	return out, nil
}

// updateStream rewrites the mutable subset of a stream — name,
// description, and (optionally) transport kind + config. Returns
// the post-update StreamDTO so the UI can replace its cached row
// without a follow-up GET. Composite key (id, orgID) is enforced
// by the repo; cross-org id-guessing returns 404.
//
// @Summary Helix-org: update a stream
// @Tags HelixOrg
// @Accept json
// @Produce json
// @Param id path string true "Stream ID"
// @Param payload body api.UpdateStreamRequest true "Stream patch"
// @Success 200 {object} api.StreamDTO
// @Failure 400 {object} api.ErrorResponse
// @Failure 404 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/streams/{id} [put]
func (a *apiHandler) updateStream(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	id := streaming.StreamID(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, errors.New("stream id is required"))
		return
	}
	var req UpdateStreamRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, errors.New("name is required"))
		return
	}
	// Build the transport patch: replace the kind when the caller
	// supplies a non-empty kind, replace the config when the caller
	// supplies one (the typical "tweak the github repo or events
	// whitelist" flow). The service owns the read-modify-write merge.
	var patch *streams.TransportPatch
	if req.Transport != nil {
		patch = &streams.TransportPatch{Kind: strings.TrimSpace(req.Transport.Kind)}
		if req.Transport.Config != nil {
			raw, err := json.Marshal(req.Transport.Config)
			if err != nil {
				writeError(w, http.StatusBadRequest, fmt.Errorf("encode transport config: %w", err))
				return
			}
			patch.Config = raw
		}
	}
	updated, err := a.deps.Streams.Update(ctx, orgID, id, streams.UpdateParams{
		Name:        req.Name,
		Description: req.Description,
		Transport:   patch,
	})
	if err != nil {
		writeError(w, errStatus(err), fmt.Errorf("update stream: %w", err))
		return
	}
	// Reuse getStream's response shape — including subscribers,
	// recent events, and the parsed config map — so the UI just
	// swaps its cached row.
	dto := StreamDTO{
		ID:          string(updated.ID),
		Name:        updated.Name,
		Description: updated.Description,
		Kind:        string(updated.Transport.Kind),
		CreatedBy:   string(updated.CreatedBy),
		CreatedAt:   updated.CreatedAt.Format(time.RFC3339),
	}
	dto.CanPublish = updated.Transport.Kind != transport.KindGitHub
	if !dto.CanPublish {
		dto.DisableReason = "github transport is inbound only — act on the repo with `gh` from the worker's environment"
		dto.EffectivePublicURL = a.resolveEffectivePublicURL(ctx, orgID)
	}
	if cfg, err := transportConfigMap(updated.Transport); err == nil {
		dto.Config = cfg
	}
	if subs, err := a.deps.Queries.StreamSubscribers(ctx, orgID, updated.ID); err == nil {
		for _, sub := range subs {
			dto.Subscribers = append(dto.Subscribers, string(sub.WorkerID))
		}
	}
	if events, err := a.deps.Queries.StreamEvents(ctx, orgID, updated.ID, 50); err == nil {
		for _, ev := range events {
			dto.RecentEvents = append(dto.RecentEvents, eventCard(ev))
		}
	}
	writeJSON(w, http.StatusOK, dto)
}

// deleteStream removes a stream row. Subscriptions and events are
// NOT cascade-deleted in this iteration — the caller is expected to
// drain them first via unsubscribe / publish flows. Empty stream
// rows are idempotent (404 → 404, no error).
//
// @Summary Helix-org: delete a stream
// @Tags HelixOrg
// @Param id path string true "Stream ID"
// @Success 204
// @Failure 404 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/streams/{id} [delete]
func (a *apiHandler) deleteStream(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	id := streaming.StreamID(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, errors.New("stream id is required"))
		return
	}
	if err := a.deps.Streams.Delete(ctx, orgID, id); err != nil {
		writeError(w, errStatus(err), fmt.Errorf("delete stream: %w", err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// eventCard converts a streaming.Event into its wire shape, expanding the
// canonical Message envelope when the body parses.
func eventCard(ev streaming.Event) EventCard {
	card := EventCard{
		ID:        string(ev.ID),
		StreamID:  string(ev.StreamID),
		Source:    string(ev.Source),
		CreatedAt: ev.CreatedAt.Format(time.RFC3339),
		Body:      ev.Body,
	}
	if msg, err := ev.Message(); err == nil {
		card.HasMessage = true
		card.From = msg.From
		card.Subject = msg.Subject
		card.MessageBody = msg.Body
		if len(msg.To) > 0 {
			card.To = strings.Join(msg.To, ", ")
		}
	}
	return card
}

// streamEventsSSE pushes EventCard JSON arrays on every Hub.Notify.
//
// Each SSE `data:` line is a JSON array of recent events (cap 50,
// newest first). Frontends replace their event list on every message
// — simpler than diffing partial updates.
//
// @Summary Helix-org: SSE stream of events for one stream
// @Tags HelixOrg
// @Produce text/event-stream
// @Param id path string true "Stream ID"
// @Success 200 {string} string "SSE: event: message / data: [EventCard,...]"
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/streams/{id}/events [get]
func (a *apiHandler) streamEventsSSE(w http.ResponseWriter, r *http.Request) {
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, errors.New("streaming unsupported"))
		return
	}
	if a.deps.Hub == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("broadcaster not configured"))
		return
	}
	streamID := r.PathValue("id")
	if streamID == "" {
		writeError(w, http.StatusBadRequest, errors.New("stream id is required"))
		return
	}
	wake := a.deps.Hub.Subscribe(orgID, []streaming.StreamID{streaming.StreamID(streamID)})
	defer a.deps.Hub.Unsubscribe([]streaming.StreamID{streaming.StreamID(streamID)}, wake)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	emit := func() error {
		events, err := a.deps.Queries.StreamEvents(r.Context(), orgID, streaming.StreamID(streamID), 50)
		if err != nil {
			return err
		}
		cards := make([]EventCard, 0, len(events))
		for _, ev := range events {
			cards = append(cards, eventCard(ev))
		}
		payload, err := json.Marshal(cards)
		if err != nil {
			return err
		}
		// SSE data lines must not embed raw newlines; JSON marshal of
		// a slice never produces newlines.
		_, _ = fmt.Fprint(w, "event: message\n")
		_, _ = fmt.Fprintf(w, "data: %s\n\n", payload)
		flusher.Flush()
		return nil
	}

	if err := emit(); err != nil {
		return
	}

	ping := time.NewTicker(15 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-wake:
			if err := emit(); err != nil {
				return
			}
		case <-ping.C:
			_, _ = fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}

// publishToStream appends a Message event attributed to the owner
// and fans it out to subscribers via the dispatcher. Consumes JSON
// and returns the new event's ID.
//
// @Summary Helix-org: publish a message to a stream
// @Tags HelixOrg
// @Accept json
// @Produce json
// @Param id path string true "Stream ID"
// @Param payload body api.PublishRequest true "Message body+optional subject/to"
// @Success 201 {object} api.PublishResponse
// @Failure 400 {object} api.ErrorResponse
// @Failure 409 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/streams/{id}/publish [post]
func (a *apiHandler) publishToStream(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	streamID := streaming.StreamID(r.PathValue("id"))
	if streamID == "" {
		writeError(w, http.StatusBadRequest, errors.New("stream id is required"))
		return
	}
	var req PublishRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if strings.TrimSpace(req.Body) == "" {
		writeError(w, http.StatusBadRequest, errors.New("body is required"))
		return
	}
	msg := streaming.Message{
		To:      req.To,
		Subject: strings.TrimSpace(req.Subject),
		Body:    req.Body,
	}
	ev, err := a.deps.Publishing.Publish(ctx, orgID, streamID, strings.TrimSpace(req.As), msg)
	if err != nil {
		if errors.Is(err, publishing.ErrPublishToGitHub) {
			writeError(w, http.StatusConflict, err)
			return
		}
		writeError(w, errStatus(err), fmt.Errorf("publish to stream %s: %w", streamID, err))
		return
	}
	writeJSON(w, http.StatusCreated, PublishResponse{EventID: string(ev.ID)})
}
