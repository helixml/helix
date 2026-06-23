package slack

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"time"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

// Connector opens one Socket Mode session and pumps inbound events to
// handle until the context is cancelled or the connection drops
// (returning the error). It is the seam the SocketMode runner depends
// on; production wraps slack-go's socketmode client, tests supply a
// fake. handle receives the already-normalised (teamID, Event).
type Connector func(ctx context.Context, handle func(teamID string, ev Event)) error

// SocketMode is the WebSocket ingress source — Slack-managed auth, no
// inbound HTTP endpoint required. One replica owns the single
// connection via SingleOwner; on disconnect it reconnects with backoff.
// Inbound events feed the same EventHandler as the REST source.
type SocketMode struct {
	onEvent   EventHandler
	owner     *SingleOwner
	connector Connector
	logger    *slog.Logger
	poll      time.Duration
	backoff   time.Duration
}

// NewSocketMode builds the runner. poll is how often a non-owning
// replica re-checks the lock (failover latency); backoff is the wait
// after a dropped connection before reconnecting.
func NewSocketMode(onEvent EventHandler, owner *SingleOwner, connector Connector, logger *slog.Logger) *SocketMode {
	if logger == nil {
		logger = slog.Default()
	}
	return &SocketMode{
		onEvent:   onEvent,
		owner:     owner,
		connector: connector,
		logger:    logger,
		poll:      10 * time.Second,
		backoff:   2 * time.Second,
	}
}

// SetIntervals overrides the poll/backoff durations (tests use short
// values to keep them fast).
func (s *SocketMode) SetIntervals(poll, backoff time.Duration) {
	s.poll = poll
	s.backoff = backoff
}

// Run blocks until ctx is cancelled. It polls for the single-owner
// lock; the replica that wins opens the connection and consumes events,
// reconnecting on drop. Losers keep polling so they take over on
// failover. Releasing the lock on exit lets another replica step in.
func (s *SocketMode) Run(ctx context.Context) error {
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if s.owner != nil && !s.owner.TryAcquire(ctx) {
			if !sleepCtx(ctx, s.poll) {
				return ctx.Err()
			}
			continue
		}
		err := s.connector(ctx, s.handle)
		if ctx.Err() != nil {
			if s.owner != nil {
				s.owner.Release(ctx)
			}
			return ctx.Err()
		}
		if err != nil {
			s.logger.Warn("slack.socketmode: connection dropped, reconnecting", "err", err)
		}
		if !sleepCtx(ctx, s.backoff) {
			if s.owner != nil {
				s.owner.Release(ctx)
			}
			return ctx.Err()
		}
	}
}

func (s *SocketMode) handle(teamID string, ev Event) {
	if err := s.onEvent(context.Background(), teamID, ev); err != nil {
		s.logger.Error("slack.socketmode: handle", "team", teamID, "err", err)
	}
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// NewConnector is the production Connector: it opens a slack-go Socket
// Mode client (app-level token authenticates the socket; bot token
// authorises API calls) and translates each Events-API event into the
// transport-neutral Event before calling handle. apiURL overrides the
// Slack API base for tests; empty uses slack.com.
func NewConnector(appToken, botToken, apiURL string, logger *slog.Logger) Connector {
	if logger == nil {
		logger = slog.Default()
	}
	return func(ctx context.Context, handle func(teamID string, ev Event)) error {
		opts := []slack.Option{slack.OptionAppLevelToken(appToken)}
		if apiURL != "" {
			opts = append(opts, slack.OptionAPIURL(apiURL))
		}
		api := slack.New(botToken, opts...)
		client := socketmode.New(api)

		go func() {
			for evt := range client.Events {
				// Logging the connection LIFECYCLE (not message content) is
				// vital to operating Socket Mode: the socket can connect
				// fine yet receive nothing if Event Subscriptions isn't
				// enabled on the app, with nothing in any log to say so.
				switch evt.Type {
				case socketmode.EventTypeConnecting:
					logger.Info("slack.socketmode: connecting")
				case socketmode.EventTypeConnected:
					logger.Info("slack.socketmode: connected — waiting for events (requires Event Subscriptions enabled on the app)")
				case socketmode.EventTypeDisconnect:
					logger.Warn("slack.socketmode: disconnected")
				case socketmode.EventTypeConnectionError, socketmode.EventTypeInvalidAuth:
					logger.Warn("slack.socketmode: connection problem", "type", string(evt.Type))
				}
				if evt.Type != socketmode.EventTypeEventsAPI {
					continue
				}
				eventsAPI, ok := evt.Data.(slackevents.EventsAPIEvent)
				if !ok {
					continue
				}
				if evt.Request != nil {
					client.Ack(*evt.Request)
				}
				if eventsAPI.Type != slackevents.CallbackEvent {
					continue
				}
				logger.Info("slack.socketmode: event received", "team", eventsAPI.TeamID, "inner_type", eventsAPI.InnerEvent.Type)
				if ev, ok := ToEvent(eventsAPI.InnerEvent.Data); ok {
					handle(eventsAPI.TeamID, ev)
				}
			}
		}()

		err := client.RunContext(ctx)
		if err == nil {
			return errors.New("socketmode: connection closed")
		}
		return err
	}
}

// --- single-owner lock --------------------------------------------------

// Locker is an exclusive, cross-replica lock. TryLock returns
// acquired=true only for the single caller that currently holds it.
// Production is a Postgres advisory lock; tests use a fake.
type Locker interface {
	TryLock(ctx context.Context) (acquired bool, err error)
	Unlock(ctx context.Context) error
}

// SingleOwner gates the Socket Mode connection so that, across a
// multi-replica deployment, exactly one replica opens the single
// outbound WebSocket and runs ingest. The winner holds the lock; losers
// poll to take over on failover.
type SingleOwner struct {
	locker Locker
	logger *slog.Logger
}

// NewSingleOwner wraps a Locker.
func NewSingleOwner(locker Locker, logger *slog.Logger) *SingleOwner {
	if logger == nil {
		logger = slog.Default()
	}
	return &SingleOwner{locker: locker, logger: logger}
}

// TryAcquire reports whether this replica won (or already holds) the
// lock. A lock error is treated as "not acquired" and logged.
func (o *SingleOwner) TryAcquire(ctx context.Context) bool {
	ok, err := o.locker.TryLock(ctx)
	if err != nil {
		o.logger.Warn("slack.singleowner: try-lock", "err", err)
		return false
	}
	return ok
}

// Release frees the lock so another replica can take over.
func (o *SingleOwner) Release(ctx context.Context) {
	if err := o.locker.Unlock(ctx); err != nil {
		o.logger.Warn("slack.singleowner: unlock", "err", err)
	}
}

// SocketModeLockKey is the constant advisory-lock key the Socket Mode
// owner contends on. Arbitrary but fixed across replicas.
const SocketModeLockKey int64 = 0x5_1ACC_50C7 // "slack soc(ket)"

// PgAdvisoryLock is the production Locker: a Postgres session-level
// advisory lock held on one dedicated connection.
type PgAdvisoryLock struct {
	db   *sql.DB
	key  int64
	conn *sql.Conn
}

// NewPgAdvisoryLock builds a Postgres advisory lock on the given key.
func NewPgAdvisoryLock(db *sql.DB, key int64) *PgAdvisoryLock {
	return &PgAdvisoryLock{db: db, key: key}
}

// TryLock attempts pg_try_advisory_lock on a fresh dedicated connection.
// On success the connection is retained (the session lock lives on it);
// on failure it is returned to the pool. Re-calling while held is a
// no-op success.
func (p *PgAdvisoryLock) TryLock(ctx context.Context) (bool, error) {
	if p.conn != nil {
		return true, nil
	}
	conn, err := p.db.Conn(ctx)
	if err != nil {
		return false, err
	}
	var acquired bool
	if err := conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", p.key).Scan(&acquired); err != nil {
		_ = conn.Close()
		return false, err
	}
	if !acquired {
		_ = conn.Close()
		return false, nil
	}
	p.conn = conn
	return true, nil
}

// Unlock releases the advisory lock and returns the dedicated
// connection to the pool. No-op when not held.
func (p *PgAdvisoryLock) Unlock(ctx context.Context) error {
	if p.conn == nil {
		return nil
	}
	_, err := p.conn.ExecContext(ctx, "SELECT pg_advisory_unlock($1)", p.key)
	_ = p.conn.Close()
	p.conn = nil
	return err
}

var _ Locker = (*PgAdvisoryLock)(nil)
