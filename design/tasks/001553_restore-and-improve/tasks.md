# Implementation Tasks

- [ ] In `frontend/src/components/account/ClaudeSubscriptionConnect.tsx` (~line 441), rewrite the Alert message to: "Enter your email address in the browser below. You'll receive a code from Anthropic — paste it when prompted to complete login. You don't need to log in via Google; the email + code path is the recommended, more secure approach."
- [ ] Change `severity="info"` to `severity="warning"` on the same Alert
- [ ] Add `fontSize: '0.95rem', fontWeight: 500` to the Alert `sx` prop for increased visual prominence
- [ ] Run `cd frontend && yarn build` to verify no TypeScript/build errors
- [ ] Visually verify the updated Alert appears amber and is easy to read when the auth dialog is open
