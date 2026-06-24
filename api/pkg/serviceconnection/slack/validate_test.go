package slack

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// slackStub serves canned ok/err JSON for a given api method path so the
// validators can be exercised without hitting slack.com.
func slackStub(t *testing.T, method, body string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, method) {
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv.URL + "/"
}

func TestValidateAppToken_Valid(t *testing.T) {
	url := slackStub(t, "apps.connections.open", `{"ok":true,"url":"wss://example.test/link"}`)
	if err := ValidateAppToken(context.Background(), "xapp-good", url); err != nil {
		t.Fatalf("valid app token: unexpected error %v", err)
	}
}

func TestValidateAppToken_Invalid(t *testing.T) {
	url := slackStub(t, "apps.connections.open", `{"ok":false,"error":"invalid_auth"}`)
	err := ValidateAppToken(context.Background(), "xapp-bad", url)
	if err == nil {
		t.Fatal("invalid app token: want error, got nil")
	}
	if !strings.Contains(err.Error(), "invalid_auth") {
		t.Fatalf("error %q should mention the slack error", err.Error())
	}
}

func TestValidateBotToken_Valid(t *testing.T) {
	url := slackStub(t, "auth.test", `{"ok":true,"team_id":"T1","team":"Acme","user_id":"U1","bot_id":"B1"}`)
	if err := ValidateBotToken(context.Background(), "xoxb-good", url); err != nil {
		t.Fatalf("valid bot token: unexpected error %v", err)
	}
}

func TestValidateBotToken_Invalid(t *testing.T) {
	url := slackStub(t, "auth.test", `{"ok":false,"error":"invalid_auth"}`)
	err := ValidateBotToken(context.Background(), "xoxb-bad", url)
	if err == nil {
		t.Fatal("invalid bot token: want error, got nil")
	}
	if !strings.Contains(err.Error(), "invalid_auth") {
		t.Fatalf("error %q should mention the slack error", err.Error())
	}
}
