# Implementation Tasks

- [~] Change `Notifications.AppURL` default in `api/pkg/config/config.go` from `https://app.helix.ml` to empty
- [ ] In `LoadServerConfig()` in `api/pkg/config/config.go`, after `envconfig.Process`, fall back `cfg.Notifications.AppURL` to `cfg.WebServer.URL` when empty
- [ ] Build: `cd api && CGO_ENABLED=0 go build ./...`
- [ ] Run focused unit tests: `cd api && CGO_ENABLED=1 go test ./pkg/trigger/slack/... ./pkg/notification/... -count=1`
- [ ] Push code branch and write PR description
