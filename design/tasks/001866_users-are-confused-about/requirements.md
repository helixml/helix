# Requirements: Claude Code Token Validation

## Problem

The "Connect Claude Subscription" dialog accepts any text as a setup token — including Anthropic API keys (`sk-ant-api03-...`), random strings, and expired tokens — and shows a green "Connected" status. Users frequently paste their Anthropic API key thinking it's the right credential. The system stores whatever is provided, marks the subscription as "active," and gives no feedback until a session fails at runtime.

## User Stories

### US-1: User pastes an Anthropic API key
**As a** user who has an Anthropic API key,
**I want** to see a clear error explaining this is not a Claude Code setup token,
**so that** I understand the difference and know what to do instead.

**Acceptance Criteria:**
- Pasting `sk-ant-api03-...` shows an inline error: "This looks like an Anthropic API key, not a Claude Code setup token. Run `claude setup-token` in your terminal to generate the correct token."
- The "Connect" button is disabled while the error is showing.
- The token is NOT submitted to the backend.

### US-2: User pastes random or invalid text
**As a** user who pastes random text or an unrelated credential,
**I want** to see a validation error before submission,
**so that** I don't end up with a broken "Connected" status.

**Acceptance Criteria:**
- Pasting text that doesn't match any known Claude Code token pattern shows an error: "This doesn't look like a valid Claude Code setup token. Run `claude setup-token` to generate one."
- The "Connect" button is disabled while the error is showing.
- The token is NOT submitted to the backend.

### US-3: Backend rejects invalid tokens
**As a** system operator,
**I want** the API to validate token format before storing,
**so that** invalid credentials never create "active" subscriptions.

**Acceptance Criteria:**
- `POST /api/v1/claude-subscriptions` with a `setup_token` that matches `sk-ant-api03-*` returns HTTP 400 with a message explaining this is an API key, not a setup token.
- `POST /api/v1/claude-subscriptions` with a `setup_token` that doesn't match any known Claude Code token pattern returns HTTP 400 with "Invalid setup token format."
- Existing OAuth flow is unaffected.

### US-4: Dialog labeling is unambiguous
**As a** user unfamiliar with Claude Code,
**I want** the dialog to clearly say this is a "Claude Code Setup Token" (not just "Your Token"),
**so that** I understand what credential is expected.

**Acceptance Criteria:**
- The text field label reads "Claude Code Setup Token" instead of "Your Token."
- The placeholder reads "Paste your Claude Code setup token here..."
