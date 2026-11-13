package janitor

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/helixml/helix/api/pkg/config"
)

func TestSendSubscriptionMessageUsesDedicatedWebhook(t *testing.T) {
	var body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		body = string(data)
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	janitor := NewJanitor(config.Janitor{SubscriptionsSlackWebhookURL: server.URL})
	if err := janitor.SendSubscriptionMessage("trial started"); err != nil {
		t.Fatal(err)
	}
	if body != `{"text":"trial started"}` {
		t.Fatalf("body = %s", body)
	}
}
