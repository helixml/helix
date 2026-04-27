package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
)

// TransportKind names the implementation that owns a Stream's I/O.
// Every Stream has one. The default — TransportLocal — means events
// live in the SQLite events table and are delivered through the
// in-process broadcaster and dispatcher; nothing crosses a network.
// Other kinds (Slack, email, webhook, RSS, tick…) compose external
// I/O over the same local store.
type TransportKind string

const (
	// TransportLocal is the default: SQLite + broadcaster + dispatcher.
	// No external I/O.
	TransportLocal TransportKind = "local"

	// TransportWebhook is a bidirectional HTTP transport.
	//
	// Inbound: POSTs to /webhooks/<streamID> are turned into Events on
	// the Stream. No config required — the path uses the Stream's own
	// ID as the secret-by-obscurity, which is enough for low-stakes
	// use; production callers should add a signing secret on top.
	//
	// Outbound: when Config sets `outbound_url`, every Event appended
	// to the Stream (regardless of who appended it — webhook handler,
	// publish tool, dm tool) is POSTed to that URL with the event body
	// as the request body. Failures are logged and dropped; the append
	// itself still succeeds.
	//
	// A Stream can be inbound-only (no config), outbound-only (config
	// with outbound_url), or both at once.
	TransportWebhook TransportKind = "webhook"
)

// Transport describes how events on a Stream move to and from the
// outside world. Internal Streams use TransportLocal — that is still a
// transport, just one whose endpoints are both inside the system.
//
// Config is opaque per-Kind JSON. The local transport ignores it; other
// transports parse it according to their own schema (see WebhookConfig).
type Transport struct {
	Kind   TransportKind
	Config json.RawMessage
}

// LocalTransport is the zero-config default returned when a caller does
// not specify a transport. Treat the returned value as immutable.
func LocalTransport() Transport {
	return Transport{Kind: TransportLocal}
}

// WebhookConfig is the parsed shape of Transport.Config when
// Kind == TransportWebhook. All fields are optional; a webhook stream
// with a zero WebhookConfig is inbound-only.
type WebhookConfig struct {
	// OutboundURL, when set, makes the Stream emit each appended Event
	// as an HTTP POST to this URL. Must be an absolute http(s) URL.
	OutboundURL string `json:"outbound_url,omitempty"`
}

// WebhookConfig parses Transport.Config as a WebhookConfig. Returns the
// zero value with no error when Config is empty. Errors only on JSON
// shape problems — semantic validation happens in Validate().
func (t Transport) WebhookConfig() (WebhookConfig, error) {
	if t.Kind != TransportWebhook {
		return WebhookConfig{}, fmt.Errorf("transport kind is %q, not webhook", t.Kind)
	}
	var c WebhookConfig
	if len(t.Config) == 0 {
		return c, nil
	}
	if err := json.Unmarshal(t.Config, &c); err != nil {
		return WebhookConfig{}, fmt.Errorf("parse webhook config: %w", err)
	}
	return c, nil
}

// Validate checks that the Kind is non-empty and recognised, and that
// any per-Kind Config parses and is internally consistent. For webhook
// streams that means OutboundURL (if set) must be a valid absolute
// http(s) URL.
func (t Transport) Validate() error {
	if t.Kind == "" {
		return errors.New("transport kind is empty")
	}
	switch t.Kind {
	case TransportLocal:
		return nil
	case TransportWebhook:
		c, err := t.WebhookConfig()
		if err != nil {
			return err
		}
		if c.OutboundURL == "" {
			return nil
		}
		u, err := url.Parse(c.OutboundURL)
		if err != nil {
			return fmt.Errorf("outbound_url: %w", err)
		}
		if !u.IsAbs() || (u.Scheme != "http" && u.Scheme != "https") {
			return fmt.Errorf("outbound_url must be an absolute http(s) URL, got %q", c.OutboundURL)
		}
		if u.Host == "" {
			return fmt.Errorf("outbound_url has no host: %q", c.OutboundURL)
		}
		return nil
	default:
		return errors.New("unknown transport kind: " + string(t.Kind))
	}
}
