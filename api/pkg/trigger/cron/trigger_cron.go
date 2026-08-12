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
	"github.com/helixml/helix/api/pkg/prompts"
	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// SpecTaskCreator creates spec tasks from prompts. Implemented by SpecDrivenTaskService.
type SpecTaskCreator interface {
	CreateTaskFromPrompt(ctx context.Context, req *types.CreateTaskRequest) (*types.SpecTask, error)
}

// ExternalAgentStarter creates external agent (Zed) sessions. Implemented by the API server.
type ExternalAgentStarter interface {
	StartExternalAgentSession(ctx context.Context, req *types.SessionChatRequest, userID string) (*types.Session, error)
}

// SpecTaskCreatorProvider resolves the spec task creator when a job actually fires.
// The scheduler starts before the API server has constructed the
// SpecDrivenTaskService, so the creator MUST be resolved late — capturing it at
// construction time captures nil and every spec_task trigger silently no-ops.
type SpecTaskCreatorProvider func() SpecTaskCreator

// ExternalAgentStarterProvider resolves the external agent starter at fire time,
// for the same reason as SpecTaskCreatorProvider.
type ExternalAgentStarterProvider func() ExternalAgentStarter

type Cron struct {
	cfg                  *config.ServerConfig
	store                store.Store
	notifier             notification.Notifier
	controller           *controller.Controller
	specTaskCreator      SpecTaskCreatorProvider
	externalAgentStarter ExternalAgentStarterProvider
	cron                 gocron.Scheduler
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

func New(cfg *config.ServerConfig, store store.Store, notifier notification.Notifier, controller *controller.Controller, specTaskCreator SpecTaskCreatorProvider, externalAgentStarter ExternalAgentStarterProvider) (*Cron, error) {
	s, err := gocron.NewScheduler()
	if err != nil {
		return nil, fmt.Errorf("failed to create scheduler: %w", err)
	}

	if specTaskCreator == nil {
		return nil, fmt.Errorf("spec task creator provider is required")
	}
	if externalAgentStarter == nil {
		return nil, fmt.Errorf("external agent starter provider is required")
	}

	return &Cron{
		cfg:                  cfg,
		store:                store,
		notifier:             notifier,
		controller:           controller,
		specTaskCreator:      specTaskCreator,
		externalAgentStarter: externalAgentStarter,
		cron:                 s,
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
		c.runCronApp(ctx, cronApp)
	})
}

// runCronApp executes one scheduled job. Split out of getCronAppTask so the
// late-resolution of specTaskCreator/externalAgentStarter is directly testable.
func (c *Cron) runCronApp(ctx context.Context, cronApp *cronApp) {
	log.Info().
		Str("app_id", cronApp.App.ID).
		Str("trigger_id", cronApp.ID).
		Msg("running app cron job")

	// Resolved here, not at construction time: the scheduler starts before the
	// API server wires these in.
	_, err := ExecuteCronTask(ctx, c.store, c.controller, c.notifier, c.specTaskCreator(), c.externalAgentStarter(), cronApp.App, cronApp.UserID, cronApp.ID, cronApp.Trigger, cronApp.Name)
	if err != nil {
		log.Error().Err(err).
			Str("app_id", cronApp.App.ID).
			Str("trigger_id", cronApp.ID).
			Msg("failed to execute cron task")
		return
	}

	log.Info().Msg("cron task completed")
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

func ExecuteCronTask(ctx context.Context, str store.Store, ctrl *controller.Controller, notifier notification.Notifier, specTaskCreator SpecTaskCreator, externalAgentStarter ExternalAgentStarter, a *types.App, userID, triggerID string, trigger *types.CronTrigger, sessionName string) (*types.TriggerExecuteResponse, error) {
	// Handle spec_task action: create a spec task instead of running a session
	if trigger.Action == "spec_task" {
		content, err := executeSpecTaskAction(ctx, str, specTaskCreator, notifier, a, userID, triggerID, trigger, sessionName)
		if err != nil {
			return nil, err
		}
		return &types.TriggerExecuteResponse{Content: content, Status: types.TriggerExecutionStatusSuccess}, nil
	}

	// Resolve prompt: use InputFile from helix-specs worktree if set, otherwise Input
	promptText := trigger.Input
	if trigger.InputFile != "" {
		fileContent, err := readInputFile(ctx, str, trigger)
		if err != nil {
			log.Warn().Err(err).Str("input_file", trigger.InputFile).Msg("failed to read input file, falling back to Input field")
		} else {
			promptText = fileContent
		}
	}

	// Tasks-page triggers omit AgentType and ProjectID. Infer the runtime from
	// the selected agent so they take the same path as an interactive chat.
	if cronAgentType(a, trigger) == types.AgentTypeZedExternal {
		sessionID, err := executeExternalAgentCronTask(ctx, str, externalAgentStarter, notifier, a, userID, triggerID, trigger, sessionName, promptText)
		if err != nil {
			return nil, err
		}
		status := types.TriggerExecutionStatusRunning
		if sessionID == "" {
			status = types.TriggerExecutionStatusSkipped
		}
		return &types.TriggerExecuteResponse{SessionID: sessionID, Status: status}, nil
	}

	// Default action: run a blocking session. Reserve the trigger before
	// creating the session so blocking agents obey the same overlap policy as
	// external agents.
	sessionID := system.GenerateSessionID()
	execution, started, err := reserveCronExecution(ctx, str, triggerID, sessionName, sessionID)
	if err != nil {
		return nil, fmt.Errorf("reserve blocking agent trigger execution: %w", err)
	}
	if !started {
		log.Info().
			Str("app_id", a.ID).
			Str("trigger_id", triggerID).
			Str("execution_id", execution.ID).
			Msg("skipped blocking agent cron execution because the previous execution is still running")
		return &types.TriggerExecuteResponse{Status: types.TriggerExecutionStatusSkipped}, nil
	}

	startedAt := execution.Created
	if startedAt.IsZero() {
		startedAt = time.Now()
	}
	markFailed := func(taskErr error) error {
		execution.Status = types.TriggerExecutionStatusError
		execution.Error = taskErr.Error()
		execution.DurationMs = time.Since(startedAt).Milliseconds()
		if _, updateErr := str.UpdateTriggerExecution(ctx, execution); updateErr != nil {
			return fmt.Errorf("%w; failed to mark trigger execution failed: %v", taskErr, updateErr)
		}
		return taskErr
	}

	app, err := str.GetAppWithTools(ctx, a.ID)
	if err != nil {
		log.Error().
			Err(err).
			Str("app_id", a.ID).
			Msg("failed to get app")
		return nil, markFailed(err)
	}

	// Prepare new session
	session := &types.Session{
		ID:             sessionID,
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
		return nil, markFailed(err)
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
		return nil, markFailed(err)
	}

	resp, err := ctrl.RunBlockingSession(ctx, &controller.RunSessionRequest{
		OrganizationID: app.OrganizationID,
		App:            app,
		Session:        session,
		User:           user,
		PromptMessage:  types.MessageContent{Parts: []any{promptText}},
	})
	if err != nil {
		log.Error().
			Err(err).
			Str("app_id", app.ID).
			Msg("failed to run task blocking session job")

		// Send failure notification
		notifyErr := notifier.Notify(ctx, &types.Notification{
			Event:       types.EventCronTriggerFailed,
			Session:     session,
			Message:     err.Error(),
			Emails:      trigger.Emails,
			CallbackURL: trigger.CallbackURL,
		})
		if notifyErr != nil {
			log.Error().
				Err(notifyErr).
				Str("app_id", app.ID).
				Str("session_id", session.ID).
				Msg("failed to send failure notification")
		}

		return nil, markFailed(err)
	}

	responseText := types.TextFromInteraction(resp)

	// Send success notification
	err = notifier.Notify(ctx, &types.Notification{
		Event:          types.EventCronTriggerComplete,
		Session:        session,
		Message:        responseText,
		RenderMarkdown: true,
		Emails:         trigger.Emails,
		CallbackURL:    trigger.CallbackURL,
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
	execution.Output = responseText
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

	return &types.TriggerExecuteResponse{
		SessionID: session.ID,
		Content:   responseText,
		Status:    types.TriggerExecutionStatusSuccess,
	}, nil
}

func executeExternalAgentCronTask(ctx context.Context, str store.Store, starter ExternalAgentStarter, notifier notification.Notifier, a *types.App, userID, triggerID string, trigger *types.CronTrigger, sessionName string, promptText string) (string, error) {
	sessionID := system.GenerateSessionID()
	execution, started, err := reserveCronExecution(ctx, str, triggerID, sessionName, sessionID)
	if err != nil {
		return "", fmt.Errorf("reserve external agent trigger execution: %w", err)
	}
	if !started {
		log.Info().
			Str("app_id", a.ID).
			Str("trigger_id", triggerID).
			Str("execution_id", execution.ID).
			Msg("skipped external agent cron execution because the previous execution is still running")
		return "", nil
	}

	fail := func(taskErr error) (string, error) {
		execution.Status = types.TriggerExecutionStatusError
		execution.Error = taskErr.Error()
		execution.DurationMs = time.Since(execution.Created).Milliseconds()
		if _, updateErr := str.UpdateTriggerExecution(ctx, execution); updateErr != nil {
			log.Error().Err(updateErr).Str("execution_id", execution.ID).Msg("failed to mark external agent cron execution as failed")
		}
		if notifyErr := notifier.Notify(ctx, &types.Notification{
			Event:       types.EventCronTriggerFailed,
			Message:     taskErr.Error(),
			Emails:      trigger.Emails,
			CallbackURL: trigger.CallbackURL,
		}); notifyErr != nil {
			log.Error().Err(notifyErr).Msg("failed to send failure notification")
		}
		return "", taskErr
	}

	if starter == nil {
		return fail(fmt.Errorf("external agent starter not configured, cannot create zed_external cron session"))
	}

	projectID, err := externalAgentProjectID(ctx, str, a, userID, trigger.ProjectID)
	if err != nil {
		return fail(err)
	}

	session, err := starter.StartExternalAgentSession(ctx, &types.SessionChatRequest{
		AppID:               a.ID,
		SessionID:           sessionID,
		AssistantID:         "0",
		OrganizationID:      a.OrganizationID,
		ProjectID:           projectID,
		AgentType:           string(types.AgentTypeZedExternal),
		ExternalAgentConfig: a.Config.Helix.ExternalAgentConfig,
		SessionRole:         "job",
		CallbackURL:         trigger.CallbackURL,
		SystemPrompt:        prompts.RecurringTaskSystemPrompt(),
		Messages: []*types.Message{
			{
				Role:    "user",
				Content: types.MessageContent{Parts: []any{promptText}},
			},
		},
	}, userID)
	if err != nil {
		log.Error().Err(err).Str("app_id", a.ID).Msg("failed to start external agent cron session")
		return fail(err)
	}

	if session.ID != sessionID {
		return fail(fmt.Errorf("external agent starter returned session %s, expected %s", session.ID, sessionID))
	}

	log.Info().
		Str("app_id", a.ID).
		Str("session_id", session.ID).
		Msg("external agent cron session started")

	return session.ID, nil
}

func reserveCronExecution(ctx context.Context, str store.Store, triggerID, sessionName, sessionID string) (*types.TriggerExecution, bool, error) {
	return str.CreateTriggerExecutionUnlessRunning(ctx, &types.TriggerExecution{
		ID:                     system.GenerateTriggerExecutionID(),
		Name:                   sessionName,
		TriggerConfigurationID: triggerID,
		Status:                 types.TriggerExecutionStatusRunning,
		SessionID:              sessionID,
	})
}

func cronAgentType(app *types.App, trigger *types.CronTrigger) types.AgentType {
	if trigger != nil && trigger.AgentType != "" {
		return types.AgentType(trigger.AgentType)
	}
	if app == nil {
		return ""
	}
	if len(app.Config.Helix.Assistants) > 0 && app.Config.Helix.Assistants[0].AgentType != "" {
		return app.Config.Helix.Assistants[0].AgentType
	}
	return app.Config.Helix.DefaultAgentType
}

func externalAgentProjectID(ctx context.Context, str store.Store, app *types.App, userID, configuredProjectID string) (string, error) {
	if configuredProjectID != "" {
		return configuredProjectID, nil
	}
	if app == nil {
		return "", fmt.Errorf("cannot resolve project for external agent: app is required")
	}

	query := &store.ListProjectsQuery{OrganizationID: app.OrganizationID}
	if app.OrganizationID == "" {
		query.UserID = userID
	}
	projects, err := str.ListProjects(ctx, query)
	if err != nil {
		return "", fmt.Errorf("failed to list projects for external agent %s: %w", app.ID, err)
	}

	var projectID string
	for _, project := range projects {
		if project.DefaultHelixAppID != app.ID &&
			project.ProjectManagerHelixAppID != app.ID &&
			project.PullRequestReviewerHelixAppID != app.ID {
			continue
		}
		if projectID != "" {
			return "", fmt.Errorf("multiple projects reference external agent %s; project_id is required", app.ID)
		}
		projectID = project.ID
	}
	if projectID == "" {
		return "", fmt.Errorf("no project references external agent %s; project_id is required", app.ID)
	}

	return projectID, nil
}

func readInputFile(ctx context.Context, str store.Store, trigger *types.CronTrigger) (string, error) {
	if trigger.ProjectID == "" {
		return "", fmt.Errorf("project_id required to read input_file")
	}

	project, err := str.GetProject(ctx, trigger.ProjectID)
	if err != nil {
		return "", fmt.Errorf("failed to get project: %w", err)
	}

	if project.DefaultRepoID == "" {
		return "", fmt.Errorf("project has no primary repository")
	}

	repo, err := str.GetGitRepository(ctx, project.DefaultRepoID)
	if err != nil {
		return "", fmt.Errorf("failed to get repository: %w", err)
	}

	gitRepo, err := services.OpenGitRepo(repo.LocalPath)
	if err != nil {
		return "", fmt.Errorf("failed to open git repository: %w", err)
	}
	defer gitRepo.Close()

	content, err := gitRepo.ReadFileFromBranch("helix-specs", trigger.InputFile)
	if err != nil {
		return "", fmt.Errorf("failed to read %s from helix-specs branch: %w", trigger.InputFile, err)
	}

	return string(content), nil
}

func executeSpecTaskAction(ctx context.Context, str store.Store, specTaskCreator SpecTaskCreator, notifier notification.Notifier, a *types.App, userID, triggerID string, trigger *types.CronTrigger, sessionName string) (string, error) {
	// Create execution record FIRST, before any validation, so a misconfigured
	// trigger leaves a visible failed execution rather than disappearing. A fire
	// that records nothing is indistinguishable from a fire that never happened —
	// that is how a dead scheduler went unnoticed for months.
	execution := &types.TriggerExecution{
		ID:                     system.GenerateUUID(),
		Name:                   sessionName,
		TriggerConfigurationID: triggerID,
		Created:                time.Now(),
		Updated:                time.Now(),
		Status:                 types.TriggerExecutionStatusRunning,
	}

	startedAt := time.Now()

	execution, err := str.CreateTriggerExecution(ctx, execution)
	if err != nil {
		return "", fmt.Errorf("failed to create trigger execution: %w", err)
	}

	if specTaskCreator == nil {
		return "", recordSpecTaskFailure(ctx, str, notifier, execution, startedAt, a, triggerID, trigger.Emails,
			fmt.Errorf("spec task creator not configured, cannot execute spec_task action"))
	}

	if trigger.ProjectID == "" {
		return "", recordSpecTaskFailure(ctx, str, notifier, execution, startedAt, a, triggerID, trigger.Emails,
			fmt.Errorf("project_id is required for spec_task action"))
	}

	task, err := specTaskCreator.CreateTaskFromPrompt(ctx, &types.CreateTaskRequest{
		ProjectID: trigger.ProjectID,
		Prompt:    trigger.Input,
		UserID:    userID,
		// Scheduled triggers explicitly want the task to run at the scheduled
		// time, so skip backlog regardless of project AutoStartBacklogTasks.
		AutoStart: true,
		// A scheduled run must authenticate as the same person a hand-dispatched
		// one does. Without this the agent falls back to the app owner — one
		// service account for the whole fleet, so one expired token breaks every
		// schedule at once and nobody can run their own agent on their own
		// subscription. Empty keeps the old behaviour; the delegation check
		// downstream still decides whether the grant actually exists.
		CredentialOwnerID: trigger.CredentialOwnerID,
		// Skip spec generation when the trigger asks for it. An unattended run
		// left in spec_review waits forever for an approval nobody gives, and —
		// because BranchName is only assigned on the transition to implementation
		// — it is also refused every git push except helix-specs. See the note on
		// CronTrigger.JustDoItMode.
		JustDoItMode: trigger.JustDoItMode,
	})
	if err != nil {
		return "", recordSpecTaskFailure(ctx, str, notifier, execution, startedAt, a, triggerID, trigger.Emails, err)
	}

	output := fmt.Sprintf("Created spec task %s: %s", task.ID, task.Name)

	// Send success notification
	err = notifier.Notify(ctx, &types.Notification{
		Event:   types.EventCronTriggerComplete,
		Message: output,
		Emails:  trigger.Emails,
	})
	if err != nil {
		log.Error().
			Err(err).
			Str("app_id", a.ID).
			Str("task_id", task.ID).
			Msg("failed to send success notification for spec task")
	}

	// Update execution with success
	execution.Status = types.TriggerExecutionStatusSuccess
	execution.Output = output
	execution.DurationMs = time.Since(startedAt).Milliseconds()

	_, err = str.UpdateTriggerExecution(ctx, execution)
	if err != nil {
		log.Error().
			Err(err).
			Str("execution_id", execution.ID).
			Msg("failed to update execution")
	}

	log.Info().
		Str("app_id", a.ID).
		Str("task_id", task.ID).
		Msg("spec task cron job completed")

	return output, nil
}

// recordSpecTaskFailure marks the execution failed, notifies, and returns cause so
// the caller can propagate it. Every spec_task failure path goes through here so a
// fire that produced no task always leaves a row explaining why.
func recordSpecTaskFailure(ctx context.Context, str store.Store, notifier notification.Notifier, execution *types.TriggerExecution, startedAt time.Time, a *types.App, triggerID string, emails []string, cause error) error {
	log.Error().
		Err(cause).
		Str("app_id", a.ID).
		Str("trigger_id", triggerID).
		Msg("failed to create spec task from cron trigger")

	if err := notifier.Notify(ctx, &types.Notification{
		Event:   types.EventCronTriggerFailed,
		Message: cause.Error(),
		Emails:  emails,
	}); err != nil {
		log.Error().
			Err(err).
			Str("app_id", a.ID).
			Str("trigger_id", triggerID).
			Msg("failed to send failure notification for spec task")
	}

	execution.Status = types.TriggerExecutionStatusError
	execution.Error = cause.Error()
	execution.DurationMs = time.Since(startedAt).Milliseconds()

	if _, err := str.UpdateTriggerExecution(ctx, execution); err != nil {
		log.Error().
			Err(err).
			Str("execution_id", execution.ID).
			Msg("failed to update execution")
	}

	return cause
}
