package trigger

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/notification"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/trigger/azure"
	"github.com/helixml/helix/api/pkg/trigger/crisp"
	"github.com/helixml/helix/api/pkg/trigger/cron"
	"github.com/helixml/helix/api/pkg/trigger/discord"
	"github.com/helixml/helix/api/pkg/trigger/notion"
	"github.com/helixml/helix/api/pkg/trigger/project"
	"github.com/helixml/helix/api/pkg/trigger/slack"
	"github.com/helixml/helix/api/pkg/trigger/teams"
	"github.com/helixml/helix/api/pkg/types"

	"github.com/rs/zerolog/log"
)

type Manager struct {
	cfg                  *config.ServerConfig
	store                store.Store
	controller           *controller.Controller
	notifier             notification.Notifier
	specTaskCreator      cron.SpecTaskCreator
	externalAgentStarter cron.ExternalAgentStarter
	azureDevOps          *azure.AzureDevOps
	teams                *teams.Teams
	helixCodeReview      *project.HelixCodeReviewTrigger
	notion               *notion.Notion

	wg sync.WaitGroup
}

func NewTriggerManager(cfg *config.ServerConfig, store store.Store, notifier notification.Notifier, controller *controller.Controller) *Manager {
	m := &Manager{
		cfg:             cfg,
		store:           store,
		controller:      controller,
		notifier:        notifier,
		azureDevOps:     azure.New(cfg, store, controller),
		teams:           teams.New(cfg, store, controller),
		helixCodeReview: project.New(cfg, store, controller),
	}
	// Notion gets a partially-constructed handler — the spec-task creator is
	// wired in via SetSpecTaskCreator after the SpecDrivenTaskService exists.
	// Until then, create events return a clear error rather than panicking.
	m.notion = notion.New(cfg, store, nil, nil, nil, &defaultEmbedURLBuilder{cfg: cfg})
	return m
}

// SetSpecTaskCreator sets the spec task creator for cron triggers that use the "spec_task" action.
// This is set after construction because the SpecDrivenTaskService is created later in the init sequence.
func (t *Manager) SetSpecTaskCreator(specTaskCreator cron.SpecTaskCreator) {
	t.specTaskCreator = specTaskCreator
	// Re-wire the Notion handler now that the spec-task creator is available.
	// We also wire idempotency lookup + cancellation via small store adapters
	// so the Notion package stays free of a direct store dependency on those
	// shapes (consistent with the design's external-source generalisation).
	lookup := &notionStoreLookupAdapter{store: t.store}
	canceller := &notionStoreCancellerAdapter{store: t.store}
	t.notion = notion.New(t.cfg, t.store, specTaskCreator, canceller, lookup, &defaultEmbedURLBuilder{cfg: t.cfg})
}

// notionStoreLookupAdapter implements notion.SpecTaskByExternalRefLookup by
// querying the spec-tasks store's JSONB external_trigger_ref column.
type notionStoreLookupAdapter struct{ store store.Store }

func (a *notionStoreLookupAdapter) GetSpecTaskByExternalRef(ctx context.Context, ref *types.ExternalTriggerRef) (*types.SpecTask, error) {
	if ref == nil || ref.Type != types.ExternalTriggerSourceNotion {
		return nil, nil
	}
	var p types.NotionTriggerPayload
	if err := json.Unmarshal(ref.Payload, &p); err != nil || p.PageID == "" {
		return nil, nil
	}
	return a.store.GetSpecTaskByExternalNotionPageID(ctx, p.PageID)
}

// notionStoreCancellerAdapter implements notion.SpecTaskCanceller by
// transitioning the spec task to the cancelled status. Doesn't tear down any
// running agent process — for the MVP "cancel before work starts" is the
// supported case; ripping out an in-flight agent run is a v2 concern.
type notionStoreCancellerAdapter struct{ store store.Store }

func (a *notionStoreCancellerAdapter) CancelTaskByExternalRef(ctx context.Context, ref *types.ExternalTriggerRef) (*types.SpecTask, error) {
	if ref == nil || ref.Type != types.ExternalTriggerSourceNotion {
		return nil, nil
	}
	var p types.NotionTriggerPayload
	if err := json.Unmarshal(ref.Payload, &p); err != nil || p.PageID == "" {
		return nil, nil
	}
	task, err := a.store.GetSpecTaskByExternalNotionPageID(ctx, p.PageID)
	if err != nil || task == nil {
		return nil, err
	}
	// Transition from any non-terminal status to cancelled. If the task is
	// already terminal the transition returns false and we no-op.
	_, _ = a.store.TransitionSpecTaskStatus(ctx, task.ID,
		[]types.SpecTaskStatus{
			types.TaskStatusBacklog,
			types.TaskStatusSpecGeneration,
			types.TaskStatusSpecReview,
			types.TaskStatusImplementation,
			types.TaskStatusImplementationQueued,
			types.TaskStatusQueuedImplementation,
			types.TaskStatusImplementationReview,
			types.TaskStatusPullRequest,
		},
		types.SpecTaskStatusCancelled,
		nil,
	)
	return task, nil
}

// defaultEmbedURLBuilder produces the embed URL pasted into Notion pages by
// the OnExternalCreate hook. URL pattern matches the EmbedTaskPage route in
// the frontend (see frontend/src/router.tsx).
type defaultEmbedURLBuilder struct {
	cfg *config.ServerConfig
}

func (b *defaultEmbedURLBuilder) BuildEmbedURL(spectask *types.SpecTask, accessToken string) string {
	base := b.cfg.WebServer.URL
	if base == "" {
		return ""
	}
	url := fmt.Sprintf("%s/embed/task/%s", base, spectask.ID)
	if accessToken != "" {
		url += "?access_token=" + accessToken
	}
	return url
}

// Notion returns the Notion trigger handler for direct invocation by the
// spec-task service's completion / cancellation hooks (not currently wired
// — see TODO in spec_driven_task_service for where this would land).
func (t *Manager) Notion() *notion.Notion {
	return t.notion
}

func (t *Manager) SetExternalAgentStarter(starter cron.ExternalAgentStarter) {
	t.externalAgentStarter = starter
}

func (t *Manager) Start(ctx context.Context) {

	log.Info().Msg("starting Helix triggers")

	if t.cfg.Triggers.Discord.Enabled && t.cfg.Triggers.Discord.BotToken != "" {
		t.wg.Add(1)
		go func() {
			defer t.wg.Done()
			t.runDiscord(ctx)
		}()
	}

	t.wg.Add(1)
	go func() {
		defer t.wg.Done()
		t.runCron(ctx)
	}()

	if t.cfg.Triggers.Slack.Enabled {
		t.wg.Add(1)
		go func() {
			defer t.wg.Done()
			t.runSlack(ctx)
		}()
	}

	if t.cfg.Triggers.Crisp.Enabled {
		t.wg.Add(1)
		go func() {
			defer t.wg.Done()
			t.runCrisp(ctx)
		}()
	}

	if t.cfg.Triggers.Teams.Enabled {
		t.wg.Add(1)
		go func() {
			defer t.wg.Done()
			t.runTeams(ctx)
		}()
	}

	// Subscribe to spec-task updates so externally-triggered tasks (Notion,
	// Sentry-future, GitHub-future) can write progress back into their
	// originating system as the task moves through phases.
	t.wg.Add(1)
	go func() {
		defer t.wg.Done()
		t.runExternalProgressUpdates(ctx)
	}()

	t.wg.Wait()
}

// runExternalProgressUpdates subscribes to spec-task updates and dispatches
// progress writebacks to each task's external source (Notion etc.).
func (t *Manager) runExternalProgressUpdates(ctx context.Context) {
	// Track last-seen status per spec task so we only write back when status
	// actually changes (not on every save — the pub/sub fires on every
	// UpdateSpecTask).
	lastStatus := map[string]types.SpecTaskStatus{}

	sub, err := t.store.SubscribeForTasks(ctx, nil, func(task *types.SpecTask) error {
		if task == nil || task.ExternalTriggerRef == nil {
			return nil
		}
		if task.ExternalTriggerRef.Type != types.ExternalTriggerSourceNotion {
			// Future: dispatch to Sentry / GitHub.
			return nil
		}
		prev, ok := lastStatus[task.ID]
		if ok && prev == task.Status {
			return nil
		}
		lastStatus[task.ID] = task.Status

		cfg, err := t.store.GetTriggerConfiguration(ctx, &store.GetTriggerConfigurationQuery{ID: task.ExternalTriggerRef.TriggerConfigID})
		if err != nil {
			log.Warn().Err(err).Str("trigger_config_id", task.ExternalTriggerRef.TriggerConfigID).Msg("trigger: load config for external progress writeback")
			return nil
		}
		if err := t.notion.OnSpecTaskStatusChanged(ctx, cfg, task); err != nil {
			log.Warn().Err(err).Str("spec_task_id", task.ID).Msg("notion: status writeback failed")
		}
		return nil
	})
	if err != nil {
		log.Error().Err(err).Msg("trigger: subscribe for external progress updates failed")
		return
	}
	defer sub.Unsubscribe()
	<-ctx.Done()
}

func (t *Manager) ProcessWebhook(ctx context.Context, triggerConfig *types.TriggerConfiguration, headers http.Header, payload []byte) error {
	switch {
	case triggerConfig.Trigger.AzureDevOps != nil:
		return t.azureDevOps.ProcessWebhook(ctx, triggerConfig, payload)
	case triggerConfig.Trigger.Notion != nil:
		return t.notion.ProcessWebhook(ctx, triggerConfig, headers, payload)
	default:
		log.Error().Any("trigger_config", triggerConfig).Msg("unknown trigger type")
		return fmt.Errorf("unknown trigger type")
	}
}

func (t *Manager) ProcessGitPushEvent(ctx context.Context, specTask *types.SpecTask, repo *types.GitRepository, commitHash string) error {
	return t.helixCodeReview.ProcessGitPushEvent(ctx, specTask, repo, commitHash)
}

func (t *Manager) runDiscord(ctx context.Context) {
	discordTrigger := discord.New(t.cfg, t.store, t.controller)

	for {
		err := discordTrigger.Start(ctx)
		if err != nil {
			log.Err(err).Msg("failed to start Discord trigger, retrying in 10 seconds")
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(10 * time.Second):
		}
	}
}

func (t *Manager) runCron(ctx context.Context) {
	cronTrigger, err := cron.New(t.cfg, t.store, t.notifier, t.controller, t.specTaskCreator, t.externalAgentStarter)
	if err != nil {
		log.Err(err).Msg("failed to create cron trigger")
		return
	}

	for {
		err := cronTrigger.Start(ctx)
		if err != nil {
			log.Err(err).Msg("failed to start cron trigger, retrying in 10 seconds")
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(10 * time.Second):
		}
	}
}

func (t *Manager) runSlack(ctx context.Context) {
	slackTrigger := slack.New(t.cfg, t.store, t.controller)

	for {
		err := slackTrigger.Start(ctx)
		if err != nil {
			log.Err(err).Msg("failed to start slack trigger, retrying in 10 seconds")
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(10 * time.Second):
		}
	}
}

func (t *Manager) runCrisp(ctx context.Context) {
	crispTrigger := crisp.New(t.cfg, t.store, t.controller)

	for {
		err := crispTrigger.Start(ctx)
		if err != nil {
			log.Err(err).Msg("failed to start crisp trigger, retrying in 10 seconds")
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(10 * time.Second):
		}
	}
}

func (t *Manager) runTeams(ctx context.Context) {
	for {
		err := t.teams.Start(ctx)
		if err != nil {
			log.Err(err).Msg("failed to start teams trigger, retrying in 10 seconds")
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(10 * time.Second):
		}
	}
}

// GetTeamsBot returns a Teams bot by app ID (used by webhook handler)
func (t *Manager) GetTeamsBot(appID string) *teams.TeamsBot {
	return t.teams.GetBot(appID)
}
