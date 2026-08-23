package webhook_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	"github.com/helixml/helix/api/pkg/org/infrastructure/transports/webhook"
)

func webhookTopic(t *testing.T, url string) streaming.Topic {
	t.Helper()
	cfg, err := json.Marshal(transport.WebhookConfig{OutboundURL: url})
	if err != nil {
		t.Fatal(err)
	}
	topic, err := streaming.NewTopic("s-webhook", "webhook", "", "b-owner", time.Now().UTC(), transport.Transport{Kind: transport.KindWebhook, Config: cfg}, "org-test")
	if err != nil {
		t.Fatal(err)
	}
	return topic
}

func webhookEvent(t *testing.T, body string) streaming.Event {
	t.Helper()
	event, err := streaming.NewEvent("e-test", "s-webhook", "b-owner", body, time.Now().UTC(), "org-test")
	if err != nil {
		t.Fatal(err)
	}
	return event
}

func emitter() *webhook.OutboundEmitter {
	return webhook.NewOutboundEmitter(slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestEmitPreservesPayloadHeadersAndPath(t *testing.T) {
	t.Parallel()
	body := "líne 1 — α\n\x00\nemoji: 🚀"
	received := make(chan struct{}, 1)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, _ := io.ReadAll(r.Body)
		if string(got) != body || r.URL.Path != "/some/where" || r.Header.Get("X-Helix-Topic") != "s-webhook" || r.Header.Get("X-Helix-Event") != "e-test" || r.Header.Get("Content-Type") != "application/octet-topic" {
			t.Errorf("request body=%q path=%q headers=%v", got, r.URL.Path, r.Header)
		}
		received <- struct{}{}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer target.Close()

	emitter().Emit(context.Background(), webhookTopic(t, target.URL+"/some/where"), webhookEvent(t, body))
	select {
	case <-received:
	case <-time.After(time.Second):
		t.Fatal("webhook not received")
	}
}

func TestEmitToleratesProviderErrorsAndSubsequentDelivery(t *testing.T) {
	t.Parallel()
	var status atomic.Int32
	status.Store(http.StatusInternalServerError)
	var calls atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(int(status.Load()))
	}))
	defer target.Close()

	e := emitter()
	topic := webhookTopic(t, target.URL)
	e.Emit(context.Background(), topic, webhookEvent(t, "first"))
	status.Store(http.StatusBadRequest)
	e.Emit(context.Background(), topic, webhookEvent(t, "second"))
	status.Store(http.StatusNoContent)
	e.Emit(context.Background(), topic, webhookEvent(t, "third"))
	if calls.Load() != 3 {
		t.Fatalf("calls = %d, want 3", calls.Load())
	}
}

func TestEmitHonoursTimeout(t *testing.T) {
	t.Parallel()
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(300 * time.Millisecond)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer target.Close()

	e := emitter()
	e.SetHTTPClient(&http.Client{Timeout: 25 * time.Millisecond})
	started := time.Now()
	e.Emit(context.Background(), webhookTopic(t, target.URL), webhookEvent(t, "slow"))
	if elapsed := time.Since(started); elapsed > 150*time.Millisecond {
		t.Fatalf("emit exceeded bounded timeout: %s", elapsed)
	}
}

func TestEmitConcurrent(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer target.Close()

	e := emitter()
	topic := webhookTopic(t, target.URL)
	const count = 25
	var wg sync.WaitGroup
	for range count {
		wg.Add(1)
		go func() {
			defer wg.Done()
			e.Emit(context.Background(), topic, webhookEvent(t, "parallel"))
		}()
	}
	wg.Wait()
	if calls.Load() != count {
		t.Fatalf("calls = %d, want %d", calls.Load(), count)
	}
}

func TestEmitNoopsForMissingOrMalformedConfig(t *testing.T) {
	t.Parallel()
	e := emitter()
	e.Emit(context.Background(), webhookTopic(t, ""), webhookEvent(t, "no target"))
	e.Emit(context.Background(), streaming.Topic{ID: "s-webhook", Transport: transport.Transport{Kind: transport.KindWebhook, Config: []byte(`{invalid`)}}, webhookEvent(t, "bad config"))
}
