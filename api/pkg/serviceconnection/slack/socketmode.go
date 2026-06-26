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

// SocketMode is the WebSocket ingress source — Slack-managed auth, no
// inbound HTTP endpoint required. It opens the connection, consumes
// events, and reconnects with backoff on drop. Inbound events feed the
// same EventHandler as the REST source.
type SocketMode struct {
	onEvent   EventHandler
	connector Connector
	logger    *slog.Logger
	backoff   time.Duration
}

// NewSocketMode builds the runner. backoff is the wait after a dropped
// connection before reconnecting.
func NewSocketMode(onEvent EventHandler, connector Connector, logger *slog.Logger) *SocketMode {
	if logger == nil {
		logger = slog.Default()
	}
	return &SocketMode{
		onEvent:   onEvent,
		connector: connector,
		logger:    logger,
		backoff:   2 * time.Second,
	}
}

// Run opens the connection and consumes events until ctx is cancelled,
// reconnecting with backoff whenever the connection drops.
func (s *SocketMode) Run(ctx context.Context) error {
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		err := s.connector(ctx, s.handle)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			s.logger.Warn("slack.socketmode: connection dropped, reconnecting", "err", err)
		}
		if !sleepCtx(ctx, s.backoff) {
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
