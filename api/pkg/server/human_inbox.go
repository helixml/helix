package server

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// humanInbox delivers ask_human messages to a person's in-app inbox by
// writing an attention-event (the notification bell) for their linked Helix
// user. Satisfies mcptools.HumanInbox. It is the composition-root adapter that
// keeps the org MCP tools decoupled from the main Helix attention service.
type humanInbox struct {
	store store.Store
}

func (h humanInbox) Notify(ctx context.Context, orgID, userID, fromBot, personName, message string) error {
	if userID == "" {
		return fmt.Errorf("person has no linked Helix user")
	}
	id := system.GenerateAttentionEventID()
	meta, _ := json.Marshal(map[string]string{"bot_id": fromBot, "person": personName})
	ev := &types.AttentionEvent{
		ID:             id,
		UserID:         userID,
		OrganizationID: orgID,
		EventType:      types.AttentionEventOrgMessage,
		Title:          fmt.Sprintf("Message from %s", fromBot),
		Description:    message,
		CreatedAt:      time.Now(),
		// Unique key per message so every ask_human is a distinct inbox item
		// (CreateAttentionEvent dedupes on idempotency_key).
		IdempotencyKey: id,
		Metadata:       meta,
	}
	if _, err := h.store.CreateAttentionEvent(ctx, ev); err != nil {
		return fmt.Errorf("create attention event: %w", err)
	}
	return nil
}

// NotifyInfo writes an informational, no-reply message to a person's inbox (the
// notification bell) — e.g. "the Chief of Staff is starting up". Unlike Notify
// (an ask_human that expects a reply), the metadata carries no_reply so the UI
// renders it as a read-only status, not a message to answer. Best-effort at the
// call site: a failure must not block whatever it hangs off (e.g. org create).
func (h humanInbox) NotifyInfo(ctx context.Context, orgID, userID, fromBot, title, message string) error {
	if userID == "" {
		return fmt.Errorf("person has no linked Helix user")
	}
	id := system.GenerateAttentionEventID()
	meta, _ := json.Marshal(map[string]string{"bot_id": fromBot, "no_reply": "true"})
	ev := &types.AttentionEvent{
		ID:             id,
		UserID:         userID,
		OrganizationID: orgID,
		EventType:      types.AttentionEventOrgMessage,
		Title:          title,
		Description:    message,
		CreatedAt:      time.Now(),
		IdempotencyKey: id,
		Metadata:       meta,
	}
	if _, err := h.store.CreateAttentionEvent(ctx, ev); err != nil {
		return fmt.Errorf("create attention event: %w", err)
	}
	return nil
}
