package anthropic

import (
	"net/http"

	"github.com/helixml/helix/api/pkg/netguard"
)

// endpointGuardTransport sends requests for tenant-controlled provider
// endpoints through a netguard-restricted transport, so a user or org
// endpoint's BaseURL cannot make the API server reach internal addresses
// (TST-01-270). Admin-managed global endpoints use the open transport.
type endpointGuardTransport struct {
	open         http.RoundTripper
	guarded      http.RoundTripper
	allowPrivate bool
}

func newEndpointGuardTransport(base http.RoundTripper, allowPrivate bool) http.RoundTripper {
	var guarded *http.Transport
	if t, ok := base.(*http.Transport); ok {
		guarded = t.Clone()
	} else {
		guarded = http.DefaultTransport.(*http.Transport).Clone()
	}
	netguard.Policy{AllowPrivate: allowPrivate}.GuardTransport(guarded)
	return &endpointGuardTransport{open: base, guarded: guarded, allowPrivate: allowPrivate}
}

func (t *endpointGuardTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	endpoint := GetRequestProviderEndpoint(req.Context())
	if endpoint != nil && endpoint.OutboundPolicy(t.allowPrivate) == nil {
		return t.open.RoundTrip(req)
	}
	return t.guarded.RoundTrip(req)
}
