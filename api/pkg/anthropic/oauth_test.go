package anthropic

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// The authorize URL must carry user:profile — that scope is the only reason
// this route can name the account, and the reason setup tokens cannot.
func TestStartClaudeLogin_RequestsProfileScopeAndBindsPKCE(t *testing.T) {
	challenge, err := StartClaudeLogin()
	if err != nil {
		t.Fatalf("StartClaudeLogin() error = %v", err)
	}
	parsed, err := url.Parse(challenge.AuthorizeURL)
	if err != nil {
		t.Fatalf("authorize URL is not parseable: %v", err)
	}
	q := parsed.Query()

	if got := q.Get("scope"); !contains(got, "user:profile") {
		t.Fatalf("scope = %q, want it to include user:profile", got)
	}
	if got := q.Get("code_challenge_method"); got != "S256" {
		t.Fatalf("code_challenge_method = %q, want S256", got)
	}
	if got := q.Get("redirect_uri"); got != claudeManualRedirectURI {
		t.Fatalf("redirect_uri = %q, want the hosted callback page", got)
	}
	if q.Get("state") != challenge.State {
		t.Fatal("state in the URL must match the state handed back to the caller")
	}
	// The challenge must actually be S256(verifier), or Anthropic rejects the exchange.
	sum := sha256.Sum256([]byte(challenge.CodeVerifier))
	if want := base64.RawURLEncoding.EncodeToString(sum[:]); q.Get("code_challenge") != want {
		t.Fatal("code_challenge is not S256 of the returned verifier")
	}
}

func TestStartClaudeLogin_IsUniquePerCall(t *testing.T) {
	a, _ := StartClaudeLogin()
	b, _ := StartClaudeLogin()
	if a.CodeVerifier == b.CodeVerifier || a.State == b.State {
		t.Fatal("each login attempt must get fresh PKCE material")
	}
}

// Anthropic's callback page hands back "<code>#<state>".
func TestSplitPastedCode(t *testing.T) {
	for _, tc := range []struct{ in, code, state string }{
		{"abc#xyz", "abc", "xyz"},
		{"  abc#xyz  ", "abc", "xyz"},
		{"abc", "abc", ""},
		{"", "", ""},
	} {
		code, state := SplitPastedCode(tc.in)
		if code != tc.code || state != tc.state {
			t.Fatalf("SplitPastedCode(%q) = (%q, %q), want (%q, %q)", tc.in, code, state, tc.code, tc.state)
		}
	}
}

func TestExchangeClaudeCode_SendsPKCEAndParsesTokens(t *testing.T) {
	var got map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"sk-ant-oat01-a","refresh_token":"sk-ant-ort01-b","expires_in":3600,"scope":"user:profile user:inference"}`))
	}))
	defer srv.Close()
	orig := claudeTokenURL
	claudeTokenURL = srv.URL
	defer func() { claudeTokenURL = orig }()

	tokens, err := ExchangeClaudeCode(context.Background(), "the-code", "the-verifier", "the-state")
	if err != nil {
		t.Fatalf("ExchangeClaudeCode() error = %v", err)
	}
	if got["grant_type"] != "authorization_code" || got["code"] != "the-code" || got["code_verifier"] != "the-verifier" {
		t.Fatalf("exchange body missing PKCE fields: %v", got)
	}
	if got["client_id"] != claudeOAuthClientID || got["redirect_uri"] != claudeManualRedirectURI {
		t.Fatalf("exchange body must echo the client and redirect used to authorize: %v", got)
	}
	if tokens.AccessToken != "sk-ant-oat01-a" || tokens.RefreshToken != "sk-ant-ort01-b" {
		t.Fatalf("tokens not parsed: %+v", tokens)
	}
	if tokens.ExpiresAt == 0 {
		t.Fatal("expires_in should become an absolute ExpiresAt")
	}
	if len(tokens.Scopes) != 2 {
		t.Fatalf("Scopes = %v, want the scope string split", tokens.Scopes)
	}
}

// A stale or half-copied code is the common failure, so the error must say so
// rather than surfacing a bare OAuth error string.
func TestExchangeClaudeCode_ExplainsRejection(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"authorization code expired"}`))
	}))
	defer srv.Close()
	orig := claudeTokenURL
	claudeTokenURL = srv.URL
	defer func() { claudeTokenURL = orig }()

	_, err := ExchangeClaudeCode(context.Background(), "stale", "verifier", "state")
	if err == nil {
		t.Fatal("expected an error for a rejected code")
	}
	if !contains(err.Error(), "authorization code expired") || !contains(err.Error(), "try connecting again") {
		t.Fatalf("error = %q, want the provider detail plus actionable advice", err)
	}
}

func TestExchangeClaudeCode_RequiresVerifier(t *testing.T) {
	if _, err := ExchangeClaudeCode(context.Background(), "code", "", ""); err == nil {
		t.Fatal("exchange without a verifier must fail before any network call")
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle || indexOf(haystack, needle) >= 0)
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
