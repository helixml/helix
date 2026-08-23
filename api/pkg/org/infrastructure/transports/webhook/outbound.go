// Package webhook is the infrastructure side of the webhook transport:
// the legacy Topic adapter that POSTs an appended Event to a Topic's
// configured OutboundURL. It owns the HTTP mechanics
// (client, timeout, headers, status handling) so that delivery detail
// stays out of the core dispatcher.
package webhook

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/publishing"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
)

// outboundTimeout caps how long an outbound webhook POST may take. A
// hung target must not stall delivery. 5 seconds is generous for HTTP
// and short enough that local listeners (nc, requestbin) which don't
// speak HTTP back fail fast.
const outboundTimeout = 5 * time.Second

// OutboundEmitter POSTs Events to a webhook Topic's OutboundURL for the
// temporary Topic compatibility adapter.
type OutboundEmitter struct {
	client *http.Client
	logger *slog.Logger
}

// NewOutboundEmitter builds the emitter with a fixed-timeout HTTP client.
func NewOutboundEmitter(logger *slog.Logger) *OutboundEmitter {
	if logger == nil {
		logger = slog.Default()
	}
	return &OutboundEmitter{client: &http.Client{Timeout: outboundTimeout}, logger: logger}
}

// SetHTTPClient replaces the HTTP client. Intended for tests.
func (e *OutboundEmitter) SetHTTPClient(c *http.Client) { e.client = c }

func (e *OutboundEmitter) Deliver(_ context.Context, topic streaming.Topic, event streaming.Event, _ streaming.Message) (publishing.DeliveryReceipt, error) {
	cfg, err := topic.Transport.WebhookConfig()
	if err != nil {
		e.logger.Warn("webhook.emit.config", "topic", event.TopicID, "err", err)
		return publishing.DeliveryReceipt{}, publishing.ErrLegacyDeliveryNotApplicable
	}
	if cfg.OutboundURL == "" {
		return publishing.DeliveryReceipt{}, publishing.ErrLegacyDeliveryNotApplicable
	}
	go func() { //nolint:gosec // legacy delivery intentionally outlives the publish request
		e.emit(topic, event, cfg)
	}()
	return publishing.DeliveryReceipt{}, publishing.ErrLegacyDeliveryWithoutReceipt
}

// Emit POSTs the Event body to the Topic's configured OutboundURL. A
// topic with no OutboundURL (or unparseable config) is a no-op. Uses a
// fresh background context bounded by the client timeout — the
// originating request context is deliberately not propagated, since the
// POST must outlive that request. Non-2xx responses and transport
// errors are logged and swallowed: the Event append already succeeded,
// so a failed outbound delivery must not surface as a publish error.
func (e *OutboundEmitter) Emit(_ context.Context, topic streaming.Topic, event streaming.Event) {
	cfg, err := topic.Transport.WebhookConfig()
	if err != nil {
		e.logger.Warn("webhook.emit.config", "topic", event.TopicID, "err", err)
		return
	}
	if cfg.OutboundURL == "" {
		return
	}
	e.emit(topic, event, cfg)
}

func (e *OutboundEmitter) emit(topic streaming.Topic, event streaming.Event, cfg transport.WebhookConfig) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, cfg.OutboundURL, bytes.NewBufferString(event.Body))
	if err != nil {
		e.logger.Warn("webhook.emit.build", "topic", event.TopicID, "url", cfg.OutboundURL, "err", err)
		return
	}
	req.Header.Set("Content-Type", "application/octet-topic")
	req.Header.Set("X-Helix-Topic", string(event.TopicID))
	req.Header.Set("X-Helix-Event", string(event.ID))
	resp, err := e.client.Do(req)
	if err != nil {
		e.logger.Warn("webhook.emit.do", "topic", event.TopicID, "url", cfg.OutboundURL, "err", err)
		return
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		e.logger.Warn("webhook.emit.status", "topic", event.TopicID, "url", cfg.OutboundURL, "status", resp.StatusCode)
		return
	}
	e.logger.Info("webhook.emit.ok", "topic", event.TopicID, "url", cfg.OutboundURL, "status", resp.StatusCode)
}

var _ publishing.LegacyDeliverer = (*OutboundEmitter)(nil)
