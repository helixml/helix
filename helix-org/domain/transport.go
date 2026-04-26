package domain

import (
	"encoding/json"
	"errors"
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
)

// Transport describes how events on a Stream move to and from the
// outside world. Internal Streams use TransportLocal — that is still a
// transport, just one whose endpoints are both inside the system.
//
// Config is opaque per-Kind JSON. The local transport ignores it; other
// transports (when they exist) parse it according to their own schema.
type Transport struct {
	Kind   TransportKind
	Config json.RawMessage
}

// LocalTransport is the zero-config default returned when a caller does
// not specify a transport. Treat the returned value as immutable.
func LocalTransport() Transport {
	return Transport{Kind: TransportLocal}
}

// Validate checks that the Kind is non-empty and recognised. Concrete
// Config validation is the transport implementation's responsibility.
func (t Transport) Validate() error {
	if t.Kind == "" {
		return errors.New("transport kind is empty")
	}
	switch t.Kind {
	case TransportLocal:
		return nil
	default:
		return errors.New("unknown transport kind: " + string(t.Kind))
	}
}
