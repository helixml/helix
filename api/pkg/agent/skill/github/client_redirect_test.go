package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/netguard"
	"github.com/stretchr/testify/require"
)

// oauth2.NewClient copies CheckRedirect/Timeout from the context client, so
// the injected client's redirect policy must hold on the GitHub path.
func TestPATClientKeepsInjectedRedirectPolicy(t *testing.T) {
	var targetHits atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		targetHits.Add(1)
	}))
	defer target.Close()

	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/user", http.StatusFound)
	}))
	defer redirector.Close()

	httpClient := netguard.Policy{AllowPrivate: true}.NewClient(5 * time.Second)
	client := NewClientWithPATBaseURLAndHTTPClient("dummy", redirector.URL+"/api/v3/", httpClient)

	_, _, _, err := client.GetAuthenticatedUserWithScopes(context.Background())
	require.Error(t, err)
	require.Zero(t, targetHits.Load(), "redirect must not be followed")
}
