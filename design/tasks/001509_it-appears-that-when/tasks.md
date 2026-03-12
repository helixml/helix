# Implementation Tasks

## Fix 1: Re-key progress tracking by session ID

- [ ] In `api/pkg/hydra/devcontainer.go`: change `goldenCopyProgress` map key from project ID to session ID
- [ ] In `api/pkg/hydra/devcontainer.go`: update `setGoldenCopyProgress(sessionID, copied, total, done)` to use session ID
- [ ] In `api/pkg/hydra/devcontainer.go`: update `GetGoldenCopyProgress(sessionID)` to look up by session ID
- [ ] In `api/pkg/hydra/devcontainer.go` `buildMounts`: pass session ID (from `req.SessionID`) instead of `req.ProjectID` to `setGoldenCopyProgress`
- [ ] In `api/pkg/hydra/server.go`: change `handleGetGoldenCopyProgress` route and handler to accept session ID instead of project ID
- [ ] In `api/pkg/hydra/client.go`: update `RevDialClient.GetGoldenCopyProgress` to accept session ID, update URL path
- [ ] In `api/pkg/external-agent/hydra_executor.go` `StartDesktop`: pass `agent.SessionID` instead of `agent.ProjectID` to `GetGoldenCopyProgress`

## Fix 2: Eliminate the 100% → 0% progress race

- [ ] In `api/pkg/hydra/golden.go` `SetupGoldenCopy`: remove the initial `onProgress(0, goldenSize)` call (line ~253) — let the first real `du -sb` tick be the first report

## Verification

- [ ] `go build ./api/pkg/hydra/ ./api/pkg/external-agent/` compiles cleanly
- [ ] Start two sessions for the same project in parallel — each shows independent progress
- [ ] Start a single session — progress starts near 0 and increases monotonically (no 100% flash)
- [ ] Progress message clears after copy completes