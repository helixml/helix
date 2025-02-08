package cron

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/go-co-op/gocron/v2"
	cronv3 "github.com/robfig/cron/v3"
	"github.com/rs/zerolog/log"
	openai "github.com/sashabaranov/go-openai"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

type Cron struct {
	cfg        *config.ServerConfig
	store      store.Store
	controller *controller.Controller
	cron       gocron.Scheduler
}

func New(cfg *config.ServerConfig, store store.Store, controller *controller.Controller) (*Cron, error) {
	s, err := gocron.NewScheduler()
	if err != nil {
		return nil, fmt.Errorf("failed to create scheduler: %w", err)
	}

	return &Cron{
		cfg:        cfg,
		store:      store,
		controller: controller,
		cron:       s,
	}, nil
}

func (c *Cron) Start(ctx context.Context) error {
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		err := c.startScheduler(ctx)
		if err != nil {
			log.Error().Err(err).Msg("failed to start scheduler")
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()

		ticker := time.NewTicker(time.Second * 10)
		defer ticker.Stop()

		//  Initial reconcile
		err := c.reconcileCronApps(ctx)
		if err != nil {
			log.Error().Err(err).Msg("failed to reconcile cron apps")
		}

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				err := c.reconcileCronApps(ctx)
				if err != nil {
					log.Error().Err(err).Msg("failed to reconcile cron apps")
				}
			}
		}
	}()

	wg.Wait()

	return nil
}

func (c *Cron) startScheduler(ctx context.Context) error {
	// start the scheduler
	c.cron.Start()

	log.Info().Msg("started app cron scheduler")

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
	apps, err := c.listApps(ctx)
	if err != nil {
		return fmt.Errorf("failed to list apps: %w", err)
	}

	jobs := c.cron.Jobs()

	return c.createOrDeleteCronApps(ctx, apps, jobs)
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
		trigger, ok := getAppSchedule(app)
		if !ok {
			continue
		}

		// If schedule is invalid or more often than every 90 seconds, skip it
		cronSchedule, err := cronv3.ParseStandard(trigger.Schedule)
		if err != nil {
			log.Error().
				Err(err).
				Str("app_id", app.ID).
				Str("app_name", app.Config.Helix.Name).
				Str("app_refresh_schedule", trigger.Schedule).
				Msg("invalid cron schedule")
			continue
		}

		nextRun := cronSchedule.Next(time.Now())
		secondRun := cronSchedule.Next(nextRun)
		if secondRun.Sub(nextRun) < 90*time.Second {
			log.Warn().
				Str("app_id", app.ID).
				Str("app_name", app.Config.Helix.Name).
				Str("app_refresh_schedule", trigger.Schedule).
				Msg("cron schedule is too frequent")
			continue
		}

		job, ok := jobsMap[app.ID]
		if !ok {

			// job doesn't exist, create it
			job, err := c.cron.NewJob(
				gocron.CronJob(trigger.Schedule, true),
				c.getCronAppTask(ctx, app.ID),
				c.getCronAppOptions(app)...,
			)
			if err != nil {
				log.Error().
					Err(err).
					Str("app_id", app.ID).
					Str("app_name", app.Config.Helix.Name).
					Str("app_refresh_schedule", trigger.Schedule).
					Msg("failed to create job")
				continue
			}

			log.Info().
				Str("job_id", job.ID().String()).
				Str("app_id", app.ID).
				Str("app_name", app.Config.Helix.Name).
				Str("app_refresh_schedule", trigger.Schedule).
				Msg("added cron job to the scheduler")

		} else {
			// Job exists, check schedule and update if needed
			currentSchedule := getCronJobSchedule(job)

			if currentSchedule != trigger.Schedule {
				log.Info().
					Str("app_id", app.ID).
					Str("app_name", app.Config.Helix.Name).
					Str("app_refresh_schedule", trigger.Schedule).
					Str("current_schedule", currentSchedule).
					Msg("updating cron job schedule")

				_, err := c.cron.Update(
					job.ID(),
					gocron.CronJob(trigger.Schedule, true),
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

		app, err := c.store.GetApp(ctx, appID)
		if err != nil {
			log.Error().
				Err(err).
				Str("app_id", appID).
				Msg("failed to get app")
			return
		}

		trigger, ok := getAppSchedule(app)
		if !ok {
			log.Error().
				Str("app_id", app.ID).
				Msg("no cron trigger found for app")
			return
		}

		messages := []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleUser,
				Content: trigger.Input,
			},
		}

		resp, _, err := c.controller.ChatCompletion(ctx, &types.User{
			ID: app.Owner,
		}, openai.ChatCompletionRequest{
			Stream:   false,
			Messages: messages,
		},
			&controller.ChatCompletionOptions{
				AppID: app.ID,
			})
		if err != nil {
			log.Error().
				Err(err).
				Str("app_id", app.ID).
				Msg("failed to run app cron job")
			return
		}

		var respContent string
		if len(resp.Choices) > 0 {
			respContent = resp.Choices[0].Message.Content
		}

		log.Info().
			Str("app_id", app.ID).
			Str("resp_content", respContent).
			Msg("app cron job completed")
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

func getAppSchedule(app *types.App) (*types.CronTrigger, bool) {
	for _, trigger := range app.Config.Helix.Triggers {
		if trigger.Cron != nil && trigger.Cron.Schedule != "" {
			return trigger.Cron, true
		}
	}

	return nil, false
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
