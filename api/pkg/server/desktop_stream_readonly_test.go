package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/helixml/helix/api/pkg/types"
)

// The desktop stream socket carries input as well as video. Allowing an embed
// key to open it is only safe while this stays true.
func TestDesktopStreamIsReadOnlyForEmbedKeys(t *testing.T) {
	cases := []struct {
		name     string
		user     *types.User
		readOnly bool
	}{
		{
			name:     "embed key — handed to the public, watches only",
			user:     &types.User{ID: "u", APIKeyType: types.APIkeytypeEmbed},
			readOnly: true,
		},
		{
			name:     "ordinary api key — owns the session, may drive it",
			user:     &types.User{ID: "u", APIKeyType: types.APIkeytypeAPI},
			readOnly: false,
		},
		{
			name:     "no key type (session/cookie auth) — may drive it",
			user:     &types.User{ID: "u"},
			readOnly: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/api/v1/external-agents/ses_A/ws/stream", nil)
			r = r.WithContext(setRequestUser(r.Context(), *c.user))
			if got := desktopStreamIsReadOnly(r); got != c.readOnly {
				t.Errorf("desktopStreamIsReadOnly = %v, want %v", got, c.readOnly)
			}
		})
	}
}

// If authz somehow let a request through without a resolved user, watching is
// the safe reading — never "full control by default".
func TestDesktopStreamFailsClosedWithoutAUser(t *testing.T) {
	r := httptest.NewRequest("GET", "/api/v1/external-agents/ses_A/ws/stream", nil)
	if !desktopStreamIsReadOnly(r) {
		t.Error("SECURITY: a request with no resolved user was granted desktop input")
	}
}

// The read-only marker must be one WE set, never one a caller can supply. If a
// client-sent header were trusted, the flag would be inverted trivially: the
// browser would simply omit it.
func TestClientCannotClearTheReadOnlyMarker(t *testing.T) {
	r := httptest.NewRequest("GET", "/api/v1/external-agents/ses_A/ws/stream", nil)
	r.Header.Set(desktopReadOnlyHeader, "")
	r.Header.Set("X-Helix-Readonly", "0")
	r = r.WithContext(setRequestUser(r.Context(), types.User{
		ID: "u", APIKeyType: types.APIkeytypeEmbed,
	}))

	if !desktopStreamIsReadOnly(r) {
		t.Error("SECURITY: a client header overrode the credential-derived decision")
	}
}

// Whatever the decision, it must be identical on reconnect. The resilient proxy
// re-runs its upgrade func after a drop, and a reconnect that forgot the header
// would silently hand input back to a viewer mid-session.
func TestReadOnlyDecisionIsStableAcrossReconnect(t *testing.T) {
	build := func() *http.Request {
		r := httptest.NewRequest("GET", "/api/v1/external-agents/ses_A/ws/stream", nil)
		return r.WithContext(setRequestUser(r.Context(), types.User{
			ID: "u", APIKeyType: types.APIkeytypeEmbed,
		}))
	}
	first := desktopStreamIsReadOnly(build())
	again := desktopStreamIsReadOnly(build())
	if !first || !again {
		t.Errorf("read-only must hold on both connect (%v) and reconnect (%v)", first, again)
	}
}
