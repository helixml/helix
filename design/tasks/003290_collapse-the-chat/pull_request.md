# Collapse chat controls in narrow panes

## Summary
Collapse optional execution controls into a single settings button when the chat composer is under 400px wide. This keeps attachment, queue, context, and send or stop controls visible without wrapping or overlapping in narrow split panes while leaving wider layouts unchanged.

## Testing
Ran `yarn test RobustPromptInput.test.tsx --run`: all 23 tests passed, including the new narrow-composer regression test. Ran `yarn tsc`: passed.
