package webhooks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	helixcrypto "github.com/helixml/helix/api/pkg/crypto"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
	standardwebhooks "github.com/standard-webhooks/standard-webhooks/libraries/go"
)

const deliveryLockDuration = 5 * time.Minute

type encryptionKeyGetter func() ([]byte, error)

type Dispatcher struct {
	store  store.Store
	getKey encryptionKeyGetter
	client *http.Client
	cfg    config.Webhooks
	now    func() time.Time
	randMu sync.Mutex
	random *rand.Rand
}

func NewDispatcher(st store.Store, getKey encryptionKeyGetter, cfg config.Webhooks) *Dispatcher {
	if cfg.WorkerInterval <= 0 {
		cfg.WorkerInterval = 2 * time.Second
	}
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 8
	}
	return &Dispatcher{
		store: st, getKey: getKey, cfg: cfg,
		client: NewHTTPClient(cfg.DeliveryTimeout, cfg.AllowPrivateEndpoints),
		now:    func() time.Time { return time.Now().UTC() },
		random: rand.New(rand.NewSource(time.Now().UnixNano())), //nolint:gosec // retry jitter is not security-sensitive
	}
}

func (d *Dispatcher) Start(ctx context.Context) {
	ticker := time.NewTicker(d.cfg.WorkerInterval)
	defer ticker.Stop()
	d.runOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.runOnce(ctx)
		}
	}
}

func (d *Dispatcher) runOnce(ctx context.Context) {
	now := d.now()
	deliveries, err := d.store.ClaimWebhookDeliveries(ctx, now, now.Add(deliveryLockDuration), 25)
	if err != nil {
		log.Error().Err(err).Msg("failed to claim webhook deliveries")
		return
	}
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for _, delivery := range deliveries {
		delivery := delivery
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}
			if err := d.deliver(ctx, delivery); err != nil {
				log.Error().Err(err).Str("delivery_id", delivery.ID).Msg("failed to persist webhook delivery attempt")
			}
		}()
	}
	wg.Wait()
}

func (d *Dispatcher) deliver(ctx context.Context, delivery *types.WebhookDelivery) error {
	now := d.now()
	attempt := delivery.AttemptCount + 1
	update := &store.WebhookDeliveryUpdate{
		ID: delivery.ID, AttemptCount: attempt, LastAttemptAt: now, NextAttemptAt: now,
	}
	event, err := d.store.GetWebhookEvent(ctx, delivery.EventID)
	if err != nil {
		return d.failPermanently(ctx, update, "webhook event no longer exists")
	}
	endpoint, err := d.store.GetWebhookEndpoint(ctx, event.OrganizationID, delivery.EndpointID)
	if err != nil {
		return d.failPermanently(ctx, update, "webhook endpoint no longer exists")
	}
	if endpoint.Status != types.WebhookEndpointStatusActive {
		update.Status = types.WebhookDeliveryStatusDisabled
		update.LastError = "endpoint is disabled"
		return d.store.CompleteWebhookDelivery(ctx, update)
	}
	if err := ValidateEndpointURL(endpoint.URL, d.cfg.AllowPrivateEndpoints); err != nil {
		return d.finishFailure(ctx, update, 0, "destination rejected by URL policy")
	}
	body, err := json.Marshal(event)
	if err != nil {
		return d.failPermanently(ctx, update, "encode webhook event")
	}
	signature, err := d.sign(endpoint, delivery.ID, now, body)
	if err != nil {
		return d.finishFailure(ctx, update, 0, "signing secret unavailable")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.URL, bytes.NewReader(body))
	if err != nil {
		return d.failPermanently(ctx, update, "create webhook request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Helix-Standard-Webhooks/1.0")
	req.Header.Set(standardwebhooks.HeaderWebhookID, delivery.ID)
	req.Header.Set(standardwebhooks.HeaderWebhookTimestamp, strconv.FormatInt(now.Unix(), 10))
	req.Header.Set(standardwebhooks.HeaderWebhookSignature, signature)
	req.Header.Set("webhook-type", event.Type)

	resp, err := d.client.Do(req)
	if err != nil {
		return d.finishFailure(ctx, update, 0, classifyDeliveryError(err))
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64*1024))
	update.LastStatusCode = resp.StatusCode
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		update.Status = types.WebhookDeliveryStatusDelivered
		update.DeliveredAt = &now
		update.LastError = ""
		return d.store.CompleteWebhookDelivery(ctx, update)
	}
	if resp.StatusCode == http.StatusGone {
		if err := d.store.DisableWebhookEndpoint(ctx, endpoint.OrganizationID, endpoint.ID, "receiver returned HTTP 410", "system"); err != nil {
			return err
		}
		update.Status = types.WebhookDeliveryStatusDisabled
		update.LastError = "receiver returned HTTP 410; endpoint disabled"
		return d.store.CompleteWebhookDelivery(ctx, update)
	}
	retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"), now)
	return d.finishFailureAfter(ctx, update, resp.StatusCode, "receiver returned HTTP "+strconv.Itoa(resp.StatusCode), retryAfter)
}

func (d *Dispatcher) sign(endpoint *types.WebhookEndpoint, messageID string, timestamp time.Time, payload []byte) (string, error) {
	key, err := d.getKey()
	if err != nil {
		return "", err
	}
	current, err := helixcrypto.DecryptAES256GCM(endpoint.SecretEncrypted, key)
	if err != nil {
		return "", err
	}
	signer, err := standardwebhooks.NewWebhook(string(current))
	if err != nil {
		return "", err
	}
	signature, err := signer.Sign(messageID, timestamp, payload)
	if err != nil {
		return "", err
	}
	if endpoint.PreviousSecretEncrypted != "" && endpoint.PreviousSecretExpiresAt != nil && timestamp.Before(*endpoint.PreviousSecretExpiresAt) {
		previous, decryptErr := helixcrypto.DecryptAES256GCM(endpoint.PreviousSecretEncrypted, key)
		if decryptErr == nil {
			previousSigner, signerErr := standardwebhooks.NewWebhook(string(previous))
			if signerErr == nil {
				previousSignature, signErr := previousSigner.Sign(messageID, timestamp, payload)
				if signErr == nil {
					signature += " " + previousSignature
				}
			}
		}
	}
	return signature, nil
}

func (d *Dispatcher) finishFailure(ctx context.Context, update *store.WebhookDeliveryUpdate, statusCode int, message string) error {
	return d.finishFailureAfter(ctx, update, statusCode, message, time.Time{})
}

func (d *Dispatcher) finishFailureAfter(ctx context.Context, update *store.WebhookDeliveryUpdate, statusCode int, message string, retryAfter time.Time) error {
	update.LastStatusCode = statusCode
	update.LastError = truncate(message, 500)
	if update.AttemptCount >= d.cfg.MaxAttempts {
		update.Status = types.WebhookDeliveryStatusFailed
		return d.store.CompleteWebhookDelivery(ctx, update)
	}
	update.Status = types.WebhookDeliveryStatusRetrying
	update.NextAttemptAt = d.nextAttempt(update.AttemptCount, update.LastAttemptAt)
	if retryAfter.After(update.NextAttemptAt) {
		update.NextAttemptAt = retryAfter
	}
	return d.store.CompleteWebhookDelivery(ctx, update)
}

func (d *Dispatcher) failPermanently(ctx context.Context, update *store.WebhookDeliveryUpdate, message string) error {
	update.Status = types.WebhookDeliveryStatusFailed
	update.LastError = truncate(message, 500)
	return d.store.CompleteWebhookDelivery(ctx, update)
}

func (d *Dispatcher) nextAttempt(attempt int, now time.Time) time.Time {
	delays := []time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute, 2 * time.Hour, 8 * time.Hour, 24 * time.Hour, 48 * time.Hour}
	index := attempt - 1
	if index < 0 {
		index = 0
	}
	if index >= len(delays) {
		index = len(delays) - 1
	}
	delay := delays[index]
	d.randMu.Lock()
	jitter := time.Duration(d.random.Int63n(int64(delay/5)+1)) - delay/10
	d.randMu.Unlock()
	return now.Add(delay + jitter)
}

func parseRetryAfter(value string, now time.Time) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 {
		return now.Add(time.Duration(seconds) * time.Second)
	}
	if parsed, err := http.ParseTime(value); err == nil {
		return parsed
	}
	return time.Time{}
}

func classifyDeliveryError(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "delivery timed out"
	}
	return "delivery failed"
}

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}
