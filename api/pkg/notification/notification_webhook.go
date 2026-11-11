package notification

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/netguard"
)

type WebhookPayload struct {
	SessionID string `json:"session_id,omitempty"`
	Status    string `json:"status"`
	Output    string `json:"output,omitempty"`
	Event     string `json:"event"`
}

const webhookTimeout = 10 * time.Second

// sendWebhook posts n to callbackURL with client, which must enforce the
// outbound policy (callback URLs are user-supplied).
func sendWebhook(ctx context.Context, client *http.Client, callbackURL string, n *Notification) error {
	status := "success"
	if n.Event == 2 { // EventCronTriggerFailed
		status = "error"
	}

	payload := WebhookPayload{
		Status: status,
		Output: n.Message,
		Event:  n.Event.String(),
	}
	if n.Session != nil {
		payload.SessionID = n.Session.ID
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal webhook payload: %w", err)
	}

	if _, err := netguard.ValidateHTTPURL(callbackURL); err != nil {
		return fmt.Errorf("webhook callback URL: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, callbackURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}

	log.Debug().
		Str("callback_url", callbackURL).
		Str("status", status).
		Msg("webhook notification sent")

	return nil
}
