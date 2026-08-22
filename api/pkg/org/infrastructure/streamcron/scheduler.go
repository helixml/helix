// Package streamcron is the in-process scheduler that fires events on
// KindCron Triggers. It mirrors the design of api/pkg/trigger/cron — a
// gocron.Scheduler held in process, reconciled every 10 seconds against
// the current set of cron-kind Triggers in the database, with each
// Trigger's schedule attached as one gocron.Job.
//
// On each fire the scheduler publishes a system-emitted event to the
// Trigger through the shared publish use case, so a cron tick looks the
// same as any other event downstream: same append → notify → route
// sequence, same attachment fan-out, same processor chain.
//
// Single-leader caveat: same as the app-cron at api/pkg/trigger/cron.
// If the API is ever run with N>1 replicas the same leader-election
// story applies to both schedulers. Out of scope for this task.
package streamcron

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	_ "time/tzdata" // load all timezones so CRON_TZ=… works on stripped images

	"github.com/go-co-op/gocron/v2"
	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	"github.com/helixml/helix/api/pkg/org/domain/trigger"
)

// reconcileInterval matches the existing app-cron cadence
// (api/pkg/trigger/cron/trigger_cron.go) so operators see the same
// "edits land within ~10s" feedback loop across both schedulers.
const reconcileInterval = 10 * time.Second

// Publisher is the subset of the publish use case the scheduler uses.
// Defined here as an interface to keep streamcron decoupled from the
// application package (and to make tests easy).
type Publisher interface {
	PublishToTrigger(ctx context.Context, orgID, triggerID, from string, msg streaming.Message) (streaming.Event, error)
}

// Scheduler reconciles KindCron Triggers onto an in-process gocron
// scheduler and fires events on each tick. Construct with New and call
// Start; Start blocks until the supplied context is cancelled.
type Scheduler struct {
	store     *store.Store
	publisher Publisher
	scheduler gocron.Scheduler

	// newID and now are pulled out so tests can pin them. Production
	// wiring uses uuid.NewString and time.Now via the constructor's
	// defaults.
	newID func() string
	now   func() time.Time
}

// New constructs a Scheduler. store + publisher are required.
func New(s *store.Store, publisher Publisher, newID func() string, now func() time.Time) (*Scheduler, error) {
	if s == nil {
		return nil, fmt.Errorf("streamcron: store is required")
	}
	if publisher == nil {
		return nil, fmt.Errorf("streamcron: publisher is required")
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
		store: s,

		publisher: publisher,
		scheduler: gs,
		newID:     newID,
		now:       now,
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
		// existing cron Triggers without waiting a full tick.
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

// triggerKey is the gocron job name for a cron Trigger. Used to find &
// dedupe the in-process job and to recover its schedule from the job's
// tags during reconcile (gocron has no first-class "current schedule"
// accessor — we stash it in a tag, mirroring trigger/cron's pattern).
func triggerKey(orgID, triggerID string) string {
	return fmt.Sprintf("%s:%s", orgID, triggerID)
}

// reconcile diffs the current set of cron Triggers in the database
// against the gocron scheduler's jobs and adds/updates/removes to
// match. Identical pattern to api/pkg/trigger/cron/trigger_cron.go's
// reconcileCronApps, kept structurally similar so they read as a pair.
func (c *Scheduler) reconcile(ctx context.Context) error {
	triggers, err := c.store.Triggers.Find(ctx, store.WithTransportKind(string(transport.KindCron)))
	if err != nil {
		return fmt.Errorf("list cron triggers: %w", err)
	}

	want := make(map[string]trigger.Trigger, len(triggers))
	for _, s := range triggers {
		want[triggerKey(s.OrganizationID, s.ID)] = s
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
		cfg, err := s.Transport().CronConfig()
		if err != nil {
			log.Error().Err(err).Str("trigger", s.ID).Str("org", s.OrganizationID).Msg("streamcron: parse cron config")
			continue
		}
		// Validate guards against sub-minimum intervals AND
		// unparseable schedules. Skip rather than panic if a row got
		// past validation somehow (manual SQL, migration, etc.).
		if err := cfg.Validate(); err != nil {
			log.Warn().Err(err).Str("trigger", s.ID).Str("org", s.OrganizationID).Str("schedule", cfg.Schedule).Msg("streamcron: skipping invalid schedule")
			continue
		}

		if existing, ok := have[key]; ok {
			// Job exists — check whether the schedule changed.
			if jobSchedule(existing) == cfg.Schedule {
				continue
			}
			log.Info().
				Str("trigger", s.ID).
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
				log.Error().Err(err).Str("trigger", s.ID).Msg("streamcron: update job failed")
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
			log.Error().Err(err).Str("trigger", s.ID).Str("org", s.OrganizationID).Str("schedule", cfg.Schedule).Msg("streamcron: create job failed")
			continue
		}
		log.Info().
			Str("job_id", job.ID().String()).
			Str("trigger", s.ID).
			Str("org", s.OrganizationID).
			Str("schedule", cfg.Schedule).
			Msg("streamcron: scheduled trigger")
	}

	return nil
}

func jobOptions(s trigger.Trigger, schedule string) []gocron.JobOption {
	return []gocron.JobOption{
		gocron.WithName(triggerKey(s.OrganizationID, s.ID)),
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
// closure over (orgID, triggerID) rather than passed as a parameter
// because gocron tasks take no arguments. Wrapped in panic recovery so
// a single bad tick can't crash the scheduler loop.
func (c *Scheduler) fireFn(orgID, triggerID string) func() {
	return func() {
		defer func() {
			if r := recover(); r != nil {
				log.Error().
					Interface("panic", r).
					Str("trigger", triggerID).
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
		if err := c.fire(ctx, orgID, triggerID); err != nil {
			log.Error().Err(err).Str("trigger", triggerID).Str("org", orgID).Msg("streamcron: fire failed")
			return
		}
		log.Info().Str("trigger", triggerID).Str("org", orgID).Msg("streamcron: fired")
	}
}

// scheduledBody is the canonical body of a cron tick event. Workers
// that care about timing can decode this; workers that don't can
// treat it as opaque markdown. Stable shape — downstream tooling can
// match on `"kind":"scheduled"`.
type scheduledBody struct {
	Kind      string `json:"kind"`
	FiredAt   string `json:"firedAt"`
	TriggerID string `json:"triggerId"`
}

// fire builds and dispatches the tick event. Extracted from fireFn so
// tests can call it directly without going through gocron.
func (c *Scheduler) fire(ctx context.Context, orgID, triggerID string) error {
	firedAt := c.now()
	rows, err := c.store.Triggers.Find(ctx, store.WithOrg(orgID), store.WithID(triggerID), store.WithLimit(1))
	if err != nil {
		return fmt.Errorf("get trigger: %w", err)
	}
	if len(rows) == 0 {
		return fmt.Errorf("trigger %q: %w", triggerID, store.ErrNotFound)
	}
	cfg, err := rows[0].Transport().CronConfig()
	if err != nil {
		return fmt.Errorf("parse cron config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("validate cron config: %w", err)
	}

	body := cfg.Message
	bodyContentType := "text/plain"
	if strings.TrimSpace(body) == "" {
		// Preserve the structured payload for existing cron Triggers that
		// do not have a configured message.
		bodyBytes, err := json.Marshal(scheduledBody{
			Kind:      "scheduled",
			FiredAt:   firedAt.UTC().Format(time.RFC3339),
			TriggerID: triggerID,
		})
		if err != nil {
			return fmt.Errorf("encode body: %w", err)
		}
		body = string(bodyBytes)
		bodyContentType = "application/json"
	}

	// Wrap as a Message envelope so downstream readers (which always
	// expect Message JSON in event bodies — see streaming.NewMessageEvent
	// callers) parse uniformly.
	msg := streaming.Message{
		From:            "", // system-emitted
		Subject:         "Scheduled trigger",
		Body:            string(body),
		BodyContentType: bodyContentType,
	}
	// from="" -> Source="" (system-emitted); see event.go.
	if _, err := c.publisher.PublishToTrigger(ctx, orgID, triggerID, "", msg); err != nil {
		return fmt.Errorf("publish tick: %w", err)
	}
	return nil
}
