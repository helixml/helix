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
	"github.com/helixml/helix/api/pkg/org/application/topics"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	"github.com/helixml/helix/api/pkg/org/interfaces/jsonapi"
)

// ---- Topics ------------------------------------------------------------

// listTopics returns every topic + a unified recent-events firehose.
//
// @Summary Helix-org: list topics
// @Tags HelixOrg
// @Produce json
// @Success 200 {object} api.TopicsResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/topics [get]
func (a *apiHandler) listTopics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	topics, err := a.deps.Queries.ListTopics(ctx, orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("list topics: %w", err))
		return
	}
	sort.SliceStable(topics, func(i, j int) bool { return topics[i].CreatedAt.Before(topics[j].CreatedAt) })

	resp := TopicsResponse{Topics: make([]TopicDTO, 0, len(topics))}
	for _, s := range topics {
		dto := TopicDTO{
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
		subs, err := a.deps.Queries.TopicSubscribers(ctx, orgID, s.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Errorf("list subscriptions for %s: %w", s.ID, err))
			return
		}
		for _, sub := range subs {
			// Subscriptions are worker-anchored — return worker ids
			// directly. The Topics page renders them as chips.
			dto.Subscribers = append(dto.Subscribers, string(sub.WorkerID))
		}
		events, err := a.deps.Queries.TopicEvents(ctx, orgID, s.ID, 50)
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Errorf("list events for %s: %w", s.ID, err))
			return
		}
		for _, ev := range events {
			dto.RecentEvents = append(dto.RecentEvents, eventCard(ev))
		}
		resp.Topics = append(resp.Topics, dto)
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

// createTopic creates a new Topic. Mirrors the MCP create_topic
// tool — same Transport shape, same "id auto-falls-back-to-s-<uuid>"
// behaviour. CreatedBy comes from req.As (the Worker the human is acting
// as) and is optional — it is cosmetic, anchoring the topic's chart node
// to a Worker. An operator creating a topic from the Topics tab (no
// worker context) leaves it empty and the topic is unanchored.
//
// @Summary Helix-org: create a topic
// @Tags HelixOrg
// @Accept json
// @Produce json
// @Param payload body api.CreateTopicRequest true "Topic spec"
// @Success 201 {object} api.TopicDTO
// @Failure 400 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/topics [post]
func (a *apiHandler) createTopic(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var req CreateTopicRequest
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
	s, err := a.deps.Topics.Create(ctx, orgID, topics.CreateParams{
		ID:          strings.TrimSpace(req.ID),
		Name:        req.Name,
		Description: req.Description,
		CreatedBy:   strings.TrimSpace(req.As),
		Transport:   tr,
	})
	if err != nil {
		writeError(w, errStatus(err), fmt.Errorf("create topic: %w", err))
		return
	}
	writeJSON(w, http.StatusCreated, TopicDTO{
		ID:          string(s.ID),
		Name:        s.Name,
		Description: s.Description,
		Kind:        string(s.Transport.Kind),
		CreatedBy:   string(s.CreatedBy),
		CreatedAt:   s.CreatedAt.Format(time.RFC3339),
	})
}

// getTopic returns a single topic + its current subscribers and
// recent events. Powers the topic detail page.
//
// @Summary Helix-org: get a topic
// @Tags HelixOrg
// @Produce json
// @Param id path string true "Topic ID"
// @Success 200 {object} api.TopicDTO
// @Failure 404 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/topics/{id} [get]
func (a *apiHandler) getTopic(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	id := streaming.TopicID(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, errors.New("topic id is required"))
		return
	}
	s, err := a.deps.Queries.GetTopic(ctx, orgID, id)
	if err != nil {
		writeError(w, errStatus(err), fmt.Errorf("get topic %s: %w", id, err))
		return
	}
	dto := TopicDTO{
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
	if subs, err := a.deps.Queries.TopicSubscribers(ctx, orgID, s.ID); err == nil {
		for _, sub := range subs {
			// Subscriptions are worker-anchored — return worker ids
			// directly. The Topics page renders them as chips.
			dto.Subscribers = append(dto.Subscribers, string(sub.WorkerID))
		}
	}
	if events, err := a.deps.Queries.TopicEvents(ctx, orgID, s.ID, 50); err == nil {
		for _, ev := range events {
			dto.RecentEvents = append(dto.RecentEvents, eventCard(ev))
		}
	}
	writeJSON(w, http.StatusOK, dto)
}

// resolveEffectivePublicURL returns helix's public base URL (SERVER_URL),
// used for github webhook payload URLs. Surfaced in TopicDTO so the detail
// page can evaluate the loopback warning. Returns "" when SERVER_URL is unset.
func (a *apiHandler) resolveEffectivePublicURL(_ context.Context, _ string) string {
	return strings.TrimSpace(a.deps.PublicServerURL)
}

// transportConfigMap unmarshals a Transport.Config raw JSON blob
// into a typed map for the TopicDTO `config` field. Returns an
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

// updateTopic rewrites the mutable subset of a topic — name,
// description, and (optionally) transport kind + config. Returns
// the post-update TopicDTO so the UI can replace its cached row
// without a follow-up GET. Composite key (id, orgID) is enforced
// by the repo; cross-org id-guessing returns 404.
//
// @Summary Helix-org: update a topic
// @Tags HelixOrg
// @Accept json
// @Produce json
// @Param id path string true "Topic ID"
// @Param payload body api.UpdateTopicRequest true "Topic patch"
// @Success 200 {object} api.TopicDTO
// @Failure 400 {object} api.ErrorResponse
// @Failure 404 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/topics/{id} [put]
func (a *apiHandler) updateTopic(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	id := streaming.TopicID(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, errors.New("topic id is required"))
		return
	}
	var req UpdateTopicRequest
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
	var patch *topics.TransportPatch
	if req.Transport != nil {
		patch = &topics.TransportPatch{Kind: strings.TrimSpace(req.Transport.Kind)}
		if req.Transport.Config != nil {
			raw, err := json.Marshal(req.Transport.Config)
			if err != nil {
				writeError(w, http.StatusBadRequest, fmt.Errorf("encode transport config: %w", err))
				return
			}
			patch.Config = raw
		}
	}
	updated, err := a.deps.Topics.Update(ctx, orgID, id, topics.UpdateParams{
		Name:        req.Name,
		Description: req.Description,
		Transport:   patch,
	})
	if err != nil {
		writeError(w, errStatus(err), fmt.Errorf("update topic: %w", err))
		return
	}
	// Reuse getTopic's response shape — including subscribers,
	// recent events, and the parsed config map — so the UI just
	// swaps its cached row.
	dto := TopicDTO{
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
	if subs, err := a.deps.Queries.TopicSubscribers(ctx, orgID, updated.ID); err == nil {
		for _, sub := range subs {
			dto.Subscribers = append(dto.Subscribers, string(sub.WorkerID))
		}
	}
	if events, err := a.deps.Queries.TopicEvents(ctx, orgID, updated.ID, 50); err == nil {
		for _, ev := range events {
			dto.RecentEvents = append(dto.RecentEvents, eventCard(ev))
		}
	}
	writeJSON(w, http.StatusOK, dto)
}

// deleteTopic removes a topic row. Subscriptions and events are
// NOT cascade-deleted in this iteration — the caller is expected to
// drain them first via unsubscribe / publish flows. Empty topic
// rows are idempotent (404 → 404, no error).
//
// @Summary Helix-org: delete a topic
// @Tags HelixOrg
// @Param id path string true "Topic ID"
// @Success 204
// @Failure 404 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/topics/{id} [delete]
func (a *apiHandler) deleteTopic(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	id := streaming.TopicID(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, errors.New("topic id is required"))
		return
	}
	// Block deletion of a processor-owned output topic: deleting it
	// independently would leave the processor with a dangling output.
	// The caller should delete the processor instead (which cascades it).
	if a.deps.Processors != nil {
		if pid, owned, ownErr := a.deps.Processors.OwnerOfOutput(ctx, orgID, id); ownErr == nil && owned {
			writeError(w, http.StatusConflict, fmt.Errorf("topic %q is an output of processor %q; delete the processor instead", id, pid))
			return
		}
		// Tear down any Automated processor bound to this topic as its input
		// (the Slack auto-router), cascading its owned route topics, before the
		// topic goes — automation's lifecycle follows the topic it reads.
		// Human-authored processors reading the topic are left alone.
		if err := a.deps.Processors.DeleteAutomatedByInput(ctx, orgID, id); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Errorf("delete topic: tear down bound auto-router: %w", err))
			return
		}
	}
	if err := a.deps.Topics.Delete(ctx, orgID, id); err != nil {
		writeError(w, errStatus(err), fmt.Errorf("delete topic: %w", err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// eventCard converts a streaming.Event into its wire shape, expanding the
// canonical Message envelope when the body parses.
func eventCard(ev streaming.Event) EventCard {
	card := EventCard{
		ID:        string(ev.ID),
		TopicID:  string(ev.TopicID),
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

// messageResource maps an Event to a JSON:API `messages` resource.
// Mirrors eventCard's decode: the parsed Message envelope when the
// body holds Message JSON, the raw body otherwise.
func messageResource(ev streaming.Event) jsonapi.Resource {
	attrs := MessageAttributes{
		TopicID:  string(ev.TopicID),
		Source:    string(ev.Source),
		CreatedAt: ev.CreatedAt.Format(time.RFC3339),
		Body:      ev.Body,
	}
	if msg, err := ev.Message(); err == nil {
		attrs.HasMessage = true
		attrs.From = msg.From
		attrs.To = msg.To
		attrs.Subject = msg.Subject
		attrs.Body = msg.Body
		attrs.Raw = ev.Body // canonical Message envelope JSON (pre-decode)
	}
	return jsonapi.Resource{Type: "messages", ID: string(ev.ID), Attributes: attrs}
}

const (
	topicMessagesDefaultPageSize = 50
	topicMessagesMaxPageSize     = 200
)

// listTopicMessages returns the messages on one Topic as a paginated
// JSON:API document, newest first. meta.total carries the full count;
// links/meta carry the page-based pagination state. The JSON:API
// document is assembled by composing independent components (TotalMeta,
// Pagination) — see api/pkg/org/interfaces/jsonapi.
//
// @Summary Helix-org: list a topic's messages (JSON:API, paginated)
// @Tags HelixOrg
// @Produce application/vnd.api+json
// @Param id path string true "Topic ID"
// @Param page[number] query int false "1-based page number (default 1)"
// @Param page[size] query int false "page size (default 50, max 200)"
// @Success 200 {object} api.MessagesDocument
// @Failure 400 {object} api.ErrorResponse
// @Failure 404 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/topics/{id}/messages [get]
func (a *apiHandler) listTopicMessages(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	id := streaming.TopicID(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, errors.New("topic id is required"))
		return
	}
	page, err := jsonapi.PageParams(r, topicMessagesDefaultPageSize, topicMessagesMaxPageSize)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	// Unknown topic → 404 (don't silently return an empty page).
	if _, err := a.deps.Queries.GetTopic(ctx, orgID, id); err != nil {
		writeError(w, errStatus(err), fmt.Errorf("get topic %s: %w", id, err))
		return
	}
	total, err := a.deps.Queries.CountTopicEvents(ctx, orgID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("count messages for %s: %w", id, err))
		return
	}
	events, err := a.deps.Queries.PageTopicEvents(ctx, orgID, id, page.Limit(), page.Offset())
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("list messages for %s: %w", id, err))
		return
	}
	resources := make([]jsonapi.Resource, 0, len(events))
	for _, ev := range events {
		resources = append(resources, messageResource(ev))
	}
	doc := jsonapi.NewDocument(
		resources,
		jsonapi.TotalMeta{Total: total},
		jsonapi.Pagination{Number: page.Number, Size: page.Size, Total: total, Query: r.URL.Query()},
	)
	jsonapi.Write(w, http.StatusOK, doc)
}

// topicEventsSSE pushes EventCard JSON arrays on every Hub.Notify.
//
// Each SSE `data:` line is a JSON array of recent events (cap 50,
// newest first). Frontends replace their event list on every message
// — simpler than diffing partial updates.
//
// @Summary Helix-org: SSE topic of events for one topic
// @Tags HelixOrg
// @Produce text/event-stream
// @Param id path string true "Topic ID"
// @Success 200 {string} string "SSE: event: message / data: [EventCard,...]"
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/topics/{id}/events [get]
func (a *apiHandler) topicEventsSSE(w http.ResponseWriter, r *http.Request) {
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
	topicID := r.PathValue("id")
	if topicID == "" {
		writeError(w, http.StatusBadRequest, errors.New("topic id is required"))
		return
	}
	wake := a.deps.Hub.Subscribe(orgID, []streaming.TopicID{streaming.TopicID(topicID)})
	defer a.deps.Hub.Unsubscribe([]streaming.TopicID{streaming.TopicID(topicID)}, wake)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	emit := func() error {
		events, err := a.deps.Queries.TopicEvents(r.Context(), orgID, streaming.TopicID(topicID), 50)
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

// publishToTopic appends a Message event attributed to the owner
// and fans it out to subscribers via the dispatcher. Consumes JSON
// and returns the new event's ID.
//
// @Summary Helix-org: publish a message to a topic
// @Tags HelixOrg
// @Accept json
// @Produce json
// @Param id path string true "Topic ID"
// @Param payload body api.PublishRequest true "Message body+optional subject/to"
// @Success 201 {object} api.PublishResponse
// @Failure 400 {object} api.ErrorResponse
// @Failure 409 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/topics/{id}/publish [post]
func (a *apiHandler) publishToTopic(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	topicID := streaming.TopicID(r.PathValue("id"))
	if topicID == "" {
		writeError(w, http.StatusBadRequest, errors.New("topic id is required"))
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
	ev, err := a.deps.Publishing.Publish(ctx, orgID, topicID, strings.TrimSpace(req.As), msg)
	if err != nil {
		if errors.Is(err, publishing.ErrPublishToGitHub) {
			writeError(w, http.StatusConflict, err)
			return
		}
		writeError(w, errStatus(err), fmt.Errorf("publish to topic %s: %w", topicID, err))
		return
	}
	writeJSON(w, http.StatusCreated, PublishResponse{EventID: string(ev.ID)})
}
