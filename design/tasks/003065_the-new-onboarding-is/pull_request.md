# Simplify onboarding with an operator-configured Helix model

## Summary
Add system settings for an onboarding Helix provider, model, and reasoning effort, then apply that configuration automatically for new users. The onboarding flow now presents the recommended model instead of requiring users to understand provider and model selection before creating their first project.

Show the organization's available Helix credit balance with a brief explanation of what credits cover. Clarify that users can alternatively connect Claude Code or Codex to use their existing subscriptions and that those runs do not consume Helix credits.

## Testing
- TypeScript compilation passed.
- All 11 focused onboarding tests passed.
- The backend server package compiled successfully.
- Updated system-settings persistence tests and generated OpenAPI schemas/client types.
