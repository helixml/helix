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

			probe := ProbeClaudeSubscription(context.Background(), tc.token)
			if probe.Result != tc.want {
				t.Fatalf("ProbeClaudeSubscription() = %v (%q), want %v", probe.Result, probe.Detail, tc.want)
			}
			// The mandatory OAuth beta header must be sent whenever we actually
			// reach the server (i.e. token was non-empty).
			if tc.token != "" && gotBeta != oauthBetaHeader {
				t.Fatalf("anthropic-beta header = %q, want %q", gotBeta, oauthBetaHeader)
			}
		})
	}
}

// The organization uuid is the only identity a setup token discloses:
// /api/oauth/profile 403s without the user:profile scope, but Anthropic returns
// anthropic-organization-id on the probe response regardless of scope.
func TestProbeClaudeSubscription_CapturesOrganizationID(t *testing.T) {
	orig := subscriptionProbeURL
	defer func() { subscriptionProbeURL = orig }()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("anthropic-organization-id", "f2f721d7-f975-426f-bb19-b0b45a3a9d52")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	subscriptionProbeURL = srv.URL

	probe := ProbeClaudeSubscription(context.Background(), "sk-ant-oat-setup")
	if probe.Result != ProbeValid {
		t.Fatalf("Result = %v (%q), want ProbeValid", probe.Result, probe.Detail)
	}
	if probe.OrganizationID != "f2f721d7-f975-426f-bb19-b0b45a3a9d52" {
		t.Fatalf("OrganizationID = %q, want the header value", probe.OrganizationID)
	}
	if probe.Token != "sk-ant-oat-setup" {
		t.Fatalf("Token = %q, want the probed bearer echoed back", probe.Token)
	}
}

// A 401 still identifies the organization — Anthropic sets the header before it
// rejects. Capturing it means a revoked setup token does not lose its identity.
func TestProbeClaudeSubscription_CapturesOrganizationIDOnRejection(t *testing.T) {
	orig := subscriptionProbeURL
	defer func() { subscriptionProbeURL = orig }()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("anthropic-organization-id", "org-uuid")
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	subscriptionProbeURL = srv.URL

	probe := ProbeClaudeSubscription(context.Background(), "sk-ant-oat-revoked")
	if probe.Result != ProbeInvalid {
		t.Fatalf("Result = %v, want ProbeInvalid", probe.Result)
	}
	if probe.OrganizationID != "org-uuid" {
		t.Fatalf("OrganizationID = %q, want it captured even on 401", probe.OrganizationID)
	}
}
