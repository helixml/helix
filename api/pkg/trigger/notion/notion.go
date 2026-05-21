package notion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// SpecTaskCreator is the slice of the spec-task service we need to create
// tasks from inbound Notion events. Re-declared (matching cron.SpecTaskCreator)
// to keep this package's deps narrow.
type SpecTaskCreator interface {
	CreateTaskFromPrompt(ctx context.Context, req *types.CreateTaskRequest) (*types.SpecTask, error)
}

// SpecTaskCanceller is the slice of the spec-task service used to cancel an
// in-flight task. Optional — if nil, cancel events are logged and dropped.
type SpecTaskCanceller interface {
	CancelTaskByExternalRef(ctx context.Context, ref *types.ExternalTriggerRef) (*types.SpecTask, error)
}

// SpecTaskByExternalRefLookup finds an existing spectask created by a previous
// "create" event for the given Notion page (idempotency on replay). Optional —
// if nil, dispatch always creates (Notion's Automation should not retry, but
// in-flight Helix outages can produce dupes).
type SpecTaskByExternalRefLookup interface {
	GetSpecTaskByExternalRef(ctx context.Context, ref *types.ExternalTriggerRef) (*types.SpecTask, error)
}

// EmbedURLBuilder produces the embed URL for a freshly-created spectask. The
// trigger configuration carries the per-trigger access token (the trigger
// creator's API key — see design "Embed Path" + auth discussion); the builder
// stitches that into a stable embed URL pattern. Implementation is supplied
// by the trigger Manager so the URL prefix can be derived from server config.
type EmbedURLBuilder interface {
	BuildEmbedURL(spectask *types.SpecTask, accessToken string) string
}

// Notion ingests webhook deliveries from Notion (Database Automations, Button
// properties, and the API webhook subscription) and dispatches them to the
// spec-task service.
type Notion struct {
	cfg             *config.ServerConfig
	store           store.Store
	specTaskCreator SpecTaskCreator
	canceller       SpecTaskCanceller
	lookup          SpecTaskByExternalRefLookup
	embedURLs       EmbedURLBuilder

	// newClient is overridable in tests so we don't talk to api.notion.com.
	newClient func(accessToken string) NotionAPI
}

// NotionAPI is the subset of *Client this package uses. Existence enables
// mocking in tests.
type NotionAPI interface {
	GetPage(ctx context.Context, pageID string) (*Page, error)
	PatchRichTextProperty(ctx context.Context, pageID, propertyName, text string) error
	PatchURLProperty(ctx context.Context, pageID, propertyName, url string) error
	AppendEmbedBlock(ctx context.Context, pageID, embedURL string) (string, error)
	DeleteBlock(ctx context.Context, blockID string) error
}

// New constructs a Notion trigger handler. specTaskCreator may be nil during
// early bring-up (the handler will reject create events with a clear error
// rather than panic).
func New(
	cfg *config.ServerConfig,
	str store.Store,
	specTaskCreator SpecTaskCreator,
	canceller SpecTaskCanceller,
	lookup SpecTaskByExternalRefLookup,
	embedURLs EmbedURLBuilder,
) *Notion {
	return &Notion{
		cfg:             cfg,
		store:           str,
		specTaskCreator: specTaskCreator,
		canceller:       canceller,
		lookup:          lookup,
		embedURLs:       embedURLs,
		newClient:       func(t string) NotionAPI { return NewClient(t) },
	}
}

// ProcessWebhook is the inbound entry point invoked from
// trigger.Manager.ProcessWebhook. It branches on the request's headers to
// pick the correct verification + dispatch path.
func (n *Notion) ProcessWebhook(ctx context.Context, triggerConfig *types.TriggerConfiguration, headers http.Header, payload []byte) error {
	cfg := triggerConfig.Trigger.Notion
	if cfg == nil {
		return errors.New("notion: trigger configuration missing notion config")
	}

	source := headers.Get(HeaderSource)
	switch {
	case source == SourceAutomation || source == SourceButton:
		return n.processAutomationWebhook(ctx, triggerConfig, headers, payload)
	case headers.Get(HeaderNotionSignature) != "":
		// Secondary path (Notion API webhook subscription). Out of scope for
		// the MVP — verify signature and log so we can confirm reachability,
		// but don't dispatch yet.
		if err := VerifyNotionSignature(headers, payload, cfg.VerificationToken); err != nil {
			return err
		}
		log.Info().
			Str("trigger_config_id", triggerConfig.ID).
			Msg("notion: secondary-path webhook received (not yet dispatched — see findings.md)")
		return nil
	default:
		return fmt.Errorf("notion: unrecognised webhook — missing %s header and no Notion signature", HeaderSource)
	}
}

func (n *Notion) processAutomationWebhook(ctx context.Context, triggerConfig *types.TriggerConfiguration, headers http.Header, payload []byte) error {
	cfg := triggerConfig.Trigger.Notion

	if err := VerifySharedSecret(headers, cfg.SharedSecret); err != nil {
		return err
	}

	action := headers.Get(HeaderAction)
	ev, pageID, err := ParseAutomationEvent(payload)
	if err != nil {
		return fmt.Errorf("notion: parse webhook body: %w", err)
	}
	if pageID == "" {
		return errors.New("notion: webhook body missing data.id (page ID)")
	}

	switch action {
	case ActionCreate:
		return n.OnExternalCreate(ctx, triggerConfig, ev, pageID)
	case ActionCancel:
		return n.OnExternalCancel(ctx, triggerConfig, pageID)
	default:
		return fmt.Errorf("notion: unrecognised %s header value %q", HeaderAction, action)
	}
}

// OnExternalCreate is the lifecycle hook fired when Notion tells us a row has
// flipped to the "create" state. Idempotent on replay — if a spectask already
// exists for the page, returns the existing one and skips creation.
//
// Generically named to match the future SpecTaskSource interface (see design
// "Generalisation").
func (n *Notion) OnExternalCreate(ctx context.Context, triggerConfig *types.TriggerConfiguration, ev *AutomationEvent, pageID string) error {
	if n.specTaskCreator == nil {
		return errors.New("notion: spec task creator not wired up")
	}

	cfg := triggerConfig.Trigger.Notion

	ref := buildExternalTriggerRef(triggerConfig.ID, pageID, ev)

	if n.lookup != nil {
		existing, err := n.lookup.GetSpecTaskByExternalRef(ctx, ref)
		if err == nil && existing != nil {
			log.Debug().
				Str("page_id", pageID).
				Str("spec_task_id", existing.ID).
				Msg("notion: create event ignored — spectask already exists for this page")
			return nil
		}
	}

	prompt := extractPrompt(ev, cfg.ColumnMapping.PromptColumn)
	name := extractTitle(ev)
	if prompt == "" {
		// Fall back to the row's Name when the Prompt column is empty (or
		// not configured). A single-column UX where the user just types
		// "make me a fish" into the title and flips Go is the most natural
		// interaction and we should honour it. Logged as info so operators
		// can observe how often the fallback path runs.
		prompt = name
		log.Info().
			Str("page_id", pageID).
			Msg("notion: prompt column empty, using row name as prompt")
	}
	if prompt == "" {
		// Both empty — genuinely nothing to send the agent.
		return fmt.Errorf("notion: page %s has neither a prompt nor a name", pageID)
	}

	if name == "" {
		name = "Notion: " + pageID
	}

	spectask, err := n.specTaskCreator.CreateTaskFromPrompt(ctx, &types.CreateTaskRequest{
		ProjectID: cfg.TargetProjectID,
		Prompt:    prompt,
		UserID:    triggerConfig.Owner,
		AppID:     triggerConfig.AppID,
		Type:      "feature",
		Priority:  types.SpecTaskPriorityMedium,
	})
	if err != nil {
		return fmt.Errorf("notion: create spectask: %w", err)
	}

	// Persist the external ref on the spectask so completion / cancellation
	// can find their way back here. Caller persists the row; we mutate.
	spectask.ExternalTriggerRef = ref
	if err := n.store.UpdateSpecTask(ctx, spectask); err != nil {
		log.Warn().Err(err).Str("spec_task_id", spectask.ID).Msg("notion: persist external trigger ref")
	}

	// Best-effort: write the spec-task URL into the row's URL column so the
	// table immediately shows a clickable link to the live Helix task.
	if err := n.writeHelixTaskURL(ctx, triggerConfig, spectask, pageID); err != nil {
		log.Warn().Err(err).
			Str("spec_task_id", spectask.ID).
			Str("page_id", pageID).
			Msg("notion: write helix task URL column failed (spectask still created)")
	}

	// Best-effort: write an initial status into the Result column so the user
	// sees Helix has acknowledged the row even before the agent finishes.
	if cfg.ColumnMapping.ResultColumn != "" {
		initial := "🟡 Helix picked this up — see the task page or the embed below."
		token, terr := n.resolveAccessToken(ctx, cfg)
		if terr == nil {
			if err := n.newClient(token).PatchRichTextProperty(ctx, pageID, cfg.ColumnMapping.ResultColumn, initial); err != nil {
				log.Warn().Err(err).Str("page_id", pageID).Msg("notion: write initial Result failed")
			}
		}
	}

	// Best-effort: append an embed block to the row's page body so the user
	// sees the live Helix UI inline. Failure here doesn't block the spectask.
	if err := n.appendEmbedBlock(ctx, triggerConfig, spectask, pageID); err != nil {
		log.Warn().Err(err).
			Str("spec_task_id", spectask.ID).
			Str("page_id", pageID).
			Msg("notion: append embed block failed (spectask still created)")
	}

	return nil
}

// OnExternalCancel cancels an in-flight spectask identified by the originating
// page ID, and best-effort removes the embed block from the page body.
func (n *Notion) OnExternalCancel(ctx context.Context, triggerConfig *types.TriggerConfiguration, pageID string) error {
	if n.lookup == nil {
		log.Debug().Msg("notion: cancel ignored — spec-task lookup not wired up")
		return nil
	}
	ref := &types.ExternalTriggerRef{
		Type:            types.ExternalTriggerSourceNotion,
		TriggerConfigID: triggerConfig.ID,
		Payload:         marshalNotionPayload(&types.NotionTriggerPayload{PageID: pageID}),
	}
	existing, err := n.lookup.GetSpecTaskByExternalRef(ctx, ref)
	if err != nil || existing == nil {
		// No live spectask for this page — idempotent no-op.
		return nil
	}
	if n.canceller != nil {
		if _, err := n.canceller.CancelTaskByExternalRef(ctx, existing.ExternalTriggerRef); err != nil {
			return fmt.Errorf("notion: cancel spectask %s: %w", existing.ID, err)
		}
	}
	// Best-effort embed-block removal.
	n.deleteEmbedBlock(ctx, triggerConfig, existing)
	return nil
}

// statusEmoji returns a single-character indicator for a SpecTaskStatus so
// the Result column reads like a progress spinner from the user's table.
func statusEmoji(s string) string {
	switch s {
	case "backlog", "":
		return "⚪"
	case "planning", "spec_generation":
		return "🟡"
	case "spec_review":
		return "🟠"
	case "implementation", "implementation_queued", "queued_implementation", "implementation_review":
		return "🔵"
	case "pull_request":
		return "🟣"
	case "done":
		return "✅"
	case "cancelled", "failed":
		return "❌"
	default:
		return "🔄"
	}
}

// OnSpecTaskStatusChanged is invoked when a Notion-originated spec task's
// status transitions. Writes a progress indicator into the Result column so
// the Notion table behaves like a status board. Cheaper, more frequent
// version of OnSpecTaskCompleted — fires for every transition.
func (n *Notion) OnSpecTaskStatusChanged(ctx context.Context, triggerConfig *types.TriggerConfiguration, spectask *types.SpecTask) error {
	cfg := triggerConfig.Trigger.Notion
	if cfg == nil || cfg.ColumnMapping.ResultColumn == "" {
		return nil
	}
	pageID := pageIDFromRef(spectask.ExternalTriggerRef)
	if pageID == "" {
		return nil
	}
	token, err := n.resolveAccessToken(ctx, cfg)
	if err != nil {
		return fmt.Errorf("notion: %w", err)
	}
	emoji := statusEmoji(string(spectask.Status))
	msg := fmt.Sprintf("%s %s — see the task page or the embed below.", emoji, spectask.Status)
	return n.newClient(token).PatchRichTextProperty(ctx, pageID, cfg.ColumnMapping.ResultColumn, msg)
}

// OnSpecTaskCompleted is invoked by the spec-task service when a Notion-
// triggered task finishes (success or failure). Writes the result into the
// configured Result column if any. Never touches the action column.
func (n *Notion) OnSpecTaskCompleted(ctx context.Context, triggerConfig *types.TriggerConfiguration, spectask *types.SpecTask, summary string) error {
	cfg := triggerConfig.Trigger.Notion
	if cfg == nil || cfg.ColumnMapping.ResultColumn == "" {
		return nil
	}
	pageID := pageIDFromRef(spectask.ExternalTriggerRef)
	if pageID == "" {
		return errors.New("notion: spectask has no notion page id in external_trigger_ref")
	}

	token, err := n.resolveAccessToken(ctx, cfg)
	if err != nil {
		return fmt.Errorf("notion: %w", err)
	}
	if err := n.newClient(token).PatchRichTextProperty(ctx, pageID, cfg.ColumnMapping.ResultColumn, summary); err != nil {
		return fmt.Errorf("notion: write result column: %w", err)
	}
	return nil
}

// OnSpecTaskCancelled is invoked when a Notion-triggered task is cancelled
// from inside Helix (rather than by the user flipping the column in Notion).
// Removes the embed block; the action column is left alone — the user owns
// it.
func (n *Notion) OnSpecTaskCancelled(ctx context.Context, triggerConfig *types.TriggerConfiguration, spectask *types.SpecTask) error {
	n.deleteEmbedBlock(ctx, triggerConfig, spectask)
	return nil
}

// --- helpers ---

// resolveAccessToken returns the bearer token to use for Notion API calls.
// Prefers a direct integration token (the simpler internal-integration
// setup); falls back to looking up an OAuthConnection by ID. Returns a clear
// error if neither is configured.
func (n *Notion) resolveAccessToken(ctx context.Context, cfg *types.NotionTrigger) (string, error) {
	if cfg.IntegrationToken != "" {
		return cfg.IntegrationToken, nil
	}
	if cfg.OAuthConnectionID == "" {
		return "", errors.New("notion: trigger config has neither integration_token nor oauth_connection_id")
	}
	connection, err := n.store.GetOAuthConnection(ctx, cfg.OAuthConnectionID)
	if err != nil {
		return "", fmt.Errorf("get oauth connection %s: %w", cfg.OAuthConnectionID, err)
	}
	return connection.AccessToken, nil
}

// publicBaseURL returns the URL to use as the host part of any link Helix
// writes back into Notion. Prefers the trigger's PublicURL (set when the
// deployment URL isn't reachable from Notion — e.g. localhost in dev), falls
// back to the server's default WebServer.URL via the embed builder.
func (n *Notion) publicBaseURL(cfg *types.NotionTrigger, spectask *types.SpecTask) string {
	if cfg.PublicURL != "" {
		return strings.TrimRight(cfg.PublicURL, "/")
	}
	if n.embedURLs == nil {
		return ""
	}
	// Defer to the embed URL builder and strip back to the base.
	probe := n.embedURLs.BuildEmbedURL(spectask, "")
	probe = strings.TrimSuffix(probe, "?access_token=")
	if idx := strings.Index(probe, "/embed/task/"); idx > 0 {
		return probe[:idx]
	}
	return ""
}

// embedURL constructs the URL we want Notion to render inside the inline
// embed block (the live Helix task page).
func (n *Notion) embedURL(cfg *types.NotionTrigger, spectask *types.SpecTask) string {
	base := n.publicBaseURL(cfg, spectask)
	if base == "" {
		return ""
	}
	url := fmt.Sprintf("%s/embed/task/%s", base, spectask.ID)
	if cfg.EmbedAccessToken != "" {
		url += "?access_token=" + cfg.EmbedAccessToken
	}
	return url
}

// taskPageURL constructs the URL that opens the Helix task in a new browser
// tab (the row's clickable "Helix Task" link, not embedded).
func (n *Notion) taskPageURL(cfg *types.NotionTrigger, spectask *types.SpecTask) string {
	base := n.publicBaseURL(cfg, spectask)
	if base == "" {
		return ""
	}
	return fmt.Sprintf("%s/task/%s", base, spectask.ID)
}

// writeHelixTaskURL writes the spec-task URL into the configured URL column
// on the source Notion row so users can click straight from their table to
// the live Helix task. No-op if no column is configured.
func (n *Notion) writeHelixTaskURL(ctx context.Context, triggerConfig *types.TriggerConfiguration, spectask *types.SpecTask, pageID string) error {
	cfg := triggerConfig.Trigger.Notion
	if cfg.ColumnMapping.HelixTaskURLColumn == "" {
		return nil
	}
	taskURL := n.taskPageURL(cfg, spectask)
	if taskURL == "" {
		return errors.New("notion: cannot construct helix task URL (no PublicURL or WebServer.URL set)")
	}
	token, err := n.resolveAccessToken(ctx, cfg)
	if err != nil {
		return err
	}
	return n.newClient(token).PatchURLProperty(ctx, pageID, cfg.ColumnMapping.HelixTaskURLColumn, taskURL)
}

func (n *Notion) appendEmbedBlock(ctx context.Context, triggerConfig *types.TriggerConfiguration, spectask *types.SpecTask, pageID string) error {
	cfg := triggerConfig.Trigger.Notion
	token, err := n.resolveAccessToken(ctx, cfg)
	if err != nil {
		return err
	}

	url := n.embedURL(cfg, spectask)
	if url == "" {
		return errors.New("notion: cannot construct embed URL (no PublicURL or WebServer.URL set)")
	}

	client := n.newClient(token)
	blockID, err := client.AppendEmbedBlock(ctx, pageID, url)
	if err != nil {
		return err
	}

	// Re-marshal the payload with the block id so cancellation can clean up.
	if spectask.ExternalTriggerRef != nil {
		var p types.NotionTriggerPayload
		_ = json.Unmarshal(spectask.ExternalTriggerRef.Payload, &p)
		p.EmbedBlockID = blockID
		spectask.ExternalTriggerRef.Payload = marshalNotionPayload(&p)
		if err := n.store.UpdateSpecTask(ctx, spectask); err != nil {
			log.Warn().Err(err).Str("spec_task_id", spectask.ID).Msg("notion: persist embed block id")
		}
	}
	return nil
}

func (n *Notion) deleteEmbedBlock(ctx context.Context, triggerConfig *types.TriggerConfiguration, spectask *types.SpecTask) {
	if spectask == nil || spectask.ExternalTriggerRef == nil {
		return
	}
	var p types.NotionTriggerPayload
	if err := json.Unmarshal(spectask.ExternalTriggerRef.Payload, &p); err != nil || p.EmbedBlockID == "" {
		return
	}
	cfg := triggerConfig.Trigger.Notion
	if cfg == nil {
		return
	}
	token, err := n.resolveAccessToken(ctx, cfg)
	if err != nil {
		log.Warn().Err(err).Msg("notion: resolve access token for embed cleanup")
		return
	}
	if err := n.newClient(token).DeleteBlock(ctx, p.EmbedBlockID); err != nil {
		log.Warn().Err(err).
			Str("block_id", p.EmbedBlockID).
			Str("spec_task_id", spectask.ID).
			Msg("notion: delete embed block failed (continuing)")
	}
}

// buildExternalTriggerRef constructs the ref for a freshly-fired create event.
// EmbedBlockID is empty here; appendEmbedBlock fills it in once the block exists.
func buildExternalTriggerRef(triggerConfigID, pageID string, ev *AutomationEvent) *types.ExternalTriggerRef {
	return &types.ExternalTriggerRef{
		Type:            types.ExternalTriggerSourceNotion,
		TriggerConfigID: triggerConfigID,
		Payload: marshalNotionPayload(&types.NotionTriggerPayload{
			PageID:     pageID,
			DatabaseID: DatabaseIDFromParent(ev.Data.Parent),
		}),
	}
}

func marshalNotionPayload(p *types.NotionTriggerPayload) json.RawMessage {
	bts, _ := json.Marshal(p)
	return bts
}

func pageIDFromRef(ref *types.ExternalTriggerRef) string {
	if ref == nil {
		return ""
	}
	var p types.NotionTriggerPayload
	if err := json.Unmarshal(ref.Payload, &p); err != nil {
		return ""
	}
	return p.PageID
}

// extractPrompt pulls a rich-text property value out of the webhook payload.
// Returns empty string if the column isn't configured or doesn't appear in
// the payload (Notion only sends fields the user picked).
func extractPrompt(ev *AutomationEvent, promptColumn string) string {
	if promptColumn == "" {
		return ""
	}
	raw, ok := ev.Data.Properties[promptColumn]
	if !ok {
		return ""
	}
	return readRichText(raw)
}

func extractTitle(ev *AutomationEvent) string {
	for _, raw := range ev.Data.Properties {
		var probe struct {
			Type  string `json:"type"`
			Title []struct {
				PlainText string `json:"plain_text"`
			} `json:"title"`
		}
		if err := json.Unmarshal(raw, &probe); err != nil || probe.Type != "title" {
			continue
		}
		var sb strings.Builder
		for _, t := range probe.Title {
			sb.WriteString(t.PlainText)
		}
		return sb.String()
	}
	return ""
}

func readRichText(raw json.RawMessage) string {
	var probe struct {
		Type     string `json:"type"`
		RichText []struct {
			PlainText string `json:"plain_text"`
		} `json:"rich_text"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil || probe.Type != "rich_text" {
		return ""
	}
	var sb strings.Builder
	for _, t := range probe.RichText {
		sb.WriteString(t.PlainText)
	}
	return sb.String()
}
