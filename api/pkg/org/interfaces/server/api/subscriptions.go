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

// listWorkerSubscriptions returns the worker's current subscription
// set. Drives the Worker detail page's Subscriptions panel.
//
// @Summary Helix-org: list a worker's subscriptions
// @Tags HelixOrg
// @Param id path string true "Worker ID"
// @Success 200 {object} api.WorkerSubscriptionsResponse
// @Failure 404 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/workers/{id}/subscriptions [get]
func (a *apiHandler) listWorkerSubscriptions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	wid := orgchart.WorkerID(r.PathValue("id"))
	if wid == "" {
		writeError(w, http.StatusBadRequest, errors.New("worker id is required"))
		return
	}
	if _, err := a.deps.Queries.GetWorker(ctx, orgID, wid); err != nil {
		writeError(w, errStatus(err), fmt.Errorf("get worker %s: %w", wid, err))
		return
	}
	subs, err := a.deps.Queries.WorkerSubscriptions(ctx, orgID, wid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("list subscriptions: %w", err))
		return
	}
	resp := WorkerSubscriptionsResponse{WorkerID: string(wid), Subscriptions: make([]WorkerSubscriptionDTO, 0, len(subs))}
	for _, sub := range subs {
		resp.Subscriptions = append(resp.Subscriptions, WorkerSubscriptionDTO{
			TopicID:  string(sub.TopicID),
			CreatedAt: sub.CreatedAt.Format(time.RFC3339),
		})
	}
	writeJSON(w, http.StatusOK, resp)
}

// subscribeWorker adds a subscription on the given worker to the
// topic in the request body. Idempotent — re-subscribing returns
// 200 with the existing row's metadata.
//
// @Summary Helix-org: subscribe a worker to a topic
// @Tags HelixOrg
// @Param id path string true "Worker ID"
// @Param payload body api.SubscribeWorkerRequest true "topic to subscribe to"
// @Success 200 {object} api.WorkerSubscriptionDTO
// @Success 201 {object} api.WorkerSubscriptionDTO
// @Failure 404 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/workers/{id}/subscriptions [post]
func (a *apiHandler) subscribeWorker(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	wid := orgchart.WorkerID(r.PathValue("id"))
	if wid == "" {
		writeError(w, http.StatusBadRequest, errors.New("worker id is required"))
		return
	}
	var req SubscribeWorkerRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	topicID := streaming.TopicID(strings.TrimSpace(req.TopicID))
	if topicID == "" {
		writeError(w, http.StatusBadRequest, errors.New("topic_id is required"))
		return
	}
	sub, created, err := a.deps.Subscriptions.Subscribe(ctx, orgID, wid, topicID)
	if err != nil {
		writeError(w, errStatus(err), fmt.Errorf("subscribe worker %s: %w", wid, err))
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, WorkerSubscriptionDTO{
		TopicID:  string(sub.TopicID),
		CreatedAt: sub.CreatedAt.Format(time.RFC3339),
	})
}

// unsubscribeWorker drops the (worker, topic) subscription row.
//
// @Summary Helix-org: unsubscribe a worker from a topic
// @Tags HelixOrg
// @Param id path string true "Worker ID"
// @Param topic_id path string true "Topic ID"
// @Success 204
// @Failure 404 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/workers/{id}/subscriptions/{topic_id} [delete]
func (a *apiHandler) unsubscribeWorker(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	wid := orgchart.WorkerID(r.PathValue("id"))
	topicID := streaming.TopicID(r.PathValue("topic_id"))
	if wid == "" || topicID == "" {
		writeError(w, http.StatusBadRequest, errors.New("worker id and topic id are required"))
		return
	}
	if err := a.deps.Subscriptions.Unsubscribe(ctx, orgID, wid, topicID); err != nil {
		writeError(w, errStatus(err), fmt.Errorf("delete subscription: %w", err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
