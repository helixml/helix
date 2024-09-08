package cron

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-co-op/gocron/v2"
	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

type Cron struct {
	cfg   *config.ServerConfig
	store store.Store
	cron  gocron.Scheduler
}

func New(cfg *config.ServerConfig, store store.Store) (*Cron, error) {
	s, err := gocron.NewScheduler()
	if err != nil {
		return nil, fmt.Errorf("failed to create scheduler: %w", err)
	}

	return &Cron{
		cfg:   cfg,
		store: store,
		cron:  s,
	}, nil
}

func (c *Cron) Start(ctx context.Context) error {
	// start the scheduler
	c.cron.Start()

	// Block until the context is done
	<-ctx.Done()

	// when you're done, shut it down
	err := c.cron.Shutdown()
	if err != nil {
		return fmt.Errorf("failed to shutdown scheduler: %w", err)
	}

	return nil
}

func (c *Cron) reconcileCronApps(ctx context.Context) error {
	apps, err := c.store.ListApps(ctx, &store.ListAppsQuery{})
	if err != nil {
		return fmt.Errorf("failed to list apps: %w", err)
	}

}

func (c *Cron) createOrDeleteCronApps(ctx context.Context, apps []*types.App, jobs []gocron.Job) error {
	appsMap := make(map[string]*types.App) // app id to app
	jobsMap := make(map[string]gocron.Job) // app id to job

	for _, app := range apps {
		appsMap[app.ID] = app
	}

	for _, job := range jobs {
		jobsMap[job.Name()] = job

		// If the job is not in the knowledges list, remove it
		if _, ok := appsMap[job.Name()]; !ok {
			log.Info().
				Str("job_id", job.ID().String()).
				Strs("job_tags", job.Tags()).
				Msg("removing job")

			err := c.cron.RemoveJob(job.ID())
			if err != nil {
				return fmt.Errorf("failed to remove job: %w", err)
			}
		}
	}

	for _, app := range apps {

		appSchedule := getAppSchedule(app)

		job, ok := jobsMap[app.ID]
		if !ok {
			log.Info().
				Str("app_id", app.ID).
				Str("app_name", app.Config.Helix.Name).
				Str("app_refresh_schedule", appSchedule).
				Msg("adding cron job to the scheduler")

			// job doesn't exist, create it
			_, err := c.cron.NewJob(
				gocron.CronJob(appSchedule, true),
				c.getCronAppTask(ctx, app.ID),
				c.getCronAppOptions(app)...,
			)
			if err != nil {
				log.Error().
					Err(err).
					Str("app_id", app.ID).
					Str("app_name", app.Config.Helix.Name).
					Str("app_refresh_schedule", appSchedule).
					Msg("failed to create job")
			}
		} else {
			// Job exists, check schedule and update if needed
			currentSchedule := getCronJobSchedule(job)

			if currentSchedule != appSchedule {
				log.Info().
					Str("app_id", app.ID).
					Str("app_name", app.Config.Helix.Name).
					Str("app_refresh_schedule", appSchedule).
					Str("current_schedule", currentSchedule).
					Msg("updating cron job schedule")

				_, err := c.cron.Update(
					job.ID(),
					gocron.CronJob(appSchedule, true),
					c.getCronAppTask(ctx, app.ID),
					c.getCronAppOptions(app)...,
				)
				if err != nil {
					return fmt.Errorf("failed to remove job: %w", err)
				}
			}
		}
	}

	return nil
}

func (c *Cron) getCronAppTask(ctx context.Context, appID string) gocron.Task {
	return gocron.NewTask(func() {
		log.Info().
			Str("app_id", appID).
			Msg("running app cron job")

	})
}

func (c *Cron) listApps(ctx context.Context) ([]*types.App, error) {
	apps, err := c.store.ListApps(ctx, &store.ListAppsQuery{})
	if err != nil {
		return nil, fmt.Errorf("failed to list apps: %w", err)
	}

	var filteredApps []*types.App

	for _, app := range apps {
		for _, trigger := range app.Config.Helix.Triggers {
			if trigger.Cron != nil && trigger.Cron.Schedule != "" {
				filteredApps = append(filteredApps, app)
			}
		}
	}

	return filteredApps, nil
}

func (c *Cron) getCronAppOptions(app *types.App) []gocron.JobOption {
	var schedule string

	for _, trigger := range app.Config.Helix.Triggers {
		if trigger.Cron != nil && trigger.Cron.Schedule != "" {
			schedule = trigger.Cron.Schedule
			break
		}
	}

	return []gocron.JobOption{
		gocron.WithName(app.ID),
		gocron.WithTags(fmt.Sprintf("schedule:%s", schedule)),
	}
}

func getAppSchedule(app *types.App) string {
	for _, trigger := range app.Config.Helix.Triggers {
		if trigger.Cron != nil && trigger.Cron.Schedule != "" {
			return trigger.Cron.Schedule
		}
	}

	return ""
}

func getCronJobSchedule(job gocron.Job) string {
	tags := job.Tags()

	// current schedule
	var currentSchedule string
	for _, tag := range tags {
		if strings.HasPrefix(tag, "schedule:") {
			currentSchedule = strings.TrimPrefix(tag, "schedule:")
			return currentSchedule
		}
	}

	return currentSchedule
}
