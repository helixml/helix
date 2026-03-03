# Requirements: Hide "Connect AI Provider" Step When Provider Exists

## User Story

As a user onboarding to a Helix system that already has AI providers configured (e.g., system-wide/global providers set up by an admin), I should not see the "Connect an AI provider" step because it's unnecessary - I can already use the system's AI features.

## Problem

Currently, the onboarding flow shows 5 steps including "Connect an AI provider" (step 2). When system providers already exist with enabled models, the step is auto-completed and skipped, but it still appears in the step list. This is confusing because:

1. Users see a step they didn't complete manually (shows as completed)
2. The step title suggests they need to connect something when they don't
3. It clutters the onboarding experience unnecessarily

## Acceptance Criteria

1. **Hide provider step when usable providers exist**: When there are enabled chat models from providers, the "Connect an AI provider" step should be completely hidden from the UI (not just skipped)

2. **Show provider step when no usable providers exist**: When no providers with enabled chat models exist, show the step as normal so users can add their API keys

3. **Step numbering adjusts**: When the provider step is hidden, remaining steps should renumber correctly (e.g., "Create your first project" becomes step 2 instead of step 3)

4. **Completion logic intact**: The total steps required for completion should adjust (4 steps instead of 5 when provider step is hidden)

5. **No functional regression**: All other onboarding behavior remains unchanged

## Technical Note: Helix Models & Runners

The backend already handles the runner availability check for Helix models. In `provider_manager.go`, the `ListProviders` function skips the Helix provider entirely if no runners are connected:

```go
// Skip the Helix provider if there are no runners
if provider == types.ProviderHelix && m.runnerController != nil {
    runnerIDs := m.runnerController.RunnerIDs()
    if len(runnerIDs) == 0 {
        continue
    }
}
```

This means the frontend does **not** need to check for runners separately - if Helix models appear in the providers list, runners are already connected. The existing `hasAnyEnabledModels` check is sufficient.