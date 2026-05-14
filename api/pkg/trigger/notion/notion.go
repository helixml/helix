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
	if prompt == "" {
		// Fallback would be to GET the page and read body blocks. For the
		// MVP if no prompt column was configured (or it's empty) we error
		// loudly — the user can re-fire after filling it in.
		return fmt.Errorf("notion: page %s has no prompt (configure prompt_column or set the column on the row)", pageID)
	}

	name := extractTitle(ev)
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

	connection, err := n.store.GetOAuthConnection(ctx, cfg.OAuthConnectionID)
	if err != nil {
		return fmt.Errorf("notion: get oauth connection: %w", err)
	}
	client := n.newClient(connection.AccessToken)
	if err := client.PatchRichTextProperty(ctx, pageID, cfg.ColumnMapping.ResultColumn, summary); err != nil {
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

func (n *Notion) appendEmbedBlock(ctx context.Context, triggerConfig *types.TriggerConfiguration, spectask *types.SpecTask, pageID string) error {
	cfg := triggerConfig.Trigger.Notion
	if cfg.OAuthConnectionID == "" {
		return errors.New("notion: oauth connection id missing — embed block not inserted")
	}
	connection, err := n.store.GetOAuthConnection(ctx, cfg.OAuthConnectionID)
	if err != nil {
		return fmt.Errorf("get oauth connection: %w", err)
	}

	embedURL := ""
	if n.embedURLs != nil {
		embedURL = n.embedURLs.BuildEmbedURL(spectask, cfg.EmbedAccessToken)
	}
	if embedURL == "" {
		return errors.New("embed url builder returned empty URL")
	}

	client := n.newClient(connection.AccessToken)
	blockID, err := client.AppendEmbedBlock(ctx, pageID, embedURL)
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
	if cfg == nil || cfg.OAuthConnectionID == "" {
		return
	}
	connection, err := n.store.GetOAuthConnection(ctx, cfg.OAuthConnectionID)
	if err != nil {
		log.Warn().Err(err).Msg("notion: get oauth connection for embed cleanup")
		return
	}
	client := n.newClient(connection.AccessToken)
	if err := client.DeleteBlock(ctx, p.EmbedBlockID); err != nil {
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
