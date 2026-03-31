# Implementation Tasks

- [ ] In `api/pkg/trigger/slack/slack_project_updates.go`, replace all uses of `s.cfg.Notifications.AppURL` with `s.cfg.WebServer.URL` as the base URL passed to `buildTaskLink` and `buildProjectLinkWithOrg`
- [ ] Remove `AppURL` field from `Notifications` struct in `api/pkg/config/config.go` (or change its default to `""` and fall back to `WebServer.URL` at runtime if backward-compat is needed)
- [ ] Update any Slack trigger tests that assert URLs contain `https://app.helix.ml` to use the test server's `SERVER_URL` value instead
- [ ] Build and run: `go build ./...` and relevant tests in `api/pkg/trigger/slack/`
- [ ] Deploy to meta.helix.ml and verify that the Optimus agent produces correct spectask links in Slack
