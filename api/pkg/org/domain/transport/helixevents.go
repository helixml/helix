package transport

import (
	"encoding/json"
	"errors"
)

// KindHelixEvents is the org-wide, inbound-only Helix event bus: a
// single Topic of this Kind per org onto which every Helix event flows
// (spec-task attention events today; project lifecycle, PR, CI,
// membership, … in future). The event family and type are carried in
// the published Message's Extra payload (`domain` + `event_type`), not
// in the transport config, so one topic serves every kind. It is
// system-managed — created by the helixevents reconciler, never by a
// user — so it is intentionally absent from the New Topic UI.
const KindHelixEvents Kind = "helix_events"

// HelixEventsConfig is the empty config for KindHelixEvents. The topic
// is a plain org-wide bus with nothing to configure; any config blob is
// ignored.
type HelixEventsConfig struct{}

// Validate always succeeds — KindHelixEvents has no rules to enforce.
func (HelixEventsConfig) Validate() error { return nil }

// helixEvents is the Strategy for KindHelixEvents.
type helixEvents struct{}

// ParseConfig ignores the raw blob and returns an empty
// HelixEventsConfig. The Helix events transport accepts any input (or
// none) as valid.
func (helixEvents) ParseConfig(_ json.RawMessage) (Config, error) {
	return HelixEventsConfig{}, nil
}

// HelixEventsConfig returns the typed config for a KindHelixEvents
// Transport.
func (t Transport) HelixEventsConfig() (HelixEventsConfig, error) {
	if t.Kind != KindHelixEvents {
		return HelixEventsConfig{}, errors.New("transport kind is not helix_events")
	}
	c, err := helixEvents{}.ParseConfig(t.Config)
	if err != nil {
		return HelixEventsConfig{}, err
	}
	return c.(HelixEventsConfig), nil
}
