package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/helixml/helix/api/pkg/types"
)

// A REST slack_app exposes only a signing secret (no app/bot token), which
// has no API to probe — testServiceConnection must report that as
// "nothing to validate", NOT a failure, so the row renders neutral rather
// than red.
func TestTestServiceConnection_RESTSlackApp_NothingToValidate(t *testing.T) {
	s := &HelixAPIServer{}
	err := s.testServiceConnection(context.Background(), types.ServiceConnectionCreateRequest{
		Type:               types.ServiceConnectionTypeSlackApp,
		SlackClientID:      "client-id",
		SlackClientSecret:  "client-secret",
		SlackSigningSecret: "signing-secret",
	})
	if !errors.Is(err, errNothingToValidate) {
		t.Fatalf("err = %v, want errNothingToValidate", err)
	}
}

// Connection tests must not reach a base or organization URL the outbound
// policy forbids (loopback, internal services, cloud metadata).
func TestTestServiceConnection_RejectsDisallowedURLs(t *testing.T) {
	s := &HelixAPIServer{}
	err := s.testServiceConnection(context.Background(), types.ServiceConnectionCreateRequest{
		Type:                 types.ServiceConnectionTypeGitHubApp,
		GitHubAppID:          1,
		GitHubInstallationID: 2,
		GitHubPrivateKey:     "unused",
		BaseURL:              "http://127.0.0.1:8080",
	})
	if !errors.Is(err, errDisallowedServiceConnectionURL) {
		t.Fatalf("github app: err = %v, want errDisallowedServiceConnectionURL", err)
	}

	err = s.testServiceConnection(context.Background(), types.ServiceConnectionCreateRequest{
		Type:               types.ServiceConnectionTypeADOServicePrincipal,
		ADOOrganizationURL: "https://169.254.169.254/org",
		ADOTenantID:        "tenant",
		ADOClientID:        "client",
		ADOClientSecret:    "secret",
	})
	if !errors.Is(err, errDisallowedServiceConnectionURL) {
		t.Fatalf("ado: err = %v, want errDisallowedServiceConnectionURL", err)
	}
}

func TestCreateServiceConnection_RejectsDisallowedBaseURL(t *testing.T) {
	s := &HelixAPIServer{}
	body := `{"name":"ghe","type":"github_app","github_app_id":1,"github_installation_id":2,"github_private_key":"k","base_url":"http://10.0.0.5"}`
	r := httptest.NewRequest(http.MethodPost, "/api/v1/service-connections", strings.NewReader(body))
	r = r.WithContext(setRequestUser(r.Context(), types.User{ID: "admin", Admin: true}))
	rec := httptest.NewRecorder()

	// Store is nil: reaching a store call would panic the test.
	s.createServiceConnection(rec, r)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "not an allowed destination") {
		t.Fatalf("rejected for the wrong reason: %s", rec.Body.String())
	}
}
