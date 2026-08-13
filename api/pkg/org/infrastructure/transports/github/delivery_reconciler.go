package github

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	githubclient "github.com/helixml/helix/api/pkg/github"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
)

const (
	deliveryPollInterval = 5 * time.Minute
	deliveryRetention    = 72 * time.Hour
)

type DeliveryReconciler struct {
	store            *store.Store
	token            TokenResolver
	baseURL          string
	logger           *slog.Logger
	checkpointByHook map[string]int64
}

func NewDeliveryReconciler(st *store.Store, token TokenResolver, baseURL string, logger *slog.Logger) *DeliveryReconciler {
	return &DeliveryReconciler{store: st, token: token, baseURL: baseURL, logger: logger, checkpointByHook: map[string]int64{}}
}

func (r *DeliveryReconciler) Run(ctx context.Context) {
	r.reconcile(ctx)
	ticker := time.NewTicker(deliveryPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.reconcile(ctx)
		}
	}
}

func (r *DeliveryReconciler) reconcile(ctx context.Context) {
	topics, err := r.store.Topics.ListByTransportKind(ctx, transport.KindGitHub)
	if err != nil {
		r.logger.Error("github delivery recovery: list topics", "err", err)
		return
	}
	for _, topic := range topics {
		if ctx.Err() != nil {
			return
		}
		cfg, err := topic.Transport.GitHubConfig()
		if err != nil || cfg.WebhookID == 0 {
			continue
		}
		owner, repo, ok := strings.Cut(cfg.Repo, "/")
		if !ok || owner == "" || repo == "" {
			continue
		}
		token, err := r.token(ctx, topic.OrganizationID)
		if err != nil || token == "" {
			r.logger.Error("github delivery recovery: resolve credentials", "org", topic.OrganizationID, "topic", topic.ID, "err", err)
			continue
		}
		client, err := githubclient.NewGithubClient(githubclient.ClientOptions{Ctx: ctx, Token: token, BaseURL: r.baseURL})
		if err != nil {
			r.logger.Error("github delivery recovery: create client", "org", topic.OrganizationID, "topic", topic.ID, "err", err)
			continue
		}
		if err := r.reconcileHook(ctx, client, owner, repo, cfg.WebhookID); err != nil {
			r.logger.Error("github delivery recovery: reconcile hook", "org", topic.OrganizationID, "topic", topic.ID, "repo", cfg.Repo, "hook", cfg.WebhookID, "err", err)
		}
	}
}

func (r *DeliveryReconciler) reconcileHook(ctx context.Context, client *githubclient.Client, owner, repo string, hookID int64) error {
	key := fmt.Sprintf("%s/%s/%d", owner, repo, hookID)
	checkpoint := r.checkpointByHook[key]

	cutoff := time.Now().Add(-deliveryRetention)
	var deliveries []githubclient.WebhookDelivery
	cursor := ""
	for {
		batch, next, err := client.ListWebhookDeliveries(owner, repo, hookID, cursor)
		if err != nil {
			return err
		}
		reached := next == ""
		for _, delivery := range batch {
			if delivery.ID == checkpoint || delivery.DeliveredAt.Before(cutoff) {
				reached = true
				break
			}
			deliveries = append(deliveries, delivery)
		}
		if reached {
			break
		}
		cursor = next
	}

	slices.Reverse(deliveries)
	latest := make(map[string]int64)
	for _, delivery := range deliveries {
		if delivery.GUID != "" {
			latest[delivery.GUID] = delivery.ID
		}
	}
	for _, delivery := range deliveries {
		if latest[delivery.GUID] == delivery.ID && (delivery.StatusCode < 200 || delivery.StatusCode >= 300) {
			if err := client.RedeliverWebhookDelivery(owner, repo, hookID, delivery.ID); err != nil {
				return fmt.Errorf("redeliver %s delivery %d: %w", delivery.GUID, delivery.ID, err)
			}
			r.logger.Info("github delivery recovery: requested redelivery", "repo", owner+"/"+repo, "hook", hookID, "delivery", delivery.ID, "guid", delivery.GUID)
		}
		r.checkpointByHook[key] = delivery.ID
	}
	return ctx.Err()
}
