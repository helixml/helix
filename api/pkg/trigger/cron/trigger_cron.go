package cron

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	_ "time/tzdata" // load all timezones

	"github.com/go-co-op/gocron/v2"
	cronv3 "github.com/robfig/cron/v3"
	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/data"
	"github.com/helixml/helix/api/pkg/notification"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

type Cron struct {
	cfg        *config.ServerConfig
	store      store.Store
	notifier   notification.Notifier
	controller *controller.Controller
	cron       gocron.Scheduler
}

func NextRun(cron *types.CronTrigger) time.Time {
	cronSchedule, err := cronv3.ParseStandard(cron.Schedule)
	if err != nil {
		return time.Time{}
	}
	return cronSchedule.Next(time.Now())
}

// NextRunFormatted returns the next run time formatted as "Next run: July 31 at 5:30pm GMT+4"
func NextRunFormatted(cron *types.CronTrigger) string {
	nextRun := NextRun(cron)
	if nextRun.IsZero() {
		return "Invalid schedule"
	}

	// Extract timezone from cron schedule
	timezone := extractTimezoneFromCron(cron.Schedule)
	if timezone == "" {
		// Fallback to UTC if no timezone found
		timezone = "UTC"
	}

	// Parse the timezone
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		// Fallback to UTC if timezone parsing fails
		loc = time.UTC
	}

	// Convert next run time to the target timezone
	nextRunInTZ := nextRun.In(loc)

	// Format the time in the desired format
	month := nextRunInTZ.Format("January")
	day := nextRunInTZ.Format("2")
	time := nextRunInTZ.Format("3:04pm")
	// zone := nextRunInTZ.Format("MST")

	return fmt.Sprintf("Next run: %s %s at %s", month, day, time)
}

// extractTimezoneFromCron extracts the timezone from a cron schedule that contains CRON_TZ
func extractTimezoneFromCron(schedule string) string {
	// Look for CRON_TZ= pattern
	if strings.HasPrefix(schedule, "CRON_TZ=") {
		// Find the space after the timezone
		spaceIndex := strings.Index(schedule, " ")
		if spaceIndex > 0 {
			// Extract the timezone part (remove "CRON_TZ=" prefix)
			timezone := schedule[8:spaceIndex] // 8 is the length of "CRON_TZ="
			return timezone
		}
	}
	return ""
}

func New(cfg *config.ServerConfig, store store.Store, notifier notification.Notifier, controller *controller.Controller) (*Cron, error) {
	s, err := gocron.NewScheduler()
	if err != nil {
		return nil, fmt.Errorf("failed to create scheduler: %w", err)
	}

	return &Cron{
		cfg:        cfg,
		store:      store,
		notifier:   notifier,
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
	cronApps, err := c.getCronApps(ctx)
	if err != nil {
		return fmt.Errorf("failed to get cron apps: %w", err)
	}

	triggerCronApps, err := c.getCronAppsFromTriggers(ctx)
	if err != nil {
		return fmt.Errorf("failed to convert triggers to apps: %w", err)
	}

	jobs := c.cron.Jobs()

	apps := append(cronApps, triggerCronApps...)

	return c.createOrDeleteCronApps(ctx, apps, jobs)
}

type cronApp struct {
	ID      string // Trigger ID
	UserID  string // Either creator of trigger or app owner
	Name    string
	App     *types.App
	Trigger *types.CronTrigger
}

func (c *Cron) getCronApps(ctx context.Context) ([]*cronApp, error) {

	apps, err := c.listApps(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list apps: %w", err)
	}

	var cronApps []*cronApp

	for _, app := range apps {
		for _, trigger := range app.Config.Helix.Triggers {
			if trigger.Cron != nil && trigger.Cron.Enabled {
				cronApps = append(cronApps, &cronApp{
					ID:      app.ID,
					UserID:  app.Owner,
					Name:    app.Config.Helix.Name,
					Trigger: trigger.Cron,
					App:     app,
				})
			}
		}
	}

	return cronApps, nil
}

func (c *Cron) getCronAppsFromTriggers(ctx context.Context) ([]*cronApp, error) {
	triggerConfigs, err := c.store.ListTriggerConfigurations(ctx, &store.ListTriggerConfigurationsQuery{
		Enabled:     true,
		TriggerType: types.TriggerTypeCron,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list trigger configurations: %w", err)
	}

	var apps []*cronApp

	// Go through triggers and convert them each into an app that can be then used by the cron scheduler to execute the workloads
	for _, triggerConfig := range triggerConfigs {
		if triggerConfig.Trigger.Cron == nil {
			continue
		}

		app, err := c.store.GetApp(ctx, triggerConfig.AppID)
		if err != nil {
			return nil, fmt.Errorf("failed to get app: %w", err)
		}

		apps = append(apps, &cronApp{
			ID:      triggerConfig.ID,
			UserID:  triggerConfig.Owner,
			Name:    triggerConfig.Name,
			App:     app,
			Trigger: triggerConfig.Trigger.Cron,
		})
	}

	return apps, nil
}

func getCronAppKey(cronApp *cronApp) string {
	return fmt.Sprintf("%s:%s", cronApp.ID, cronApp.App.ID)
}

func (c *Cron) createOrDeleteCronApps(ctx context.Context, cronApps []*cronApp, jobs []gocron.Job) error {
	appsMap := make(map[string]*cronApp)   // app id to app
	jobsMap := make(map[string]gocron.Job) // app id to job

	for _, cronApp := range cronApps {
		appsMap[getCronAppKey(cronApp)] = cronApp
	}

	for _, job := range jobs {
		jobsMap[job.Name()] = job

		if _, ok := appsMap[job.Name()]; !ok {
			log.Info().
				Str("job_id", job.ID().String()).
				Strs("job_tags", job.Tags()).
				Str("job_name", job.Name()).
				Msg("removing job")

			err := c.cron.RemoveJob(job.ID())
			if err != nil {
				return fmt.Errorf("failed to remove job: %w", err)
			}
		}
	}

	for _, cronApp := range cronApps {

		// If schedule is invalid or more often than every 90 seconds, skip it
		cronSchedule, err := cronv3.ParseStandard(cronApp.Trigger.Schedule)
		if err != nil {
			log.Error().
				Err(err).
				Str("app_id", cronApp.App.ID).
				Str("app_name", cronApp.App.Config.Helix.Name).
				Str("schedule", cronApp.Trigger.Schedule).
				Msg("invalid cron schedule")
			continue
		}

		nextRun := cronSchedule.Next(time.Now())
		secondRun := cronSchedule.Next(nextRun)
		if secondRun.Sub(nextRun) < 90*time.Second {
			log.Warn().
				Str("app_id", cronApp.App.ID).
				Str("app_name", cronApp.App.Config.Helix.Name).
				Str("schedule", cronApp.Trigger.Schedule).
				Msg("cron schedule is too frequent")
			continue
		}

		job, ok := jobsMap[getCronAppKey(cronApp)]
		if !ok {

			// job doesn't exist, create it
			job, err := c.cron.NewJob(
				gocron.CronJob(cronApp.Trigger.Schedule, true),
				c.getCronAppTask(ctx, cronApp),
				c.getCronAppOptions(cronApp)...,
			)
			if err != nil {
				log.Error().
					Err(err).
					Str("app_id", cronApp.App.ID).
					Str("app_name", cronApp.App.Config.Helix.Name).
					Str("schedule", cronApp.Trigger.Schedule).
					Msg("failed to create job")
				continue
			}

			log.Info().
				Str("job_id", job.ID().String()).
				Str("app_id", cronApp.App.ID).
				Str("app_name", cronApp.App.Config.Helix.Name).
				Str("schedule", cronApp.Trigger.Schedule).
				Msg("added cron job to the scheduler")

		} else {
			// Job exists, check schedule and update if needed
			currentSchedule := getCronJobSchedule(job)

			if currentSchedule != cronApp.Trigger.Schedule {
				log.Info().
					Str("app_id", cronApp.App.ID).
					Str("app_name", cronApp.App.Config.Helix.Name).
					Str("schedule", cronApp.Trigger.Schedule).
					Str("current_schedule", currentSchedule).
					Msg("updating cron job schedule")

				_, err := c.cron.Update(
					job.ID(),
					gocron.CronJob(cronApp.Trigger.Schedule, true),
					c.getCronAppTask(ctx, cronApp),
					c.getCronAppOptions(cronApp)...,
				)
				if err != nil {
					return fmt.Errorf("failed to remove job: %w", err)
				}
			}
		}
	}

	return nil
}

func (c *Cron) getCronAppTask(ctx context.Context, cronApp *cronApp) gocron.Task {
	return gocron.NewTask(func() {
		log.Info().
			Str("app_id", cronApp.App.ID).
			Msg("running app cron job")

		_, err := ExecuteCronTask(ctx, c.store, c.controller, c.notifier, cronApp.App, cronApp.UserID, cronApp.ID, cronApp.Trigger, cronApp.Name)
		if err != nil {
			log.Error().Err(err).Msg("failed to execute cron task")
			return
		}

		log.Info().Msg("cron task completed")
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
			if trigger.Cron != nil && trigger.Cron.Schedule != "" && trigger.Cron.Enabled {
				filteredApps = append(filteredApps, app)
			}
		}
	}

	return filteredApps, nil
}

func (c *Cron) getCronAppOptions(cronApp *cronApp) []gocron.JobOption {

	return []gocron.JobOption{
		gocron.WithName(getCronAppKey(cronApp)),
		gocron.WithTags(fmt.Sprintf("schedule:%s", cronApp.Trigger.Schedule)),
	}
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

func ExecuteCronTask(ctx context.Context, str store.Store, ctrl *controller.Controller, notifier notification.Notifier, a *types.App, userID, triggerID string, trigger *types.CronTrigger, sessionName string) (string, error) {
	app, err := str.GetAppWithTools(ctx, a.ID)
	if err != nil {
		log.Error().
			Err(err).
			Str("app_id", a.ID).
			Msg("failed to get app")
		return "", err
	}

	// Prepare new session
	session := &types.Session{
		ID:             system.GenerateSessionID(),
		Name:           sessionName,
		Created:        time.Now(),
		Updated:        time.Now(),
		Mode:           types.SessionModeInference,
		Type:           types.SessionTypeText,
		ParentApp:      app.ID,
		OrganizationID: app.OrganizationID,
		Owner:          userID,
		OwnerType:      types.OwnerTypeUser,
		Metadata: types.SessionMetadata{
			Stream:       false,
			SystemPrompt: "",
			AssistantID:  "",
			HelixVersion: data.GetHelixVersion(),
		},
	}

	// Write session to the database
	err = ctrl.WriteSession(ctx, session)
	if err != nil {
		log.Error().
			Err(err).
			Str("app_id", app.ID).
			Msg("failed to create session")
		return "", err
	}

	user, err := str.GetUser(ctx, &store.GetUserQuery{
		ID: userID,
	})
	if err != nil {
		log.Error().
			Err(err).
			Str("app_id", app.ID).
			Str("user_id", userID).
			Msg("failed to get user")
		return "", err
	}

	// Create execution
	execution := &types.TriggerExecution{
		ID:                     system.GenerateUUID(),
		Name:                   sessionName,
		TriggerConfigurationID: triggerID,
		Created:                time.Now(),
		Updated:                time.Now(),
		Status:                 types.TriggerExecutionStatusRunning,
		SessionID:              session.ID,
	}

	startedAt := time.Now()

	execution, err = str.CreateTriggerExecution(ctx, execution)
	if err != nil {
		log.Error().
			Err(err).
			Str("app_id", app.ID).
			Msg("failed to create trigger execution")
		return "", fmt.Errorf("failed to create trigger execution: %w", err)
	}

	resp, err := ctrl.RunBlockingSession(ctx, &controller.RunSessionRequest{
		OrganizationID: app.OrganizationID,
		App:            app,
		Session:        session,
		User:           user,
		PromptMessage:  types.MessageContent{Parts: []any{trigger.Input}},
	})
	if err != nil {
		log.Error().
			Err(err).
			Str("app_id", app.ID).
			Msg("failed to run task blocking session job")

		// Send failure notification
		notifyErr := notifier.Notify(ctx, &types.Notification{
			Event:   types.EventCronTriggerFailed,
			Session: session,
			Message: err.Error(),
		})
		if notifyErr != nil {
			log.Error().
				Err(notifyErr).
				Str("app_id", app.ID).
				Str("session_id", session.ID).
				Msg("failed to send failure notification")
		}

		// Update execution with error
		execution.Status = types.TriggerExecutionStatusError
		execution.Error = err.Error()
		execution.DurationMs = time.Since(startedAt).Milliseconds()

		execution, err = str.UpdateTriggerExecution(ctx, execution)
		if err != nil {
			log.Error().
				Err(err).
				Str("app_id", app.ID).
				Str("execution_id", execution.ID).
				Msg("failed to update execution")
		}

		return "", err
	}

	// Send success notification
	err = notifier.Notify(ctx, &types.Notification{
		Event:          types.EventCronTriggerComplete,
		Session:        session,
		Message:        resp.ResponseMessage,
		RenderMarkdown: true,
	})
	if err != nil {
		log.Error().
			Err(err).
			Str("app_id", app.ID).
			Str("session_id", session.ID).
			Msg("failed to send success notification")
	}

	// Update execution with success
	execution.Status = types.TriggerExecutionStatusSuccess
	execution.Output = resp.ResponseMessage
	execution.DurationMs = time.Since(startedAt).Milliseconds()

	execution, err = str.UpdateTriggerExecution(ctx, execution)

	if err != nil {
		log.Error().
			Err(err).
			Str("app_id", app.ID).
			Str("execution_id", execution.ID).
			Msg("failed to update execution")
	}

	log.Info().
		Str("app_id", app.ID).
		Msg("app cron job completed")

	return resp.ResponseMessage, nil
}
