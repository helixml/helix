package anthropic

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func profileServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	orig := claudeProfileURL
	t.Cleanup(func() { claudeProfileURL = orig })
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer sk-ant-oat-test" {
			t.Fatalf("Authorization = %q, want Bearer token", got)
		}
		w.WriteHeader(status)
		if body != "" {
			_, _ = io.WriteString(w, body)
		}
	}))
	claudeProfileURL = srv.URL
	return srv
}

const profileFixture = `{
	"account": {
		"uuid": "acc_1",
		"email": "phil@winder.ai",
		"display_name": "Phil Winder",
		"created_at": "2024-01-01T00:00:00Z"
	},
	"organization": {
		"uuid": "org_1",
		"organization_type": "claude_max",
		"rate_limit_tier": "20x",
		"seat_tier": "max20"
	}
}`

func TestFetchClaudeProfile(t *testing.T) {
	t.Run("200 maps account and plan", func(t *testing.T) {
		profileServer(t, http.StatusOK, profileFixture)
		p, err := FetchClaudeProfile(context.Background(), "sk-ant-oat-test")
		if err != nil {
			t.Fatalf("FetchClaudeProfile() error = %v", err)
		}
		if p.AccountEmail != "phil@winder.ai" {
			t.Errorf("AccountEmail = %q, want phil@winder.ai", p.AccountEmail)
		}
		if p.AccountDisplayName != "Phil Winder" {
			t.Errorf("AccountDisplayName = %q, want Phil Winder", p.AccountDisplayName)
		}
		if p.Plan != "max" {
			t.Errorf("Plan = %q, want max (mapped from claude_max)", p.Plan)
		}
		if p.RateLimitTier != "20x" {
			t.Errorf("RateLimitTier = %q, want 20x", p.RateLimitTier)
		}
	})

	t.Run("unknown org type leaves plan empty", func(t *testing.T) {
		body := `{"account": {"email": "x@y.z"}, "organization": {"organization_type": "claude_new_thing"}}`
		profileServer(t, http.StatusOK, body)
		p, err := FetchClaudeProfile(context.Background(), "sk-ant-oat-test")
		if err != nil {
			t.Fatalf("FetchClaudeProfile() error = %v", err)
		}
		if p.Plan != "" {
			t.Errorf("Plan = %q, want empty for unknown org type", p.Plan)
		}
		if p.AccountEmail != "x@y.z" {
			t.Errorf("AccountEmail = %q, want x@y.z", p.AccountEmail)
		}
	})

	t.Run("401 is an error, not a panic", func(t *testing.T) {
		profileServer(t, http.StatusUnauthorized, `{"error": {"type": "authentication_error"}}`)
		if _, err := FetchClaudeProfile(context.Background(), "sk-ant-oat-test"); err == nil {
			t.Fatal("FetchClaudeProfile() expected error on 401")
		}
	})

	t.Run("malformed body is an error", func(t *testing.T) {
		profileServer(t, http.StatusOK, "not json")
		if _, err := FetchClaudeProfile(context.Background(), "sk-ant-oat-test"); err == nil {
			t.Fatal("FetchClaudeProfile() expected error on malformed body")
		}
	})

	t.Run("empty token is an error without a request", func(t *testing.T) {
		_, err := FetchClaudeProfile(context.Background(), "")
		if err == nil || !strings.Contains(err.Error(), "no token") {
			t.Fatalf("FetchClaudeProfile() = %v, want a no-token error", err)
		}
	})
}
