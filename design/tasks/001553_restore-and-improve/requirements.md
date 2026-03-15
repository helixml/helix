# Requirements: Restore and Improve Authentication Guidance Message

## Background

PR #1912 (`feature/001552-bug-report-from-user`, commit `8cdc52d37`) regressed the guidance
message shown during the Claude authentication dialog. The message was simplified to:

> "A browser will open below. Sign in to your Claude account and complete the authentication flow in the browser."

This removed actionable guidance, leaving users without clear instruction on what to do — or what *not* to do — in the embedded browser.

## User Stories

**US-1**: As a user starting the Claude auth flow, I need clear instructions so I know to enter my email address and paste the Anthropic code, rather than logging in via Google on the desktop.

**US-2**: As a user who sees a Google login prompt in the embedded browser, I need to understand that using Google on this desktop environment is unnecessary and that the email+code path is preferred, so I am not alarmed.

**US-3**: As a user with low reading tolerance, I need the guidance to be visually prominent enough that I cannot miss it before I start interacting with the browser.

## Acceptance Criteria

- AC-1: The guidance Alert is visible immediately when the embedded browser appears (`isRunning && loginCommandSent`)
- AC-2: The message instructs users to enter their email address
- AC-3: The message explains that a code will be shown and should be pasted
- AC-4: The message reassures users that logging into Google on the desktop is not necessary
- AC-5: The tone is reassuring, not alarming
- AC-6: The message is concise — readable in under 10 seconds
- AC-7: The Alert is visually more prominent than a standard `severity="info"` alert (e.g. larger text, stronger colour, bold key phrase)
- AC-8: No changes to authentication logic, flow steps, or API calls
