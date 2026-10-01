# Notify Slack about Stripe trial and credit events

## Summary
Send separate Slack notifications when a Stripe customer starts a free trial, converts that trial to a paid subscription, or adds credits. Billing notifications use a dedicated `HELIX_SUBSCRIPTIONS_SLACK_WEBHOOK_URL` for the `#helix-subscriptions` channel, identify organization wallets by organization name and initiating user, include the purchased credit amount, and avoid duplicate top-up notifications from retried or paired Stripe webhook events.

## Testing
- `go test ./pkg/janitor ./pkg/stripe ./cmd/helix` — passed
- `docker compose -f docker-compose.dev.yaml config --quiet` — passed
