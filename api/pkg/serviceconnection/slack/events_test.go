package slack

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Not a real credential — a fixed HMAC key the test signs and verifies
// against itself. The inline directive keeps the secret scanner from
// flagging its entropy.
const testSecret = "8f742231b10e8888abcd99yyyzzz85a5" //gitleaks:allow

// sign computes the Slack v0 request signature for a body + timestamp.
func sign(secret, ts, body string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("v0:" + ts + ":" + body))
	return "v0=" + hex.EncodeToString(mac.Sum(nil))
}

func signedRequest(body string) *http.Request {
	ts := fmt.Sprintf("%d", time.Now().Unix())
	r := httptest.NewRequest(http.MethodPost, "/api/v1/slack/events", strings.NewReader(body))
	r.Header.Set("X-Slack-Request-Timestamp", ts)
	r.Header.Set("X-Slack-Signature", sign(testSecret, ts, body))
	return r
}

func secretFn(_ context.Context) ([]string, error) { return []string{testSecret}, nil }

// messageEventBody is a real (signature-gated) delivery — used by the
// tests that assert the signature/secret gate, which the unauthenticated
// url_verification handshake bypasses.
const messageEventBody = `{"type":"event_callback","team_id":"T1","event":{"type":"message","channel":"C1","user":"U1","text":"hi","ts":"1700.1"}}`

func TestEventsAPI_URLVerification_EchoesChallenge(t *testing.T) {
	h := EventsAPIHandler(secretFn, func(context.Context, string, Event) error {
		t.Fatal("onEvent must not be called for url_verification")
		return nil
	}, nil)

	body := `{"type":"url_verification","challenge":"abc123"}`
	w := httptest.NewRecorder()
	h.ServeHTTP(w, signedRequest(body))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if got := w.Body.String(); got != "abc123" {
		t.Fatalf("body = %q, want challenge echoed", got)
	}
}

// The handshake is answered even with NO signing secret configured and an
// invalid signature — this is what lets a manifest-set Request URL verify
// at app-create time, before the operator has copied the secret in.
func TestEventsAPI_URLVerification_EchoesBeforeSecretExists(t *testing.T) {
	empty := func(context.Context) ([]string, error) { return nil, nil }
	h := EventsAPIHandler(empty, func(context.Context, string, Event) error {
		t.Fatal("onEvent must not be called for url_verification")
		return nil
	}, nil)

	body := `{"type":"url_verification","challenge":"create-time"}`
	r := httptest.NewRequest(http.MethodPost, "/api/v1/slack/events", strings.NewReader(body))
	r.Header.Set("X-Slack-Signature", "v0=unsigned") // no valid signature yet
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusOK || w.Body.String() != "create-time" {
		t.Fatalf("status=%d body=%q, want 200 + challenge echoed", w.Code, w.Body.String())
	}
}

func TestEventsAPI_BadSignature_Rejected(t *testing.T) {
	h := EventsAPIHandler(secretFn, func(context.Context, string, Event) error {
		t.Fatal("onEvent must not be called on bad signature")
		return nil
	}, nil)

	r := httptest.NewRequest(http.MethodPost, "/api/v1/slack/events", strings.NewReader(messageEventBody))
	r.Header.Set("X-Slack-Request-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
	r.Header.Set("X-Slack-Signature", "v0=deadbeef")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func TestEventsAPI_StaleTimestamp_Rejected(t *testing.T) {
	// A correctly-signed real event whose timestamp is outside Slack's
	// 5-minute window is a replay — NewSecretsVerifier must reject it, so
	// a captured-and-resent delivery can't be replayed against us.
	h := EventsAPIHandler(secretFn, func(context.Context, string, Event) error {
		t.Fatal("onEvent must not be called for a stale (replayed) request")
		return nil
	}, nil)

	staleTS := fmt.Sprintf("%d", time.Now().Add(-10*time.Minute).Unix())
	r := httptest.NewRequest(http.MethodPost, "/api/v1/slack/events", strings.NewReader(messageEventBody))
	r.Header.Set("X-Slack-Request-Timestamp", staleTS)
	r.Header.Set("X-Slack-Signature", sign(testSecret, staleTS, messageEventBody))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 (stale timestamp rejected)", w.Code)
	}
}

func TestEventsAPI_NoSigningSecret_Inert(t *testing.T) {
	empty := func(context.Context) ([]string, error) { return nil, nil }
	h := EventsAPIHandler(empty, func(context.Context, string, Event) error {
		t.Fatal("a real event must not dispatch when no secret is configured")
		return nil
	}, nil)

	w := httptest.NewRecorder()
	h.ServeHTTP(w, signedRequest(messageEventBody))

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 (inert)", w.Code)
	}
}

func TestEventsAPI_MessageEvent_DispatchedWithTeamID(t *testing.T) {
	var gotTeam string
	var gotEvent Event
	h := EventsAPIHandler(secretFn, func(_ context.Context, team string, ev Event) error {
		gotTeam = team
		gotEvent = ev
		return nil
	}, nil)

	body := `{
		"type":"event_callback",
		"team_id":"T123",
		"event":{"type":"message","channel":"C999","user":"U1","text":"hello bot","ts":"1700000000.000100"}
	}`
	w := httptest.NewRecorder()
	h.ServeHTTP(w, signedRequest(body))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if gotTeam != "T123" {
		t.Fatalf("team = %q, want T123", gotTeam)
	}
	if gotEvent.Channel != "C999" || gotEvent.User != "U1" || gotEvent.Text != "hello bot" || gotEvent.TS != "1700000000.000100" {
		t.Fatalf("event mismatch: %+v", gotEvent)
	}
}

func TestEventsAPI_NonMessageEvent_Ignored(t *testing.T) {
	called := false
	h := EventsAPIHandler(secretFn, func(context.Context, string, Event) error {
		called = true
		return nil
	}, nil)

	// reaction_added is not a message-bearing event; it must not dispatch.
	body := `{"type":"event_callback","team_id":"T1","event":{"type":"reaction_added","user":"U1"}}`
	w := httptest.NewRecorder()
	h.ServeHTTP(w, signedRequest(body))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if called {
		t.Fatal("onEvent should not be called for non-message events")
	}
}
