// Socket Mode runner tests (FR-15, NFR-2/NFR-5). A fake Connector
// stands in for the slack-go WebSocket so the Run loop — acquire lock,
// consume events into the shared ingest, reconnect on drop — is tested
// without a real Slack connection.
package slack_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	slacktransport "github.com/helixml/helix/api/pkg/org/infrastructure/transports/slack"
)

func runSocket(t *testing.T, rec slacktransport.Receiver, connector slacktransport.Connector) (context.CancelFunc, func()) {
	t.Helper()
	box := &lockbox{}
	owner := newOwner(box)
	sm := slacktransport.NewSocketMode(rec, owner, connector, slog.New(slog.NewTextHandler(io.Discard, nil)))
	sm.SetIntervals(5*time.Millisecond, 5*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = sm.Run(ctx)
		close(done)
	}()
	wait := func() {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatalf("socket Run did not exit after cancel")
		}
	}
	return cancel, wait
}

func TestSocketMode_ForwardsEventToIngest(t *testing.T) {
	rec := &recordingReceiver{}
	var calls int32
	connector := func(ctx context.Context, handle func(teamID string, ev slacktransport.Event)) error {
		if atomic.AddInt32(&calls, 1) == 1 {
			handle("TAAA", slacktransport.Event{Channel: "C1", User: "U1", Text: "hi", TS: "1.1"})
			return nil // clean disconnect → runner will reconnect
		}
		<-ctx.Done() // subsequent connects idle until shutdown
		return ctx.Err()
	}
	cancel, wait := runSocket(t, rec, connector)

	// Wait for the event to be ingested.
	deadline := time.After(2 * time.Second)
	for rec.count() < 1 {
		select {
		case <-deadline:
			t.Fatalf("event not ingested")
		case <-time.After(5 * time.Millisecond):
		}
	}
	cancel()
	wait()

	if rec.count() != 1 {
		t.Fatalf("ingest count = %d, want exactly 1", rec.count())
	}
	team, ev := rec.last()
	if team != "TAAA" || ev.Channel != "C1" {
		t.Fatalf("forwarded event mismapped: team=%q ev=%+v", team, ev)
	}
}

func TestSocketMode_ReconnectsAfterError(t *testing.T) {
	rec := &recordingReceiver{}
	var calls int32
	connector := func(ctx context.Context, handle func(teamID string, ev slacktransport.Event)) error {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			return errors.New("boom: first connect fails") // drop before any event
		}
		if n == 2 {
			handle("TBBB", slacktransport.Event{Channel: "C2", User: "U2", Text: "second", TS: "2.2"})
			return nil
		}
		<-ctx.Done()
		return ctx.Err()
	}
	cancel, wait := runSocket(t, rec, connector)

	deadline := time.After(2 * time.Second)
	for rec.count() < 1 {
		select {
		case <-deadline:
			t.Fatalf("event not ingested after reconnect")
		case <-time.After(5 * time.Millisecond):
		}
	}
	cancel()
	wait()

	if atomic.LoadInt32(&calls) < 2 {
		t.Fatalf("connector called %d times, want >= 2 (reconnect)", calls)
	}
	team, _ := rec.last()
	if team != "TBBB" {
		t.Fatalf("ingested team = %q, want TBBB (from second connect)", team)
	}
}
