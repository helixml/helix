package processor_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/helixml/helix/api/pkg/org/domain/streaming"
)

// Scripts are written by org members and run in the API server, so by default
// their http.* calls must not reach loopback or other internal addresses.
func TestJSHTTPRefusesInternalDestinations(t *testing.T) {
	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		_, _ = io.WriteString(w, "internal")
	}))
	defer srv.Close()

	code := `function process(event, ctx) {
  const res = ctx.http.get(` + "`" + srv.URL + "`" + `);
  event.body = res.body;
  return event;
}`
	p := newJSProc(t, code, nil)
	_, err := p.Process(context.Background(), streaming.Message{Body: "x"})
	if err == nil {
		t.Fatal("expected the request to a loopback address to be refused")
	}
	if !strings.Contains(err.Error(), "destination not allowed") {
		t.Errorf("error = %v, want the outbound policy rejection", err)
	}
	if hit {
		t.Error("loopback server was reached")
	}
}
