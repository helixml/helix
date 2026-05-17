# Persist Chrome state and auto-restore on session resume

## Summary

When a Helix desktop session is stopped and resumed, the browser was previously
wiped clean — open tabs gone, history gone, every login starting over. This
change persists the Chrome / Chromium profile on the workspace volume and
auto-relaunches Chrome on resume if it was running when the session ended.

## Why

`/home/retro/work` is the only bind-mounted directory in the desktop
containers (see `api/pkg/sandbox/controller_provision.go`), so anything under
`~/.config/` is ephemeral. The `.claude-state` symlink at
`desktop/shared/helix-workspace-setup.sh:535-567` already uses this trick for
Claude credentials; this PR applies the same pattern to Chrome.

## Changes

- **`desktop/shared/helix-workspace-setup.sh`** — symlink `~/.config/google-chrome`
  and `~/.config/chromium` to `$WORK_DIR/.chrome-state` before Zed/agent launch.
  Seeds first-run sentinel files so the welcome dialog stays suppressed on a
  freshly-created persistent profile. Mirrors the existing `.claude-state` block.

- **`Dockerfile.sway-helix`** and **`Dockerfile.ubuntu-helix`** — flip the
  Chrome and Chromium enterprise policies' `RestoreOnStartup` from `5`
  (New Tab Page) to `1` (restore last session), so the tabs the user had open
  reappear on relaunch. Chromium's policy on Sway previously had no
  `RestoreOnStartup` field at all — added it explicitly.

- **`desktop/sway-config/startup-app.sh`** and
  **`desktop/ubuntu-config/startup-app.sh`** — once the compositor is up,
  check `$WORK_DIR/.chrome-state/.was-running`: if the marker is fresher than
  5 minutes, clear any stale `Singleton*` lock files and launch
  `google-chrome-stable` in the background (workspace 3 via the existing Sway
  assignment rule). A background heartbeat touches the marker every 30 s while
  Chrome is running and removes it when Chrome stops, so a clean close prevents
  auto-launch next time.

- **`desktop/sway-config/SWAY-USER-GUIDE.md`** — short note explaining the new
  behaviour and that Chrome's `--password-store=basic` means any saved
  passwords are stored unencrypted on the workspace volume (same trust
  boundary as `~/.git-credentials`).

## Tested

- `bash -n` on the three modified shell scripts passes.
- The Ubuntu start_gnome heredoc body expands to syntactically valid bash
  (verified by extracting the heredoc, applying unquoted-heredoc expansion
  rules, and parsing the result).

End-to-end verification (open Chrome → restart container → see tabs)
requires `./stack build-ubuntu` (and the sway equivalent) plus a fresh
session, which would terminate the spec-task agent that authored this PR —
reviewer to run those checks. See
`design/tasks/002027_can-we-persist-browser/design.md` for the rationale.

## Notes for reviewers

- arm64 / Chromium uses the same code path because the Dockerfiles already
  symlink `chromium` → `google-chrome-stable`. The `~/.config/chromium`
  symlink covers the profile-path difference between Chrome and Chromium.
- Persistent sandboxes are sticky to one host
  (`controller_provision.go:336-364`), so two processes never race for the
  same profile directory. The `Singleton*` cleanup is purely for recovering
  from a hard container kill on the same host.
- The heartbeat loop is detached from any individual Sway run — Sway crashes
  do not respawn a duplicate loop (gated by `SERVICES_STARTED=true`).
