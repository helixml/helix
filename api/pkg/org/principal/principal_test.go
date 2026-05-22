package principal_test

import (
	"encoding/json"
	"testing"

	"github.com/helixml/helix/api/pkg/org/principal"
)

// TestPrincipalConstructorsPinKindAndID covers the three sender
// kinds the org graph distinguishes today (M6 / 08-migration §M6):
// internal Worker, an external transport-native sender that hasn't
// resolved to a Worker, and an unauthenticated human reaching in
// (e.g. operator typing into /ui/ without an account). Each
// constructor stamps its Kind so callers can't accidentally produce
// a kind-less Principal.
func TestPrincipalConstructorsPinKindAndID(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		got       principal.Principal
		wantKind  principal.Kind
		wantID    string
		wantValid bool
	}{
		{"worker", principal.NewWorker("w-alice"), principal.KindWorker, "w-alice", true},
		{"transport", principal.NewTransport("alice@example.com"), principal.KindTransport, "alice@example.com", true},
		{"human", principal.NewHuman("ops-1"), principal.KindHuman, "ops-1", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if tc.got.Kind != tc.wantKind {
				t.Errorf("Kind = %q, want %q", tc.got.Kind, tc.wantKind)
			}
			if tc.got.ID != tc.wantID {
				t.Errorf("ID = %q, want %q", tc.got.ID, tc.wantID)
			}
			if err := tc.got.Validate(); (err == nil) != tc.wantValid {
				t.Errorf("Validate() error = %v, wantValid = %v", err, tc.wantValid)
			}
			if tc.got.IsZero() {
				t.Errorf("constructed principal reports IsZero=true")
			}
		})
	}
}

// TestPrincipalZeroValueIsValidAbsence — the zero value represents
// "no principal" (system-emitted event, anonymous transport inbound
// without a resolved sender). It must validate cleanly so callers
// can use zero-Principal as "unset".
func TestPrincipalZeroValueIsValidAbsence(t *testing.T) {
	t.Parallel()
	var p principal.Principal
	if !p.IsZero() {
		t.Fatal("zero-Principal reports IsZero=false")
	}
	if err := p.Validate(); err != nil {
		t.Fatalf("zero-Principal Validate() = %v, want nil", err)
	}
}

// TestPrincipalValidateRejectsPartial — populated Kind with empty ID
// (or vice versa) is a programming bug; reject so it can't reach
// storage where it would silently fail later joins/filters.
func TestPrincipalValidateRejectsPartial(t *testing.T) {
	t.Parallel()
	cases := []principal.Principal{
		{Kind: principal.KindWorker},               // ID missing
		{ID: "w-alice"},                            // Kind missing
		{Kind: principal.Kind("admin"), ID: "x"},   // unknown Kind
	}
	for i, p := range cases {
		p := p
		t.Run(string(rune('a'+i)), func(t *testing.T) {
			t.Parallel()
			if err := p.Validate(); err == nil {
				t.Fatalf("Validate(%+v) = nil; want error", p)
			}
		})
	}
}

// TestPrincipalJSONRoundTrip — pins the on-wire shape so the alpha
// transitional period (where Event.Source / Message.From still hold
// raw strings) can layer the Principal type on the same JSON columns
// later without a schema break.
func TestPrincipalJSONRoundTrip(t *testing.T) {
	t.Parallel()
	want := principal.NewWorker("w-alice")
	encoded, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if got := string(encoded); got != `{"kind":"worker","id":"w-alice"}` {
		t.Fatalf("encoded = %s, want canonical shape", got)
	}
	var back principal.Principal
	if err := json.Unmarshal(encoded, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back != want {
		t.Errorf("round-trip = %+v, want %+v", back, want)
	}

	// Zero-Principal also round-trips cleanly so "no sender" survives
	// a write/read cycle as the zero value, not as some legacy sentinel.
	encodedZero, err := json.Marshal(principal.Principal{})
	if err != nil {
		t.Fatalf("marshal zero: %v", err)
	}
	if got := string(encodedZero); got != `{"kind":"","id":""}` {
		t.Fatalf("zero encoded = %s, want canonical empty shape", got)
	}
}
