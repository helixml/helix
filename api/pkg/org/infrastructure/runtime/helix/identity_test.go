package helix_test

import (
	"context"
	"testing"

	helixruntime "github.com/helixml/helix/api/pkg/org/infrastructure/runtime/helix"
)

// TestHelixIdentityZeroValueIsAbsence pins the "no identity"
// semantics — the helix runtime's request context starts without an
// identity (anonymous / service paths) and a zero HelixIdentity
// signals that explicitly. IsZero returns true; downstream code
// uses it to decide between "use the static service key" and "use
// the request-time identity."
func TestHelixIdentityZeroValueIsAbsence(t *testing.T) {
	t.Parallel()
	var id helixruntime.HelixIdentity
	if !id.IsZero() {
		t.Fatal("zero HelixIdentity reports IsZero=false; want true")
	}
}

// TestHelixIdentityCarriesUserOrgBearer pins the populated shape:
// every field round-trips, IsZero flips to false the moment any
// field is set.
func TestHelixIdentityCarriesUserOrgBearer(t *testing.T) {
	t.Parallel()
	id := helixruntime.HelixIdentity{
		UserID:         "user-123",
		OrganizationID: "org-acme",
		BearerToken:    "key-xyz",
	}
	if id.IsZero() {
		t.Fatal("populated HelixIdentity reports IsZero=true")
	}
	if id.UserID != "user-123" || id.OrganizationID != "org-acme" || id.BearerToken != "key-xyz" {
		t.Errorf("fields don't round-trip: %+v", id)
	}
}

// TestHelixIdentityIsZeroWhenAnyOneFieldSet pins one-field
// populated states — "I know the user but not their bearer yet"
// is a legitimate state (the spawner holds it before resolving the
// per-activation key).
func TestHelixIdentityIsZeroWhenAnyOneFieldSet(t *testing.T) {
	t.Parallel()
	cases := []helixruntime.HelixIdentity{
		{UserID: "user-only"},
		{OrganizationID: "org-only"},
		{BearerToken: "bearer-only"},
	}
	for _, id := range cases {
		if id.IsZero() {
			t.Errorf("HelixIdentity %+v reports IsZero=true; want false", id)
		}
	}
}

// TestHelixIdentityContextRoundTrip pins the standard context
// stash/retrieve. nil-safe — calling WithHelixIdentity with the
// zero value returns the parent context unchanged so callers
// don't have to gate on IsZero.
func TestHelixIdentityContextRoundTrip(t *testing.T) {
	t.Parallel()
	want := helixruntime.HelixIdentity{
		UserID:         "user-abc",
		OrganizationID: "org-acme",
		BearerToken:    "key-1",
	}
	ctx := helixruntime.WithHelixIdentity(context.Background(), want)
	got, ok := helixruntime.HelixIdentityFromContext(ctx)
	if !ok {
		t.Fatal("HelixIdentityFromContext(_, with stashed identity) = _, false; want true")
	}
	if got != want {
		t.Errorf("round-trip = %+v, want %+v", got, want)
	}

	// Empty context returns zero + false.
	empty, ok := helixruntime.HelixIdentityFromContext(context.Background())
	if ok {
		t.Errorf("HelixIdentityFromContext(empty ctx) = %+v, true; want zero, false", empty)
	}

	// Zero-identity write is a no-op (parent ctx unchanged).
	ctx2 := helixruntime.WithHelixIdentity(context.Background(), helixruntime.HelixIdentity{})
	_, ok = helixruntime.HelixIdentityFromContext(ctx2)
	if ok {
		t.Error("WithHelixIdentity(zero) stashed something; want no-op")
	}
}
