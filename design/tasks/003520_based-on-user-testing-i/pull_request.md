# Notify Slack about Stripe trial and credit events

## Summary
Send separate Slack notifications when a Stripe customer starts a free trial, converts that trial to a paid subscription, or adds credits. The notifications reuse the existing Janitor Slack webhook, identify the user or organization, include the purchased credit amount, and avoid duplicate top-up notifications from retried or paired Stripe webhook events.

## Testing
- `go test ./pkg/stripe` — passed
- `go test ./cmd/helix` — passed
