package server

import (
	"context"
	"errors"
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
