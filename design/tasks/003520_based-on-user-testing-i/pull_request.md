# Notify Slack about Stripe trial and credit events

## Summary
Send separate Slack notifications when a Stripe customer starts a free trial, converts that trial to a paid subscription, or adds credits. Billing notifications use a dedicated `HELIX_SUBSCRIPTIONS_SLACK_WEBHOOK_URL` for the `#helix-subscriptions` channel, identify organization wallets by organization name and initiating user, include the purchased credit amount, and avoid duplicate top-up notifications from retried or paired Stripe webhook events. Duplicate detection uses the wallet transaction result, and Slack requests have a five-second timeout so billing webhooks cannot hang indefinitely.

## Testing
- `go test ./pkg/stripe ./pkg/janitor` — passed
- `docker compose -f docker-compose.dev.yaml config --quiet` — passed
