package anthropic

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProbeClaudeSubscription(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		token      string
		want       ProbeResult
	}{
		{name: "401 is invalid", statusCode: http.StatusUnauthorized, token: "sk-ant-oat-x", want: ProbeInvalid},
		{name: "200 is valid", statusCode: http.StatusOK, token: "sk-ant-oat-x", want: ProbeValid},
		{name: "429 throttle is valid", statusCode: http.StatusTooManyRequests, token: "sk-ant-oat-x", want: ProbeValid},
		{name: "500 is inconclusive", statusCode: http.StatusInternalServerError, token: "sk-ant-oat-x", want: ProbeInconclusive},
		{name: "empty token is invalid", statusCode: http.StatusOK, token: "", want: ProbeInvalid},
	}

	orig := subscriptionProbeURL
	defer func() { subscriptionProbeURL = orig }()

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var gotBeta string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotBeta = r.Header.Get("anthropic-beta")
				w.WriteHeader(tc.statusCode)
			}))
			defer srv.Close()
			subscriptionProbeURL = srv.URL

			got, detail := ProbeClaudeSubscription(context.Background(), tc.token)
			if got != tc.want {
				t.Fatalf("ProbeClaudeSubscription() = %v (%q), want %v", got, detail, tc.want)
			}
			// The mandatory OAuth beta header must be sent whenever we actually
			// reach the server (i.e. token was non-empty).
			if tc.token != "" && gotBeta != oauthBetaHeader {
				t.Fatalf("anthropic-beta header = %q, want %q", gotBeta, oauthBetaHeader)
			}
		})
	}
}
