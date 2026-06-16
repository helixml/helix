// REST source tests (Slack Events API), §9.1 / FR-14 / NFR-4. The
// handler verifies the request signature against the global app's
// signing secret, answers the url_verification challenge, and forwards
// real events to the shared ingest path. These tests drive it through
// an httptest.Server against a fake Receiver so the REST seam is
// exercised independently of the DB-backed ingest.
package slack_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	slacktransport "github.com/helixml/helix/api/pkg/org/infrastructure/transports/slack"
)

const testSigningSecret = "8f742231b10e8888abcd99yyyzzz85a5"

// recordingReceiver captures Receive calls for the REST/Socket tests.
type recordingReceiver struct {
	mu    sync.Mutex
	teams []string
	evs   []slacktransport.Event
}

func (r *recordingReceiver) Receive(_ context.Context, teamID string, ev slacktransport.Event) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.teams = append(r.teams, teamID)
	r.evs = append(r.evs, ev)
	return nil
}

func (r *recordingReceiver) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.evs)
}

func (r *recordingReceiver) last() (string, slacktransport.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.evs) == 0 {
		return "", slacktransport.Event{}
	}
	return r.teams[len(r.teams)-1], r.evs[len(r.evs)-1]
}

// signSlack computes the X-Slack-Signature header for a body+timestamp.
func signSlack(secret string, ts int64, body []byte) string {
	base := fmt.Sprintf("v0:%d:%s", ts, body)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(base))
	return "v0=" + hex.EncodeToString(mac.Sum(nil))
}

func newEventsAPI(r slacktransport.Receiver) http.Handler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	api := slacktransport.NewEventsAPI(r, func(context.Context) (string, error) {
		return testSigningSecret, nil
	}, logger)
	return api.Handler()
}

// slackPost posts a body with Slack signature headers. ts=0 uses now;
// sig="" computes a correct signature; sig="-" sends none.
func slackPost(t *testing.T, h http.Handler, body []byte, ts int64, sig string) *http.Response {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	if ts == 0 {
		ts = time.Now().Unix()
	}
	if sig == "" {
		sig = signSlack(testSigningSecret, ts, body)
	}
	req, err := http.NewRequest(http.MethodPost, srv.URL, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Slack-Request-Timestamp", strconv.FormatInt(ts, 10))
	if sig != "-" {
		req.Header.Set("X-Slack-Signature", sig)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	return resp
}

func TestEventsAPI_URLVerification(t *testing.T) {
	rec := &recordingReceiver{}
	h := newEventsAPI(rec)
	body := []byte(`{"type":"url_verification","challenge":"abc123challenge"}`)
	resp := slackPost(t, h, body, 0, "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	got, _ := io.ReadAll(resp.Body)
	if string(got) != "abc123challenge" {
		t.Fatalf("challenge echo = %q, want abc123challenge", string(got))
	}
	if rec.count() != 0 {
		t.Fatalf("receiver called %d times for url_verification, want 0", rec.count())
	}
}

func TestEventsAPI_ValidMessageForwarded(t *testing.T) {
	rec := &recordingReceiver{}
	h := newEventsAPI(rec)
	body := []byte(`{
		"type":"event_callback",
		"team_id":"TAAA",
		"api_app_id":"A001",
		"event":{"type":"message","user":"U999","text":"hello there","channel":"C123","ts":"1700000000.000100","thread_ts":"1699999999.000001"}
	}`)
	resp := slackPost(t, h, body, 0, "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if rec.count() != 1 {
		t.Fatalf("receiver count = %d, want 1", rec.count())
	}
	team, ev := rec.last()
	if team != "TAAA" {
		t.Errorf("team = %q, want TAAA", team)
	}
	if ev.Channel != "C123" || ev.User != "U999" || ev.Text != "hello there" {
		t.Errorf("event mismapped: %+v", ev)
	}
	if ev.TS != "1700000000.000100" || ev.ThreadTS != "1699999999.000001" {
		t.Errorf("ts/thread mismapped: %+v", ev)
	}
}

func TestEventsAPI_BadSignatureRejected(t *testing.T) {
	rec := &recordingReceiver{}
	h := newEventsAPI(rec)
	body := []byte(`{"type":"event_callback","team_id":"TAAA","event":{"type":"message","user":"U1","text":"x","channel":"C1","ts":"1.1"}}`)
	resp := slackPost(t, h, body, 0, "v0=deadbeef")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
	if rec.count() != 0 {
		t.Fatalf("receiver called %d times on bad signature, want 0", rec.count())
	}
}

func TestEventsAPI_MissingSignatureRejected(t *testing.T) {
	rec := &recordingReceiver{}
	h := newEventsAPI(rec)
	body := []byte(`{"type":"event_callback","team_id":"TAAA","event":{"type":"message","user":"U1","text":"x","channel":"C1","ts":"1.1"}}`)
	resp := slackPost(t, h, body, 0, "-")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
	if rec.count() != 0 {
		t.Fatalf("receiver called %d times on missing signature, want 0", rec.count())
	}
}

func TestEventsAPI_StaleTimestampRejected(t *testing.T) {
	rec := &recordingReceiver{}
	h := newEventsAPI(rec)
	body := []byte(`{"type":"event_callback","team_id":"TAAA","event":{"type":"message","user":"U1","text":"x","channel":"C1","ts":"1.1"}}`)
	stale := time.Now().Add(-10 * time.Minute).Unix()
	resp := slackPost(t, h, body, stale, "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 for stale timestamp", resp.StatusCode)
	}
	if rec.count() != 0 {
		t.Fatalf("receiver called on stale timestamp, want 0")
	}
}

func TestEventsAPI_AppMentionForwarded(t *testing.T) {
	rec := &recordingReceiver{}
	h := newEventsAPI(rec)
	body := []byte(`{
		"type":"event_callback",
		"team_id":"TBBB",
		"event":{"type":"app_mention","user":"U777","text":"<@U0BOT> hi","channel":"C9","ts":"1700000001.000200"}
	}`)
	resp := slackPost(t, h, body, 0, "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if rec.count() != 1 {
		t.Fatalf("receiver count = %d, want 1", rec.count())
	}
	team, ev := rec.last()
	if team != "TBBB" || ev.Channel != "C9" || ev.User != "U777" {
		t.Errorf("app_mention mismapped: team=%q ev=%+v", team, ev)
	}
}

func TestEventsAPI_BotMessageStillForwardedToIngest(t *testing.T) {
	// The events_api source forwards bot-authored messages verbatim; the
	// self-echo guard lives in the ingest (so both ingress modes share
	// it). Here we just confirm BotID is threaded through, not dropped at
	// the REST seam.
	rec := &recordingReceiver{}
	h := newEventsAPI(rec)
	body := []byte(`{
		"type":"event_callback",
		"team_id":"TAAA",
		"event":{"type":"message","user":"U1","text":"x","channel":"C1","ts":"1.1","bot_id":"B0001"}
	}`)
	resp := slackPost(t, h, body, 0, "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	_, ev := rec.last()
	if ev.BotID != "B0001" {
		t.Errorf("BotID = %q, want B0001 threaded through", ev.BotID)
	}
}
