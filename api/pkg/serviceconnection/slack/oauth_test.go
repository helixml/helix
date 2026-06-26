package slack

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestAuthorizeURL_CarriesClientScopesState(t *testing.T) {
	got := AuthorizeURL("CID", "https://helix.example/cb", []string{"chat:write", "channels:history"}, "STATE123")
	if !strings.HasPrefix(got, AuthorizeBaseURL) {
		t.Fatalf("authorize URL %q must hit %q", got, AuthorizeBaseURL)
	}
	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	q := u.Query()
	if q.Get("client_id") != "CID" {
		t.Errorf("client_id = %q", q.Get("client_id"))
	}
	// Slack wants the bot scopes comma-joined in a single scope param.
	if q.Get("scope") != "chat:write,channels:history" {
		t.Errorf("scope = %q, want comma-joined", q.Get("scope"))
	}
	if q.Get("redirect_uri") != "https://helix.example/cb" {
		t.Errorf("redirect_uri = %q", q.Get("redirect_uri"))
	}
	if q.Get("state") != "STATE123" {
		t.Errorf("state = %q (the org round-trips through here)", q.Get("state"))
	}
}

// The SaaS install happy path: Slack returns a bot token + team, and the
// code + client credentials are forwarded to its token endpoint.
func TestExchangeCode_HappyPath(t *testing.T) {
	var gotForm url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotForm = r.PostForm
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"access_token":"xoxb-abc","bot_user_id":"U99","app_id":"A1","team":{"id":"T7","name":"Acme"}}`))
	}))
	defer srv.Close()

	install, err := CodeExchanger{TokenURL: srv.URL}.ExchangeCode(context.Background(), "CID", "SECRET", "the-code", "https://helix.example/cb")
	if err != nil {
		t.Fatalf("ExchangeCode: %v", err)
	}
	if install.BotToken != "xoxb-abc" || install.TeamID != "T7" || install.TeamName != "Acme" || install.BotUserID != "U99" || install.AppID != "A1" {
		t.Fatalf("install mismatch: %+v", install)
	}
	if gotForm.Get("code") != "the-code" || gotForm.Get("client_id") != "CID" ||
		gotForm.Get("client_secret") != "SECRET" || gotForm.Get("redirect_uri") != "https://helix.example/cb" {
		t.Fatalf("exchange must forward code + credentials; got %v", gotForm)
	}
}

// A Slack-reported failure (ok=false) surfaces the error code, not a
// silent empty install.
func TestExchangeCode_SlackError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"ok":false,"error":"invalid_code"}`))
	}))
	defer srv.Close()

	if _, err := (CodeExchanger{TokenURL: srv.URL}).ExchangeCode(context.Background(), "CID", "SECRET", "bad", "cb"); err == nil || !strings.Contains(err.Error(), "invalid_code") {
		t.Fatalf("want error mentioning invalid_code, got %v", err)
	}
}

// A malformed success (ok=true but no token/team) is rejected so a useless
// workspace row can't be persisted.
func TestExchangeCode_MissingToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true,"team":{"id":""}}`))
	}))
	defer srv.Close()

	if _, err := (CodeExchanger{TokenURL: srv.URL}).ExchangeCode(context.Background(), "CID", "SECRET", "c", "cb"); err == nil {
		t.Fatal("want error on missing access_token/team id, got nil")
	}
}
