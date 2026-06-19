package transport

import (
	"encoding/json"
	"fmt"
	"net/url"
)

// KindWebhook is a bidirectional HTTP transport.
//
// Inbound: POSTs to /webhooks/<topicID> are turned into Events on the
// Topic. No config required — the path uses the Topic's own ID as
// the secret-by-obscurity, which is enough for low-stakes use;
// production callers should add a signing secret on top.
//
// Outbound: when Config sets `outbound_url`, every Event appended to
// the Topic (regardless of who appended it — webhook handler,
// publish tool, dm tool) is POSTed to that URL with the event body as
// the request body. Failures are logged and dropped; the append itself
// still succeeds.
//
// A Topic can be inbound-only (no config), outbound-only (config
// with outbound_url), or both at once.
const KindWebhook Kind = "webhook"

// WebhookConfig is the parsed shape of Transport.Config when
// Kind == KindWebhook. All fields are optional; a webhook topic with
// a zero WebhookConfig is inbound-only.
type WebhookConfig struct {
	// OutboundURL, when set, makes the Topic emit each appended Event
	// as an HTTP POST to this URL. Must be an absolute http(s) URL.
	OutboundURL string `json:"outbound_url,omitempty"`
}

// Validate enforces that OutboundURL (if set) is an absolute http(s)
// URL with a host. An empty WebhookConfig is valid (inbound-only).
func (w WebhookConfig) Validate() error {
	if w.OutboundURL == "" {
		return nil
	}
	u, err := url.Parse(w.OutboundURL)
	if err != nil {
		return fmt.Errorf("outbound_url: %w", err)
	}
	if !u.IsAbs() || (u.Scheme != "http" && u.Scheme != "https") {
		return fmt.Errorf("outbound_url must be an absolute http(s) URL, got %q", w.OutboundURL)
	}
	if u.Host == "" {
		return fmt.Errorf("outbound_url has no host: %q", w.OutboundURL)
	}
	return nil
}

// WebhookConfig parses t.Config as a WebhookConfig. Returns the zero
// value with no error when Config is empty. Errors on wrong-kind and
// on JSON shape problems; semantic validation lives in
// WebhookConfig.Validate() (called from Transport.Validate()).
//
// This method lives in webhook.go, not transport.go, so the umbrella
// stays Kind-agnostic — adding a new Kind doesn't touch this file.
func (t Transport) WebhookConfig() (WebhookConfig, error) {
	if t.Kind != KindWebhook {
		return WebhookConfig{}, fmt.Errorf("transport kind is %q, not webhook", t.Kind)
	}
	return parseWebhookConfig(t.Config)
}

// webhook is the Strategy for KindWebhook.
type webhook struct{}

// ParseConfig satisfies Strategy. Delegates to the typed parser so
// the umbrella Transport.WebhookConfig() accessor and the Strategy
// dispatch path share one implementation.
func (webhook) ParseConfig(raw json.RawMessage) (Config, error) {
	c, err := parseWebhookConfig(raw)
	return c, err
}

// parseWebhookConfig is the typed parser. Returns the zero value with
// no error when Config is empty. Errors only on JSON shape problems;
// semantic validation happens in WebhookConfig.Validate.
func parseWebhookConfig(raw json.RawMessage) (WebhookConfig, error) {
	var c WebhookConfig
	if len(raw) == 0 {
		return c, nil
	}
	if err := json.Unmarshal(raw, &c); err != nil {
		return WebhookConfig{}, fmt.Errorf("parse webhook config: %w", err)
	}
	return c, nil
}
