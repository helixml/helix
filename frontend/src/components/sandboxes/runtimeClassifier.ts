// Centralised runtime classification. There are now several non-desktop
// runtimes (`headless-ubuntu`, `node22`, `python313`, …) and only one desktop
// runtime (`ubuntu-desktop`). The previous heuristic of "anything not
// containing 'headless' has a desktop" silently put node22/python313 into the
// desktop bucket and tried to stream a non-existent display.
//
// Source of truth: a sandbox renders a desktop ONLY when its runtime is
// exactly `ubuntu-desktop`. Anything else — known or unknown — is headless.
// Keep this in sync with `SandboxRuntimeUbuntuDesktop` in api/pkg/types/sandbox.go.

import { TypesSandbox } from '../../api/api'

export const SANDBOX_DESKTOP_RUNTIME = 'ubuntu-desktop'

// hasDesktop returns true only for runtimes that actually expose a streaming
// display + desktop-bridge. Used to gate the Desktop tab and the card preview.
export const hasDesktop = (runtime?: string | null): boolean =>
  runtime === SANDBOX_DESKTOP_RUNTIME

export const isHeadless = (sandbox: TypesSandbox | { runtime?: string | null }): boolean =>
  !hasDesktop(sandbox?.runtime ?? undefined)
