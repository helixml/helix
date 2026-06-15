// Package webhook is the infrastructure side of the webhook transport:
// the outbound emitter that POSTs an appended Event to a Stream's
// configured OutboundURL. The dispatcher (application) dispatches to it
// via the streams.Outbound port — it owns the HTTP mechanics
// (client, timeout, headers, status handling) so that delivery detail
// stays out of the core dispatcher.
package webhook

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/streams"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
)

// outboundTimeout caps how long an outbound webhook POST may take. A
// hung target must not stall delivery. 5 seconds is generous for HTTP
// and short enough that local listeners (nc, requestbin) which don't
// speak HTTP back fail fast.
const outboundTimeout = 5 * time.Second

// OutboundEmitter POSTs Events to a webhook Stream's OutboundURL. It
// satisfies streams.Outbound.
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

// Emit POSTs the Event body to the Stream's configured OutboundURL. A
// stream with no OutboundURL (or unparseable config) is a no-op. Uses a
// fresh background context bounded by the client timeout — the
// originating request context is deliberately not propagated, since the
// POST must outlive that request. Non-2xx responses and transport
// errors are logged and swallowed: the Event append already succeeded,
// so a failed outbound delivery must not surface as a publish error.
func (e *OutboundEmitter) Emit(_ context.Context, stream streaming.Stream, event streaming.Event) error {
	cfg, err := stream.Transport.WebhookConfig()
	if err != nil {
		e.logger.Warn("webhook.emit.config", "stream", event.StreamID, "err", err)
		return nil
	}
	if cfg.OutboundURL == "" {
		return nil
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, cfg.OutboundURL, bytes.NewBufferString(event.Body))
	if err != nil {
		e.logger.Warn("webhook.emit.build", "stream", event.StreamID, "url", cfg.OutboundURL, "err", err)
		return nil
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("X-Helix-Stream", string(event.StreamID))
	req.Header.Set("X-Helix-Event", string(event.ID))
	resp, err := e.client.Do(req)
	if err != nil {
		e.logger.Warn("webhook.emit.do", "stream", event.StreamID, "url", cfg.OutboundURL, "err", err)
		return nil
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		e.logger.Warn("webhook.emit.status", "stream", event.StreamID, "url", cfg.OutboundURL, "status", resp.StatusCode)
		return nil
	}
	e.logger.Info("webhook.emit.ok", "stream", event.StreamID, "url", cfg.OutboundURL, "status", resp.StatusCode)
	return nil
}

// compile-time assertion that the emitter satisfies the port.
var _ streams.Outbound = (*OutboundEmitter)(nil)
