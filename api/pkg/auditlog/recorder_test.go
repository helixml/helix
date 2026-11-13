package auditlog

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/require"
)

type fakeWriter struct {
	mu        sync.Mutex
	batches   [][]*types.RequestAuditEvent
	pruned    []time.Time
	batchSize int
}

func (f *fakeWriter) InsertRequestAuditEvents(_ context.Context, events []*types.RequestAuditEvent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.batches = append(f.batches, append([]*types.RequestAuditEvent(nil), events...))
	return nil
}

func (f *fakeWriter) PruneRequestAuditEvents(_ context.Context, before time.Time, batchSize int) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pruned = append(f.pruned, before)
	f.batchSize = batchSize
	return 0, nil
}

func (f *fakeWriter) allEvents() []*types.RequestAuditEvent {
	f.mu.Lock()
	defer f.mu.Unlock()
	var events []*types.RequestAuditEvent
	for _, batch := range f.batches {
		events = append(events, batch...)
	}
	return events
}

func sampleEvent(id string) *types.RequestAuditEvent {
	return &types.RequestAuditEvent{ID: id, Created: time.Now(), Method: "POST", Path: "/api/v1/things"}
}

func TestRecorderDropsWhenBufferIsFullAndNeverBlocks(t *testing.T) {
	recorder := NewRecorder(Options{
		Enabled:    true,
		Database:   true,
		BufferSize: 1,
		Stdout:     false,
	}, &fakeWriter{})

	started := time.Now()
	for i := 0; i < 25; i++ {
		recorder.Record(sampleEvent("evt"))
	}
	require.Less(t, time.Since(started), time.Second, "Record must not block when the buffer is full")
	require.EqualValues(t, 24, recorder.Dropped())
}

func TestRecorderFlushesInBatches(t *testing.T) {
	writer := &fakeWriter{}
	recorder := NewRecorder(Options{
		Enabled:       true,
		Database:      true,
		BufferSize:    16,
		BatchSize:     2,
		FlushInterval: 10 * time.Millisecond,
	}, writer)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go recorder.Run(ctx)

	recorder.Record(sampleEvent("one"))
	recorder.Record(sampleEvent("two"))
	recorder.Record(sampleEvent("three"))

	require.Eventually(t, func() bool {
		return len(writer.allEvents()) == 3
	}, 2*time.Second, 10*time.Millisecond)

	ids := make(map[string]bool)
	for _, event := range writer.allEvents() {
		ids[event.ID] = true
	}
	require.Equal(t, map[string]bool{"one": true, "two": true, "three": true}, ids)
}

func TestRecorderFlushesOnShutdown(t *testing.T) {
	writer := &fakeWriter{}
	recorder := NewRecorder(Options{
		Enabled:       true,
		Database:      true,
		BufferSize:    16,
		BatchSize:     100,
		FlushInterval: time.Hour,
	}, writer)

	ctx, cancel := context.WithCancel(context.Background())
	go recorder.Run(ctx)

	recorder.Record(sampleEvent("last"))
	cancel()

	require.Eventually(t, func() bool {
		return len(writer.allEvents()) == 1
	}, 2*time.Second, 10*time.Millisecond)
}

func TestRecorderStdoutLineIgnoresGlobalLogLevel(t *testing.T) {
	var stdout bytes.Buffer
	recorder := NewRecorder(Options{
		Enabled:      true,
		Stdout:       true,
		Database:     false,
		StdoutWriter: &stdout,
	}, nil)

	previous := log.Logger
	log.Logger = zerolog.Nop()
	defer func() { log.Logger = previous }()

	event := sampleEvent("evt_stdout")
	event.UserID = "usr_1"
	event.Status = 403
	recorder.Record(event)

	line := strings.TrimSpace(stdout.String())
	require.NotEmpty(t, line)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal([]byte(line), &parsed))
	require.Equal(t, "audit", parsed["log"])
	require.Equal(t, "evt_stdout", parsed["id"])
	require.Equal(t, "usr_1", parsed["user_id"])
	require.EqualValues(t, 403, parsed["status"])
}

func TestRecorderPruneUsesRetentionWindow(t *testing.T) {
	writer := &fakeWriter{}
	recorder := NewRecorder(Options{
		Enabled:        true,
		Database:       true,
		RetentionDays:  30,
		PruneBatchSize: 250,
	}, writer)

	_, err := recorder.Prune(context.Background())
	require.NoError(t, err)

	require.Len(t, writer.pruned, 1)
	require.WithinDuration(t, time.Now().AddDate(0, 0, -30), writer.pruned[0], time.Minute)
	require.Equal(t, 250, writer.batchSize)
}

func TestDisabledRecorderWritesNothing(t *testing.T) {
	var stdout bytes.Buffer
	writer := &fakeWriter{}
	recorder := NewRecorder(Options{
		Enabled:      false,
		Stdout:       true,
		Database:     true,
		StdoutWriter: &stdout,
	}, writer)

	require.False(t, recorder.Enabled())
	recorder.Record(sampleEvent("evt"))
	require.Empty(t, stdout.String())
	require.Empty(t, writer.allEvents())
}
