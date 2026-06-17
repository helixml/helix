package slack

import (
	"context"
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

// SocketMode is the WebSocket ingress source (Socket Mode) — Slack-
// managed auth, no inbound HTTP endpoint required (FR-15, US-6). One
// replica owns the single connection (NFR-2) via SingleOwner; on
// disconnect it reconnects with backoff (NFR-5). Inbound events feed the
// same shared ingest path as the REST source (FR-19).
type SocketMode struct {
	receiver  Receiver
	owner     *SingleOwner
	connector Connector
	logger    *slog.Logger
	poll      time.Duration
	backoff   time.Duration
}

// NewSocketMode builds the runner. poll is how often a non-owning
// replica re-checks the lock (failover latency); backoff is the wait
// after a dropped connection before reconnecting.
func NewSocketMode(receiver Receiver, owner *SingleOwner, connector Connector, logger *slog.Logger) *SocketMode {
	if logger == nil {
		logger = slog.Default()
	}
	return &SocketMode{
		receiver:  receiver,
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

// Run blocks until ctx is cancelled. It polls for the single-owner lock;
// the replica that wins opens the connection and consumes events,
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
		// We own the connection now. Consume until it drops or ctx ends.
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
		// Reconnect after backoff; keep the lock so we remain the owner.
		if !sleepCtx(ctx, s.backoff) {
			if s.owner != nil {
				s.owner.Release(ctx)
			}
			return ctx.Err()
		}
	}
}

// handle forwards a normalised event to the shared ingest path. Errors
// are logged, not propagated — a single bad event must not tear down the
// connection.
func (s *SocketMode) handle(teamID string, ev Event) {
	if err := s.receiver.Receive(context.Background(), teamID, ev); err != nil {
		s.logger.Error("slack.socketmode: ingest", "team", teamID, "err", err)
	}
}

// sleepCtx sleeps for d unless ctx is cancelled first. Returns false if
// the context was cancelled.
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

// NewSlackConnector is the production Connector: it opens a slack-go
// Socket Mode client (app-level token authenticates the socket; bot
// token authorises API calls) and translates each Events-API event into
// the transport-neutral Event before calling handle. apiURL overrides
// the Slack API base for tests; empty uses slack.com.
func NewSlackConnector(appToken, botToken, apiURL string, logger *slog.Logger) Connector {
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
				// NOTE: logging the connection LIFECYCLE (not message
				// content) is vital to operating Socket Mode. A Socket Mode
				// app fails silently in a way REST never does: the socket
				// connects fine, but if Event Subscriptions isn't enabled on
				// the Slack app — or the right bot events aren't subscribed —
				// no message events ever arrive, and there is nothing in any
				// log to say so. Emitting connecting/connected/disconnect/
				// error transitions is what lets an operator tell "the socket
				// is up and Slack just isn't sending us anything" apart from
				// "the socket never came up". We deliberately log only
				// connection-state transitions and per-event metadata
				// (channel/team, in the ingest) — never message bodies:
				// that keeps the signal high and avoids logging user content.
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
					// Hello / connecting / disconnect / ping are connection
					// lifecycle, not application events. Ack interactive
					// requests we do handle below; ignore the rest.
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
				// Metadata only (the event type, never the body): confirms
				// events are flowing and surfaces ones we don't yet map.
				logger.Info("slack.socketmode: event received", "team", eventsAPI.TeamID, "inner_type", eventsAPI.InnerEvent.Type)
				if ev, ok := toIngestEvent(eventsAPI.InnerEvent.Data); ok {
					handle(eventsAPI.TeamID, ev)
				} else {
					logger.Debug("slack.socketmode: event not mapped to ingest", "inner_type", eventsAPI.InnerEvent.Type)
				}
			}
		}()

		err := client.RunContext(ctx)
		if err == nil {
			// RunContext returned without error but the stream is over —
			// surface a sentinel so Run treats it as a reconnect.
			return errors.New("socketmode: connection closed")
		}
		return err
	}
}
