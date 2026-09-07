# Fix light mode release text and provider icon visibility

## Summary
Use theme-aware foreground, background, and divider colors for the admin release banner and provider cards. Render provider logos through the shared ProviderMark component so current-color icons remain visible in both light and dark modes.

## Testing
Ran the targeted provider icon tests in light and dark themes (7 tests passed), TypeScript compilation, and the production frontend build successfully.
