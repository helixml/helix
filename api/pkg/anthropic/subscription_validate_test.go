package anthropic

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/crypto"
	"github.com/helixml/helix/api/pkg/types"
)

// encSub builds a ClaudeSubscription whose EncryptedCredentials hold creds.
func encSub(t *testing.T, credType string, creds interface{}) *types.ClaudeSubscription {
	t.Helper()
	plaintext, err := json.Marshal(creds)
	if err != nil {
		t.Fatalf("marshal credentials: %v", err)
	}
	key, err := crypto.GetEncryptionKey()
	if err != nil {
		t.Fatalf("get encryption key: %v", err)
	}
	enc, err := crypto.EncryptAES256GCM(plaintext, key)
	if err != nil {
		t.Fatalf("encrypt credentials: %v", err)
	}
	return &types.ClaudeSubscription{CredentialType: credType, EncryptedCredentials: enc}
}

// A long-expired OAuth access token means the in-container refresh is not
// working: a healthy Claude Code pushes refreshed credentials back, which moves
// ExpiresAt forward. Reporting that as inconclusive pinned Status at a stale
// "active", so a dead subscription rendered as connected-and-valid in settings
// while every desktop failed to authenticate.
func TestValidateSubscription_LongExpiredOAuthIsInvalid(t *testing.T) {
	sub := encSub(t, "oauth", types.ClaudeOAuthCredentials{
		AccessToken:  "sk-ant-oat-stale",
		RefreshToken: "sk-ant-ort-stale",
		ExpiresAt:    time.Now().Add(-20 * 24 * time.Hour).UnixMilli(),
	})

	got, detail := ValidateSubscription(context.Background(), sub)
	if got != ProbeInvalid {
		t.Fatalf("ValidateSubscription() = %v (%q), want ProbeInvalid", got, detail)
	}
	if detail == "" {
		t.Fatal("expected a human-readable detail explaining the staleness")
	}
}

// Just-expired tokens keep the benefit of the doubt — Claude Code refreshes
// those in-container, and probing the stale access token would 401 misleadingly.
func TestValidateSubscription_RecentlyExpiredOAuthIsInconclusive(t *testing.T) {
	sub := encSub(t, "oauth", types.ClaudeOAuthCredentials{
		AccessToken:  "sk-ant-oat-fresh",
		RefreshToken: "sk-ant-ort-fresh",
		ExpiresAt:    time.Now().Add(-5 * time.Minute).UnixMilli(),
	})

	got, detail := ValidateSubscription(context.Background(), sub)
	if got != ProbeInconclusive {
		t.Fatalf("ValidateSubscription() = %v (%q), want ProbeInconclusive", got, detail)
	}
}

// With no refresh token there is nothing to refresh with, so an expired access
// token is probed directly and a 401 is definitive.
func TestValidateSubscription_ExpiredOAuthWithoutRefreshTokenIsProbed(t *testing.T) {
	orig := subscriptionProbeURL
	defer func() { subscriptionProbeURL = orig }()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	subscriptionProbeURL = srv.URL

	sub := encSub(t, "oauth", types.ClaudeOAuthCredentials{
		AccessToken: "sk-ant-oat-norefresh",
		ExpiresAt:   time.Now().Add(-20 * 24 * time.Hour).UnixMilli(),
	})

	got, detail := ValidateSubscription(context.Background(), sub)
	if got != ProbeInvalid {
		t.Fatalf("ValidateSubscription() = %v (%q), want ProbeInvalid", got, detail)
	}
}

// A live OAuth token is probed and reported valid.
func TestValidateSubscription_LiveOAuthIsValid(t *testing.T) {
	orig := subscriptionProbeURL
	defer func() { subscriptionProbeURL = orig }()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	subscriptionProbeURL = srv.URL

	sub := encSub(t, "oauth", types.ClaudeOAuthCredentials{
		AccessToken:  "sk-ant-oat-live",
		RefreshToken: "sk-ant-ort-live",
		ExpiresAt:    time.Now().Add(8 * time.Hour).UnixMilli(),
	})

	got, detail := ValidateSubscription(context.Background(), sub)
	if got != ProbeValid {
		t.Fatalf("ValidateSubscription() = %v (%q), want ProbeValid", got, detail)
	}
}

// Setup tokens carry no expiry — a 401 from Anthropic is the only signal, and it
// is definitive. This is the shape of the token Chris pasted that was rejected.
func TestValidateSubscription_SetupTokenRejectedIsInvalid(t *testing.T) {
	orig := subscriptionProbeURL
	defer func() { subscriptionProbeURL = orig }()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	subscriptionProbeURL = srv.URL

	sub := encSub(t, "setup_token", types.ClaudeSetupTokenCredentials{SetupToken: "sk-ant-oat-setup"})

	got, detail := ValidateSubscription(context.Background(), sub)
	if got != ProbeInvalid {
		t.Fatalf("ValidateSubscription() = %v (%q), want ProbeInvalid", got, detail)
	}
}
