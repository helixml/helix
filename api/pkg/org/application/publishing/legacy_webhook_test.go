package publishing_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/publishing"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	"github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
	"github.com/helixml/helix/api/pkg/org/infrastructure/transports/webhook"
)

func TestLegacyWebhookDeliveryIsIsolatedFromInbound(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	received := make(chan string, 1)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		body, _ := io.ReadAll(r.Body)
		received <- string(body)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer target.Close()

	st := memory.New()
	topic, err := streaming.NewTopic("s-webhook", "webhook", "", "b-owner", time.Now().UTC(), transport.Transport{Kind: transport.KindWebhook, Config: []byte(`{"outbound_url":"` + target.URL + `"}`)}, "org-test")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Topics.Create(context.Background(), topic); err != nil {
		t.Fatal(err)
	}
	var ids atomic.Int32
	svc := publishing.New(publishing.Deps{Topics: st.Topics, Events: st.Events, NewID: func() string { return fmt.Sprint(ids.Add(1)) }})
	svc.RegisterDeliverer(transport.KindWebhook, webhook.NewOutboundEmitter(slog.New(slog.NewTextHandler(io.Discard, nil))))

	result, err := svc.PublishWithReceipt(context.Background(), "org-test", topic.ID, "b-owner", streaming.Message{Body: "outbound"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Delivery != nil {
		t.Fatalf("webhook compatibility delivery must not claim a synchronous receipt: %#v", result.Delivery)
	}
	select {
	case body := <-received:
		if body == "" {
			t.Fatal("empty webhook body")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("legacy webhook was not delivered")
	}

	if _, err := svc.PublishInbound(context.Background(), "org-test", topic.ID, "", streaming.Message{Body: "inbound"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Publish(context.Background(), "org-test", topic.ID, "", streaming.Message{Body: "processor output"}); err != nil {
		t.Fatal(err)
	}
	select {
	case body := <-received:
		t.Fatalf("empty-source event echoed to webhook: %q", body)
	case <-time.After(250 * time.Millisecond):
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("webhook calls = %d, want exactly one outbound call", got)
	}
}
