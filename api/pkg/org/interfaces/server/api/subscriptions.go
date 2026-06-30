package api

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
)

// listBotSubscriptions returns the bot's current subscription set.
// Drives the Bot detail page's Subscriptions panel.
//
// @Summary Helix-org: list a bot's subscriptions
// @Tags HelixOrg
// @Param id path string true "Bot ID"
// @Success 200 {object} api.BotSubscriptionsResponse
// @Failure 404 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/bots/{id}/subscriptions [get]
func (a *apiHandler) listBotSubscriptions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	bid := orgchart.BotID(r.PathValue("id"))
	if bid == "" {
		writeError(w, http.StatusBadRequest, errors.New("bot id is required"))
		return
	}
	if _, err := a.deps.Queries.GetBot(ctx, orgID, bid); err != nil {
		writeError(w, errStatus(err), fmt.Errorf("get bot %s: %w", bid, err))
		return
	}
	subs, err := a.deps.Queries.BotSubscriptions(ctx, orgID, bid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("list subscriptions: %w", err))
		return
	}
	resp := BotSubscriptionsResponse{BotID: string(bid), Subscriptions: make([]BotSubscriptionDTO, 0, len(subs))}
	for _, sub := range subs {
		resp.Subscriptions = append(resp.Subscriptions, BotSubscriptionDTO{
			TopicID:   string(sub.TopicID),
			CreatedAt: sub.CreatedAt.Format(time.RFC3339),
		})
	}
	writeJSON(w, http.StatusOK, resp)
}

// subscribeBot adds a subscription on the given bot to the topic in the
// request body. Idempotent — re-subscribing returns 200 with the
// existing row's metadata.
//
// @Summary Helix-org: subscribe a bot to a topic
// @Tags HelixOrg
// @Param id path string true "Bot ID"
// @Param payload body api.SubscribeBotRequest true "topic to subscribe to"
// @Success 200 {object} api.BotSubscriptionDTO
// @Success 201 {object} api.BotSubscriptionDTO
// @Failure 404 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/bots/{id}/subscriptions [post]
func (a *apiHandler) subscribeBot(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	bid := orgchart.BotID(r.PathValue("id"))
	if bid == "" {
		writeError(w, http.StatusBadRequest, errors.New("bot id is required"))
		return
	}
	var req SubscribeBotRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	topicID := streaming.TopicID(strings.TrimSpace(req.TopicID))
	if topicID == "" {
		writeError(w, http.StatusBadRequest, errors.New("topic_id is required"))
		return
	}
	sub, created, err := a.deps.Subscriptions.Subscribe(ctx, orgID, bid, topicID)
	if err != nil {
		writeError(w, errStatus(err), fmt.Errorf("subscribe bot %s: %w", bid, err))
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, BotSubscriptionDTO{
		TopicID:   string(sub.TopicID),
		CreatedAt: sub.CreatedAt.Format(time.RFC3339),
	})
}

// unsubscribeBot drops the (bot, topic) subscription row.
//
// @Summary Helix-org: unsubscribe a bot from a topic
// @Tags HelixOrg
// @Param id path string true "Bot ID"
// @Param topic_id path string true "Topic ID"
// @Success 204
// @Failure 404 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/bots/{id}/subscriptions/{topic_id} [delete]
func (a *apiHandler) unsubscribeBot(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	bid := orgchart.BotID(r.PathValue("id"))
	topicID := streaming.TopicID(r.PathValue("topic_id"))
	if bid == "" || topicID == "" {
		writeError(w, http.StatusBadRequest, errors.New("bot id and topic id are required"))
		return
	}
	if err := a.deps.Subscriptions.Unsubscribe(ctx, orgID, bid, topicID); err != nil {
		writeError(w, errStatus(err), fmt.Errorf("delete subscription: %w", err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
