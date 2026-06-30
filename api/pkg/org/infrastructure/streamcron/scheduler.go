// Package streamcron is the in-process scheduler that fires events on
// KindCron topics. It mirrors the design of api/pkg/trigger/cron — a
// gocron.Scheduler held in process, reconciled every 10 seconds against
// the current set of cron-kind topics in the database, with each
// topic's schedule attached as one gocron.Job.
//
// On each fire the scheduler publishes a system-emitted streaming.Event
// to the topic and lets the existing dispatcher fan it out to every
// subscribed Worker. The call sequence (Events.Append → Hub.Notify →
// Dispatcher.Dispatch) is identical to the `publish` MCP tool's path,
// so cron ticks look the same as any other publish downstream.
//
// Single-leader caveat: same as the app-cron at api/pkg/trigger/cron.
// If the API is ever run with N>1 replicas the same leader-election
// story applies to both schedulers. Out of scope for this task.
package streamcron

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	_ "time/tzdata" // load all timezones so CRON_TZ=… works on stripped images

	"github.com/go-co-op/gocron/v2"
	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	"github.com/helixml/helix/api/pkg/org/infrastructure/wakebus"
)

// reconcileInterval matches the existing app-cron cadence
// (api/pkg/trigger/cron/trigger_cron.go) so operators see the same
// "edits land within ~10s" feedback loop across both schedulers.
const reconcileInterval = 10 * time.Second

// Dispatcher is the subset of dispatch.Dispatcher the scheduler uses.
// Defined here as an interface to keep streamcron decoupled from the
// dispatcher's concrete type (and to make tests easy).
type Dispatcher interface {
	Dispatch(ctx context.Context, event streaming.Event)
}

// Scheduler reconciles KindCron topics onto an in-process gocron
// scheduler and fires events on each tick. Construct with New and call
// Start; Start blocks until the supplied context is cancelled.
type Scheduler struct {
	store      *store.Store
	hub        *wakebus.Bus
	dispatcher Dispatcher
	scheduler  gocron.Scheduler

	// newID and now are pulled out so tests can pin them. Production
	// wiring uses uuid.NewString and time.Now via the constructor's
	// defaults.
	newID func() string
	now   func() time.Time
}

// New constructs a Scheduler. store + dispatcher are required; hub may
// be nil (skipping long-poll wakeups is fine — dispatch is the load-
// bearing fan-out for Worker activation).
func New(s *store.Store, hub *wakebus.Bus, dispatcher Dispatcher, newID func() string, now func() time.Time) (*Scheduler, error) {
	if s == nil {
		return nil, fmt.Errorf("streamcron: store is required")
	}
	if dispatcher == nil {
		return nil, fmt.Errorf("streamcron: dispatcher is required")
	}
	gs, err := gocron.NewScheduler()
	if err != nil {
		return nil, fmt.Errorf("streamcron: create scheduler: %w", err)
	}
	if newID == nil {
		newID = func() string { return fmt.Sprintf("evt-%d", time.Now().UnixNano()) }
	}
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Scheduler{
		store:      s,
		hub:        hub,
		dispatcher: dispatcher,
		scheduler:  gs,
		newID:      newID,
		now:        now,
	}, nil
}

// Start runs the scheduler until ctx is cancelled. Blocks. Intended to
// be launched in its own goroutine from the caller. Returns nil on
// clean shutdown, or any error from the underlying gocron Shutdown.
func (c *Scheduler) Start(ctx context.Context) error {
	c.scheduler.Start()
	log.Info().Msg("streamcron scheduler started")

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Initial reconcile so a freshly-started API picks up
		// existing cron topics without waiting a full tick.
		if err := c.reconcile(ctx); err != nil {
			log.Error().Err(err).Msg("streamcron: initial reconcile failed")
		}
		ticker := time.NewTicker(reconcileInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := c.reconcile(ctx); err != nil {
					log.Error().Err(err).Msg("streamcron: reconcile failed")
				}
			}
		}
	}()

	<-ctx.Done()
	wg.Wait()

	if err := c.scheduler.Shutdown(); err != nil {
		return fmt.Errorf("streamcron: shutdown: %w", err)
	}
	log.Info().Msg("streamcron scheduler stopped")
	return nil
}

// topicKey is the gocron job name for a cron topic. Used to find &
// dedupe the in-process job and to recover its schedule from the job's
// tags during reconcile (gocron has no first-class "current schedule"
// accessor — we stash it in a tag, mirroring trigger/cron's pattern).
func topicKey(orgID string, topicID streaming.TopicID) string {
	return fmt.Sprintf("%s:%s", orgID, topicID)
}

// reconcile diffs the current set of cron topics in the database
// against the gocron scheduler's jobs and adds/updates/removes to
// match. Identical pattern to api/pkg/trigger/cron/trigger_cron.go's
// reconcileCronApps, kept structurally similar so they read as a pair.
func (c *Scheduler) reconcile(ctx context.Context) error {
	topics, err := c.store.Topics.ListByTransportKind(ctx, transport.KindCron)
	if err != nil {
		return fmt.Errorf("list cron topics: %w", err)
	}

	want := make(map[string]streaming.Topic, len(topics))
	for _, s := range topics {
		want[topicKey(s.OrganizationID, s.ID)] = s
	}

	jobs := c.scheduler.Jobs()
	have := make(map[string]gocron.Job, len(jobs))
	for _, j := range jobs {
		have[j.Name()] = j
		if _, keep := want[j.Name()]; !keep {
			if err := c.scheduler.RemoveJob(j.ID()); err != nil {
				log.Error().Err(err).Str("job", j.Name()).Msg("streamcron: remove job failed")
			} else {
				log.Info().Str("job", j.Name()).Msg("streamcron: removed job")
			}
		}
	}

	for key, s := range want {
		cfg, err := s.Transport.CronConfig()
		if err != nil {
			log.Error().Err(err).Str("topic", string(s.ID)).Str("org", s.OrganizationID).Msg("streamcron: parse cron config")
			continue
		}
		// Validate guards against sub-minimum intervals AND
		// unparseable schedules. Skip rather than panic if a row got
		// past validation somehow (manual SQL, migration, etc.).
		if err := cfg.Validate(); err != nil {
			log.Warn().Err(err).Str("topic", string(s.ID)).Str("org", s.OrganizationID).Str("schedule", cfg.Schedule).Msg("streamcron: skipping invalid schedule")
			continue
		}

		if existing, ok := have[key]; ok {
			// Job exists — check whether the schedule changed.
			if jobSchedule(existing) == cfg.Schedule {
				continue
			}
			log.Info().
				Str("topic", string(s.ID)).
				Str("org", s.OrganizationID).
				Str("from", jobSchedule(existing)).
				Str("to", cfg.Schedule).
				Msg("streamcron: updating schedule")
			if _, err := c.scheduler.Update(
				existing.ID(),
				gocron.CronJob(cfg.Schedule, false),
				gocron.NewTask(c.fireFn(s.OrganizationID, s.ID)),
				jobOptions(s, cfg.Schedule)...,
			); err != nil {
				log.Error().Err(err).Str("topic", string(s.ID)).Msg("streamcron: update job failed")
			}
			continue
		}

		// New job.
		job, err := c.scheduler.NewJob(
			gocron.CronJob(cfg.Schedule, false),
			gocron.NewTask(c.fireFn(s.OrganizationID, s.ID)),
			jobOptions(s, cfg.Schedule)...,
		)
		if err != nil {
			log.Error().Err(err).Str("topic", string(s.ID)).Str("org", s.OrganizationID).Str("schedule", cfg.Schedule).Msg("streamcron: create job failed")
			continue
		}
		log.Info().
			Str("job_id", job.ID().String()).
			Str("topic", string(s.ID)).
			Str("org", s.OrganizationID).
			Str("schedule", cfg.Schedule).
			Msg("streamcron: scheduled topic")
	}

	return nil
}

func jobOptions(s streaming.Topic, schedule string) []gocron.JobOption {
	return []gocron.JobOption{
		gocron.WithName(topicKey(s.OrganizationID, s.ID)),
		// Tag carries the schedule string verbatim. Reconcile reads
		// this to decide whether to re-create the job; gocron itself
		// has no public accessor for the cron expression.
		gocron.WithTags("schedule:" + schedule),
	}
}

func jobSchedule(j gocron.Job) string {
	for _, tag := range j.Tags() {
		if len(tag) > len("schedule:") && tag[:len("schedule:")] == "schedule:" {
			return tag[len("schedule:"):]
		}
	}
	return ""
}

// fireFn returns the closure gocron invokes on each tick. Stored as a
// closure over (orgID, topicID) rather than passed as a parameter
// because gocron tasks take no arguments. Wrapped in panic recovery so
// a single bad tick can't crash the scheduler loop.
func (c *Scheduler) fireFn(orgID string, topicID streaming.TopicID) func() {
	return func() {
		defer func() {
			if r := recover(); r != nil {
				log.Error().
					Interface("panic", r).
					Str("topic", string(topicID)).
					Str("org", orgID).
					Msg("streamcron: panic during fire — schedule continues")
			}
		}()
		// Use a fresh background context per fire — the scheduler's
		// ctx governs *whether* we keep firing; once a tick begins,
		// dispatch should run to completion even if Start's ctx is
		// later cancelled (the dispatcher's own enqueue is fast and
		// non-blocking).
		ctx := context.Background()
		if err := c.fire(ctx, orgID, topicID); err != nil {
			log.Error().Err(err).Str("topic", string(topicID)).Str("org", orgID).Msg("streamcron: fire failed")
			return
		}
		log.Info().Str("topic", string(topicID)).Str("org", orgID).Msg("streamcron: fired")
	}
}

// scheduledBody is the canonical body of a cron tick event. Workers
// that care about timing can decode this; workers that don't can
// treat it as opaque markdown. Stable shape — downstream tooling can
// match on `"kind":"scheduled"`.
type scheduledBody struct {
	Kind    string `json:"kind"`
	FiredAt string `json:"firedAt"`
	TopicID string `json:"topicId"`
}

// fire builds and dispatches the tick event. Extracted from fireFn so
// tests can call it directly without going through gocron.
func (c *Scheduler) fire(ctx context.Context, orgID string, topicID streaming.TopicID) error {
	firedAt := c.now()
	body, err := json.Marshal(scheduledBody{
		Kind:    "scheduled",
		FiredAt: firedAt.UTC().Format(time.RFC3339),
		TopicID: string(topicID),
	})
	if err != nil {
		return fmt.Errorf("encode body: %w", err)
	}

	// Wrap as a Message envelope so downstream readers (which always
	// expect Message JSON in event bodies — see streaming.NewMessageEvent
	// callers) parse uniformly.
	msg := streaming.Message{
		From:            "", // system-emitted
		Subject:         "Scheduled trigger",
		Body:            string(body),
		BodyContentType: "application/json",
	}
	event, err := streaming.NewMessageEvent(
		streaming.EventID("e-"+c.newID()),
		topicID,
		"", // empty source = system-emitted; see event.go:58-63
		msg,
		firedAt,
		orgID,
	)
	if err != nil {
		return fmt.Errorf("construct event: %w", err)
	}

	if err := c.store.Events.Append(ctx, event); err != nil {
		return fmt.Errorf("append event: %w", err)
	}
	if c.hub != nil {
		c.hub.Notify(orgID, topicID)
	}
	c.dispatcher.Dispatch(ctx, event)
	return nil
}
