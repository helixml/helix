package gitlab_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/helixml/helix/api/pkg/org/application/configregistry"

	"github.com/helixml/helix/api/pkg/org/application/publishing"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	"github.com/helixml/helix/api/pkg/org/domain/trigger"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
	gitlabtransport "github.com/helixml/helix/api/pkg/org/infrastructure/transports/gitlab"
)

var signingKey = bytes.Repeat([]byte{7}, 32)
var signingToken = "whsec_" + base64.StdEncoding.EncodeToString(signingKey)

const secretToken = "legacy-secret"

// recordingPublisher is the real publish use case wired to the same
// store, wrapped so tests can count what a delivery produced.
type recordingPublisher struct {
	inner *publishing.Publishing
	sync.Mutex
	events []streaming.Event
}

func (p *recordingPublisher) PublishDelivery(ctx context.Context, orgID, triggerID string, eventID streaming.EventID, msg streaming.Message) (streaming.Event, error) {
	ev, err := p.inner.PublishDelivery(ctx, orgID, triggerID, eventID, msg)
	if err != nil {
		return ev, err
	}
	p.Lock()
	defer p.Unlock()
	p.events = append(p.events, ev)
	return ev, nil
}

func testTransport(t *testing.T) (*gitlabtransport.Transport, *store.Store, *recordingPublisher) {
	t.Helper()
	st := orggorm.GetOrgTestDB(t)
	registry := configregistry.New(st.Configs)
	registry.Register(configregistry.Spec{Key: "transport.gitlab", Type: configregistry.TypeObject, Secrets: []string{"signing_token", "secret_token"}})
	raw, _ := json.Marshal(map[string]string{"signing_token": signingToken, "secret_token": secretToken})
	if err := registry.Set(context.Background(), "org-test", "transport.gitlab", string(raw)); err != nil {
		t.Fatal(err)
	}
	d := &recordingPublisher{inner: publishing.New(publishing.Deps{
		Triggers: st.Triggers, Events: st.Events,
		Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString,
	})}
	return gitlabtransport.New("org-test", registry, st, d, slog.New(slog.NewTextHandler(io.Discard, nil))), st, d
}

func seedTrigger(t *testing.T, st *store.Store, id, repo string, events []string, kind transport.Kind) {
	t.Helper()
	config, _ := json.Marshal(map[string]any{"repo": repo, "repository_id": "repo-1", "events": events})
	row, err := trigger.New(id, "org-test", id, "", kind, config, "b-owner", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Triggers.Create(context.Background(), row); err != nil {
		t.Fatal(err)
	}
}

func payload(repo string) []byte {
	body, _ := json.Marshal(map[string]any{
		"object_kind":       "merge_request",
		"project":           map[string]any{"path_with_namespace": repo},
		"user":              map[string]any{"username": "reviewer"},
		"object_attributes": map[string]any{"iid": 42, "title": "Improve review", "description": "Please review"},
	})
	return body
}

func request(handler http.Handler, body []byte, event, signature string) *httptest.ResponseRecorder {
	return requestAt(handler, http.MethodPost, body, event, "delivery-1", time.Now(), signature)
}

func requestAt(handler http.Handler, method string, body []byte, event, id string, at time.Time, signature string) *httptest.ResponseRecorder {
	timestamp := strconv.FormatInt(at.Unix(), 10)
	if signature == "" {
		mac := hmac.New(sha256.New, signingKey)
		mac.Write([]byte(id + "." + timestamp + "."))
		mac.Write(body)
		signature = "v1," + base64.StdEncoding.EncodeToString(mac.Sum(nil))
	}
	req := httptest.NewRequest(method, "/", bytes.NewReader(body))
	req.Header.Set("webhook-id", id)
	req.Header.Set("webhook-timestamp", timestamp)
	if signature != "-" {
		req.Header.Set("webhook-signature", signature)
	}
	req.Header.Set("X-Gitlab-Event", event)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func legacyRequest(handler http.Handler, body []byte, token, id string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.Header.Set("X-Gitlab-Token", token)
	req.Header.Set("X-Gitlab-Event-UUID", id)
	req.Header.Set("X-Gitlab-Event", "Merge Request Hook")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestInboundForTopicPublishesAndDispatchesRawMergeRequest(t *testing.T) {
	tp, st, d := testTransport(t)
	seedTrigger(t, st, "s-mr", "helixml/helix", []string{"Merge Request Hook"}, transport.KindGitLab)
	body := payload("helixml/helix")
	rec := request(tp.HandleInboundForTrigger("s-mr"), body, "Merge Request Hook", "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	events, err := st.Events.ListForStream(context.Background(), "org-test", "s-mr", 10)
	if err != nil || len(events) != 1 {
		t.Fatalf("events=%d err=%v", len(events), err)
	}
	message, err := events[0].Message()
	if err != nil || !bytes.Equal(message.Extra, body) || message.ThreadID != "!42" {
		t.Fatalf("message=%+v err=%v", message, err)
	}
	if len(d.events) != 1 {
		t.Fatalf("dispatches=%d", len(d.events))
	}
}

func TestInboundForTopicRejectsBadSignature(t *testing.T) {
	tp, st, _ := testTransport(t)
	seedTrigger(t, st, "s-mr", "helixml/helix", []string{"Merge Request Hook"}, transport.KindGitLab)
	if rec := request(tp.HandleInboundForTrigger("s-mr"), payload("helixml/helix"), "Merge Request Hook", "v1,bad"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d", rec.Code)
	}
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(payload("helixml/helix")))
	req.Header.Set("webhook-signature", "v1,bad")
	req.Header.Set("webhook-id", "bad-with-legacy")
	req.Header.Set("webhook-timestamp", strconv.FormatInt(time.Now().Unix(), 10))
	req.Header.Set("X-Gitlab-Token", secretToken)
	rec := httptest.NewRecorder()
	tp.HandleInboundForTrigger("s-mr").ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("signature fell back to legacy: status=%d", rec.Code)
	}
}

func TestInboundForTopicSignatureValidation(t *testing.T) {
	tp, st, _ := testTransport(t)
	seedTrigger(t, st, "s-mr", "helixml/helix", []string{"Merge Request Hook"}, transport.KindGitLab)
	handler := tp.HandleInboundForTrigger("s-mr")
	body := payload("helixml/helix")
	for _, test := range []struct {
		name      string
		at        time.Time
		signature string
	}{{"stale", time.Now().Add(-6 * time.Minute), ""}, {"future", time.Now().Add(6 * time.Minute), ""}, {"missing", time.Now(), "-"}} {
		t.Run(test.name, func(t *testing.T) {
			signature := test.signature
			if rec := requestAt(handler, http.MethodPost, body, "Merge Request Hook", "delivery-"+test.name, test.at, signature); rec.Code != http.StatusUnauthorized {
				t.Fatalf("status=%d", rec.Code)
			}
		})
	}
	if rec := legacyRequest(handler, body, secretToken, "legacy-delivery"); rec.Code != http.StatusNoContent {
		t.Fatalf("legacy status=%d", rec.Code)
	}
	if rec := legacyRequest(handler, body, "wrong", "bad-legacy-delivery"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("bad legacy status=%d", rec.Code)
	}
}

func TestInboundForTopicDeduplicatesLegacyEventUUID(t *testing.T) {
	tp, st, d := testTransport(t)
	seedTrigger(t, st, "s-mr", "helixml/helix", []string{"Merge Request Hook"}, transport.KindGitLab)
	handler := tp.HandleInboundForTrigger("s-mr")
	for _, id := range []string{"legacy-1", "legacy-2", "legacy-1"} {
		if rec := legacyRequest(handler, payload("helixml/helix"), secretToken, id); rec.Code != http.StatusNoContent {
			t.Fatalf("id=%q status=%d", id, rec.Code)
		}
	}
	events, _ := st.Events.ListForStream(context.Background(), "org-test", "s-mr", 10)
	if len(events) != 2 || len(d.events) != 2 {
		t.Fatalf("events=%d dispatches=%d", len(events), len(d.events))
	}
}

func TestInboundForTopicDeduplicatesDelivery(t *testing.T) {
	tp, st, d := testTransport(t)
	seedTrigger(t, st, "s-mr", "helixml/helix", []string{"Merge Request Hook"}, transport.KindGitLab)
	handler := tp.HandleInboundForTrigger("s-mr")
	for range 2 {
		if rec := request(handler, payload("helixml/helix"), "Merge Request Hook", ""); rec.Code != http.StatusNoContent {
			t.Fatalf("status=%d", rec.Code)
		}
	}
	events, _ := st.Events.ListForStream(context.Background(), "org-test", "s-mr", 10)
	if len(events) != 1 || len(d.events) != 1 {
		t.Fatalf("events=%d dispatches=%d", len(events), len(d.events))
	}
}

func TestInboundForTopicMapsNotes(t *testing.T) {
	tp, st, _ := testTransport(t)
	seedTrigger(t, st, "s-notes", "helixml/helix", []string{"Note Hook"}, transport.KindGitLab)
	handler := tp.HandleInboundForTrigger("s-notes")
	makeNote := func(id string, mr bool) []byte {
		value := map[string]any{"project": map[string]any{"path_with_namespace": "helixml/helix"}, "user": map[string]any{"username": "reviewer"}, "object_attributes": map[string]any{"note": "review note"}}
		if mr {
			value["merge_request"] = map[string]any{"iid": 17}
		}
		body, _ := json.Marshal(value)
		return body
	}
	for _, test := range []struct {
		id     string
		mr     bool
		thread string
	}{{"mr-note", true, "!17"}, {"commit-note", false, ""}} {
		rec := requestAt(handler, http.MethodPost, makeNote(test.id, test.mr), "Note Hook", test.id, time.Now(), "")
		if rec.Code != http.StatusNoContent {
			t.Fatalf("status=%d", rec.Code)
		}
	}
	events, _ := st.Events.ListForStream(context.Background(), "org-test", "s-notes", 10)
	if len(events) != 2 {
		t.Fatalf("events=%d", len(events))
	}
	threads := map[string]string{}
	for _, event := range events {
		message, _ := event.Message()
		threads[message.MessageID] = message.ThreadID
		if message.Body != "review note" {
			t.Fatalf("body=%q", message.Body)
		}
	}
	if threads["mr-note"] != "!17" || threads["commit-note"] != "" {
		t.Fatalf("threads=%v", threads)
	}
}

func TestInboundForTopicRejectsMethodAndMalformedBody(t *testing.T) {
	tp, st, _ := testTransport(t)
	seedTrigger(t, st, "s-mr", "helixml/helix", []string{"Merge Request Hook"}, transport.KindGitLab)
	handler := tp.HandleInboundForTrigger("s-mr")
	if rec := requestAt(handler, http.MethodGet, payload("helixml/helix"), "Merge Request Hook", "get", time.Now(), ""); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("method=%d", rec.Code)
	}
	if rec := requestAt(handler, http.MethodPost, []byte("{"), "Merge Request Hook", "bad-body", time.Now(), ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("body=%d", rec.Code)
	}
}

func TestInboundForTopicDropsWrongRepoAndEvent(t *testing.T) {
	tp, st, _ := testTransport(t)
	seedTrigger(t, st, "s-mr", "helixml/helix", []string{"Merge Request Hook"}, transport.KindGitLab)
	for _, test := range []struct{ repo, event string }{{"other/repo", "Merge Request Hook"}, {"helixml/helix", "Push Hook"}} {
		if rec := request(tp.HandleInboundForTrigger("s-mr"), payload(test.repo), test.event, ""); rec.Code != http.StatusNoContent {
			t.Fatalf("status=%d", rec.Code)
		}
	}
	events, _ := st.Events.ListForStream(context.Background(), "org-test", "s-mr", 10)
	if len(events) != 0 {
		t.Fatalf("events=%d", len(events))
	}
}

func TestInboundForTopicRejectsMissingOrWrongKindTopic(t *testing.T) {
	tp, st, _ := testTransport(t)
	seedTrigger(t, st, "s-local", "helixml/helix", []string{"Merge Request Hook"}, transport.KindWebhook)
	if rec := request(tp.HandleInboundForTrigger("s-missing"), payload("helixml/helix"), "Merge Request Hook", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("missing status=%d", rec.Code)
	}
	if rec := request(tp.HandleInboundForTrigger("s-local"), payload("helixml/helix"), "Merge Request Hook", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("wrong kind status=%d", rec.Code)
	}
}
