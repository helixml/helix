# Implementation Tasks

## Login copy — SaaS vs enterprise

- [ ] Set `HELIX_EDITION=cloud` in the app.helix.ml SaaS deployment environment config
- [ ] In `frontend/src/pages/Login.tsx`: add `const isCloud = config?.edition === 'cloud'` alongside the existing `isRegular` check (~line 82)
- [ ] In `frontend/src/pages/Login.tsx`: change the OIDC description text (~line 426) to show `"Sign in to your Helix account."` when `isCloud`, keeping the existing `"Use your organization's single sign-on to access Helix."` otherwise
- [ ] In `frontend/src/pages/Login.tsx`: change the OIDC button label (~line 444) to show `"Sign in"` when `isCloud`, keeping `"Sign in with SSO"` otherwise
- [ ] In `frontend/src/pages/Session.tsx`: import `useGetConfig` and add `const isCloud = config?.edition === 'cloud'`
- [ ] In `frontend/src/pages/Session.tsx`: change the login prompt text (~line 1628) to show `"Sign in to your Helix account to continue."` when `isCloud`, keeping the existing text otherwise

## Dead code cleanup — stale check in useLiveInteraction

- [ ] In `frontend/src/hooks/useLiveInteraction.ts`: remove `isAppTryHelixDomain` memo (~line 42-44)
- [ ] In `frontend/src/hooks/useLiveInteraction.ts`: remove `isStale` state, `recentTimestamp` state, and all `setRecentTimestamp`/`setIsStale` calls
- [ ] In `frontend/src/hooks/useLiveInteraction.ts`: remove the stale-check `useEffect` (~line 101-117)
- [ ] In `frontend/src/hooks/useLiveInteraction.ts`: remove `isStale` from the `LiveInteractionResult` interface and from the returned result object

## Dead code cleanup — upgrade button in InteractionInference

- [ ] In `frontend/src/components/session/InteractionInference.tsx`: remove the `upgrade` prop from the component interface and destructuring
- [ ] In `frontend/src/components/session/InteractionInference.tsx`: remove the `{upgrade && ...}` JSX block (~lines 462-481) containing the "Upgrade" button and `queue_upgrade_clicked` event
- [ ] In `frontend/src/components/session/Interaction.tsx`: remove `upgrade={false}` from both `<InteractionInference>` call sites (~lines 218, 302)

## Verification

- [ ] Verify `yarn build` passes with no errors
- [ ] Test on a local instance with `HELIX_EDITION=cloud` set — confirm SaaS copy appears
- [ ] Test on a local instance without `HELIX_EDITION` set — confirm enterprise copy is unchanged