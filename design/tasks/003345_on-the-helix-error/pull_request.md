# Show agent turn errors inline

## Summary
Replace the generic error message and details popup with an inline error alert that shows the full failure reason immediately. Insufficient-balance failures include an Add credits CTA to the organization's billing page. Recovered historical failures now show their details as quiet inline text, while existing retry behavior remains unchanged.

## Testing
- `yarn test InteractionInference.errorDisplay.test.tsx Interaction.test.tsx` — 16 tests passed.
- `yarn tsc` — passed.
- `yarn build` — production frontend build passed.
