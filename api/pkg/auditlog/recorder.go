package auditlog

import (
	"context"
	"io"
	"os"
	"sync/atomic"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// EventWriter is the persistence half of the audit trail. It is deliberately a
// narrow interface so the recorder cannot drag the whole store into a request
// path.
type EventWriter interface {
	InsertRequestAuditEvents(ctx context.Context, events []*types.RequestAuditEvent) error
	PruneRequestAuditEvents(ctx context.Context, before time.Time, batchSize int) (int64, error)
}

// Options configures a Recorder. The zero value is disabled.
type Options struct {
	Enabled       bool
	Stdout        bool
	Database      bool
	RetentionDays int
	BufferSize    int

	// StdoutWriter defaults to os.Stdout. Tests point it at a buffer.
	StdoutWriter io.Writer
	// BatchSize is how many events one INSERT carries.
	BatchSize int
	// FlushInterval bounds how long an event can sit in the batch.
	FlushInterval time.Duration
	// PruneInterval is how often retention runs.
	PruneInterval time.Duration
	// PruneBatchSize is how many rows one DELETE statement removes.
	PruneBatchSize int
}

const (
	defaultBufferSize    = 8192
	defaultBatchSize     = 200
	defaultFlushInterval = 2 * time.Second
	defaultPruneInterval = 24 * time.Hour
	defaultPruneBatch    = 5000
	defaultRetentionDays = 180
	defaultPruneTimeout  = 10 * time.Minute
	dropWarnInterval     = 30 * time.Second
)

// Recorder owns the audit pipeline. Record never blocks: when the buffer is
// full the event is dropped and counted, because losing an audit row must not
// fail a user request.
type Recorder struct {
	opts   Options
	writer EventWriter
	logger zerolog.Logger

	events  chan *types.RequestAuditEvent
	dropped atomic.Uint64
	// lastDropWarn is a UnixNano timestamp; it rate-limits the drop warning.
	lastDropWarn atomic.Int64
}

func NewRecorder(opts Options, writer EventWriter) *Recorder {
	if opts.BufferSize <= 0 {
		opts.BufferSize = defaultBufferSize
	}
	if opts.BatchSize <= 0 {
		opts.BatchSize = defaultBatchSize
	}
	if opts.FlushInterval <= 0 {
		opts.FlushInterval = defaultFlushInterval
	}
	if opts.PruneInterval <= 0 {
		opts.PruneInterval = defaultPruneInterval
	}
	if opts.PruneBatchSize <= 0 {
		opts.PruneBatchSize = defaultPruneBatch
	}
	if opts.RetentionDays <= 0 {
		opts.RetentionDays = defaultRetentionDays
	}

	stdout := opts.StdoutWriter
	if stdout == nil {
		stdout = os.Stdout
	}

	return &Recorder{
		opts:   opts,
		writer: writer,
		logger: zerolog.New(stdout).With().Timestamp().Logger(),
		events: make(chan *types.RequestAuditEvent, opts.BufferSize),
	}
}

// Enabled reports whether the trail is on. A nil recorder is disabled.
func (r *Recorder) Enabled() bool {
	return r != nil && r.opts.Enabled
}

// Dropped is the number of events discarded because the buffer was full.
func (r *Recorder) Dropped() uint64 {
	if r == nil {
		return 0
	}
	return r.dropped.Load()
}

// Record fans one event out to the configured sinks.
func (r *Recorder) Record(event *types.RequestAuditEvent) {
	if !r.Enabled() || event == nil {
		return
	}
	if r.opts.Stdout {
		r.logStdout(event)
	}
	if !r.opts.Database || r.writer == nil {
		return
	}
	select {
	case r.events <- event:
	default:
		r.noteDrop()
	}
}

func (r *Recorder) noteDrop() {
	total := r.dropped.Add(1)
	now := time.Now().UnixNano()
	last := r.lastDropWarn.Load()
	if now-last < int64(dropWarnInterval) {
		return
	}
	if r.lastDropWarn.CompareAndSwap(last, now) {
		log.Warn().
			Uint64("dropped_total", total).
			Int("buffer_size", r.opts.BufferSize).
			Msg("request audit buffer full; dropping audit events")
	}
}

// logStdout writes one JSON line. It does not go through the global logger, so
// LOG_LEVEL=warn cannot silence it.
func (r *Recorder) logStdout(event *types.RequestAuditEvent) {
	r.logger.Info().
		Str("log", "audit").
		Str("id", event.ID).
		Time("created", event.Created).
		Int64("duration_ms", event.DurationMS).
		Str("method", event.Method).
		Str("route_template", event.RouteTemplate).
		Str("path", event.Path).
		Str("query", event.Query).
		Int("status", event.Status).
		Int64("response_bytes", event.ResponseBytes).
		Str("user_id", event.UserID).
		Str("user_email", event.UserEmail).
		Bool("user_admin", event.UserAdmin).
		Str("organization_id", event.OrganizationID).
		Str("token_type", event.TokenType).
		Str("api_key_type", event.APIKeyType).
		Str("credential_source", event.CredentialSource).
		Str("credential_fingerprint", event.CredentialFingerprint).
		Str("remote_addr", event.RemoteAddr).
		Str("remote_ip", event.RemoteIP).
		Int("remote_port", event.RemotePort).
		Str("client_ip", event.ClientIP).
		Str("cf_connecting_ip", event.CFConnectingIP).
		Str("x_forwarded_for", event.XForwardedFor).
		Str("x_real_ip", event.XRealIP).
		Str("user_agent", event.UserAgent).
		Str("request_id", event.RequestID).
		Str("content_type", event.ContentType).
		Int64("body_size", event.BodySize).
		Bool("body_truncated", event.BodyTruncated).
		Str("body", event.Body).
		Msg("request")
}

// Run drains the buffer into Postgres in batches until ctx is cancelled, then
// flushes what is left.
func (r *Recorder) Run(ctx context.Context) {
	if !r.Enabled() || !r.opts.Database || r.writer == nil {
		return
	}

	ticker := time.NewTicker(r.opts.FlushInterval)
	defer ticker.Stop()

	batch := make([]*types.RequestAuditEvent, 0, r.opts.BatchSize)
	for {
		select {
		case <-ctx.Done():
			for {
				drained := false
				select {
				case event := <-r.events:
					batch = append(batch, event)
					drained = true
				default:
				}
				if !drained {
					break
				}
				if len(batch) >= r.opts.BatchSize {
					r.flush(batch)
					batch = batch[:0]
				}
			}
			if len(batch) > 0 {
				r.flush(batch)
			}
			return
		case event := <-r.events:
			batch = append(batch, event)
			if len(batch) >= r.opts.BatchSize {
				r.flush(batch)
				batch = batch[:0]
			}
		case <-ticker.C:
			if len(batch) > 0 {
				r.flush(batch)
				batch = batch[:0]
			}
		}
	}
}

func (r *Recorder) flush(batch []*types.RequestAuditEvent) {
	// The request lifecycle that produced these events is long gone, so do not
	// inherit its cancellation.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := r.writer.InsertRequestAuditEvents(ctx, batch); err != nil {
		log.Warn().Err(err).Int("events", len(batch)).Msg("failed to persist request audit events")
	}
}

// RunPruner deletes rows older than RetentionDays once at startup (after a
// short delay so it does not compete with boot) and then daily.
func (r *Recorder) RunPruner(ctx context.Context) {
	if !r.Enabled() || !r.opts.Database || r.writer == nil {
		return
	}

	timer := time.NewTimer(time.Minute)
	defer timer.Stop()
	ticker := time.NewTicker(r.opts.PruneInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			r.pruneOnce(ctx)
		case <-ticker.C:
			r.pruneOnce(ctx)
		}
	}
}

func (r *Recorder) pruneOnce(ctx context.Context) {
	deleted, err := r.Prune(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("failed to prune request audit events")
		return
	}
	if deleted > 0 {
		log.Info().Int64("deleted", deleted).Int("retention_days", r.opts.RetentionDays).Msg("pruned request audit events")
	}
}

// Prune removes events older than the retention window, in chunks.
func (r *Recorder) Prune(ctx context.Context) (int64, error) {
	if !r.Enabled() || r.writer == nil {
		return 0, nil
	}
	cutoff := time.Now().AddDate(0, 0, -r.opts.RetentionDays)
	ctx, cancel := context.WithTimeout(ctx, defaultPruneTimeout)
	defer cancel()
	return r.writer.PruneRequestAuditEvents(ctx, cutoff, r.opts.PruneBatchSize)
}

// NewEventID returns a fresh event primary key. It is exported for the
// middleware, which stamps events as it creates them.
func NewEventID() string { return "rau_" + system.GenerateUUID() }
