package slack

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	goslack "github.com/slack-go/slack"
)

type fakeMessageAPI struct {
	channel string
	options int
	err     error
}

func (f *fakeMessageAPI) PostMessageContext(_ context.Context, channel string, options ...goslack.MsgOption) (string, string, error) {
	f.channel, f.options = channel, len(options)
	return channel, "1700000000.000100", f.err
}

func TestDeliverText(t *testing.T) {
	var channel, text, thread string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		channel, text, thread = r.FormValue("channel"), r.FormValue("text"), r.FormValue("thread_ts")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"channel":"C123","ts":"1700000000.000100"}`))
	}))
	defer server.Close()
	client := goslack.New("xoxb-test", goslack.OptionAPIURL(server.URL+"/"))
	receipt, err := DeliverText(context.Background(), client, "C123", "hello", "1699999999.000001")
	if err != nil {
		t.Fatal(err)
	}
	if channel != "C123" || text != "hello" || thread != "1699999999.000001" || receipt.Destination != "C123" || receipt.MessageID != "1700000000.000100" {
		t.Fatalf("form = channel:%q text:%q thread:%q, receipt = %#v", channel, text, thread, receipt)
	}
}

func TestDeliverTextFailsExplicitly(t *testing.T) {
	if _, err := DeliverText(context.Background(), &fakeMessageAPI{}, "", "hello", ""); err == nil {
		t.Fatal("missing destination must fail")
	}
	if _, err := DeliverText(context.Background(), &fakeMessageAPI{err: errors.New("not_in_channel")}, "C123", "hello", ""); err == nil {
		t.Fatal("Slack rejection must fail")
	}
}
