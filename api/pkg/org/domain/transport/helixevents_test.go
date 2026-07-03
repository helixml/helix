package transport_test

import (
	"encoding/json"
	"testing"

	"github.com/helixml/helix/api/pkg/org/domain/transport"
)

// TestKindHelixEvents_InKindValues pins that the Helix events transport is
// a registered Kind, so it is surfaced everywhere KindValues() drives
// (JSON Schema enum, validation error lists).
func TestKindHelixEvents_InKindValues(t *testing.T) {
	t.Parallel()
	for _, k := range transport.KindValues() {
		if k == transport.KindHelixEvents {
			return
		}
	}
	t.Fatalf("KindHelixEvents not in KindValues(): %v", transport.KindValues())
}

// TestHelixEventsConfig_ValidatesWithoutConfig pins that the Helix events
// transport needs no config: an empty (or absent) config blob validates.
func TestHelixEventsConfig_ValidatesWithoutConfig(t *testing.T) {
	t.Parallel()
	for _, raw := range []json.RawMessage{nil, json.RawMessage(`{}`), json.RawMessage(`{"junk":1}`)} {
		tr := transport.Transport{Kind: transport.KindHelixEvents, Config: raw}
		if err := tr.Validate(); err != nil {
			t.Fatalf("Validate() with config %q = %v, want nil", string(raw), err)
		}
	}
}

// TestHelixEventsConfig_Accessor pins the typed accessor returns cleanly
// for a KindHelixEvents transport and errors for a mismatched kind.
func TestHelixEventsConfig_Accessor(t *testing.T) {
	t.Parallel()
	if _, err := (transport.Transport{Kind: transport.KindHelixEvents}).HelixEventsConfig(); err != nil {
		t.Fatalf("HelixEventsConfig() on helix_events kind = %v, want nil", err)
	}
	if _, err := (transport.Transport{Kind: transport.KindLocal}).HelixEventsConfig(); err == nil {
		t.Fatal("HelixEventsConfig() on local kind = nil, want error")
	}
}
