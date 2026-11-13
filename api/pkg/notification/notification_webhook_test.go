package notification

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/netguard"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

type nopStore struct{ store.Store }

// Callback URLs are set by users on cron triggers and sessions and posted to
// from the API server, so the default policy must refuse internal addresses.
func TestNotify_CallbackURLFollowsOutboundPolicy(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := &Notification{Event: types.EventCronTriggerComplete, CallbackURL: srv.URL}

	strict, err := New(&config.Notifications{}, nopStore{}, netguard.Policy{})
	require.NoError(t, err)
	require.NoError(t, strict.Notify(context.Background(), n))
	require.Zero(t, hits, "callback to a loopback address was sent")

	allowing, err := New(&config.Notifications{}, nopStore{}, netguard.Policy{AllowPrivate: true})
	require.NoError(t, err)
	require.NoError(t, allowing.Notify(context.Background(), n))
	require.Equal(t, 1, hits)
}

func TestSendWebhook_RefusesNonHTTPScheme(t *testing.T) {
	err := sendWebhook(context.Background(), http.DefaultClient, "file:///etc/passwd", &Notification{})
	require.ErrorIs(t, err, netguard.ErrBlockedDestination)
}
