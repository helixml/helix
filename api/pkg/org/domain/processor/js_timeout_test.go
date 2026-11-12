package processor_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/netguard"
	"github.com/helixml/helix/api/pkg/org/domain/eventsource"
	"github.com/helixml/helix/api/pkg/org/domain/processor"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
)

func withJSRunTimeout(t *testing.T, d time.Duration) {
	t.Helper()
	processor.SetJSRunTimeout(d)
	t.Cleanup(func() { processor.SetJSRunTimeout(0) })
}

func assertJSTimeout(t *testing.T, err error, started time.Time) {
	t.Helper()
	if !errors.Is(err, processor.ErrJSTimeout) {
		t.Fatalf("err = %v, want ErrJSTimeout", err)
	}
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("run took %s, timeout not enforced", elapsed)
	}
}

func TestJSProcessLoopTimesOut(t *testing.T) {
	withJSRunTimeout(t, 200*time.Millisecond)
	p := newJSProc(t, `function process(event) { while (true) {} }`, nil)

	before := runtime.NumGoroutine()
	started := time.Now()
	_, err := p.Process(context.Background(), streaming.Message{Body: "hi"})
	assertJSTimeout(t, err, started)

	// The watcher goroutine is released once the run returns.
	deadline := time.Now().Add(2 * time.Second)
	for runtime.NumGoroutine() > before && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if n := runtime.NumGoroutine(); n > before {
		t.Errorf("goroutines = %d after run, want <= %d", n, before)
	}

	// A well-behaved script still runs normally afterwards.
	ok := newJSProc(t, `function process(event) { return event; }`, nil)
	if _, err := ok.Process(context.Background(), streaming.Message{Body: "hi"}); err != nil {
		t.Fatalf("Process: %v", err)
	}
}

func TestJSTopLevelLoopTimesOut(t *testing.T) {
	withJSRunTimeout(t, 200*time.Millisecond)

	// Validation runs the script's top level too, so it is bounded as well.
	started := time.Now()
	_, err := processor.NewProcessor(
		"p-js", "JS", eventsource.Trigger("s-in"), processor.KindJS,
		cfg(t, map[string]string{"code": `for (;;) {} function process(e) { return e; }`}),
		out("s-out"), "w-owner", time.Now(), "org-1",
	)
	assertJSTimeout(t, err, started)
}

func TestJSHTTPCallSharesRunDeadline(t *testing.T) {
	withJSRunTimeout(t, 300*time.Millisecond)
	processor.SetJSHTTPPolicy(netguard.Policy{AllowPrivate: true})
	t.Cleanup(func() { processor.SetJSHTTPPolicy(netguard.Policy{}) })

	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	defer close(release)

	p := newJSProc(t, `function process(event) {
  http.get("`+srv.URL+`", { timeout_ms: 30000 });
  while (true) {}
}`, nil)
	started := time.Now()
	_, err := p.Process(context.Background(), streaming.Message{Body: "hi"})
	assertJSTimeout(t, err, started)
}

func TestJSParentCancellationStillInterrupts(t *testing.T) {
	p := newJSProc(t, `function process(event) { while (true) {} }`, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err := p.Process(ctx, streaming.Message{Body: "hi"})
	if err == nil || errors.Is(err, processor.ErrJSTimeout) {
		t.Fatalf("err = %v, want a cancellation error", err)
	}
	if time.Since(started) > 5*time.Second {
		t.Fatal("parent cancellation did not stop the script")
	}
}
