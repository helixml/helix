package github

import (
	"cmp"
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
	deliveryPollInterval    = 5 * time.Minute
	deliveryRetention       = 72 * time.Hour
	deliveryListPageLimit   = 10
	deliveryRedeliveryLimit = 20
)

type deliveryScan struct {
	cursor     string
	deliveries []githubclient.WebhookDelivery
	complete   bool
}

func deliveryHookKey(owner, repo string, hookID int64) string {
	return fmt.Sprintf("%s/%s/%d", strings.ToLower(owner), strings.ToLower(repo), hookID)
}

type DeliveryReconciler struct {
	store            *store.Store
	token            TokenResolver
	baseURL          string
	logger           *slog.Logger
	checkpointByHook map[string]int64
	scanByHook       map[string]*deliveryScan
}

func NewDeliveryReconciler(st *store.Store, token TokenResolver, baseURL string, logger *slog.Logger) *DeliveryReconciler {
	return &DeliveryReconciler{
		store:            st,
		token:            token,
		baseURL:          baseURL,
		logger:           logger,
		checkpointByHook: map[string]int64{},
		scanByHook:       map[string]*deliveryScan{},
	}
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
	live := make(map[string]struct{})
	seen := make(map[string]struct{})
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
		key := deliveryHookKey(owner, repo, cfg.WebhookID)
		live[key] = struct{}{}
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
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		if err := r.reconcileHook(ctx, client, owner, repo, cfg.WebhookID); err != nil {
			r.logger.Error("github delivery recovery: reconcile hook", "org", topic.OrganizationID, "topic", topic.ID, "repo", cfg.Repo, "hook", cfg.WebhookID, "err", err)
		}
	}
	for key := range r.checkpointByHook {
		if _, ok := live[key]; !ok {
			delete(r.checkpointByHook, key)
		}
	}
	for key := range r.scanByHook {
		if _, ok := live[key]; !ok {
			delete(r.scanByHook, key)
		}
	}
}

func (r *DeliveryReconciler) reconcileHook(ctx context.Context, client *githubclient.Client, owner, repo string, hookID int64) error {
	key := deliveryHookKey(owner, repo, hookID)
	checkpoint := r.checkpointByHook[key]
	scan := r.scanByHook[key]
	if scan == nil {
		scan = &deliveryScan{}
		r.scanByHook[key] = scan
	}

	cutoff := time.Now().Add(-deliveryRetention)
	requests := 0
	known := make(map[int64]struct{}, len(scan.deliveries))
	for _, delivery := range scan.deliveries {
		known[delivery.ID] = struct{}{}
	}
	if !scan.complete && scan.cursor != "" && len(scan.deliveries) > 0 {
		refreshCursor := ""
		refreshReady := false
		var refreshed []githubclient.WebhookDelivery
		refreshSeen := make(map[int64]struct{})
		for requests < deliveryListPageLimit {
			batch, next, err := client.ListWebhookDeliveries(owner, repo, hookID, refreshCursor)
			if err != nil {
				return err
			}
			requests++
			overlapped := false
			for _, delivery := range batch {
				if _, ok := known[delivery.ID]; ok {
					overlapped = true
					continue
				}
				if _, ok := refreshSeen[delivery.ID]; !ok {
					refreshed = append(refreshed, delivery)
					refreshSeen[delivery.ID] = struct{}{}
				}
			}
			if overlapped {
				scan.deliveries = append(scan.deliveries, refreshed...)
				for id := range refreshSeen {
					known[id] = struct{}{}
				}
				refreshReady = true
				break
			}
			if next == "" {
				scan.deliveries = refreshed
				known = refreshSeen
				scan.cursor = ""
				scan.complete = true
				refreshReady = true
				break
			}
			refreshCursor = next
		}
		if !refreshReady {
			return ctx.Err()
		}
	}
	for requests < deliveryListPageLimit {
		if scan.complete {
			break
		}
		batch, next, err := client.ListWebhookDeliveries(owner, repo, hookID, scan.cursor)
		if err != nil {
			return err
		}
		requests++
		reached := next == ""
		for _, delivery := range batch {
			if delivery.ID == checkpoint || delivery.DeliveredAt.Before(cutoff) {
				reached = true
				break
			}
			if _, ok := known[delivery.ID]; !ok {
				scan.deliveries = append(scan.deliveries, delivery)
				known[delivery.ID] = struct{}{}
			}
		}
		if reached {
			slices.SortFunc(scan.deliveries, func(a, b githubclient.WebhookDelivery) int {
				return cmp.Compare(a.ID, b.ID)
			})
			scan.complete = true
			continue
		}
		scan.cursor = next
	}
	if !scan.complete {
		return ctx.Err()
	}

	latest := make(map[string]int64)
	for _, delivery := range scan.deliveries {
		if delivery.GUID != "" {
			latest[delivery.GUID] = delivery.ID
		}
	}
	redeliveries := 0
	for len(scan.deliveries) > 0 {
		delivery := scan.deliveries[0]
		if latest[delivery.GUID] == delivery.ID && (delivery.StatusCode < 200 || delivery.StatusCode >= 300) {
			if redeliveries == deliveryRedeliveryLimit {
				return ctx.Err()
			}
			redeliveries++
			if err := client.RedeliverWebhookDelivery(owner, repo, hookID, delivery.ID); err != nil {
				return fmt.Errorf("redeliver %s delivery %d: %w", delivery.GUID, delivery.ID, err)
			}
			r.logger.Info("github delivery recovery: requested redelivery", "repo", owner+"/"+repo, "hook", hookID, "delivery", delivery.ID, "guid", delivery.GUID)
		}
		r.checkpointByHook[key] = delivery.ID
		scan.deliveries = scan.deliveries[1:]
	}
	delete(r.scanByHook, key)
	return ctx.Err()
}
