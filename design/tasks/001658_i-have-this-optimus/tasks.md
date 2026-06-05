# Implementation Tasks

- [x] Change `Notifications.AppURL` default in `api/pkg/config/config.go` from `https://app.helix.ml` to empty
- [x] In `LoadServerConfig()` in `api/pkg/config/config.go`, after `envconfig.Process`, fall back `cfg.Notifications.AppURL` to `cfg.WebServer.URL` when empty
- [x] Build affected packages (`config`, `trigger/slack`, `notification`, `services`) — pass
- [x] Run focused unit tests (`trigger/slack`, `notification`, `config`) — pass
- [x] Merge `origin/main`, commit code, push `feature/001658-i-have-this-optimus`, write PR description
