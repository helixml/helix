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

const testSecret = "8f742231b10e8888abcd99yyyzzz85a5"

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

func secretFn(_ context.Context) (string, error) { return testSecret, nil }

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

func TestEventsAPI_BadSignature_Rejected(t *testing.T) {
	h := EventsAPIHandler(secretFn, func(context.Context, string, Event) error {
		t.Fatal("onEvent must not be called on bad signature")
		return nil
	}, nil)

	body := `{"type":"url_verification","challenge":"x"}`
	r := httptest.NewRequest(http.MethodPost, "/api/v1/slack/events", strings.NewReader(body))
	r.Header.Set("X-Slack-Request-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
	r.Header.Set("X-Slack-Signature", "v0=deadbeef")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func TestEventsAPI_NoSigningSecret_Inert(t *testing.T) {
	empty := func(context.Context) (string, error) { return "", nil }
	h := EventsAPIHandler(empty, func(context.Context, string, Event) error { return nil }, nil)

	w := httptest.NewRecorder()
	h.ServeHTTP(w, signedRequest(`{"type":"url_verification","challenge":"x"}`))

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
