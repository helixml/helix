# Design: Restore and Improve Authentication Guidance Message

## File

`frontend/src/components/account/ClaudeSubscriptionConnect.tsx`

The Alert is rendered inside `ClaudeLoginDialogInner` at line ~441, shown only when
`isRunning && loginCommandSent`.

## Current (Regressed) State

```tsx
<Alert severity="info" sx={{ mx: 2, mt: 1, flexShrink: 0 }}>
  A browser will open below. Sign in to your Claude account and complete the authentication flow in the browser.
</Alert>
```

`severity="info"` renders as a low-contrast blue bar. The message gives no actionable guidance.

## Proposed Message

```
Enter your email address in the browser below. You'll receive a code from Anthropic — paste it when prompted to complete login. You don't need to log in via Google; the email + code path is the recommended, more secure approach.
```

This message:
1. Tells users what to do (enter email)
2. Sets expectation for the code step (paste it when prompted)
3. Explicitly discourages Google desktop login without being alarming

## Visual Treatment

Change `severity="info"` → `severity="warning"` so the bar is amber/orange — higher visual
contrast and naturally draws the eye. Add `fontWeight: 'bold'` to the key action phrase or
increase `fontSize` slightly to ensure it isn't skimmed.

Suggested sx:
```tsx
<Alert
  severity="warning"
  sx={{ mx: 2, mt: 1, flexShrink: 0, fontSize: '0.95rem', fontWeight: 500 }}
>
```

Using `severity="warning"` is preferable to `severity="error"` (would imply something is
wrong) and more visible than `severity="info"`.

## Why Not a Dialog or Separate Component?

The message is short, contextual, and temporary (disappears once login completes). An Alert
in-place is the right MUI primitive. No new component is needed.

## Patterns Found

- Component uses MUI `Alert` with `severity` prop for contextual messages throughout
- `sx={{ mx: 2, mt: 1, flexShrink: 0 }}` is the established layout constraint for alerts inside this DialogContent — keep it

## Constraints

- No changes to auth logic (`claude auth login` command, polling, credential capture)
- No changes to dialog layout or `ExternalAgentDesktopViewer`
- Frontend only (`yarn build` required if running in production frontend mode)
