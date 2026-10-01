package janitor

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/system"
)

type recordingTransport struct {
	events []*sentry.Event
}

func (*recordingTransport) Configure(sentry.ClientOptions) {}
func (*recordingTransport) Flush(time.Duration) bool       { return true }
func (t *recordingTransport) SendEvent(event *sentry.Event) {
	t.events = append(t.events, event)
}

func TestInitializeReportsOnlyServerHTTPErrors(t *testing.T) {
	previousHTTPErrorHandler := system.HTTPErrorHandler
	previousErrorHandler := system.ErrorHandler
	previousClient := sentry.CurrentHub().Client()
	t.Cleanup(func() {
		system.HTTPErrorHandler = previousHTTPErrorHandler
		system.ErrorHandler = previousErrorHandler
		sentry.CurrentHub().BindClient(previousClient)
	})

	janitor := NewJanitor(config.Janitor{SentryDsnAPI: "http://public@example.com/1"})
	if err := janitor.Initialize(); err != nil {
		t.Fatal(err)
	}

	transport := &recordingTransport{}
	sentry.CurrentHub().Client().Transport = transport
	request := httptest.NewRequest("GET", "/", nil)

	system.HTTPErrorHandler(system.NewHTTPError422("unprocessable"), request)
	if len(transport.events) != 0 {
		t.Fatalf("422 captured %d Sentry events, want 0", len(transport.events))
	}

	system.HTTPErrorHandler(system.NewHTTPError500("server error"), request)
	if len(transport.events) != 1 {
		t.Fatalf("500 captured %d Sentry events, want 1", len(transport.events))
	}
}
